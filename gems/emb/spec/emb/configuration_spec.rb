# frozen_string_literal: true

require 'spec_helper'

RSpec.describe Emb::Configuration do
  after { Emb.instance_variable_set(:@configuration, described_class.new) }

  def with_env(vars)
    old = vars.keys.to_h { |k| [k, ENV.fetch(k, nil)] }
    vars.each { |k, v| ENV[k] = v }
    yield
  ensure
    old.each { |k, v| v.nil? ? ENV.delete(k) : ENV[k] = v }
  end

  describe 'defaults' do
    it 'ships the evidence-based out-of-the-box config' do
      expect(Emb.configuration.host).to eq('localhost')
      expect(Emb.configuration.port).to eq(6379)
      expect(Emb.configuration.pool).to eq(5)
      expect(Emb.configuration.lazy).to be false
      expect(Emb.configuration.protocol).to eq(2)
      expect(Emb.configuration.read_timeout).to eq(10)
      expect(Emb.configuration.write_timeout).to eq(10)
      expect(Emb.configuration.reconnect_attempts).to eq(0)
      expect(Emb.configuration.to_h[:reconnect_attempts]).to eq(0)
      expect(Emb.configuration.driver).to be_nil
    end
  end

  describe 'Emb.configure' do
    it 'applies to clients created afterwards' do
      Emb.configure do |c|
        c.pool = 8
        c.lazy = :multi
      end

      client = Emb::Client.new(port: 16_379)
      expect(client.pools.first.size).to eq(8)
      expect(client.lazy_mode).to eq(:multi)
    end

    it 'lets per-call options win' do
      Emb.configure { |c| c.pool = 5 }

      client = Emb::Client.new(port: 16_379, pool: 20)
      expect(client.pools.first.size).to eq(20)
    end

    it 'mutates the shared config in place' do
      Emb.configure { |c| c.pool = 12 }

      expect(Emb.configuration.pool).to eq(12)
    end
  end

  describe 'EMB_URL' do
    it 'is the only environment variable affecting the gem' do
      with_env('EMB_POOL' => '12', 'EMB_LAZY' => ':multi') do
        expect(Emb.configuration.pool).to eq(5)
        expect(Emb.configuration.lazy).to be false
      end
    end
  end

  describe 'lazy mode' do
    it 'defaults to eager (false): one EMB round trip per call' do
      expect(Emb::Client.new.lazy?).to be false
    end

    it 'accepts all three modes and rejects anything else' do
      expect(Emb::Client.new(lazy: false).lazy_mode).to be false
      expect(Emb::Client.new(lazy: :multi).lazy_mode).to eq(:multi)
      expect(Emb::Client.new(lazy: :batch).lazy_mode).to eq(:batch)
      expect(Emb::Client.new(lazy: :multi).parallel_batch?).to be false
      expect(Emb::Client.new(lazy: :batch).parallel_batch?).to be true

      expect { Emb::Client.new(lazy: :eager) }.to raise_error(ArgumentError, /lazy must be false, :multi, or :batch/)
    end

    it 'rejects an invalid global value at configuration time' do
      expect { Emb.configure { |c| c.lazy = :sometimes } }
        .to raise_error(ArgumentError, /lazy must be false, :multi, or :batch/)
    end

    it 'applies globally via Emb.configure' do
      Emb.configure { |c| c.lazy = :batch }

      expect(Emb::Client.new.lazy_mode).to eq(:batch)
    end

    it 'lets per-call options win over the global mode' do
      Emb.configure { |c| c.lazy = :batch }

      expect(Emb::Client.new(lazy: false).lazy?).to be false
    end
  end

  describe 'batch_size' do
    it 'defaults to 512' do
      expect(Emb.configuration.batch_size).to eq(512)
    end

    it 'applies globally via Emb.configure' do
      Emb.configure { |c| c.batch_size = 64 }

      client = Emb::Client.new(port: 16_379)
      expect(client.batch_size).to eq(64)
      expect(client.pools.first.size).to eq(5)
    end

    it 'lets per-call options win' do
      Emb.configure { |c| c.batch_size = 64 }

      client = Emb::Client.new(port: 16_379, batch_size: 32)
      expect(client.batch_size).to eq(32)
    end
  end

  describe 'url as an array' do
    it 'creates one pool per instance' do
      client = Emb::Client.new(url: %w[redis://emb-a:6379 redis://emb-b:6379], pool: 2)
      expect(client.pools.size).to eq(2)
      expect(client.pools).to all(be_a(Emb::RoundRobinPool))
      expect(client.pools.map(&:size)).to eq([2, 2])
    end

    it 'keeps a single pool for a String url' do
      client = Emb::Client.new(url: 'redis://emb-a:6379', pool: 3)
      expect(client.pools.size).to eq(1)
      expect(client.pools.first.size).to eq(3)
    end

    it 'rejects an empty url array' do
      expect { Emb::Client.new(url: []) }.to raise_error(ArgumentError, /empty/)
    end
  end
end
