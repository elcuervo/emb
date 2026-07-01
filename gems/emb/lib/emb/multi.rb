# frozen_string_literal: true

module Emb
  class MultiProxy
    def initialize
      @pairs = []
    end

    def [](name)
      PairCollector.new(@pairs, name.to_sym)
    end

    def run
      args = @pairs.flat_map { |pair| [pair[:model].to_s, pair[:text]] }
      Emb.send_command("EMB.MULTI", *args)
    end

    class PairCollector
      def initialize(pairs, model)
        @pairs = pairs
        @model = model
      end

      def [](text)
        @pairs << {model: @model, text: text}
      end
    end
  end

  class << self
    def multi
      mp = MultiProxy.new
      yield mp
      mp.run
    end
  end
end
