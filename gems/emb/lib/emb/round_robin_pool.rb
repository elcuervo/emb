# frozen_string_literal: true

module Emb
  # A thread-safe pool of N RedisClient connections with round-robin selection.
  # Behind connection-level load balancers (AWS Service Connect, an NLB, or an
  # Envoy TCP proxy) each keep-alive connection is pinned to one upstream
  # instance, so rotating commands across the pool spreads traffic across every
  # instance — even single-threaded at zero concurrency. Connections are created
  # up front but connect lazily on first use.
  #
  # Two behaviors deliberately match the connection_pool gem it replaces: a
  # nested `with` from the same thread re-enters the held connection, and after
  # `fork` (Puma preload_app, unicorn, resque) the pool closes inherited sockets
  # and rebuilds its mutexes in the child so parent and child never share a
  # connection.
  class RoundRobinPool
    # Pools are tracked only to reset them in forked children. WeakMap so a
    # pool is reclaimed together with its client.
    INSTANCES = Process.respond_to?(:fork) ? ObjectSpace::WeakMap.new : nil
    private_constant :INSTANCES

    THREAD_KEY = :emb_round_robin_pool_held
    private_constant :THREAD_KEY

    attr_reader :size, :connections

    def self.after_fork
      INSTANCES&.each_value(&:reload_after_fork!)
    end

    def initialize(size, &)
      raise ArgumentError, "pool size must be >= 1 (got #{size})" if size < 1

      @size = size
      @connections = Array.new(size, &)
      @locks = Array.new(size) { Mutex.new }
      @next = 0
      @index_mutex = Mutex.new
      INSTANCES&.[]=(self, self)
    end

    # Yields the next connection in rotation order. Safe from multiple threads:
    # up to `size` commands run in parallel, each on its own connection. A
    # nested `with` from the same thread re-enters the held connection without
    # re-locking.
    def with
      held = Thread.current[THREAD_KEY]
      if held
        yield @connections[held]
      else
        idx, connection = pick
        Thread.current[THREAD_KEY] = idx
        begin
          @locks[idx].synchronize { yield connection }
        ensure
          Thread.current[THREAD_KEY] = nil
        end
      end
    end

    # Child side of after_fork(): drop inherited sockets and sync state.
    def reload_after_fork!
      @connections.each { |conn| conn.close if conn.respond_to?(:close) }
      @locks = Array.new(@size) { Mutex.new }
      @next = 0
      @index_mutex = Mutex.new
    end

    if Process.respond_to?(:fork)
      # Hooks Process._fork (MRI 3.1+) so registered pools reset in the child.
      module ForkTracker
        def _fork
          pid = super
          RoundRobinPool.after_fork if pid.zero?
          pid
        end
      end
      Process.singleton_class.prepend(ForkTracker)
    end

    private

    def pick
      @index_mutex.synchronize do
        idx = @next % @size
        @next += 1
        [idx, @connections[idx]]
      end
    end
  end
end
