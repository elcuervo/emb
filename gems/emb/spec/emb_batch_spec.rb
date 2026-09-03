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

# FailingEmbClient records commands and always raises like a timed-out server.
class FailingEmbClient < FakeEmbClient
  def send_command(*args)
    @commands << args
    raise RedisClient::ReadTimeoutError, 'simulated timeout'
  end
end

RSpec.describe Emb do
  after { BatchLoader::Executor.clear_current }

  describe 'Emb::BATCH_BLOCK (unit)' do
    it 'expands a single-text item into one MULTI pair and unpacks with e*' do
      client = FakeEmbClient.new(
        ['EMB.MULTI', 'minilm', 'hello'] => [FakeEmbClient.vec(1.5, dim: 3)]
      )
      item = [client, :minilm, 'hello']
      loaded = []

      Emb::BATCH_BLOCK.call([item], ->(i, v) { loaded << [i, v] }, {})

      expect(client.commands).to eq([['EMB.MULTI', 'minilm', 'hello']])
      expect(loaded).to eq([[item, [1.5, 1.5, 1.5]]])
    end

    it 'expands a multi-text item into one pair per text and regroups to an array' do
      client = FakeEmbClient.new(
        ['EMB.MULTI', 'minilm', 'a', 'minilm', 'b'] => [FakeEmbClient.vec(3.0), FakeEmbClient.vec(4.0)]
      )
      item = [client, :minilm, %w[a b]]
      loaded = []

      Emb::BATCH_BLOCK.call([item], ->(i, v) { loaded << [i, v] }, {})

      expect(client.commands).to eq([['EMB.MULTI', 'minilm', 'a', 'minilm', 'b']])
      expect(loaded.first.last).to eq([[3.0, 3.0], [4.0, 4.0]])
    end

    it 'returns a single vector for a one-text multi-text item (eager shape parity)' do
      client = FakeEmbClient.new(
        ['EMB.MULTI', 'minilm', 'solo'] => [FakeEmbClient.vec(9.0)]
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
      c1 = FakeEmbClient.new(['EMB.MULTI', 'minilm', 'a'] => [FakeEmbClient.vec(1.0)])
      c2 = FakeEmbClient.new(['EMB.MULTI', 'bge', 'b'] => [FakeEmbClient.vec(2.0)])
      loaded = []

      Emb::BATCH_BLOCK.call(
        [[c1, :minilm, 'a'], [c2, :bge, 'b']],
        ->(i, v) { loaded << [i, v] },
        {}
      )

      expect(c1.commands).to eq([['EMB.MULTI', 'minilm', 'a']])
      expect(c2.commands).to eq([['EMB.MULTI', 'bge', 'b']])
    end
  end

  describe 'Emb.batch / client.batch' do
    it 'returns a memoized per-model lazy proxy chain' do
      client = FakeEmbClient.new
      batch = Emb::BatchProxy.new(client)

      expect(batch[:minilm]).to be_a(Emb::BatchModelProxy)
      expect(batch[:minilm]).to equal(batch[:minilm])
    end

    it 'exposes the explicit API on the default client' do
      expect(described_class.batch).to be_a(Emb::BatchProxy)
    end

    it 'exposes the explicit API on instance clients' do
      client = Emb::Client.new(batch: false)
      expect(client.batch).to be_a(Emb::BatchProxy)
    end

    it 'is lazy until first use' do
      client = FakeEmbClient.new(
        ['EMB.MULTI', 'minilm', 'hello'] => [FakeEmbClient.vec(2.5)]
      )
      loader = Emb::BatchProxy.new(client)[:minilm]['hello']

      expect(client.commands).to be_empty
      expect(loader.first).to eq(2.5)
      expect(loader.first).to eq(2.5)
      expect(client.commands).to eq([['EMB.MULTI', 'minilm', 'hello']])
    end
  end

  describe 'create-then-consume coalescing' do
    # Different helper definitions = different source locations: loaders must
    # still land in one batch (batch-key regression guard).
    def loader_from_site_a(client, text)
      Emb::BatchProxy.new(client)[:minilm][text]
    end

    def loader_from_site_b(client, text)
      Emb::BatchProxy.new(client)[:minilm][text]
    end

    it 'coalesces loaders created from different call sites into one MULTI' do
      client = FakeEmbClient.new(
        ['EMB.MULTI', 'minilm', 'a', 'minilm', 'b', 'minilm', 'c'] =>
          [FakeEmbClient.vec(1.0), FakeEmbClient.vec(2.0), FakeEmbClient.vec(3.0)]
      )

      l1 = loader_from_site_a(client, 'a')
      l2 = loader_from_site_b(client, 'b')
      l3 = loader_from_site_a(client, 'c')

      expect(l1.first).to eq(1.0)
      expect(l2.first).to eq(2.0)
      expect(l3.first).to eq(3.0)

      expect(client.commands).to eq([['EMB.MULTI', 'minilm', 'a', 'minilm', 'b', 'minilm', 'c']])
    end

    it 'coalesces loaders for different models into one MULTI' do
      client = FakeEmbClient.new(
        ['EMB.MULTI', 'minilm', 'a', 'bge', 'b'] => [FakeEmbClient.vec(1.0), FakeEmbClient.vec(2.0)]
      )

      minilm = Emb::BatchProxy.new(client)[:minilm]['a']
      bge = Emb::BatchProxy.new(client)[:bge]['b']

      expect(minilm.first).to eq(1.0)
      expect(bge.first).to eq(2.0)
      expect(client.commands).to eq([['EMB.MULTI', 'minilm', 'a', 'bge', 'b']])
    end

    it 'sends a fresh MULTI only for loaders created after a flush' do
      client = FakeEmbClient.new(
        ['EMB.MULTI', 'minilm', 'a', 'minilm', 'b'] => [FakeEmbClient.vec(1.0), FakeEmbClient.vec(2.0)],
        ['EMB.MULTI', 'minilm', 'c'] => [FakeEmbClient.vec(3.0)]
      )

      first = Emb::BatchProxy.new(client)[:minilm]['a']
      second = Emb::BatchProxy.new(client)[:minilm]['b']
      first.sum
      second.sum

      third = Emb::BatchProxy.new(client)[:minilm]['c']
      expect(third.first).to eq(3.0)

      expect(client.commands).to eq(
        [
          ['EMB.MULTI', 'minilm', 'a', 'minilm', 'b'],
          ['EMB.MULTI', 'minilm', 'c']
        ]
      )
    end
  end

  describe 'caching within a scope' do
    it 'sends exactly one command for repeated use of the same loader' do
      client = FakeEmbClient.new(
        ['EMB.MULTI', 'minilm', 'hello'] => [FakeEmbClient.vec(5.0)]
      )
      loader = Emb::BatchProxy.new(client)[:minilm]['hello']

      3.times { expect(loader.first).to eq(5.0) }

      expect(client.commands.size).to eq(1)
    end

    it 'deduplicates identical pairs into one pair sent once' do
      client = FakeEmbClient.new(
        ['EMB.MULTI', 'minilm', 'dup'] => [FakeEmbClient.vec(7.0)]
      )

      a = Emb::BatchProxy.new(client)[:minilm]['dup']
      b = Emb::BatchProxy.new(client)[:minilm]['dup']

      expect(a.first).to eq(7.0)
      expect(b.first).to eq(7.0)
      expect(client.commands).to eq([['EMB.MULTI', 'minilm', 'dup']])
    end

    it 'materializes failed pairs as nil without re-sending' do
      client = FakeEmbClient.new(
        ['EMB.MULTI', 'minilm', 'ok', 'ghost', 'x'] => [FakeEmbClient.vec(1.0), nil]
      )

      ok = Emb::BatchProxy.new(client)[:minilm]['ok']
      ghost = Emb::BatchProxy.new(client)[:ghost]['x']

      expect(ok.first).to eq(1.0)
      expect(ghost).to be_nil
      expect(ghost).to be_nil # cached nil, no second command
      expect(client.commands.size).to eq(1)

      # A later loader for the already-synced failed item does not re-send.
      again = Emb::BatchProxy.new(client)[:ghost]['x']
      expect(again).to be_nil
      expect(client.commands.size).to eq(1)
    end
  end

  describe 'the batch configuration option' do
    it 'defaults to lazy batching: no command until the value is used' do
      client = Emb::Client.new
      expect(client.batch?).to be true

      log = []
      client.define_singleton_method(:send_command) do |*args|
        log << args
        [FakeEmbClient.vec(4.0, dim: 384)]
      end

      result = client[:minilm]['hello world']
      expect(log).to be_empty

      expect(result.sum).to be_within(0.001).of(4.0 * 384)
      expect(log).to eq([['EMB.MULTI', 'minilm', 'hello world']])
    end

    it 'opts out to eager with batch: false' do
      client = Emb::Client.new(batch: false)
      expect(client.batch?).to be false

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

    it 'routes the proxy API lazily when batch: true' do
      client = Emb::Client.new(batch: true)
      expect(client.batch?).to be true

      log = []
      client.define_singleton_method(:send_command) do |*args|
        log << args
        [FakeEmbClient.vec(2.5, dim: 384)]
      end

      vec = client[:minilm]['hello world']
      expect(log).to be_empty

      expect(vec.sum).to be_within(0.001).of(2.5 * 384)
      expect(log).to eq([['EMB.MULTI', 'minilm', 'hello world']])
    end

    it 'returns Array of Array of Float for multi-text in batch mode' do
      client = Emb::Client.new(batch: true)
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
      expect(log).to eq([['EMB.MULTI', 'minilm', 'hello', 'minilm', 'world']])
    end

    it 'keeps the explicit batch API lazy regardless of the option' do
      eager = FakeEmbClient.new(
        ['EMB.MULTI', 'minilm', 'hello'] => [FakeEmbClient.vec(6.0)]
      )
      loader = Emb::BatchProxy.new(eager)[:minilm]['hello']

      expect(eager.commands).to be_empty
      expect(loader.first).to eq(6.0)
    end
  end

  describe 'Emb.multi remains eager and untouched' do
    it 'works as before on a default (batch: false) client' do
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

    it 'still works on a batch: true client' do
      client = Emb::Client.new(batch: true)
      client.define_singleton_method(:send_command) do |*_args|
        [FakeEmbClient.vec(3.0, dim: 384)]
      end

      results = client.multi do |m|
        m[:minilm]['hello']
      end

      expect(results.first.first).to eq(3.0)
    end

    it 'does not interact with the batch scope' do
      client = Emb::Client.new(batch: true)
      log = []
      client.define_singleton_method(:send_command) do |*args|
        log << args
        args.first == 'EMB.MULTI' ? [FakeEmbClient.vec(1.0, dim: 384), FakeEmbClient.vec(2.0, dim: 384)] : nil
      end

      _pending = Emb::BatchProxy.new(client)[:minilm]['never used']
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
    it 'clears the per-thread scope at the end of each request' do
      client = FakeEmbClient.new(
        ['EMB.MULTI', 'minilm', 'hello'] => [FakeEmbClient.vec(1.0)]
      )
      app = lambda do |_env|
        Emb::BatchProxy.new(client)[:minilm]['hello'].first
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
  end

  describe 'Emb::BATCH_BLOCK chunking (unit)' do
    it 'chunks a large scope into multiple MULTIs at batch_size pairs' do
      client = FakeEmbClient.new(
        ['EMB.MULTI', 'minilm', 'a', 'minilm', 'b'] => [FakeEmbClient.vec(1.0), FakeEmbClient.vec(2.0)],
        ['EMB.MULTI', 'minilm', 'c'] => [FakeEmbClient.vec(3.0)]
      )
      client.batch_size = 2
      items = [[client, :minilm, 'a'], [client, :minilm, 'b'], [client, :minilm, 'c']]
      loaded = []

      Emb::BATCH_BLOCK.call(items, ->(i, v) { loaded << [i, v] }, {})

      expect(client.commands).to eq(
        [
          ['EMB.MULTI', 'minilm', 'a', 'minilm', 'b'],
          ['EMB.MULTI', 'minilm', 'c']
        ]
      )
      expect(loaded.map(&:last)).to eq([[1.0, 1.0], [2.0, 2.0], [3.0, 3.0]])
    end

    it 'does not chunk when the scope fits within batch_size' do
      client = FakeEmbClient.new(
        ['EMB.MULTI', 'minilm', 'a', 'minilm', 'b'] => [FakeEmbClient.vec(1.0), FakeEmbClient.vec(2.0)]
      )
      client.batch_size = 512
      items = [[client, :minilm, 'a'], [client, :minilm, 'b']]
      loaded = []

      Emb::BATCH_BLOCK.call(items, ->(i, v) { loaded << [i, v] }, {})

      expect(client.commands).to eq([['EMB.MULTI', 'minilm', 'a', 'minilm', 'b']])
    end

    it 'preserves MGET nil propagation across chunk boundaries' do
      client = FakeEmbClient.new(
        ['EMB.MULTI', 'minilm', 'a', 'minilm', 'b'] => [FakeEmbClient.vec(1.0), nil],
        ['EMB.MULTI', 'minilm', 'c'] => [FakeEmbClient.vec(3.0)]
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

  describe 'fail-closed batches (secure-client-batch-defaults)' do
    it 'surfaces the error once and clears the pending batch' do
      client = FailingEmbClient.new
      l1 = described_class.build_batch_loader(client, :minilm, 'a')
      described_class.build_batch_loader(client, :minilm, 'b')

      expect { l1.__send__(:__sync) }.to raise_error(RedisClient::ReadTimeoutError)
      expect(client.commands.length).to eq(1)
      expect(BatchLoader::Executor.current.items_by_block.values.sum(&:size)).to eq(0)
    end

    it 'resolves failed items to [] afterwards, with no further I/O' do
      client = FailingEmbClient.new
      l1 = described_class.build_batch_loader(client, :minilm, 'a')
      expect { l1.__send__(:__sync) }.to raise_error(RedisClient::ReadTimeoutError)

      expect(l1.__send__(:__sync)).to eq([])
      expect(client.commands.length).to eq(1) # no re-send
    end

    it 'excludes failed items from subsequent batches in the same scope' do
      client = FailingEmbClient.new
      l1 = described_class.build_batch_loader(client, :minilm, 'a')
      expect { l1.__send__(:__sync) }.to raise_error(RedisClient::ReadTimeoutError)

      l2 = described_class.build_batch_loader(client, :minilm, 'c')
      expect { l2.__send__(:__sync) }.to raise_error(RedisClient::ReadTimeoutError)
      expect(client.commands.length).to eq(2)
      expect(client.commands.last).to eq(['EMB.MULTI', 'minilm', 'c'])
    end

    it 'keeps the pending set bounded across repeated failed batches' do
      client = FailingEmbClient.new
      3.times do |i|
        loader = described_class.build_batch_loader(client, :minilm, "t#{i}")
        expect { loader.__send__(:__sync) }.to raise_error(RedisClient::ReadTimeoutError)
        expect(BatchLoader::Executor.current.items_by_block.values.sum(&:size)).to eq(0)
      end
    end
  end
end
