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
        results = Array(client.send_command('EMB.MULTI', *pairs))

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
      BatchLoader.for([client, model, text]).batch(key: BATCH_KEY, &BATCH_BLOCK)
    end
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
