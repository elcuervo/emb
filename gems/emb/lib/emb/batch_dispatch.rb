# frozen_string_literal: true

require_relative 'script_reply_decode'

module Emb
  # Poison value for an item whose share failed terminally under parallel
  # dispatch: any use raises the share's Emb::ServerError. BasicObject so only
  # method_missing is reachable — batch-loader's __replace_with! calls
  # `value.methods` on first use, and Array()/respond_to? raise too.
  #
  # No respond_to_missing?: BasicObject has no respond_to?, so it would never
  # be called (dead code).
  # rubocop:disable Style/MissingRespondToMissing
  class FailedShare < BasicObject
    def initialize(error)
      @error = error
    end

    def method_missing(*)
      ::Kernel.raise(@error)
    end
  end
  # rubocop:enable Style/MissingRespondToMissing

  # Share-dispatch mechanics for the deferred batch path: EMB/MULTI wire
  # shaping, per-slice result mapping, and the bounded concurrent fan-out used
  # by `lazy: :batch`. Extended into Emb by batch.rb.
  module BatchDispatch
    # Script batch items are [client, SCRIPT_ITEM, argv, decode]; embed items are
    # [client, model, text, format]. argv is the fully built EMB.EVAL/EMB.EVSHA
    # command and argv[3] its text count.
    def script_item?(item)
      item[1].equal?(SCRIPT_ITEM)
    end

    def item_model(item)
      script_item?(item) ? item[2][1] : item[1]
    end

    def item_text_count(item)
      script_item?(item) ? item[2][3].to_i : Array(item[2]).size
    end

    # :batch shares: per-model chunked EMB shares plus one share per script
    # call. group_by preserves first-deferral order of the models.
    def batch_shares(embeds, scripts, chunk)
      slices = []
      embeds.group_by { |item| item[1] }.each_value { |group| slices.concat(pack_slices(group, chunk)) }
      scripts.each { |item| slices << [item] }
      slices
    end

    # Serial dispatch: one share in flight at a time. Redis errors fail closed
    # with context; anything else is a local bug and is re-raised unchanged
    # after the pending set is dropped.
    def dispatch_serial(client, slices, loader)
      slices.each do |slice|
        resolve_slice(loader, slice, dispatch_slice(client, slice))
      rescue RedisClient::Error => e
        clear_batch_pending!
        raise batch_error(e, slice: slice, budget: retry_budget(client))
      rescue StandardError
        clear_batch_pending!
        raise
      end
    end

    # Sends one chunk share and returns the raw reply entries. Single-model
    # shares use plain `EMB <model> <text>...` (one inference, model once);
    # mixed-model shares keep EMB.MULTI per-pair nil semantics. Errors after
    # the command may have been sent are terminal and propagate so the forcing
    # thread can fail closed.
    def dispatch_slice(client, slice)
      # Scripts keep their prebuilt argv and their raw reply (a Hash reply must
      # not be Array()-wrapped into pairs).
      return client.send_command(*slice.first[2]) if script_item?(slice.first)

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
      return resolve_script_slice(loader, slice.first, results) if script_item?(slice.first)

      if slice_format(slice) == :values
        return ValuesBatch.resolve(loader, slice, results)
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

    # A script call is always a singleton slice; its reply is parsed on the
    # forcing thread. multi: comes from the argv text count (argv[3]).
    def resolve_script_slice(loader, item, results)
      multi = item[2][3].to_i > 1
      loader.call(item, ScriptReplyDecode.parse_script_reply(results, multi: multi, decode: item[3]))
    end

    # The worker captures failures as outcomes; only the forcing thread
    # resolves loaders (see resolve_slice).
    def dispatch_share(client, slice)
      [:ok, slice, dispatch_slice(client, slice)]
    rescue StandardError => e
      [:error, slice, e]
    end

    # Dispatches all shares concurrently over at most the client's connection
    # capacity workers, then resolves the outcomes (isolating a failed share's
    # error to its own items).
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

    # Redis errors are isolated to their own share: every item of the failed
    # share resolves to a FailedShare that re-raises that share's
    # Emb::ServerError on use (no re-send), while healthy shares resolve
    # normally and the force never raises for a sibling. batch-loader then
    # prunes the pending items itself once the block returns. Non-redis errors
    # are local bugs: resolve the other shares, then clear the pending set and
    # re-raise the first one.
    def resolve_outcomes(client, outcomes, loader)
      first_error = nil
      outcomes.each do |status, slice, result|
        error = outcome_error(loader, status, slice, result)
        next unless error

        if error.is_a?(RedisClient::Error)
          fail_share(loader, slice, error, client)
        else
          first_error ||= error
        end
      end
      return unless first_error

      clear_batch_pending!
      raise first_error
    end

    # The error an outcome carries: the worker's captured error, or a
    # resolve_slice failure (e.g. ShortReplyError) against an :ok reply.
    def outcome_error(loader, status, slice, result)
      return result unless status == :ok

      resolve_slice(loader, slice, result)
      nil
    rescue StandardError => e
      e
    end

    # Resolves every item of a failed share to a poison value that raises the
    # share's Emb::ServerError on use. Building the error once keeps its
    # message/context and cause identical for all of the share's items.
    def fail_share(loader, slice, error, client)
      poison = batch_error(error, slice: slice, budget: retry_budget(client))
      slice.each { |item| loader.call(item, FailedShare.new(poison)) }
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
