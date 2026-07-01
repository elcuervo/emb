# frozen_string_literal: true

module Emb
  @registry = {}

  class << self
    def [](name)
      @registry[name] ||= Proxy.new(name.to_sym)
    end

    def reset_registry!
      @registry.clear
    end
  end

  class Proxy
    attr_reader :name

    def initialize(name)
      @name = name
    end

    def [](text, *texts)
      set = Array(Emb.send_command("EMB", @name.to_s, text, *texts))
      result = set.map { |entry| entry.unpack("e*") }

      return result.first if result.size == 1

      result
    end

    def inspect
      "#<Emb::Proxy #{@name}>"
    end
  end
end
