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
      expect(Emb.configuration.batch).to be true
      expect(Emb.configuration.protocol).to eq(2)
      expect(Emb.configuration.read_timeout).to eq(10)
      expect(Emb.configuration.write_timeout).to eq(10)
      expect(Emb.configuration.reconnect_attempts).to eq(0)
      expect(Emb.configuration.driver).to be_nil
    end
  end

  describe 'Emb.configure' do
    it 'applies to clients created afterwards' do
      Emb.configure do |c|
        c.pool = 8
        c.batch = false
      end

      client = Emb::Client.new(port: 16_379)
      expect(client.pool.size).to eq(8)
      expect(client.batch?).to be false
    end

    it 'lets per-call options win' do
      Emb.configure { |c| c.pool = 5 }

      client = Emb::Client.new(port: 16_379, pool: 20)
      expect(client.pool.size).to eq(20)
    end

    it 'mutates the shared config in place' do
      Emb.configure { |c| c.pool = 12 }

      expect(Emb.configuration.pool).to eq(12)
    end
  end

  describe 'EMB_URL' do
    it 'is the only environment variable affecting the gem' do
      with_env('EMB_POOL' => '12', 'EMB_BATCH' => 'false') do
        expect(Emb.configuration.pool).to eq(5)
        expect(Emb.configuration.batch).to be true
      end
    end
  end

  describe 'batching default' do
    it 'creates lazy clients by default (out-of-the-box)' do
      expect(Emb::Client.new.batch?).to be true
    end

    it 'opts out globally via Emb.configure' do
      Emb.configure { |c| c.batch = false }

      expect(Emb::Client.new.batch?).to be false
    end

    it 'opts out per call' do
      expect(Emb::Client.new(batch: false).batch?).to be false
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
      expect(client.pool.size).to eq(5)
    end

    it 'lets per-call options win' do
      Emb.configure { |c| c.batch_size = 64 }

      client = Emb::Client.new(port: 16_379, batch_size: 32)
      expect(client.batch_size).to eq(32)
    end
  end
end
