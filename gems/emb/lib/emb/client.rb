# frozen_string_literal: true

require 'connection_pool'
require 'redis_client'

module Emb
  DEFAULTS = { host: 'localhost', port: 6379, pool: 5 }.freeze

  class Client
    attr_reader :pool

    def initialize(url: nil, host: nil, port: nil, pool: DEFAULTS[:pool])
      url ||= ENV.fetch('EMB_URL', nil)
      host ||= DEFAULTS[:host]
      port ||= DEFAULTS[:port]

      @pool = ConnectionPool.new(size: pool) do
        if url
          RedisClient.new(url: url, protocol: 2, reconnect_attempts: 3)
        else
          RedisClient.new(host: host, port: port, protocol: 2, reconnect_attempts: 3)
        end
      end

      @registry = {}
    end

    def send_command(*args)
      @pool.with { |r| r.call(*args) }
    end

    def [](name)
      @registry[name] ||= Proxy.new(self, name.to_sym)
    end

    def models
      raw = send_command('EMB.MODELS')
      return [] if raw.nil?

      raw.map do |name, dim, status|
        { name: name, dim: dim.to_i, status: status }
      end
    end

    def info(name)
      raw = send_command('EMB.INFO', name.to_s)
      return {} if raw.nil?

      raw
        .each_slice(2)
        .to_h { |k, v| [k.to_sym, v] }
    end

    def stats = send_command('EMB.STATS')

    def help = send_command('EMB.HELP')

    def ping = send_command('PING')

    def reset_registry!
      @registry = {}
    end

    def multi(&)
      mp = MultiProxy.new(self)
      yield mp
      mp.run
    end
  end
end
