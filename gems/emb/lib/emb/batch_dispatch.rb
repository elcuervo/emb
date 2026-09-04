# frozen_string_literal: true

module Emb
  # Share-dispatch mechanics for the deferred batch path: wire shaping (EMB
  # for single-model chunks, EMB.MULTI for mixed), per-slice result mapping,
  # and the bounded concurrent fan-out used by `lazy: :batch`. Extended into
  # Emb by batch.rb, which owns the batch-loader contract (BATCH_BLOCK, the
  # fail-closed tail, and the retry-budget bookkeeping).
  module BatchDispatch
    # Sends one chunk share and returns the raw reply entries. Single-model
    # shares use plain `EMB <model> <text>...` — the server packs them into one
    # batched inference with no pair fan-out, and the command carries half the
    # arguments (model repeated once, not per pair). Mixed-model shares (the
    # only case EMB.MULTI exists for) keep EMB.MULTI per-pair semantics.
    # Pre-send connection failures (CannotConnectError: nothing was written)
    # retry on the next instance inside the connection router; anything else
    # (timeout, mid-flight connection loss, server error reply) is terminal and
    # propagates so the forcing thread can fail closed.
    def dispatch_slice(client, slice)
      models = slice.map { |_, model, _| model }.uniq
      args = models.size == 1 ? same_model_args(slice, models.first) : mixed_model_args(slice)
      Array(client.send_command(*args))
    end

    # EMB <model> <text>... — the server packs the texts into one inference.
    def same_model_args(slice, model)
      ['EMB', model.to_s, *slice.flat_map { |_, _, text| Array(text) }]
    end

    # EMB.MULTI <model> <text>... pairs — per-pair nil semantics across models.
    def mixed_model_args(slice)
      ['EMB.MULTI', *slice.flat_map { |_, model, text| Array(text).flat_map { |t| [model.to_s, t] } }]
    end

    # Resolves one slice's items against the raw reply entries, preserving
    # deferral order and per-pair nil (MGET semantics). Runs on the forcing
    # thread: batch-loader's executor is per-thread, so worker threads must
    # never resolve loaders. A reply shorter than the slice's texts is a
    # protocol violation and fails the batch (RedisClient::ProtocolError, which
    # the fail-closed path wraps in Emb::ServerError).
    def resolve_slice(loader, slice, results)
      expected = slice.sum { |_, _, text| Array(text).size }
      unless results.size >= expected
        raise RedisClient::ProtocolError, "expected #{expected} reply entries, got #{results.size}"
      end

      offset = 0
      slice.each do |item|
        _, _, text = item
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

    # One worker's outcome for a chunk share: [:ok, slice, results] or
    # [:error, slice, error]. Method-level rescue keeps the error capture safe
    # inside the worker thread; only this thread touches the client, and the
    # forcing thread stays free to resolve loaders (the batch executor is
    # per-thread).
    def dispatch_share(client, slice)
      [:ok, slice, dispatch_slice(client, slice)]
    rescue StandardError => e
      [:error, slice, e]
    end

    # Dispatches every chunk share concurrently over a bounded worker pool (at
    # most the client's total connection capacity workers — shares beyond that
    # would only queue on the pools) and fails closed on the first terminal
    # error: the force raises once on the forcing thread, successful shares
    # stay consumed (never re-sent), and the failed share's items clear from
    # the scope's pending set.
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

    # How many shares may be in flight at once: the client's total connection
    # capacity when the pools are visible, else all shares. Clients without a
    # pools accessor are fakes in unit tests, where unbounded workers are fine.
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

    # Resolves successful shares in slice order and raises the failure. Redis
    # errors fail closed with full context (Emb::ServerError, matching the
    # serial path); a non-redis error (e.g. a local bug such as CommandBuilder
    # TypeError from bad arguments) is re-raised unchanged after the pending
    # set is dropped.
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

    # Packs deferred items into chunk shares by accumulated TEXT count (an item
    # may defer several texts: `Emb[:m]["a", "b"]`), so a single command stays
    # within `chunk` texts and the server's max_texts/max_pairs cap. An item
    # whose own text count already exceeds the chunk goes alone — the server
    # truncates the overflow with null reply slots, exactly as the eager path
    # does for an oversized single call.
    def pack_slices(items, chunk)
      slices = []
      items.each do |item|
        size = Array(item[2]).size
        if slices.empty? || slices.last.sum { |i| Array(i[2]).size } + size > chunk
          slices << [item]
        else
          slices.last << item
        end
      end
      slices
    end
  end
end
