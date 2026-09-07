# frozen_string_literal: true

require 'spec_helper'

class RecordingRedisClient
  attr_reader :calls

  def initialize
    @calls = []
  end

  def call(*args)
    @calls << args
    'PONG'
  end
end

class Tracker
  attr_reader :max_active

  def initialize
    @mutex = Mutex.new
    @active = 0
    @max_active = 0
  end

  def enter
    @mutex.synchronize do
      @active += 1
      @max_active = [@max_active, @active].max
    end
  end

  def leave
    @mutex.synchronize { @active -= 1 }
  end
end

class ConcurrentRedisClient
  attr_reader :max_overlap, :calls

  def initialize(tracker)
    @tracker = tracker
    @guard = Mutex.new
    @active = 0
    @max_overlap = 0
    @calls = []
  end

  def call(*args)
    @tracker.enter
    begin
      @guard.synchronize do
        @active += 1
        @max_overlap = [@max_overlap, @active].max
        @calls << args
      end
      sleep 0.005
      "reply #{args.last}"
    ensure
      @guard.synchronize { @active -= 1 }
      @tracker.leave
    end
  end
end

class RaisingRedisClient
  def call(*)
    raise RedisClient::CommandError, 'ERR busy: max concurrent requests exceeded (10)'
  end
end

class FlakyRedisClient
  def initialize
    @calls = 0
  end

  def call(*_args)
    @calls += 1
    raise Errno::ECONNRESET if @calls == 1

    'PONG'
  end
end

class VecRedisClient
  attr_reader :calls

  def initialize
    @calls = []
  end

  def call(*args)
    @calls << args
    texts = (args.size - 1) / 2
    Array.new(texts) { [1.0].pack('e') * 2 }
  end
end

class StubRedisClient
  attr_reader :options

  def initialize(**options)
    @options = options
  end
end

class QueuedRedisClient
  class << self
    attr_accessor :queue

    def new(*)
      queue.shift
    end
  end
end

RSpec.describe Emb::RoundRobinPool do
  describe 'round-robin selection' do
    it 'rotates through connections in order and wraps around' do
      pool = described_class.new(3) { RecordingRedisClient.new }
      ids = 5.times.map { pool.with(&:object_id) }
      expect(ids[0, 3].uniq).to eq(ids[0, 3])
      expect(ids[3]).to eq(ids[0])
      expect(ids[4]).to eq(ids[1])
    end

    it 'routes every command on a single-connection pool through that connection' do
      pool = described_class.new(1) { RecordingRedisClient.new }
      ids = 3.times.map { pool.with(&:object_id) }
      expect(ids.uniq.size).to eq(1)
    end

    it 'rejects a pool size below 1' do
      expect { described_class.new(0) { RecordingRedisClient.new } }
        .to raise_error(ArgumentError, />= 1/)
    end

    it 'exposes size and the connection list' do
      pool = described_class.new(2) { RecordingRedisClient.new }
      expect(pool.size).to eq(2)
      expect(pool.connections.size).to eq(2)
    end
  end

  describe 'concurrency' do
    it 'never uses one connection from two threads at once and runs in parallel' do
      tracker = Tracker.new
      pool = described_class.new(4) { ConcurrentRedisClient.new(tracker) }
      threads = 16.times.map do |i|
        Thread.new { pool.with { |c| c.call('EMB', 'minilm', "text#{i}") } }
      end
      replies = threads.map(&:value)
      expect(replies.size).to eq(16)
      expect(pool.connections.map(&:max_overlap)).to all(eq(1))
      expect(pool.connections.sum { |c| c.calls.size }).to eq(16)
      expect(tracker.max_active).to be >= 4
    end

    it 'returns each thread its own command reply' do
      tracker = Tracker.new
      pool = described_class.new(4) { ConcurrentRedisClient.new(tracker) }
      threads = 12.times.map do |i|
        Thread.new { pool.with { |c| c.call('EMB', 'minilm', "text#{i}") } }
      end
      replies = threads.map(&:value)
      expect(replies.sort).to eq((0...12).map { |i| "reply text#{i}" }.sort)
    end
  end

  describe 'error propagation' do
    it 'raises server error replies unchanged' do
      pool = described_class.new(2) { RaisingRedisClient.new }
      expect { pool.with { |c| c.call('EMB', 'minilm', 'hello') } }
        .to raise_error(RedisClient::CommandError, /busy/)
    end

    it 'propagates a transient transport error and recovers on the next use' do
      pool = described_class.new(1) { FlakyRedisClient.new }
      expect { pool.with { |c| c.call('PING') } }.to raise_error(Errno::ECONNRESET)
      expect(pool.with { |c| c.call('PING') }).to eq('PONG')
    end
  end

  describe 'reentrancy' do
    it 'hands the same connection to a nested with from the same thread' do
      pool = described_class.new(1) { RecordingRedisClient.new }
      result = pool.with do |conn|
        expect(pool.with { |c| c }).to equal(conn)
        'done'
      end
      expect(result).to eq('done')
    end

    it 're-enters the held connection without re-locking even with pool > 1' do
      pool = described_class.new(2) { RecordingRedisClient.new }
      inner = nil
      pool.with { |_conn| inner = pool.with { |c| c } }
      expect(inner).to equal(pool.connections[0])
    end

    it 'keeps held state per pool when pools nest (regression: shared thread slot)' do
      big = described_class.new(2) { RecordingRedisClient.new }
      small = described_class.new(1) { RecordingRedisClient.new }

      # Advance big's rotation so the nested big.with holds index 1, then call
      # small.with inside: with a thread-wide index slot the index would leak
      # across pools and yield nil (or an unlocked shared connection).
      big.with { |_conn| :advance }

      held = nil
      big.with do |conn|
        expect(conn).to equal(big.connections[1])
        held = small.with { |c| c }
      end

      expect(held).to equal(small.connections[0])
    end
  end

  describe 'fork safety' do
    it 'keeps working in the child after fork without sharing connections' do
      skip 'Process.fork unavailable' unless Process.respond_to?(:fork)

      pool = described_class.new(2) { RedisClient.new(url: 'redis://localhost:16379', protocol: 2) }
      expect(pool.with { |c| c.call('PING') }).to eq('PONG')

      pid = Process.fork do
        result = pool.with { |c| c.call('PING') }
        exit!(result == 'PONG' ? 0 : 1)
      rescue StandardError
        exit!(2)
      end
      _, status = Process.wait2(pid)
      expect(status.exitstatus).to eq(0)
    end
  end

  describe 'Emb::Client integration' do
    it 'creates exactly pool RedisClients, eagerly' do
      client = Emb::Client.new(port: 16_379, pool: 3)
      expect(client.pools).to contain_exactly(be_a(described_class))
      expect(client.pools.first.size).to eq(3)
      expect(client.pools.first.connections).to all(be_a(RedisClient))
    end

    it 'forwards redis options to every pooled connection' do
      stub_const('RedisClient', StubRedisClient)
      client = Emb::Client.new(
        port: 16_379, pool: 2,
        reconnect_attempts: 2, protocol: 2,
        connect_timeout: 1.5, read_timeout: 3, write_timeout: 4,
        driver: :hiredis,
        ssl: true, ssl_params: { verify_mode: 0 }
      )
      options = client.pools.first.connections.map(&:options)
      expect(options.size).to eq(2)
      expect(options).to all(include(
                               reconnect_attempts: 2, protocol: 2,
                               connect_timeout: 1.5, read_timeout: 3, write_timeout: 4,
                               driver: :hiredis,
                               ssl: true, ssl_params: { verify_mode: 0 }
                             ))
      expect(options.uniq.size).to eq(1)
    end
  end

  describe 'multi-instance distribution' do
    it 'rotates across instances first, then across connections within an instance' do
      conns = %w[a1 a2 b1 b2 c1 c2].map do |id|
        RecordingRedisClient.new.tap { |c| c.instance_variable_set(:@id, id) }
      end
      # queue.shift drains the array it is handed; feed it a copy so `conns`
      # still names every connection for the assertions below.
      QueuedRedisClient.queue = conns.dup
      stub_const('RedisClient', QueuedRedisClient)
      client = Emb::Client.new(url: %w[redis://a redis://b redis://c], pool: 2)

      expect(client.pools.size).to eq(3)
      ids = 6.times.map do |n|
        client.send_command('PING', n)
        conns.find { |c| c.calls.last == ['PING', n] }.instance_variable_get(:@id)
      end

      expect(ids).to eq(%w[a1 b1 c1 a2 b2 c2])
    end

    it 'sends each command to the instance selected in rotation' do
      conns = [RecordingRedisClient.new, RecordingRedisClient.new]
      QueuedRedisClient.queue = conns.dup
      stub_const('RedisClient', QueuedRedisClient)
      # pool 1 keeps construction at exactly one connection per instance; the
      # default pool (5) would drain the two-entry queue into nil connections.
      client = Emb::Client.new(url: %w[redis://a redis://b], pool: 1)

      client.send_command('PING')
      client.send_command('PING')
      first, second = conns.map { |c| c.calls.size }

      expect(first).to eq(1)
      expect(second).to eq(1) # one command per instance
    end

    it 'keeps working in the child after fork with multiple per-instance pools' do
      skip 'Process.fork unavailable' unless Process.respond_to?(:fork)

      client = Emb::Client.new(
        url: ['redis://localhost:16379', 'redis://localhost:16379'], pool: 1
      )
      expect(client.pools.size).to eq(2)
      expect(client.ping).to eq('PONG')

      pid = Process.fork do
        result = client.ping
        exit!(result == 'PONG' ? 0 : 1)
      rescue StandardError
        exit!(2)
      end
      _, status = Process.wait2(pid)
      expect(status.exitstatus).to eq(0)
    end
  end

  describe 'Emb::Batch chunk rotation' do
    after { BatchLoader::Executor.clear_current }

    it 'spreads EMB chunks across rotated connections within one window (serial :multi mode)' do
      conn_a = VecRedisClient.new
      conn_b = VecRedisClient.new
      QueuedRedisClient.queue = [conn_a, conn_b]
      stub_const('RedisClient', QueuedRedisClient)
      client = Emb::Client.new(pool: 2, lazy: :multi, batch_size: 1)

      first = client[:minilm]['a']
      second = client[:minilm]['b']

      expect(first.first).to eq(1.0)
      expect(second.first).to eq(1.0)

      expect(conn_a.calls).to eq([%w[EMB minilm a]])
      expect(conn_b.calls).to eq([%w[EMB minilm b]])
    end
  end
end
