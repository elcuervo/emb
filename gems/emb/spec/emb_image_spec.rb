# frozen_string_literal: true

require 'spec_helper'
require 'stringio'

RSpec.describe Emb::Proxy do
  describe '#image' do
    let(:client) { double('client') }
    let(:proxy) { described_class.new(client, :clip) }

    it 'sends raw bytes unchanged as an ASCII-8BIT bulk' do
      # Deliberately mislabelled encoding (UTF-8) with invalid/high bytes,
      # including NUL, 0xFF, CR and LF: the client must not transcode them.
      bytes = "\x89PNG\r\n\x1a\n\x00\xff\xfe".dup.force_encoding(Encoding::UTF_8)
      reply = ['embedding', [1.5, -2.5].pack('e*')]

      expect(client).to receive(:send_command) do |*args|
        expect(args[0]).to eq('EMB.IMG')
        expect(args[1]).to eq('clip')
        sent = args[2]
        expect(sent.encoding).to eq(Encoding::BINARY)
        expect(sent.bytes).to eq(bytes.bytes)
        reply
      end

      expect(proxy.image(bytes)).to eq('embedding' => [1.5, -2.5])
    end

    it 'reuses the float32 reply decode path, keyed by output name' do
      reply = ['embedding', [0.25, 0.5, 1.0].pack('e*')]
      allow(client).to receive(:send_command).and_return(reply)

      expect(proxy.image('bytes')).to eq('embedding' => [0.25, 0.5, 1.0])
    end

    it 'returns every configured output under its own key' do
      reply = [
        'embedding', [0.25, 0.5].pack('e*'),
        'patches', [1.0, 2.0, 3.0, 4.0].pack('e*')
      ]
      allow(client).to receive(:send_command).and_return(reply)

      expect(proxy.image('bytes')).to eq('embedding' => [0.25, 0.5], 'patches' => [1.0, 2.0, 3.0, 4.0])
    end

    it 'narrows the projection with outputs:' do
      sent = nil
      allow(client).to receive(:send_command) do |*args|
        sent = args
        ['patches', [1.0, 2.0].pack('e*')]
      end

      expect(proxy.image('bytes', outputs: ['patches'])).to eq('patches' => [1.0, 2.0])
      expect(sent).to eq(['EMB.IMG', 'clip', 'OUTPUTS', 'patches', 'bytes'])
    end

    it 'places OUTPUTS after the VALUES keyword' do
      sent = nil
      allow(client).to receive(:send_command) do |*args|
        sent = args
        ['patches', ['dtype', 'FLOAT', 'shape', [2, 1], 'values', %w[0.5 1.5], 'grid', [1, 2]]]
      end

      expect(proxy.image('bytes', format: :values, outputs: 'patches')).to eq(
        'patches' => { dtype: 'FLOAT', shape: [2, 1], values: [[0.5], [1.5]], grid: [1, 2] }
      )
      expect(sent).to eq(['EMB.IMG', 'clip', 'VALUES', 'OUTPUTS', 'patches', 'bytes'])
    end

    it 'reads IO arguments and returns one keyed map per image' do
      io_a = StringIO.new("\x89PNG\x00".b)
      io_b = StringIO.new("\xff\xd8\xff\xe0".b)
      replies = [
        ['embedding', [1.0, 2.0].pack('e*')],
        ['embedding', [3.0, 4.0].pack('e*')]
      ]

      expect(client).to receive(:send_command) do |*args|
        expect(args[0..1]).to eq(['EMB.IMG', 'clip'])
        expect(args[2].encoding).to eq(Encoding::BINARY)
        expect(args[3].encoding).to eq(Encoding::BINARY)
        replies
      end

      expect(proxy.image(io_a, io_b)).to eq([{ 'embedding' => [1.0, 2.0] }, { 'embedding' => [3.0, 4.0] }])
    end

    it 'maps a failed or truncated slot to nil' do
      replies = [['embedding', [1.0, 2.0].pack('e*')], nil]
      allow(client).to receive(:send_command).and_return(replies)

      expect(proxy.image('a', 'b')).to eq([{ 'embedding' => [1.0, 2.0] }, nil])
    end

    it 'decodes a VALUES envelope keyed by output name' do
      reply = ['embedding', ['dtype', 'FLOAT', 'shape', [1, 2], 'values', %w[0.5 1.5]]]
      allow(client).to receive(:send_command).and_return(reply)

      expect(proxy.image('bytes', format: :values)).to eq(
        'embedding' => { dtype: 'FLOAT', shape: [1, 2], values: [[0.5, 1.5]] }
      )
    end
  end
end
