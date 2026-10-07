# frozen_string_literal: true

module Emb
  # A thread-safe pool of N RedisClient connections with work-conserving
  # round-robin selection. Behind connection-level load balancers (AWS Service
  # Connect, an NLB, or an Envoy TCP proxy) each keep-alive connection is pinned
  # to one upstream instance, so rotating commands across the pool spreads
  # traffic across every instance — even single-threaded at zero concurrency.
  # Connections are created up front but connect lazily on first use.
  #
  # Selection is work-conserving: a command takes the next *available*
  # connection, never waiting behind a busy connection while another is free.
  # A released connection returns to the tail of the free queue, so sequential
  # commands still rotate in order.
  #
  # Two behaviors deliberately match the connection_pool gem it replaces: a
  # nested `with` from the same thread re-enters the held connection, and after
  # `fork` (Puma preload_app, unicorn, resque) the pool closes inherited sockets
  # and rebuilds its free queue in the child so parent and child never share a
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
      @free = free_indices
      INSTANCES&.[]=(self, self)
    end

    # Yields an available connection. Safe from multiple threads: up to `size`
    # commands run in parallel, each on its own connection; a command beyond that
    # waits for the first connection to free. A nested `with` from the same
    # thread re-enters the connection this pool already holds; other pools are
    # unaffected.
    def with(&)
      held = Thread.current[THREAD_KEY]
      if held&.key?(self)
        yield @connections[held[self]]
      else
        acquire(held, &)
      end
    end

    # Child side of after_fork(): drop inherited sockets and sync state.
    def reload_after_fork!
      @connections.each { |conn| conn.close if conn.respond_to?(:close) }
      @free = free_indices
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

    def free_indices
      free = Queue.new
      @size.times { |idx| free << idx }
      free
    end

    # Takes the next free connection, records it as held by this thread/pool,
    # and returns it to the queue on exit — even when the block raises.
    def acquire(held)
      idx = @free.pop
      held ||= {}
      Thread.current[THREAD_KEY] = held
      held[self] = idx
      begin
        yield @connections[idx]
      ensure
        held.delete(self)
        Thread.current[THREAD_KEY] = nil if held.empty?
        @free << idx
      end
    end
  end
end
