# frozen_string_literal: true

require 'spec_helper'

class FakeEmbClient
  attr_reader :commands
  attr_accessor :batch_size

  def initialize(responses = {})
    @responses = responses
    @commands = []
  end

  def send_command(*args)
    @commands << args
    @responses.fetch(args)
  end

  # float32 binary blob holding `dim` copies of `value`
  def self.vec(value, dim: 2)
    [value].pack('e') * dim
  end
end

# FailingEmbClient simulates redis-client's ensure_connected retry loop: a
# transient failure (ConnectionError/ProtocolError) re-sends up to
# `reconnect_attempts` extra times (recording each wire send) before raising.
# `timeouts: k` fails the first k sends then succeeds; nil (default) always
# fails. `error:` selects the raised class — only transient errors retry,
# mirroring redis-client; operation errors surface on the first send.
class FailingEmbClient < FakeEmbClient
  attr_accessor :reconnect_attempts, :timeouts

  def initialize(responses = {}, reconnect_attempts: 0, timeouts: nil, error: RedisClient::ReadTimeoutError)
    super(responses)
    @reconnect_attempts = reconnect_attempts
    @timeouts = timeouts
    @error = error
  end

  def send_command(*args)
    1.upto(total_attempts) do |attempt|
      @commands << args
      return @responses.fetch(args) if recovered?(attempt)

      raise @error, 'simulated failure' unless retry?(attempt)
    end
  end

  private

  def total_attempts
    budget = @reconnect_attempts
    budget = budget.count(&:itself) if budget.is_a?(Array)
    budget = 0 if budget == false
    budget + 1
  end

  def recovered?(attempt)
    @timeouts && attempt > @timeouts
  end

  def retry?(attempt)
    (@error <= RedisClient::ConnectionError || @error <= RedisClient::ProtocolError) && attempt < total_attempts
  end
end

# MixedFailureEmbClient models redis-client's mixed-failure sequence: within a
# single send_command the command times out (would be retried internally) and
# the re-send comes back as an operation error. Records both wire sends.
class MixedFailureEmbClient < FakeEmbClient
  def send_command(*args)
    2.times { @commands << args }
    raise RedisClient::CommandError, 'server rejected the re-send'
  end
end

# Latches until all `n` workers are inside a command, so parallel dispatch is
# provably overlapped without wall-clock sleeps. @arrived is monotonic: a
# worker that reaches the barrier first waits for the rest, and finished
# workers never decrement it, so a recheck after the wake can never miss.
class LatchTracker
  attr_reader :max_overlap

  def initialize(count)
    @n = count
    @mutex = Mutex.new
    @cv = ConditionVariable.new
    @arrived = 0
    @released = 0
    @max_overlap = 0
  end

  def enter
    @mutex.synchronize do
      @arrived += 1
      @max_overlap = [@max_overlap, @arrived].max
      if @arrived == @n
        @cv.broadcast
      else
        @cv.wait(@mutex) while @arrived < @n
      end
    end
  end

  def leave
    @mutex.synchronize { @released += 1 }
  end
end

# Simulates a lazy: :batch client at the BATCH_BLOCK level: parallel_batch?
# selects the concurrent dispatch path regardless of class.
class ParallelFakeEmbClient < FakeEmbClient
  attr_reader :tracker

  def initialize(responses = {}, tracker: nil)
    super(responses)
    @tracker = tracker
  end

  def parallel_batch? = true

  def send_command(*args)
    @tracker&.enter
    super
  ensure
    @tracker&.leave
  end
end

# Connection-level error classes, captured at load BEFORE any stub_const
# replaces the RedisClient constant, so pool/rescue paths and fake clients can
# raise the real classes regardless of stubbing.
CONNECTION_REFUSED = RedisClient::CannotConnectError
READ_TIMEOUT = RedisClient::ReadTimeoutError

# Call-capable fake connection for pool-backed clients (the pool calls #call).
class QueueCallClient
  attr_reader :commands

  def initialize(error: nil, responses: {})
    @error = error
    @responses = responses
    @commands = []
  end

  def call(*args)
    @commands << args
    raise @error, 'simulated' if @error

    @responses.fetch(args)
  end
end

# Fails one specific command like a timed-out server, succeeds on the rest.
class OneFailClient < ParallelFakeEmbClient
  def initialize(fail_args, responses = {})
    super(responses)
    @fail_args = fail_args
  end

  def send_command(*args)
    if args == @fail_args
      @commands << args
      raise READ_TIMEOUT, 'simulated timeout'
    end

    super
  end
end

# Hands out pre-built connections in order (same trick as the pool spec).
class QueueRedisClient
  class << self
    attr_accessor :queue

    def new(*) = queue.shift
  end
end

RSpec.describe Emb do
  after { BatchLoader::Executor.clear_current }

  describe 'Emb::BATCH_BLOCK (unit)' do
    it 'expands a single-text item into one EMB command and unpacks with e*' do
      client = FakeEmbClient.new(
        %w[EMB minilm hello] => [FakeEmbClient.vec(1.5, dim: 3)]
      )
      item = [client, :minilm, 'hello']
      loaded = []

      Emb::BATCH_BLOCK.call([item], ->(i, v) { loaded << [i, v] }, {})

      expect(client.commands).to eq([%w[EMB minilm hello]])
      expect(loaded).to eq([[item, [1.5, 1.5, 1.5]]])
    end

    it 'expands a multi-text item into one pair per text and regroups to an array' do
      client = FakeEmbClient.new(
        %w[EMB minilm a b] => [FakeEmbClient.vec(3.0), FakeEmbClient.vec(4.0)]
      )
      item = [client, :minilm, %w[a b]]
      loaded = []

      Emb::BATCH_BLOCK.call([item], ->(i, v) { loaded << [i, v] }, {})

      expect(client.commands).to eq([%w[EMB minilm a b]])
      expect(loaded.first.last).to eq([[3.0, 3.0], [4.0, 4.0]])
    end

    it 'returns a single vector for a one-text multi-text item (eager shape parity)' do
      client = FakeEmbClient.new(
        %w[EMB minilm solo] => [FakeEmbClient.vec(9.0)]
      )
      item = [client, :minilm, ['solo']]
      loaded = []

      Emb::BATCH_BLOCK.call([item], ->(i, v) { loaded << [i, v] }, {})

      expect(loaded.first.last).to eq([9.0, 9.0])
    end

    it 'coalesces mixed models into one MULTI preserving per-pair order' do
      client = FakeEmbClient.new(
        ['EMB.MULTI', 'minilm', 'a', 'bge', 'b'] => [FakeEmbClient.vec(1.0), FakeEmbClient.vec(2.0)]
      )
      items = [[client, :minilm, 'a'], [client, :bge, 'b']]
      loaded = []

      Emb::BATCH_BLOCK.call(items, ->(i, v) { loaded << [i, v] }, {})

      expect(client.commands).to eq([['EMB.MULTI', 'minilm', 'a', 'bge', 'b']])
      expect(loaded.map(&:last)).to eq([[1.0, 1.0], [2.0, 2.0]])
    end

    it 'preserves per-pair ordering across mixed single and multi-text items' do
      client = FakeEmbClient.new(
        ['EMB.MULTI', 'minilm', 'x', 'minilm', 'y', 'minilm', 'z', 'bge', 'w'] =>
          [FakeEmbClient.vec(1.0), FakeEmbClient.vec(2.0), FakeEmbClient.vec(3.0), FakeEmbClient.vec(4.0)]
      )
      items = [[client, :minilm, %w[x y]], [client, :minilm, 'z'], [client, :bge, 'w']]
      loaded = []

      Emb::BATCH_BLOCK.call(items, ->(i, v) { loaded << [i, v] }, {})

      expect(loaded.map(&:last)).to eq(
        [
          [[1.0, 1.0], [2.0, 2.0]],
          [3.0, 3.0],
          [4.0, 4.0]
        ]
      )
    end

    it 'maps server nulls to nil per pair with healthy siblings intact' do
      client = FakeEmbClient.new(
        ['EMB.MULTI', 'minilm', 'ok', 'ghost', 'nope'] => [FakeEmbClient.vec(1.0), nil]
      )
      items = [[client, :minilm, 'ok'], [client, :ghost, 'nope']]
      loaded = []

      Emb::BATCH_BLOCK.call(items, ->(i, v) { loaded << [i, v] }, {})

      expect(loaded.map(&:last)).to eq([[1.0, 1.0], nil])
    end

    it 'groups items by client so instance clients hit their own server' do
      c1 = FakeEmbClient.new(%w[EMB minilm a] => [FakeEmbClient.vec(1.0)])
      c2 = FakeEmbClient.new(%w[EMB bge b] => [FakeEmbClient.vec(2.0)])
      loaded = []

      Emb::BATCH_BLOCK.call(
        [[c1, :minilm, 'a'], [c2, :bge, 'b']],
        ->(i, v) { loaded << [i, v] },
        {}
      )

      expect(c1.commands).to eq([%w[EMB minilm a]])
      expect(c2.commands).to eq([%w[EMB bge b]])
    end
  end

  describe 'create-then-consume coalescing' do
    # Different helper definitions = different source locations: loaders must
    # still land in one batch (batch-key regression guard).
    def loader_from_site_a(client, text)
      Emb.build_batch_loader(client, :minilm, text)
    end

    def loader_from_site_b(client, text)
      Emb.build_batch_loader(client, :minilm, text)
    end

    it 'coalesces loaders from different call sites into one EMB' do
      client = FakeEmbClient.new(
        %w[EMB minilm a b c] =>
          [FakeEmbClient.vec(1.0), FakeEmbClient.vec(2.0), FakeEmbClient.vec(3.0)]
      )

      l1 = loader_from_site_a(client, 'a')
      l2 = loader_from_site_b(client, 'b')
      l3 = loader_from_site_a(client, 'c')

      expect(l1.first).to eq(1.0)
      expect(l2.first).to eq(2.0)
      expect(l3.first).to eq(3.0)

      expect(client.commands).to eq([%w[EMB minilm a b c]])
    end

    it 'coalesces loaders for different models into one MULTI' do
      client = FakeEmbClient.new(
        ['EMB.MULTI', 'minilm', 'a', 'bge', 'b'] => [FakeEmbClient.vec(1.0), FakeEmbClient.vec(2.0)]
      )

      minilm = described_class.build_batch_loader(client, :minilm, 'a')
      bge = described_class.build_batch_loader(client, :bge, 'b')

      expect(minilm.first).to eq(1.0)
      expect(bge.first).to eq(2.0)
      expect(client.commands).to eq([['EMB.MULTI', 'minilm', 'a', 'bge', 'b']])
    end

    it 'sends a fresh command only for loaders created after a flush' do
      client = FakeEmbClient.new(
        %w[EMB minilm a b] => [FakeEmbClient.vec(1.0), FakeEmbClient.vec(2.0)],
        %w[EMB minilm c] => [FakeEmbClient.vec(3.0)]
      )

      first = described_class.build_batch_loader(client, :minilm, 'a')
      second = described_class.build_batch_loader(client, :minilm, 'b')
      first.sum
      second.sum

      third = described_class.build_batch_loader(client, :minilm, 'c')
      expect(third.first).to eq(3.0)

      expect(client.commands).to eq(
        [
          %w[EMB minilm a b],
          %w[EMB minilm c]
        ]
      )
    end
  end

  describe 'caching within a scope' do
    it 'sends exactly one command for repeated use of the same loader' do
      client = FakeEmbClient.new(
        %w[EMB minilm hello] => [FakeEmbClient.vec(5.0)]
      )
      loader = described_class.build_batch_loader(client, :minilm, 'hello')

      3.times { expect(loader.first).to eq(5.0) }

      expect(client.commands.size).to eq(1)
    end

    it 'deduplicates identical pairs into one pair sent once' do
      client = FakeEmbClient.new(
        %w[EMB minilm dup] => [FakeEmbClient.vec(7.0)]
      )

      a = described_class.build_batch_loader(client, :minilm, 'dup')
      b = described_class.build_batch_loader(client, :minilm, 'dup')

      expect(a.first).to eq(7.0)
      expect(b.first).to eq(7.0)
      expect(client.commands).to eq([%w[EMB minilm dup]])
    end

    it 'materializes failed pairs as nil without re-sending' do
      client = FakeEmbClient.new(
        ['EMB.MULTI', 'minilm', 'ok', 'ghost', 'x'] => [FakeEmbClient.vec(1.0), nil]
      )

      ok = described_class.build_batch_loader(client, :minilm, 'ok')
      ghost = described_class.build_batch_loader(client, :ghost, 'x')

      expect(ok.first).to eq(1.0)
      expect(ghost).to be_nil
      expect(ghost).to be_nil # cached nil, no second command
      expect(client.commands.size).to eq(1)

      # A later loader for the already-synced failed item does not re-send.
      again = described_class.build_batch_loader(client, :ghost, 'x')
      expect(again).to be_nil
      expect(client.commands.size).to eq(1)
    end
  end

  describe 'removed explicit batch API' do
    it 'raises NoMethodError on Emb.batch and client.batch' do
      expect { described_class.batch }.to raise_error(NoMethodError)

      client = Emb::Client.new(port: 16_379)
      expect { client.batch }.to raise_error(NoMethodError)
    end
  end

  describe 'the lazy mode option' do
    it 'defaults to eager: one immediate EMB per call' do
      client = Emb::Client.new
      expect(client.lazy?).to be false

      log = []
      client.define_singleton_method(:send_command) do |*args|
        log << args
        [FakeEmbClient.vec(4.0, dim: 384)]
      end

      result = client[:minilm]['hello world']

      expect(log).to eq([['EMB', 'minilm', 'hello world']])
      expect(result).to be_a(Array)
      expect(result.size).to eq(384)
      expect(result.first).to be_a(Float)
    end

    it 'defers and coalesces under lazy: :multi' do
      client = Emb::Client.new(lazy: :multi)
      expect(client.lazy?).to be true

      log = []
      client.define_singleton_method(:send_command) do |*args|
        log << args
        [FakeEmbClient.vec(4.0, dim: 384)]
      end

      result = client[:minilm]['hello world']
      expect(log).to be_empty

      expect(result.sum).to be_within(0.001).of(4.0 * 384)
      expect(log).to eq([['EMB', 'minilm', 'hello world']])
    end

    it 'defers and flushes under lazy: :batch (single chunk is serial)' do
      client = Emb::Client.new(lazy: :batch)
      log = []
      client.define_singleton_method(:send_command) do |*args|
        log << args
        [FakeEmbClient.vec(2.5, dim: 384)]
      end

      vec = client[:minilm]['hello world']
      expect(log).to be_empty

      expect(vec.sum).to be_within(0.001).of(2.5 * 384)
      expect(log).to eq([['EMB', 'minilm', 'hello world']])
    end

    it 'returns Array of Array of Float for multi-text in a deferred mode' do
      client = Emb::Client.new(lazy: :multi)
      log = []
      client.define_singleton_method(:send_command) do |*args|
        log << args
        [FakeEmbClient.vec(1.0, dim: 384), FakeEmbClient.vec(2.0, dim: 384)]
      end

      vecs = client[:minilm]['hello', 'world']
      expect(vecs).to be_an(Array)
      expect(vecs.length).to eq(2)
      expect(vecs.first).to be_an(Array)
      expect(vecs.first.size).to eq(384)
      expect(vecs.last.first).to eq(2.0)
      expect(log).to eq([%w[EMB minilm hello world]])
    end
  end

  describe 'Emb.multi remains eager and untouched' do
    it 'works as before on a default (eager) client' do
      client = Emb::Client.new
      log = []
      client.define_singleton_method(:send_command) do |*args|
        log << args
        [FakeEmbClient.vec(1.0, dim: 384), FakeEmbClient.vec(2.0, dim: 384)]
      end

      results = client.multi do |m|
        m[:minilm]['hello']
        m[:minilm]['world']
      end

      expect(results.length).to eq(2)
      expect(results.first.first).to eq(1.0)
      expect(log).to eq([['EMB.MULTI', 'minilm', 'hello', 'minilm', 'world']])
      expect(results.first).to be_an(Array)
    end

    it 'still works on a lazy: :multi client' do
      client = Emb::Client.new(lazy: :multi)
      client.define_singleton_method(:send_command) do |*_args|
        [FakeEmbClient.vec(3.0, dim: 384)]
      end

      results = client.multi do |m|
        m[:minilm]['hello']
      end

      expect(results.first.first).to eq(3.0)
    end

    it 'does not interact with the batch scope' do
      client = Emb::Client.new(lazy: :multi)
      log = []
      client.define_singleton_method(:send_command) do |*args|
        log << args
        [FakeEmbClient.vec(1.0, dim: 384), FakeEmbClient.vec(2.0, dim: 384)]
      end

      _pending = client[:minilm]['never used']
      results = client.multi do |m|
        m[:minilm]['hello']
        m[:minilm]['world']
      end

      # The explicit multi ran its own EMB.MULTI; the unused lazy loader
      # (same thread, same scope) must not ride along or fire.
      expect(results.length).to eq(2)
      expect(log.size).to eq(1)
      expect(log.first).to eq(['EMB.MULTI', 'minilm', 'hello', 'minilm', 'world'])
    end
  end

  describe 'Emb::Middleware' do
    before { described_class.configure { |c| c.lazy = :multi } }
    after { described_class.instance_variable_set(:@configuration, Emb::Configuration.new) }

    it 'clears the per-thread scope at the end of each request' do
      client = FakeEmbClient.new(
        %w[EMB minilm hello] => [FakeEmbClient.vec(1.0)]
      )
      app = lambda do |_env|
        described_class.build_batch_loader(client, :minilm, 'hello').first
        [200, {}, []]
      end
      middleware = Emb::Middleware.new(app)

      expect(middleware.call({})).to eq([200, {}, []])
      expect(client.commands.size).to eq(1)

      # A fresh request starts a new scope: the same pair must re-send.
      expect(middleware.call({})).to eq([200, {}, []])
      expect(client.commands.size).to eq(2)
    end

    it 'clears the scope even when the app raises' do
      middleware = Emb::Middleware.new(->(_env) { raise 'boom' })

      expect { middleware.call({}) }.to raise_error('boom')
      expect(BatchLoader::Executor.current).to be_nil
    end

    it 'is inert under the eager default: passthrough, nothing deferred, no scope left' do
      described_class.configure { |c| c.lazy = false }
      middleware = Emb::Middleware.new(->(_env) { [200, {}, []] })

      expect(middleware.call({})).to eq([200, {}, []])
      expect(BatchLoader::Executor.current).to be_nil
    end
  end

  describe 'parallel batch execution (lazy: :batch, unit)' do
    it 'dispatches chunk shares concurrently and returns results in deferral order' do
      tracker = LatchTracker.new(2)
      client = ParallelFakeEmbClient.new(
        { %w[EMB minilm a] => [FakeEmbClient.vec(1.0)],
          %w[EMB minilm b] => [FakeEmbClient.vec(2.0)] },
        tracker: tracker
      )
      client.batch_size = 1
      items = [[client, :minilm, 'a'], [client, :minilm, 'b']]
      loaded = []

      Emb::BATCH_BLOCK.call(items, ->(i, v) { loaded << [i, v] }, {})

      # Both workers entered send_command before either finished: provably
      # concurrent, no sleeps.
      expect(tracker.max_overlap).to eq(2)
      expect(loaded.map(&:last)).to eq([[1.0, 1.0], [2.0, 2.0]])
    end

    it 'reassembles results in deferral order across many shares' do
      responses = {}
      ('a'..'d').each { |t| responses[['EMB', 'minilm', t]] = [FakeEmbClient.vec(t.ord.to_f)] }
      client = ParallelFakeEmbClient.new(responses)
      client.batch_size = 1
      items = ('a'..'d').map { |t| [client, :minilm, t] }
      loaded = []

      Emb::BATCH_BLOCK.call(items, ->(i, v) { loaded << [i, v] }, {})

      expect(loaded.map(&:last)).to eq([[97.0, 97.0], [98.0, 98.0], [99.0, 99.0], [100.0, 100.0]])
      expect(client.commands.size).to eq(4)
    end

    it 'executes a single-chunk scope without spinning up workers' do
      client = ParallelFakeEmbClient.new(
        { %w[EMB minilm solo] => [FakeEmbClient.vec(5.0)] }
      )
      loaded = []

      Emb::BATCH_BLOCK.call([[client, :minilm, 'solo']], ->(i, v) { loaded << [i, v] }, {})

      expect(loaded.map(&:last)).to eq([[5.0, 5.0]])
      expect(client.commands).to eq([%w[EMB minilm solo]])
    end

    it 'fails closed on a terminal share failure: raises once, successful share consumed, failed items cleared' do
      client = OneFailClient.new(
        %w[EMB minilm a],
        { %w[EMB minilm b] => [FakeEmbClient.vec(2.0)] }
      )
      client.batch_size = 1
      a = described_class.build_batch_loader(client, :minilm, 'a')
      b = described_class.build_batch_loader(client, :minilm, 'b')

      expect { a.first }.to raise_error(Emb::ServerError) do |e|
        expect(e.cause).to be_a(READ_TIMEOUT)
      end

      # The successful share resolved normally and its command is not re-sent.
      expect(b).to eq([2.0, 2.0])
      # The failed item resolves to the [] default with no further I/O.
      expect(a.__send__(:__sync)).to eq([])
      expect(client.commands.size).to eq(2)
      expect(BatchLoader::Executor.current.items_by_block.values.sum(&:size)).to eq(0)
    end
  end

  describe 'multi-instance pre-send retry (real client)' do
    it 'retries a connection-refused instance on the next instance' do
      dead = QueueCallClient.new(error: CONNECTION_REFUSED)
      alive = QueueCallClient.new(responses: { %w[EMB minilm hello] => [FakeEmbClient.vec(9.0)] })
      QueueRedisClient.queue = [dead, alive]
      stub_const('RedisClient', QueueRedisClient)
      client = Emb::Client.new(url: %w[redis://a redis://b], pool: 1)

      result = client[:minilm]['hello']

      expect(result).to eq([9.0, 9.0])
      expect(dead.commands).to eq([%w[EMB minilm hello]])
      expect(alive.commands).to eq([%w[EMB minilm hello]])
    end

    it 'never re-dispatches after a read timeout' do
      timeout = QueueCallClient.new(error: READ_TIMEOUT)
      alive = QueueCallClient.new(responses: { %w[EMB minilm hello] => [FakeEmbClient.vec(9.0)] })
      QueueRedisClient.queue = [timeout, alive]
      stub_const('RedisClient', QueueRedisClient)
      client = Emb::Client.new(url: %w[redis://a redis://b], pool: 1)

      expect { client[:minilm]['hello'] }.to raise_error(READ_TIMEOUT, 'simulated')

      expect(timeout.commands).to eq([%w[EMB minilm hello]])
      expect(alive.commands).to be_empty
    end

    it 'raises after all instances refuse a connection' do
      dead1 = QueueCallClient.new(error: CONNECTION_REFUSED)
      dead2 = QueueCallClient.new(error: CONNECTION_REFUSED)
      QueueRedisClient.queue = [dead1, dead2]
      stub_const('RedisClient', QueueRedisClient)
      client = Emb::Client.new(url: %w[redis://a redis://b], pool: 1)

      expect { client[:minilm]['hello'] }.to raise_error(CONNECTION_REFUSED, 'simulated')
      expect(dead1.commands.size).to eq(1)
      expect(dead2.commands.size).to eq(1)
    end
  end

  describe 'Emb::BATCH_BLOCK chunking (unit)' do
    it 'chunks a large scope into multiple EMBs at batch_size texts' do
      client = FakeEmbClient.new(
        %w[EMB minilm a b] => [FakeEmbClient.vec(1.0), FakeEmbClient.vec(2.0)],
        %w[EMB minilm c] => [FakeEmbClient.vec(3.0)]
      )
      client.batch_size = 2
      items = [[client, :minilm, 'a'], [client, :minilm, 'b'], [client, :minilm, 'c']]
      loaded = []

      Emb::BATCH_BLOCK.call(items, ->(i, v) { loaded << [i, v] }, {})

      expect(client.commands).to eq(
        [
          %w[EMB minilm a b],
          %w[EMB minilm c]
        ]
      )
      expect(loaded.map(&:last)).to eq([[1.0, 1.0], [2.0, 2.0], [3.0, 3.0]])
    end

    it 'does not chunk when the scope fits within batch_size' do
      client = FakeEmbClient.new(
        %w[EMB minilm a b] => [FakeEmbClient.vec(1.0), FakeEmbClient.vec(2.0)]
      )
      client.batch_size = 512
      items = [[client, :minilm, 'a'], [client, :minilm, 'b']]
      loaded = []

      Emb::BATCH_BLOCK.call(items, ->(i, v) { loaded << [i, v] }, {})

      expect(client.commands).to eq([%w[EMB minilm a b]])
    end

    it 'preserves MGET nil propagation across chunk boundaries' do
      client = FakeEmbClient.new(
        %w[EMB minilm a b] => [FakeEmbClient.vec(1.0), nil],
        %w[EMB minilm c] => [FakeEmbClient.vec(3.0)]
      )
      client.batch_size = 2
      items = [[client, :minilm, 'a'], [client, :minilm, 'b'], [client, :minilm, 'c']]
      loaded = []

      Emb::BATCH_BLOCK.call(items, ->(i, v) { loaded << [i, v] }, {})

      expect(loaded.map(&:last)).to eq([[1.0, 1.0], nil, [3.0, 3.0]])
    end
  end

  describe 'Emb::MultiProxy (unit)' do
    it 'chunks collected pairs into multiple MULTIs at batch_size and reassembles in order' do
      client = FakeEmbClient.new(
        ['EMB.MULTI', 'minilm', 'a', 'minilm', 'b'] => [FakeEmbClient.vec(1.0), FakeEmbClient.vec(2.0)],
        ['EMB.MULTI', 'minilm', 'c', 'bge', 'd'] => [FakeEmbClient.vec(3.0), FakeEmbClient.vec(4.0)]
      )
      client.batch_size = 2
      mp = Emb::MultiProxy.new(client)
      mp[:minilm]['a']
      mp[:minilm]['b']
      mp[:minilm]['c']
      mp[:bge]['d']

      expect(mp.run).to eq([[1.0, 1.0], [2.0, 2.0], [3.0, 3.0], [4.0, 4.0]])
      expect(client.commands).to eq(
        [
          ['EMB.MULTI', 'minilm', 'a', 'minilm', 'b'],
          ['EMB.MULTI', 'minilm', 'c', 'bge', 'd']
        ]
      )
    end

    it 'returns nil for null reply entries across chunks' do
      client = FakeEmbClient.new(
        ['EMB.MULTI', 'minilm', 'a', 'minilm', 'b'] => [FakeEmbClient.vec(1.0), nil]
      )
      client.batch_size = 2
      mp = Emb::MultiProxy.new(client)
      mp[:minilm]['a']
      mp[:minilm]['b']

      expect(mp.run).to eq([[1.0, 1.0], nil])
    end
  end

  describe 'fail-closed batches with retries (batch-failure-retries)' do
    it 'surfaces Emb::ServerError once and clears the pending batch' do
      client = FailingEmbClient.new
      l1 = described_class.build_batch_loader(client, :minilm, 'a')
      described_class.build_batch_loader(client, :minilm, 'b')

      expect { l1.__send__(:__sync) }.to raise_error(Emb::ServerError) do |e|
        expect(e.attempts).to eq(1)
        expect(e.cause).to be_a(RedisClient::ReadTimeoutError)
      end
      expect(client.commands.length).to eq(1)
      expect(BatchLoader::Executor.current.items_by_block.values.sum(&:size)).to eq(0)
    end

    it 'resolves failed items to [] afterwards, with no further I/O' do
      client = FailingEmbClient.new
      l1 = described_class.build_batch_loader(client, :minilm, 'a')
      expect { l1.__send__(:__sync) }.to raise_error(Emb::ServerError)

      expect(l1.__send__(:__sync)).to eq([])
      expect(client.commands.length).to eq(1) # no re-send
    end

    it 'excludes failed items from subsequent batches in the same scope' do
      client = FailingEmbClient.new
      l1 = described_class.build_batch_loader(client, :minilm, 'a')
      expect { l1.__send__(:__sync) }.to raise_error(Emb::ServerError)

      l2 = described_class.build_batch_loader(client, :minilm, 'c')
      expect { l2.__send__(:__sync) }.to raise_error(Emb::ServerError)
      expect(client.commands.length).to eq(2)
      expect(client.commands.last).to eq(%w[EMB minilm c])
    end

    it 'keeps the pending set bounded across repeated failed batches' do
      client = FailingEmbClient.new
      3.times do |i|
        loader = described_class.build_batch_loader(client, :minilm, "t#{i}")
        expect { loader.__send__(:__sync) }.to raise_error(Emb::ServerError)
        expect(BatchLoader::Executor.current.items_by_block.values.sum(&:size)).to eq(0)
      end
    end

    it 'recovers within the retry budget and materializes real embeddings' do
      client = FailingEmbClient.new(
        { %w[EMB minilm a b] => [FakeEmbClient.vec(1.0), FakeEmbClient.vec(2.0)] },
        reconnect_attempts: 2, timeouts: 2
      )
      l1 = described_class.build_batch_loader(client, :minilm, 'a')
      l2 = described_class.build_batch_loader(client, :minilm, 'b')

      expect(l1.__send__(:__sync)).to eq([1.0, 1.0])
      expect(l2.__send__(:__sync)).to eq([2.0, 2.0])
      expect(client.commands.length).to eq(3) # 2 timeouts + 1 success
    end

    it 'raises Emb::ServerError with context once the retry budget is exhausted' do
      client = FailingEmbClient.new(reconnect_attempts: 2)
      loader = described_class.build_batch_loader(client, :minilm, 'hello')

      expect { loader.__send__(:__sync) }.to raise_error(Emb::ServerError) do |e|
        expect(e.attempts).to eq(3)
        expect(e.cause).to be_a(RedisClient::ConnectionError) # exhaustion surfaces the last error
        expect(e.message).to include('minilm').and include('1 text(s)').and include('3 attempt(s)')
      end
      expect(client.commands.length).to eq(3)
      expect(BatchLoader::Executor.current.items_by_block.values.sum(&:size)).to eq(0)
    end

    it 'retries protocol errors like redis-client does' do
      client = FailingEmbClient.new(reconnect_attempts: 2, error: RedisClient::ProtocolError)
      loader = described_class.build_batch_loader(client, :minilm, 'hello')

      expect { loader.__send__(:__sync) }.to raise_error(Emb::ServerError) do |e|
        expect(e.attempts).to eq(3)
        expect(e.cause).to be_a(RedisClient::ProtocolError)
      end
      expect(client.commands.length).to eq(3)
    end

    it 'includes every model and the total text count in the message' do
      client = FailingEmbClient.new
      l1 = described_class.build_batch_loader(client, :minilm, 'a')
      described_class.build_batch_loader(client, :bge, %w[b c])

      expect { l1.__send__(:__sync) }.to raise_error(Emb::ServerError) do |e|
        expect(e.message).to include('minilm, bge').and include('3 text(s)')
      end
    end

    it 'reports the real per-client retry budget end to end' do
      client = Emb::Client.new(port: 1, reconnect_attempts: 2)
      expect(client.reconnect_attempts).to eq(2)
      loader = described_class.build_batch_loader(client, :minilm, 'hello')

      expect { loader.__send__(:__sync) }.to raise_error(Emb::ServerError) do |e|
        expect(e.attempts).to eq(3) # redis-client really re-sent 3 times to the dead port
        expect(e.cause).to be_a(RedisClient::CannotConnectError)
      end
    end

    it 'treats an explicit false retry setting as no retries' do
      client = FailingEmbClient.new(reconnect_attempts: false)
      loader = described_class.build_batch_loader(client, :minilm, 'hello')

      expect { loader.__send__(:__sync) }.to raise_error(Emb::ServerError) do |e|
        expect(e.attempts).to eq(1)
      end
      expect(client.commands.length).to eq(1)
    end

    it 'counts delay-array reconnect_attempts as one attempt per entry' do
      client = FailingEmbClient.new(reconnect_attempts: [0, 0.5])
      loader = described_class.build_batch_loader(client, :minilm, 'hello')

      expect { loader.__send__(:__sync) }.to raise_error(Emb::ServerError) do |e|
        expect(e.attempts).to eq(3) # two delay entries + the first attempt
      end
      expect(client.commands.length).to eq(3)
    end

    it 're-raises non-redis errors unchanged after clearing the pending batch' do
      client = FakeEmbClient.new
      def client.send_command(*)
        raise TypeError, 'unsupported command argument'
      end
      loader = described_class.build_batch_loader(client, :minilm, 'hello')

      expect { loader.__send__(:__sync) }.to raise_error(TypeError)
      expect(BatchLoader::Executor.current.items_by_block.values.sum(&:size)).to eq(0)
      expect(client.commands).to be_empty # nothing ever reached the server
    end

    it 'reports the final error class for mixed timeout-then-operation sequences' do
      client = MixedFailureEmbClient.new
      loader = described_class.build_batch_loader(client, :minilm, 'hello')

      # redis-client 0.30 does not expose an accumulated send count, so the
      # attempts number follows the final error (nominal, documented).
      expect { loader.__send__(:__sync) }.to raise_error(Emb::ServerError) do |e|
        expect(e.attempts).to eq(1)
        expect(e.cause).to be_a(RedisClient::CommandError)
      end
      expect(client.commands.length).to eq(2) # timeout send + re-send
    end

    it 'never retries operation errors even with a retry budget' do
      client = FailingEmbClient.new(reconnect_attempts: 2, error: RedisClient::CommandError)
      loader = described_class.build_batch_loader(client, :minilm, 'hello')

      expect { loader.__send__(:__sync) }.to raise_error(Emb::ServerError) do |e|
        expect(e.attempts).to eq(1)
        expect(e.cause).to be_a(RedisClient::CommandError)
      end
      expect(client.commands.length).to eq(1)
    end
  end
end
