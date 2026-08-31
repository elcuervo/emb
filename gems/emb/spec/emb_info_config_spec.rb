# frozen_string_literal: true

require 'spec_helper'

RSpec.describe Emb do
  before(:all) { described_class.setup(port: 16_379, batch: false) }
  after(:all) { described_class.reset_registry! }

  describe '.stats' do
    it 'returns a hash with symbol keys' do
      stats = described_class.stats
      expect(stats).to be_a(Hash)
      expect(stats).to have_key(:uptime_secs)
      expect(stats).to have_key(:total_requests)
      expect(stats).to have_key(:active_requests)
      expect(stats).to have_key(:total_tokens)
      expect(stats).to have_key(:total_errors)
      expect(stats).to have_key(:models_loaded)
      expect(stats).to have_key(:per_model)
      expect(stats).to have_key(:cache_hits)
      expect(stats).to have_key(:cache_misses)
      expect(stats).to have_key(:cache_evictions)
    end

    it 'keeps values as the server sent them' do
      stats = described_class.stats
      expect(stats[:models_loaded]).to be_an(Integer)
      expect(stats[:per_model]).to be_a(String)
    end
  end

  describe '.server_info' do
    it 'parses all sections by default' do
      info = described_class.server_info
      expect(info.keys).to include(:Server, :Cache, :Keyspace, :Stats, :Clients)
      expect(info[:Server][:redis_version]).to be_a(String)
      expect(info[:Server][:emb_version]).to eq(info[:Server][:redis_version])
      expect(info[:Cache][:cache_hit_rate]).to be_a(String)
    end

    it 'filters by section names' do
      info = described_class.server_info(:server, :cache)
      expect(info.keys).to contain_exactly(:Server, :Cache)
    end
  end

  describe '.config_get' do
    it 'returns string-keyed config with string values' do
      config = described_class.config_get
      expect(config).to be_a(Hash)
      %w[cache password listen cache_file cache_save models tls_cert tls_key].each do |k|
        expect(config).to have_key(k)
        expect(config[k]).to be_a(String)
      end
    end

    it 'filters by glob' do
      config = described_class.config_get('cache*')
      expect(config.keys).to all(start_with('cache'))
    end

    it 'returns an empty hash for an unmatched pattern' do
      expect(described_class.config_get('nope*')).to eq({})
    end
  end

  describe '.config_set' do
    it 'returns true on success' do
      expect(described_class.config_set(:cache_file, '/tmp/emb-spec.rdb')).to be(true)
    end

    it 'raises on read-only parameters' do
      expect { described_class.config_set(:listen, ':9999') }
        .to raise_error(RedisClient::CommandError, /read-only/)
    end

    it 'raises when setting a cache that was disabled at boot' do
      expect { described_class.config_set(:cache, '100MB') }
        .to raise_error(RedisClient::CommandError, /disabled at boot/)
    end
  end

  describe 'parse_info' do
    it 'parses section text into a nested symbol-keyed hash with values as-is' do
      client = Emb::Client.new(port: 16_379)
      text = "# Server\r\nredis_version:0.2.4\r\nuptime_secs:7\r\n\r\n" \
             "# Cache\r\ncache_hit_rate:0.0%\r\ncache_hits:0\r\n"
      parsed = client.send(:parse_info, text)
      expect(parsed).to eq(
        Server: { redis_version: '0.2.4', uptime_secs: '7' },
        Cache: { cache_hit_rate: '0.0%', cache_hits: '0' }
      )
    end
  end
end
