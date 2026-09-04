# frozen_string_literal: true

require_relative 'connection_router'
require 'redis_client'

module Emb
  class Client
    include Commands

    # Accepts a single redis URL (String) or several (Array of Strings) naming
    # interchangeable emb instances serving the same model set; each url gets
    # its own pool of `pool` connections (see ConnectionRouter).
    def initialize(pool: nil, lazy: nil, **redis_options)
      cfg = Emb.configuration
      @lazy_mode = lazy.nil? ? cfg.lazy : validate_lazy!(lazy)
      @batch_size = redis_options.delete(:batch_size) || cfg.batch_size
      url = extract_url!(redis_options, cfg)
      # Captured before ConnectionRouter consumes the merged options, so
      # fail-closed batches can report the retry budget (Emb::ServerError).
      redis_options = merged_redis_options(redis_options, cfg, url)
      @reconnect_attempts = redis_options.fetch(:reconnect_attempts, cfg.reconnect_attempts)
      @router = ConnectionRouter.new(pool || cfg.pool, instance_urls(url), redis_options)
      @registry = {}
    end

    def send_command(...) = @router.call(...)

    def pools = @router.pools

    attr_reader :batch_size, :lazy_mode, :reconnect_attempts

    def [](name)
      @registry[name] ||= Proxy.new(self, name.to_sym)
    end

    def lazy? = @lazy_mode != false

    def parallel_batch? = @lazy_mode == :batch

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

    def validate_lazy!(value)
      unless Configuration::LAZY_MODES.include?(value)
        raise ArgumentError, "lazy must be false, :multi, or :batch (got #{value.inspect})"
      end

      value
    end

    def instance_urls(url)
      raise ArgumentError, 'url array must not be empty' if url.is_a?(Array) && url.empty?

      url.nil? ? [nil] : Array(url)
    end

    def merged_redis_options(opts, cfg, url)
      defaults = cfg.to_h
      keys = defaults.keys - %i[url pool lazy batch_size]
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
