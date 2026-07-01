# frozen_string_literal: true

require "connection_pool"
require "redis_client"

module Emb
  @pool = nil

  DEFAULTS = {host: "localhost", port: 6379, pool: 5}.freeze

  class << self
    def setup(host: DEFAULTS[:host], port: DEFAULTS[:port], pool: DEFAULTS[:pool])
      @pool = ConnectionPool.new(size: pool) do
        RedisClient.new(host: host, port: port, protocol: 2, reconnect_attempts: 3)
      end
    end
    alias_method :config, :setup

    def send_command(*args)
      pool.with { |r| r.call(*args) }
    end

    private

    def pool
      @pool ||= default_pool
    end

    def default_pool
      ConnectionPool.new(size: DEFAULTS[:pool]) do
        RedisClient.new(host: DEFAULTS[:host], port: DEFAULTS[:port], protocol: 2, reconnect_attempts: 3)
      end
    end
  end
end
