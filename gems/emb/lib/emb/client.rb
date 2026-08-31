# frozen_string_literal: true

require 'connection_pool'
require 'redis_client'

module Emb
  class Client
    attr_reader :pool

    def initialize(pool: nil, batch: nil, **redis_options)
      cfg = Emb.configuration
      @batch_enabled = batch.nil? ? cfg.batch : batch
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

    def stats
      raw = send_command('EMB.STATS')
      raw.each_slice(2).to_h { |key, value| [key.to_sym, value] }
    end

    # Server-wide INFO (the Redis-style sectioned command). No sections =
    # all sections; any number of sections filter the reply.
    def server_info(*sections)
      parse_info(send_command('INFO', *sections.map(&:to_s)))
    end

    # Hot config read: CONFIG GET with optional glob pattern(s).
    # Returns a Hash of String parameter → String value (config is text; no
    # numeric coercion so values round-trip into config_set).
    def config_get(*patterns)
      raw = send_command('CONFIG', 'GET', *patterns.map(&:to_s))
      raw.each_slice(2).to_h { |key, value| [key, value] }
    end

    # Hot config change: CONFIG SET. Returns true on success; server errors
    # (unknown/read-only parameter, invalid value, NOAUTH) propagate as
    # exceptions.
    # rubocop:disable Naming/PredicateMethod -- intentional API: success ⇒ true
    def config_set(param, value)
      send_command('CONFIG', 'SET', param.to_s, value.to_s)
      true
    end
    # rubocop:enable Naming/PredicateMethod

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
      keys = defaults.keys - %i[url pool batch]
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

    # Parse Redis INFO section text into a nested Hash:
    #   {Server: {redis_version: "0.2.4", uptime_secs: "7"}, Cache: {…}, …}
    # Section names and keys are Symbols; values pass through as the server
    # sent them (redis_client decodes RESP integers as Integer already).
    # Lines that don't fit the grammar are ignored.
    def parse_info(text)
      sections = {}
      current = nil

      text.split("\r\n").each do |line|
        if line.start_with?('# ')
          current = line[2..].to_sym
          sections[current] ||= {}
        elsif current && line.include?(':')
          key, value = line.split(':', 2)
          sections[current][key.to_sym] = value
        end
      end

      sections
    end
  end
end
