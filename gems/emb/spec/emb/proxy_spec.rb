# frozen_string_literal: true

require 'spec_helper'

RSpec.describe Emb::Proxy do
  let(:client) { instance_double(Emb::Client, lazy?: false) }
  let(:proxy) { described_class.new(client, :minilm) }

  describe '#[]' do
    it 'maps a null slot to nil and unpacks the others' do
      packed = [1.0, 2.0].pack('e*')
      allow(client).to receive(:send_command).and_return([packed, nil])

      expect(proxy['a', 'b']).to eq([[1.0, 2.0], nil])
    end

    it 'returns nil for a single null reply instead of raising' do
      allow(client).to receive(:send_command).and_return(nil)

      expect(proxy['a']).to be_nil
    end

    it 'returns a single vector for one text' do
      packed = [0.5, 1.5].pack('e*')
      allow(client).to receive(:send_command).and_return(packed)

      expect(proxy['a']).to eq([0.5, 1.5])
    end
  end
end
