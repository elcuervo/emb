# frozen_string_literal: true

require 'batch_loader'
require 'redis_client'
require_relative 'batch_dispatch'
require_relative 'values_batch'

module Emb
  extend BatchDispatch

  BATCH_KEY = :emb

  # Marker in slot 1 of a script batch item ([client, :script, argv, decode]);
  # embed items are [client, model, text, format].
  SCRIPT_ITEM = :script

  # Raised by resolve_slice when the server reply carries fewer entries than
  # the slice's texts. Subclasses RedisClient::ProtocolError so the existing
  # fail-closed rescue/wrap paths treat it as a client error, but stays
  # distinct so transient_error? does not count it as a transport retry: the
  # reply shape was wrong on the single send — nothing was re-sent.
  ShortReplyError = Class.new(RedisClient::ProtocolError)

  BATCH_BLOCK = lambda do |items, loader, _args|
    items.group_by(&:first).each do |client, client_items|
      chunk = client.respond_to?(:batch_size) && client.batch_size ? client.batch_size : Emb.configuration.batch_size
      # Script items never coalesce (each call is its own share); split them out
      # before packing embeds by model.
      embeds, scripts = client_items.partition { |item| !script_item?(item) }

      if client.respond_to?(:parallel_batch?) && client.parallel_batch?
        # :batch — one or more plain EMB shares per model plus one share per
        # script call, all dispatched concurrently. A lone share stays on the
        # forcing thread (no worker spawn).
        slices = batch_shares(embeds, scripts, chunk)
        if slices.size > 1
          dispatch_parallel(client, slices, loader)
        else
          dispatch_serial(client, slices, loader)
        end
      else
        # :multi — embeds coalesce across models into EMB/EMB.MULTI chunks,
        # then scripts are sent one after another.
        dispatch_serial(client, pack_slices(embeds, chunk) + scripts.map { |item| [item] }, loader)
      end
    end
  end

  class << self
    # BatchDispatch mechanics (pack_slices, dispatch_parallel, resolve_slice,
    # ...) are internal to BATCH_BLOCK; nothing outside Emb calls them with an
    # explicit receiver.
    private(*BatchDispatch.instance_methods(false))

    def build_batch_loader(client, model, text, format: :binary)
      unless %i[binary values].include?(format)
        raise ArgumentError, "unknown format #{format.inspect} (expected :binary or :values)"
      end

      # default_value []: an item whose batch failed (fail-closed) resolves to
      # an empty vector collection instead of nil, so resolver methods like
      # `loader.sum` do not blow up with NoMethodError-on-nil.
      # Items carry [client, model, text, format] so the dispatch knows how to
      # shape and parse the reply.
      BatchLoader.for([client, model, text, format]).batch(default_value: [], key: BATCH_KEY, &BATCH_BLOCK)
    end

    # Scripts join the same batch scope as embeds (same BATCH_KEY) so forcing
    # either resolves both. default_value nil (not []): a failed script resolves
    # to nil. The item carries the prebuilt EMB.EVAL/EMB.EVSHA argv and the
    # normalized decode for resolve_slice.
    def build_script_loader(client, argv, decode)
      BatchLoader.for([client, SCRIPT_ITEM, argv, decode]).batch(default_value: nil, key: BATCH_KEY, &BATCH_BLOCK)
    end

    # Removes every pending item of the batch scope. batch-loader prunes
    # pending items only after a successful batch block, so failed batches
    # would otherwise stay queued: retries would re-run the whole batch and
    # stale items would be re-sent by later batches in the same scope. Guarded
    # for a nil executor (no batch scope).
    def clear_batch_pending!
      key = [BATCH_BLOCK.source_location, BATCH_KEY]
      BatchLoader::Executor.current&.items_by_block&.delete(key)
    end

    # Fail-closed tail for a failed batch: clear the pending set, then raise
    # Emb::ServerError carrying the cause and the models/texts/attempts
    # context. The raise happens inside a rescue of the original error so Ruby
    # attaches it as `cause` regardless of whether this runs while a rescue is
    # active (serial path) or from the forcing thread after parallel workers
    # captured the error (parallel path). `attempts` counts the error's retry
    # class: connection/protocol errors (the ones redis-client actually
    # re-sends) report `budget + 1`; operation errors and read timeouts (never
    # re-sent — a timeout may already have executed server work) report 1. A
    # pre-send connection refusal is retried across instances by the
    # connection router before it can reach here as a terminal error.
    def fail_batch!(error, slice:, budget:)
      clear_batch_pending!
      attempts = transient_error?(error) ? budget + 1 : 1
      models = slice.map { |item| item_model(item) }.uniq.join(', ')
      texts = slice.sum { |item| item_text_count(item) }
      message = "batch failed after #{attempts} attempt(s) " \
                "(models: #{models}, #{texts} text(s)) #{error.class}: #{error.message}"
      begin
        raise error
      rescue StandardError
        raise ServerError.new(message, attempts: attempts)
      end
    end

    # How many additional re-sends redis-client performs for transient
    # failures: the per-client option when set, else the global default.
    # Normalized to an Integer (redis-client also accepts a delay Array, whose
    # truthy slots each grant one retry).
    def retry_budget(client)
      value = client.reconnect_attempts if client.respond_to?(:reconnect_attempts)
      value = Emb.configuration.reconnect_attempts if value.nil?
      return value if value.is_a?(Integer)

      value.is_a?(Array) ? value.count(&:itself) : 0
    end

    # The error classes redis-client actually re-dispatches under
    # reconnect_attempts: ConnectionError (connect/transport breaks) and
    # ProtocolError. ReadTimeoutError is intercepted before the retry loop
    # (a timed-out command may already have executed server work), so it is
    # terminal and counts a single attempt. Emb::ShortReplyError is raised
    # locally by resolve_slice (reply shape wrong on the single send), so it
    # is likewise terminal and counts one attempt.
    def transient_error?(error)
      (error.is_a?(RedisClient::ConnectionError) && !error.is_a?(RedisClient::ReadTimeoutError)) ||
        (error.is_a?(RedisClient::ProtocolError) && !error.is_a?(ShortReplyError))
    end

    private :clear_batch_pending!, :fail_batch!, :retry_budget, :transient_error?
  end
end
