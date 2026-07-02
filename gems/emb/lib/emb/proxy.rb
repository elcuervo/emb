# frozen_string_literal: true

module Emb
  class Proxy
    attr_reader :name

    def initialize(client, name)
      @client = client
      @name = name
    end

    def [](text, *texts)
      set = Array(@client.send_command('EMB', @name.to_s, text, *texts))
      result = set.map { |entry| entry.unpack('e*') }

      return result.first if result.size == 1

      result
    end

    def inspect
      "#<Emb::Proxy #{@name}>"
    end
  end
end
