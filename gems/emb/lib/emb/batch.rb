# frozen_string_literal: true

require 'batch_loader'
require_relative 'batch_dispatch'

module Emb
  extend BatchDispatch

  BATCH_KEY = :emb

  BATCH_BLOCK = lambda do |items, loader, _args|
    items.group_by(&:first).each do |client, client_items|
      chunk = client.respond_to?(:batch_size) && client.batch_size ? client.batch_size : Emb.configuration.batch_size

      slices = pack_slices(client_items, chunk)
      if client.respond_to?(:parallel_batch?) && client.parallel_batch? && slices.size > 1
        dispatch_parallel(client, slices, loader)
      else
        # Serial dispatch: one share in flight at a time. Redis errors fail
        # closed with context; anything else is a local bug and is re-raised
        # unchanged after the pending set is dropped.
        slices.each do |slice|
          resolve_slice(loader, slice, dispatch_slice(client, slice))
        rescue RedisClient::Error => e
          fail_batch!(e, slice: slice, budget: retry_budget(client))
        rescue StandardError
          clear_batch_pending!
          raise
        end
      end
    end
  end

  class << self
    def build_batch_loader(client, model, text)
      # default_value []: an item whose batch failed (fail-closed) resolves to
      # an empty vector collection instead of nil, so resolver methods like
      # `loader.sum` do not blow up with NoMethodError-on-nil.
      BatchLoader.for([client, model, text]).batch(default_value: [], key: BATCH_KEY, &BATCH_BLOCK)
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
      models = slice.map { |_, model, _| model }.uniq.join(', ')
      texts = slice.sum { |_, _, text| Array(text).size }
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
    # terminal and counts a single attempt.
    def transient_error?(error)
      (error.is_a?(RedisClient::ConnectionError) && !error.is_a?(RedisClient::ReadTimeoutError)) ||
        error.is_a?(RedisClient::ProtocolError)
    end

    private :clear_batch_pending!, :fail_batch!, :retry_budget, :transient_error?
  end
end
