# frozen_string_literal: true

require 'uri'

require_relative 'round_robin_pool'
require 'redis_client'

module Emb
  # Owns a client's instance fan-out: one RoundRobinPool per configured url,
  # instance-level round-robin on top of each pool's connection rotation, and
  # the pre-send retry across instances.
  #
  # Retry is deliberately limited to PRESEND_CONNECTION_ERROR: a connection
  # that was never established means the command was not written, so moving it
  # to another instance is safe. Errors after a command may have been sent
  # (timeouts, mid-flight connection loss) are never re-dispatched — retrying
  # them would duplicate inference on the server.
  class ConnectionRouter
    # Captured at load so `rescue` keeps working when tests or apps replace the
    # RedisClient constant (stub_const) — a runtime constant lookup in the
    # rescue clause would otherwise resolve against the replacement.
    PRESEND_CONNECTION_ERROR = RedisClient::CannotConnectError

    attr_reader :pools

    # Thread-local label of the instance that last served a command, read by
    # telemetry right after a command returns. host:port only: a redis URL can
    # carry a password in its userinfo, which must never reach a span.
    LAST_INSTANCE = :emb_last_instance

    def self.last_instance = Thread.current[LAST_INSTANCE]

    def initialize(size, urls, redis_options)
      @pools = urls.map do |url|
        RoundRobinPool.new(size) do
          RedisClient.new(url: url, **redis_options)
        end
      end
      @labels = urls.map { |url| instance_label(url) }
      @next_instance = 0
      @instance_mutex = Mutex.new
    end

    def call(*args)
      attempts = @pools.size
      idx = pick_instance
      last_error = nil
      attempts.times do
        return perform_command(@pools[idx], args, @labels[idx])
      rescue PRESEND_CONNECTION_ERROR => e
        last_error = e
        idx = (idx + 1) % @pools.size if @pools.size > 1
      end
      raise last_error
    end

    private

    # host:port of a redis URL. Never the raw URL: its userinfo can hold a
    # password. nil when no URL is configured (redis-client's own defaults), so
    # the attribute is simply absent.
    def instance_label(url)
      return nil if url.nil?

      label = parse_instance_label(url)
      label unless label.nil? || label.empty?
    end

    def parse_instance_label(url)
      uri = URI.parse(url)
      # Everything after the last "@" is safe even when the URL does not parse
      # or has no authority: stripping only before a "/" would leave a password
      # that contains a slash in place.
      return safe_tail(url) if uri.host.to_s.empty?

      "#{uri.host}#{":#{uri.port}" if uri.port}"
    rescue URI::Error, TypeError
      safe_tail(url)
    end

    # host[:port] from a URL that does not parse: everything after the last
    # "@", minus any path.
    def safe_tail(url)
      url.split('@').last.to_s.split('/').first
    end

    def pick_instance
      return 0 if @pools.size == 1

      @instance_mutex.synchronize do
        idx = @next_instance % @pools.size
        @next_instance += 1
        idx
      end
    end

    def perform_command(pool, args, label)
      Thread.current[LAST_INSTANCE] = label if Emb::Telemetry.available?
      debug = Emb.debug?
      started = Process.clock_gettime(Process::CLOCK_MONOTONIC) if debug
      result = pool.with { |r| r.call(*args) }
      log_command(args, started) if debug
      result
    end

    def log_command(args, started)
      elapsed = (Process.clock_gettime(Process::CLOCK_MONOTONIC) - started) * 1000
      $stdout.puts "[EMB] #{args.map(&:inspect).join(' ')} (#{format('%.2f', elapsed)}ms)"
    end
  end
end
