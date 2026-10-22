# frozen_string_literal: true

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

    # host[:port] of a redis URL. Never the URL text itself: its userinfo can
    # hold a password and its query can hold one. Parsed by hand rather than
    # with URI, which accepts some credential shapes as host and port.
    # nil when nothing is left (no URL configured) so the attribute is absent.
    def instance_label(url)
      return nil if url.nil?

      label = url.to_s.split(/[?#]/, 2).first.split('://', 2).last.to_s
      userinfo = label.rindex('@')
      label = label[(userinfo + 1)..] if userinfo
      label = label.split('/').first.to_s
      label unless label.empty?
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
