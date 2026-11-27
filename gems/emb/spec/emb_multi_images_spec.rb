# frozen_string_literal: true

require 'spec_helper'

RSpec.describe Emb do
  after { BatchLoader::Executor.clear_current }

  def blob(*values) = values.pack('e*')

  # Returns the client, the commands it sent, and the reply queue that the next
  # send_command call pops from.
  def recording_client(**options)
    client = Emb::Client.new(**options)
    sent = []
    replies = []
    allow(client).to receive(:send_command) do |*args|
      sent << args
      replies.shift
    end
    [client, sent, replies]
  end

  describe '.multi with images' do
    let(:envelope) { ['dtype', 'FLOAT', 'shape', [1, 2], 'values', %w[1.0 2.0]] }

    it 'sends one EMB.IMGMULTI for compatible pairs and keeps submission order' do
      client, sent, replies = recording_client
      replies << [['embedding', blob(1.0)], ['embedding', blob(2.0)]]

      result = client.multi do |m|
        m[:clip].image('a'.b)
        m[:dino].image('b'.b)
      end

      expect(sent).to eq([['EMB.IMGMULTI', 'clip', 'a', 'dino', 'b']])
      expect(result).to eq([{ 'embedding' => [1.0] }, { 'embedding' => [2.0] }])
    end

    it 'gives a multi-image call one result per image, like separate calls' do
      client, sent, replies = recording_client
      replies << [['embedding', blob(1.0)], ['embedding', blob(2.0)]]

      result = client.multi { |m| m[:clip].image('a'.b, 'b'.b) }

      expect(sent).to eq([['EMB.IMGMULTI', 'clip', 'a', 'clip', 'b']])
      expect(result).to eq([{ 'embedding' => [1.0] }, { 'embedding' => [2.0] }])
    end

    it 'accepts the same format and outputs options as a single image call' do
      client, sent, replies = recording_client
      replies << [['model', 'clip', 'patches', envelope]]
      replies << [['embedding', blob(3.0)]]

      result = client.multi do |m|
        m[:clip].image('a'.b, format: :values, outputs: ['patches'])
        m[:clip].image('b'.b)
      end

      expect(sent).to contain_exactly(
        ['EMB.IMGMULTI', 'VALUES', 'OUTPUTS', 'patches', 'clip', 'a'],
        ['EMB.IMGMULTI', 'clip', 'b']
      )
      expect(result).to eq([
                             { 'model' => 'clip',
                               'patches' => { dtype: 'FLOAT', shape: [1, 2], values: [[1.0, 2.0]] } },
                             { 'embedding' => [3.0] }
                           ])
    end

    it 'keeps text and image results interleaved in submission order' do
      client, sent, replies = recording_client
      replies << [blob(1.0), blob(3.0)]
      replies << [['embedding', blob(2.0)]]

      result = client.multi do |m|
        m[:minilm]['hello']
        m[:clip].image('a'.b)
        m[:bge]['world']
      end

      expect(sent).to contain_exactly(
        ['EMB.MULTI', 'minilm', 'hello', 'bge', 'world'],
        ['EMB.IMGMULTI', 'clip', 'a']
      )
      expect(result).to eq([[1.0], { 'embedding' => [2.0] }, [3.0]])
    end

    it 'maps a null image pair to nil and decodes its siblings' do
      client, _, replies = recording_client
      replies << [nil, ['embedding', blob(4.0)]]

      result = client.multi do |m|
        m[:clip].image('a'.b)
        m[:dino].image('b'.b)
      end

      expect(result).to eq([nil, { 'embedding' => [4.0] }])
    end

    it 'sends binary bytes unchanged' do
      bytes = "\x89PNG\r\n\x1a\n\x00\xff".dup.force_encoding(Encoding::UTF_8)
      client, sent, replies = recording_client
      replies << [['embedding', blob(1.0)]]

      client.multi { |m| m[:clip].image(bytes) }

      sent_bytes = sent.first.last
      expect(sent_bytes.encoding).to eq(Encoding::BINARY)
      expect(sent_bytes.bytes).to eq(bytes.bytes)
    end

    it 'captures text and image bytes at call time' do
      client, sent, replies = recording_client
      replies << [blob(1.0)]
      replies << [['embedding', blob(2.0)]]

      text = +'hello'
      bytes = +'abc'
      client.multi do |m|
        m[:minilm][text]
        m[:clip].image(bytes)
        text << ' MUTATED'
        bytes << 'ZZZ'
      end

      expect(sent).to contain_exactly(
        ['EMB.MULTI', 'minilm', 'hello'],
        ['EMB.IMGMULTI', 'clip', 'abc']
      )
    end

    it 'rejects an unknown format before sending' do
      client, = recording_client

      expect { client.multi { |m| m[:clip].image('a'.b, format: :json) } }
        .to raise_error(ArgumentError, /unknown format/)
      expect(client).not_to have_received(:send_command)
    end

    it 'stays eager under a lazy client, like the text block' do
      sent = []
      client = Emb::Client.new(lazy: :multi)
      allow(client).to receive(:send_command) do |*args|
        sent << args
        args.first == 'EMB.IMGMULTI' ? [['embedding', blob(5.0)]] : ['embedding', blob(5.0)]
      end

      deferred = client[:clip].image('deferred'.b)
      expect(sent).to be_empty

      result = client.multi { |m| m[:clip].image('now'.b) }
      expect(result).to eq([{ 'embedding' => [5.0] }])
      expect(sent).to eq([['EMB.IMGMULTI', 'clip', 'now']])
      expect(deferred).not_to be_nil
    end
  end
end
