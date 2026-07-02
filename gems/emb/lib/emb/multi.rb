# frozen_string_literal: true

module Emb
  class MultiProxy
    def initialize(client)
      @client = client
      @pairs = []
    end

    def [](name)
      PairCollector.new(@pairs, name.to_sym)
    end

    def run
      args = @pairs.flat_map { |pair| [pair[:model].to_s, pair[:text]] }

      @client
        .send_command('EMB.MULTI', *args)
        .map { |entry| entry.unpack('e*') }
    end

    class PairCollector
      def initialize(pairs, model)
        @pairs = pairs
        @model = model
      end

      def [](text)
        @pairs << { model: @model, text: text }
      end
    end
  end
end
