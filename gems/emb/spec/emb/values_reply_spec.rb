# frozen_string_literal: true

require 'spec_helper'

RSpec.describe Emb::ValuesReply do
  describe '.parse' do
    it 'decodes a single-text envelope into floats' do
      reply = ['dtype', 'FLOAT', 'shape', [1, 3], 'values', %w[0.1 0.2 0.3]]
      parsed = described_class.parse(reply)
      expect(parsed).to eq(dtype: 'FLOAT', shape: [1, 3], values: [0.1, 0.2, 0.3])
      expect(parsed[:values].first).to be_a(Float)
    end

    it 'raises on an unparsable decimal' do
      reply = ['dtype', 'FLOAT', 'shape', [1, 1], 'values', %w[not-a-number]]
      expect { described_class.parse(reply) }.to raise_error(ArgumentError, /not-a-number/)
    end
  end

  describe '.rows' do
    it 'reshapes flat values per text using shape [m, dim]' do
      rows = described_class.rows([2, 2], [1.0, 2.0, 3.0, 4.0])
      expect(rows).to eq([[1.0, 2.0], [3.0, 4.0]])
    end
  end
end