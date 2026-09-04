# frozen_string_literal: true

require 'batch_loader'

module Emb
  BATCH_KEY = :emb

  BATCH_BLOCK = lambda do |items, loader, _args|
    items.group_by(&:first).each do |client, client_items|
      chunk = client.respond_to?(:batch_size) && client.batch_size ? client.batch_size : Emb.configuration.batch_size

      # Chunk at batch_size pairs per EMB.MULTI so a single command stays well
      # under the server's max_pairs cap and within client read timeouts.
      client_items.each_slice(chunk) do |slice|
        pairs = slice.flat_map { |_, model, text| Array(text).flat_map { |t| [model.to_s, t] } }

        begin
          results = Array(client.send_command('EMB.MULTI', *pairs))
        rescue StandardError => e
          # Fail closed: raise Emb::ServerError (cause = the original error) and
          # drop the batch's pending items. Post-failure resolutions yield [].
          fail_batch!(e, slice: slice, budget: retry_budget(client))
        end

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
    # context. Transient errors were already re-sent `budget` extra times by
    # redis-client; operation errors surface on the first attempt.
    def fail_batch!(error, slice:, budget:)
      clear_batch_pending!
      attempts = transient_error?(error) ? budget + 1 : 1
      models = slice.map { |_, model, _| model }.uniq.join(', ')
      texts = slice.sum { |_, _, text| Array(text).size }
      raise ServerError.new(
        "EMB.MULTI failed after #{attempts} attempt(s) " \
        "(models: #{models}, #{texts} text(s)) #{error.class}: #{error.message}",
        attempts: attempts
      )
    end

    # How many additional re-sends redis-client performs for transient
    # failures: the per-client option when set, else the global default.
    # Normalized to an Integer (redis-client also accepts a delay Array, whose
    # truthy slots each grant one retry).
    def retry_budget(client)
      value = client.reconnect_attempts if client.respond_to?(:reconnect_attempts)
      value ||= Emb.configuration.reconnect_attempts
      return value if value.is_a?(Integer)

      value.is_a?(Array) ? value.count(&:itself) : 0
    end

    def transient_error?(error)
      error.is_a?(RedisClient::ConnectionError) || error.is_a?(RedisClient::ProtocolError)
    end

    private :clear_batch_pending!, :fail_batch!, :retry_budget, :transient_error?
  end

  class BatchProxy
    def initialize(client)
      @client = client
      @models = {}
    end

    def [](name)
      @models[name.to_sym] ||= BatchModelProxy.new(@client, name.to_sym)
    end

    def inspect
      "#<Emb::BatchProxy client=#{@client.inspect}>"
    end
  end

  class BatchModelProxy
    attr_reader :name

    def initialize(client, name)
      @client = client
      @name = name
    end

    def [](text, *texts)
      Emb.build_batch_loader(@client, @name, texts.empty? ? text : [text, *texts])
    end

    def inspect
      "#<Emb::BatchModelProxy #{@name}>"
    end
  end
end
