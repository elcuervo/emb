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
      Emb.send_command("EMB", @name.to_s, text, *texts)
    end

    def inspect
      "#<Emb::Proxy #{@name}>"
    end
  end
end
