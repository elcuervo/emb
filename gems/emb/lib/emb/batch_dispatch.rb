# frozen_string_literal: true

module Emb
  # Share-dispatch mechanics for the deferred batch path: EMB/MULTI wire
  # shaping, per-slice result mapping, and the bounded concurrent fan-out used
  # by `lazy: :batch`. Extended into Emb by batch.rb.
  module BatchDispatch
    # Sends one chunk share and returns the raw reply entries. Single-model
    # shares use plain `EMB <model> <text>...` (one inference, model once);
    # mixed-model shares keep EMB.MULTI per-pair nil semantics. Errors after
    # the command may have been sent are terminal and propagate so the forcing
    # thread can fail closed.
    def dispatch_slice(client, slice)
      models = slice.map { |_, model, _, _| model }.uniq
      args = models.size == 1 ? same_model_args(slice, models.first) : mixed_model_args(slice)
      Array(client.send_command(*args))
    end

    def same_model_args(slice, model)
      texts = slice.flat_map { |_, _, text, _| Array(text) }
      if slice_format(slice) == :values
        ['EMB', model.to_s, 'VALUES', *texts]
      else
        ['EMB', model.to_s, *texts]
      end
    end

    def mixed_model_args(slice)
      pairs = slice.flat_map { |_, model, text, _| Array(text).flat_map { |t| [model.to_s, t] } }
      if slice_format(slice) == :values
        ['EMB.MULTI', 'VALUES', *pairs]
      else
        ['EMB.MULTI', *pairs]
      end
    end

    # slice_format: the batch item layout is [client, model, text, format];
    # items created before the format slot existed default to :binary.
    def slice_format(slice)
      formats = slice.map { |item| item[3] }.uniq
      if formats.size > 1
        raise ArgumentError, "cannot mix :binary and :values formats in one batch"
      end
      formats.first || :binary
    end

    # Maps a slice's reply entries onto its items in deferral order. Runs on
    # the forcing thread (batch-loader's executor is per-thread), so workers
    # never resolve loaders. A reply shorter than the slice's texts is a
    # protocol violation: fail the batch via Emb::ShortReplyError (distinct
    # from a client-raised ProtocolError so it is not counted as a transport
    # retry).
    def resolve_slice(loader, slice, results)
      if slice_format(slice) == :values
        return resolve_values(loader, slice, results)
      end

      expected = slice.sum { |_, _, text, _| Array(text).size }
      unless results.size >= expected
        raise ShortReplyError, "expected #{expected} reply entries, got #{results.size}"
      end

      offset = 0
      slice.each do |item|
        _, _, text, _ = item
        texts = Array(text)
        values = entry_values(results, offset, texts)
        offset += texts.size

        # eager Proxy#[] shape: vector for a single text, vectors for many
        loader.call(item, values)
      end
    end

    def entry_values(results, offset, texts)
      values = results[offset, texts.size].map { |entry| entry&.unpack('e*') }
      values.size == 1 ? values.first : values
    end

    # ---- values format ----

    # resolve_values replaces resolve_slice's per-entry mapping for VALUES
    # slices. Single-model slices get ONE envelope reply carrying every text's
    # flattened values (row-major), so the rows are re-sliced per item;
    # mixed-model slices get per-pair envelopes and map one row each. Failures
    # stay nil, mirroring the BLOB path's MGET semantics.
    def resolve_values(loader, slice, results)
      if slice.map { |item| item[1] }.uniq.size == 1
        # dispatch_slice's Array(...) wrap is identity for the single envelope
        # (a flat 6-element array), so `results` IS the envelope.
        envelope = Emb::ValuesReply.parse(results)
        rows = Emb::ValuesReply.rows(envelope[:shape], envelope[:values])
        expected = slice.sum { |_, _, text, _| Array(text).size }
        if rows.size < expected
          raise ShortReplyError, "expected #{expected} VALUES rows, got #{rows.size}"
        end

        offset = 0
        slice.each do |item|
          _, _, text, _ = item
          texts = Array(text)
          values = rows[offset, texts.size]
          offset += texts.size
          loader.call(item, values.size == 1 ? values.first : values)
        end
      else
        offset = 0
        slice.each do |item|
          _, _, text, _ = item
          texts = Array(text)
          values = results[offset, texts.size].map do |entry|
            next nil if entry.nil?

            envelope = Emb::ValuesReply.parse(entry)
            Emb::ValuesReply.rows(envelope[:shape], envelope[:values]).first
          end
          offset += texts.size
          loader.call(item, values.size == 1 ? values.first : values)
        end
      end
    end

    # The worker captures failures as outcomes; only the forcing thread
    # resolves loaders (see resolve_slice).
    def dispatch_share(client, slice)
      [:ok, slice, dispatch_slice(client, slice)]
    rescue StandardError => e
      [:error, slice, e]
    end

    # Dispatches all shares concurrently over at most the client's connection
    # capacity workers, then fails closed on the first terminal error.
    def dispatch_parallel(client, slices, loader)
      workers = slices.size.clamp(1, worker_capacity(client))
      queue = share_queue(slices, workers)
      outcomes = Array.new(slices.size)
      run_worker_threads(client, queue, outcomes, workers)
      resolve_outcomes(client, outcomes, loader)
    end

    def share_queue(slices, workers)
      queue = Queue.new
      slices.each_with_index { |slice, index| queue << [index, slice] }
      workers.times { queue << nil }
      queue
    end

    def worker_capacity(client)
      return Float::INFINITY unless client.respond_to?(:pools)

      client.pools.sum { |pool| pool.respond_to?(:size) ? pool.size : 1 }
    end

    def run_worker_threads(client, queue, outcomes, workers)
      workers.times.map do
        Thread.new do
          while (job = queue.pop)
            index, slice = job
            outcomes[index] = dispatch_share(client, slice)
          end
        end
      end.each(&:join)
    end

    # Redis errors fail closed with context (Emb::ServerError, matching the
    # serial path); non-redis errors are local bugs and re-raise unchanged.
    def resolve_outcomes(client, outcomes, loader)
      first_error, failed_slice = collect_outcomes(outcomes, loader)
      return unless first_error

      raise first_error unless first_error.is_a?(RedisClient::Error)

      fail_batch!(first_error, slice: failed_slice, budget: retry_budget(client))
    rescue StandardError
      clear_batch_pending!
      raise
    end

    def collect_outcomes(outcomes, loader)
      first_error = nil
      failed_slice = nil
      outcomes.each do |status, slice, result|
        if status == :ok
          resolve_slice(loader, slice, result)
        else
          first_error ||= result
          failed_slice ||= slice
        end
      end
      [first_error, failed_slice]
    end

    # Packs items into shares by accumulated text count so one command stays
    # within `chunk` texts. An item larger than the chunk goes alone; the
    # server truncates it with null reply slots, as in the eager path. `used`
    # carries the running text count of the current slice instead of
    # re-summing it for every item (O(n) per item would be O(n²)).
    def pack_slices(items, chunk)
      slices = []
      used = 0
      items.each do |item|
        size = Array(item[2]).size
        if slices.empty? || used + size > chunk
          slices << [item]
          used = size
        else
          slices.last << item
          used += size
        end
      end
      slices
    end
  end
end
