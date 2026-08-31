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
      chunk = @client.respond_to?(:batch_size) && @client.batch_size ? @client.batch_size : Emb.configuration.batch_size

      # Chunk at batch_size pairs per EMB.MULTI so a single command stays well
      # under the server's max_pairs cap and within client read timeouts.
      @pairs.each_slice(chunk).flat_map do |slice|
        args = slice.flat_map { |pair| [pair[:model].to_s, pair[:text]] }

        @client
          .send_command('EMB.MULTI', *args)
          .map { |entry| entry&.unpack('e*') }
      end
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
