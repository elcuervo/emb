# frozen_string_literal: true

require 'connection_pool'
require 'redis_client'

module Emb
  class Client
    include Commands

    attr_reader :pool, :batch_size

    def initialize(pool: nil, batch: nil, **redis_options)
      cfg = Emb.configuration
      @batch_enabled = batch.nil? ? cfg.batch : batch
      @batch_size = redis_options.delete(:batch_size) || cfg.batch_size
      size = pool.nil? ? cfg.pool : pool
      url = extract_url!(redis_options, cfg)
      redis_options = merged_redis_options(redis_options, cfg, url)

      @pool = ConnectionPool.new(size: size) do
        RedisClient.new(url: url, **redis_options)
      end

      @registry = {}
    end

    def send_command(*args)
      return @pool.with { |r| r.call(*args) } unless Emb.debug?

      start = Process.clock_gettime(Process::CLOCK_MONOTONIC)
      result = @pool.with { |r| r.call(*args) }
      elapsed = (Process.clock_gettime(Process::CLOCK_MONOTONIC) - start) * 1000

      $stdout.puts "[EMB] #{args.map(&:inspect).join(' ')} (#{format('%.2f', elapsed)}ms)"

      result
    end

    def [](name)
      @registry[name] ||= Proxy.new(self, name.to_sym)
    end

    def batch
      @batch ||= BatchProxy.new(self)
    end

    def batch?
      @batch_enabled
    end

    # Live view of the server's runtime configuration (CONFIG GET/SET).
    def config
      @config ||= RuntimeConfig.new(self)
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

    def help = send_command('EMB.HELP')

    def ping = send_command('PING')

    def ready
      send_command('EMB.READY')

      'ready'
    rescue RedisClient::CommandError => e
      e.message
    end

    def ready?
      ready

      true
    rescue RedisClient::CommandError
      false
    end

    def reset_registry!
      @registry = {}
    end

    def multi(&)
      mp = MultiProxy.new(self)
      yield mp
      mp.run
    end

    private

    def merged_redis_options(opts, cfg, url)
      defaults = cfg.to_h
      keys = defaults.keys - %i[url pool batch batch_size]
      keys -= %i[host port] if url

      keys.each do |key|
        opts[key] = defaults[key] if opts[key].nil? && !defaults[key].nil?
      end

      opts
    end

    def extract_url!(opts, cfg)
      url = opts.delete(:url)
      return url unless url.nil?

      cfg.url || ENV.fetch('EMB_URL', nil)
    end
  end
end
