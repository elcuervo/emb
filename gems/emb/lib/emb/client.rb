# frozen_string_literal: true

require 'connection_pool'
require 'redis_client'

module Emb
  DEFAULTS = { host: 'localhost', port: 6379, pool: 5 }.freeze

  class Client
    attr_reader :pool

    def initialize(pool: DEFAULTS[:pool], **redis_options)
      url = extract_url!(redis_options)
      redis_options[:host] ||= DEFAULTS[:host] unless url
      redis_options[:port] ||= DEFAULTS[:port] unless url
      redis_options[:protocol] ||= 2
      redis_options[:reconnect_attempts] ||= 3

      @pool = ConnectionPool.new(size: pool) do
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

    private

    def extract_url!(opts)
      url = opts.delete(:url)
      url.nil? ? ENV.fetch('EMB_URL', nil) : url
    end
  end
end
