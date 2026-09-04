# frozen_string_literal: true

require 'batch_loader'

module Emb
  BATCH_KEY = :emb

  # Sends one chunk share and returns the raw reply entries. Single-model
  # shares use plain `EMB <model> <text>...` — the server packs them into one
  # batched inference with no pair fan-out, and the command carries half the
  # arguments (model repeated once, not per pair). Mixed-model shares (the only
  # case EMB.MULTI exists for) keep EMB.MULTI per-pair semantics.
  # Pre-send connection failures (CannotConnectError: nothing was written)
  # retry on the next instance inside the connection router; anything else
  # (timeout, mid-flight connection loss, server error reply) is terminal and
  # propagates so the forcing thread can fail closed.
  def self.dispatch_slice(client, slice)
    models = slice.map { |_, model, _| model }.uniq
    args = models.size == 1 ? same_model_args(slice, models.first) : mixed_model_args(slice)
    Array(client.send_command(*args))
  end

  # EMB <model> <text>... — the server packs the texts into one inference.
  def self.same_model_args(slice, model)
    ['EMB', model.to_s, *slice.flat_map { |_, _, text| Array(text) }]
  end

  # EMB.MULTI <model> <text>... pairs — per-pair nil semantics across models.
  def self.mixed_model_args(slice)
    ['EMB.MULTI', *slice.flat_map { |_, model, text| Array(text).flat_map { |t| [model.to_s, t] } }]
  end

  # Resolves one slice's items against the raw reply entries, preserving
  # deferral order and per-pair nil (MGET semantics). Runs on the forcing
  # thread: batch-loader's executor is per-thread, so worker threads must
  # never resolve loaders.
  def self.resolve_slice(loader, slice, results)
    offset = 0
    slice.each do |item|
      _, _, text = item
      texts = Array(text)
      values = results[offset, texts.size].map { |entry| entry&.unpack('e*') }
      offset += texts.size

      # eager Proxy#[] shape: vector for a single text, vectors for many
      loader.call(item, values.size == 1 ? values.first : values)
    end
  end

  # One worker's outcome for a chunk share: [:ok, slice, results] or
  # [:error, slice, error]. Method-level rescue keeps the error capture safe
  # inside the worker thread; only this thread touches the client, and the
  # forcing thread stays free to resolve loaders (the batch executor is
  # per-thread).
  def self.dispatch_share(client, slice)
    [:ok, slice, dispatch_slice(client, slice)]
  rescue StandardError => e
    [:error, slice, e]
  end

  # Dispatches every chunk share concurrently (worker thread per share), joins
  # all of them, resolves successful shares in slice order, and fails closed
  # on the first terminal error: the force raises once on the forcing thread,
  # successful shares stay consumed (never re-sent), and the failed share's
  # items clear from the scope's pending set.
  def self.dispatch_parallel(client, slices, loader)
    outcomes = slices.map { |slice| Thread.new { dispatch_share(client, slice) } }.map(&:value)
    resolve_outcomes(client, outcomes, loader)
  end

  # Resolves successful shares in slice order and raises the failure. Redis
  # errors fail closed with full context (Emb::ServerError); a non-redis error
  # (e.g. a local bug such as CommandBuilder TypeError from bad arguments) is
  # re-raised unchanged after the pending set is dropped.
  def self.resolve_outcomes(client, outcomes, loader)
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
    fail_batch!(first_error, slice: failed_slice, budget: retry_budget(client)) if first_error
  rescue StandardError
    clear_batch_pending!
    raise
  end

  BATCH_BLOCK = lambda do |items, loader, _args|
    items.group_by(&:first).each do |client, client_items|
      chunk = client.respond_to?(:batch_size) && client.batch_size ? client.batch_size : Emb.configuration.batch_size

      slices = client_items.each_slice(chunk).to_a
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
    # captured the error (parallel path). Transient errors were already
    # re-sent `budget` extra times by redis-client; operation errors surface
    # on the first attempt. `attempts` reflects the final error's retry class
    # — redis-client 0.30 does not expose an accumulated send count, so a
    # mixed sequence (timeout retried, then an operation error) reports the
    # operation error's count. A pre-send connection refusal is retried across
    # instances by the connection router before it can reach here as a
    # terminal error.
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

    def transient_error?(error)
      error.is_a?(RedisClient::ConnectionError) || error.is_a?(RedisClient::ProtocolError)
    end

    private :clear_batch_pending!, :fail_batch!, :retry_budget, :transient_error?
  end
end
