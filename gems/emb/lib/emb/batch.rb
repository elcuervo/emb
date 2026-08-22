# frozen_string_literal: true

require 'batch_loader'

module Emb
  BATCH_KEY = :emb

  BATCH_BLOCK = lambda do |items, loader, _args|
    items.group_by(&:first).each do |client, client_items|
      pairs = client_items.flat_map { |_, model, text| Array(text).flat_map { |t| [model.to_s, t] } }
      results = Array(client.send_command('EMB.MULTI', *pairs))

      client_items.each do |item|
        _, _, text = item
        values = Array(text).map { results.shift&.unpack('e*') }

        loader.call(item, values.size == 1 ? values.first : values)
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
