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
      reply = [1.5, -2.5].pack('e*')

      expect(client).to receive(:send_command) do |*args|
        expect(args[0]).to eq('EMB.IMG')
        expect(args[1]).to eq('clip')
        sent = args[2]
        expect(sent.encoding).to eq(Encoding::BINARY)
        expect(sent.bytes).to eq(bytes.bytes)
        reply
      end

      expect(proxy.image(bytes)).to eq([1.5, -2.5])
    end

    it 'reuses the float32 reply decode path for image replies' do
      reply = [0.25, 0.5, 1.0].pack('e*')
      allow(client).to receive(:send_command).and_return(reply)

      expect(proxy.image('bytes')).to eq([0.25, 0.5, 1.0])
    end

    it 'reads IO arguments and returns one slot per image' do
      io_a = StringIO.new("\x89PNG\x00".b)
      io_b = StringIO.new("\xff\xd8\xff\xe0".b)
      replies = [[1.0, 2.0].pack('e*'), [3.0, 4.0].pack('e*')]

      expect(client).to receive(:send_command) do |*args|
        expect(args[0..1]).to eq(['EMB.IMG', 'clip'])
        expect(args[2].encoding).to eq(Encoding::BINARY)
        expect(args[3].encoding).to eq(Encoding::BINARY)
        replies
      end

      expect(proxy.image(io_a, io_b)).to eq([[1.0, 2.0], [3.0, 4.0]])
    end

    it 'maps a failed or truncated slot to nil' do
      replies = [[1.0, 2.0].pack('e*'), nil]
      allow(client).to receive(:send_command).and_return(replies)

      expect(proxy.image('a', 'b')).to eq([[1.0, 2.0], nil])
    end

    it 'decodes a VALUES envelope with :values format' do
      reply = ['dtype', 'FLOAT', 'shape', [1, 2], 'values', %w[0.5 1.5]]
      allow(client).to receive(:send_command).and_return(reply)

      expect(proxy.image('bytes', format: :values)).to eq(
        dtype: 'FLOAT', shape: [1, 2], values: [0.5, 1.5]
      )
    end
  end
end
