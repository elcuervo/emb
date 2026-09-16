# frozen_string_literal: true

require 'spec_helper'

# The readiness predicate is a stub-driven unit test so it can cover the
# not-ready and unreachable cases without a server.
RSpec.describe Emb::Client do
  # No server is contacted: send_command is stubbed for every example.
  let(:client) { described_class.new(port: 1) }

  describe '#ready' do
    it 'returns OK when the server is ready' do
      allow(client).to receive(:send_command).with('EMB.READY').and_return('OK')

      expect(client.ready).to eq('OK')
    end

    it 'returns the server error text when the server is not ready' do
      allow(client).to receive(:send_command)
        .with('EMB.READY')
        .and_raise(RedisClient::CommandError.new('ERR loading'))

      expect(client.ready).to eq('ERR loading')
    end

    it 'returns the connection error text when the server is unreachable' do
      allow(client).to receive(:send_command)
        .and_raise(RedisClient::CannotConnectError.new('Connection refused'))

      expect(client.ready).to eq('Connection refused')
    end
  end

  describe '#ready?' do
    it 'is true when the server answers OK' do
      allow(client).to receive(:send_command).and_return('OK')

      expect(client.ready?).to be(true)
    end

    it 'is false when the server reports it is not ready' do
      allow(client).to receive(:send_command)
        .and_raise(RedisClient::CommandError.new('ERR loading'))

      expect(client.ready?).to be(false)
    end

    it 'is false when the server is unreachable, without raising' do
      allow(client).to receive(:send_command)
        .and_raise(RedisClient::CannotConnectError.new('Connection refused'))

      expect { client.ready? }.not_to raise_error
      expect(client.ready?).to be(false)
    end
  end
end
