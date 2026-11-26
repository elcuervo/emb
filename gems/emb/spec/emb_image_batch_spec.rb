# frozen_string_literal: true

require 'spec_helper'
require 'stringio'

RSpec.describe Emb, '#image' do
  after { BatchLoader::Executor.clear_current }

  def blob(*values) = values.pack('e*')

  def lazy_client(mode, **options)
    Emb::Client.new(lazy: mode, **options)
  end

  it 'stays eager by default: the command is sent during the call' do
    client = Emb::Client.new
    sent = []
    allow(client).to receive(:send_command) do |*args|
      sent << args
      ['embedding', blob(1.0)]
    end

    expect(client[:clip].image('a'.b)).to eq('embedding' => [1.0])
    expect(sent).to eq([['EMB.IMG', 'clip', 'a']])
  end

  it 'defers under lazy: :multi and coalesces compatible calls into one EMB.IMG' do
    client = lazy_client(:multi)
    sent = []
    allow(client).to receive(:send_command) do |*args|
      sent << args
      [['embedding', blob(1.0)], ['embedding', blob(2.0)]]
    end

    first = client[:clip].image('a'.b)
    second = client[:clip].image('b'.b)
    expect(sent).to be_empty

    expect(first['embedding']).to eq([1.0])
    expect(second['embedding']).to eq([2.0])
    expect(sent).to eq([['EMB.IMG', 'clip', 'a', 'b']])
  end

  it 'keeps the eager shape for a multi-image call and for one-image calls' do
    client = lazy_client(:multi)
    stub = [['embedding', blob(1.0)], ['embedding', blob(2.0)]]
    allow(client).to receive(:send_command).and_return(stub)

    many = client[:clip].image('a'.b, 'b'.b)
    expect(many).to eq([{ 'embedding' => [1.0] }, { 'embedding' => [2.0] }])
  end

  # The server writes one keyed map — a flat name/value array under RESP2 — when
  # EMB.IMG carries a single image, and an array of maps only for several.
  it 'decodes a one-image share from the flat keyed map the server writes' do
    client = lazy_client(:multi)
    allow(client).to receive(:send_command).and_return(['embedding', blob(1.0)])

    expect(client[:clip].image('a'.b)).to eq('embedding' => [1.0])
  end

  it 'groups by model, format, and projection instead of merging' do
    client = lazy_client(:multi)
    sent = []
    allow(client).to receive(:send_command) do |*args|
      sent << args
      if args.include?('VALUES')
        ['patches', ['dtype', 'FLOAT', 'shape', [1, 1], 'values', ['1.0']]]
      else
        ['embedding', blob(2.0)]
      end
    end

    plain = client[:clip].image('a'.b)
    projected = client[:clip].image('b'.b, outputs: ['patches'])
    values = client[:clip].image('c'.b, format: :values)
    [plain, projected, values].each { |value| value['embedding'] }

    expect(sent).to contain_exactly(
      ['EMB.IMG', 'clip', 'a'],
      ['EMB.IMG', 'clip', 'OUTPUTS', 'patches', 'b'],
      ['EMB.IMG', 'clip', 'VALUES', 'c']
    )
  end

  it 'keeps a failed or truncated image slot nil and decodes its siblings' do
    client = lazy_client(:multi)
    allow(client).to receive(:send_command).and_return([['embedding', blob(1.0)], nil])

    first = client[:clip].image('a'.b)
    second = client[:clip].image('b'.b)

    expect(first['embedding']).to eq([1.0])
    expect(second).to be_nil
  end

  it 'captures bytes at deferral time so later mutation cannot change the request' do
    client = lazy_client(:multi)
    sent = []
    allow(client).to receive(:send_command) do |*args|
      sent << args
      ['embedding', blob(1.0)]
    end

    bytes = +'original'
    value = client[:clip].image(bytes)
    bytes << '-mutated'
    value['embedding']

    expect(sent).to eq([['EMB.IMG', 'clip', 'original']])
  end

  it 'reads an IO at deferral time instead of at force time' do
    client = lazy_client(:multi)
    sent = []
    allow(client).to receive(:send_command) do |*args|
      sent << args
      ['embedding', blob(1.0)]
    end

    io = StringIO.new("a\x00".b)
    value = client[:clip].image(io)
    io.close
    value['embedding']

    expect(sent.first.last.bytes).to eq("a\x00".bytes)
  end

  it 'rejects an image that would exceed the batch_size image count' do
    client = lazy_client(:multi, batch_size: 2)
    sent = []
    allow(client).to receive(:send_command) do |*args|
      sent << args
      [['embedding', blob(1.0)], ['embedding', blob(2.0)]]
    end

    first = client[:clip].image('a'.b)
    second = client[:clip].image('b'.b)
    expect { client[:clip].image('c'.b) }
      .to raise_error(Emb::ImageBatchLimitError, /batch_size \(2 images\)/)

    expect(sent).to be_empty
    # The rejected image was not queued: resolving the scope sends only a and b.
    first['embedding']
    second['embedding']
    expect(sent).to eq([['EMB.IMG', 'clip', 'a', 'b']])
  end

  it 'rejects an image that would exceed max_image_batch_bytes' do
    client = lazy_client(:multi, max_image_batch_bytes: 8)
    allow(client).to receive(:send_command).and_return([['embedding', blob(1.0)]])

    client[:clip].image('123456'.b)
    expect { client[:clip].image('789'.b) }
      .to raise_error(Emb::ImageBatchLimitError, /max_image_batch_bytes \(8 bytes\)/)
  end

  it 'refuses an oversized IO while reading it, before anything is queued' do
    client = lazy_client(:multi, max_image_batch_bytes: 8)
    sent = []
    allow(client).to receive(:send_command) { |*args| sent << args }

    expect { client[:clip].image(StringIO.new('x' * 64)) }
      .to raise_error(Emb::ImageBatchLimitError, /max_image_batch_bytes/)
    expect(sent).to be_empty
  end

  it 'releases the pending bounds with the batch scope, including after an exception' do
    client = lazy_client(:multi, batch_size: 1)
    allow(client).to receive(:send_command).and_return([['embedding', blob(1.0)]])

    expect do
      Emb::BatchScope.wrap do
        client[:clip].image('a'.b)
        raise 'boom'
      end
    end.to raise_error('boom')

    expect { client[:clip].image('b'.b) }.not_to raise_error
  end

  it 'dispatches independent model groups concurrently under lazy: :batch' do
    client = lazy_client(:batch)
    mutex = Mutex.new
    cv = ConditionVariable.new
    arrived = 0
    allow(client).to receive(:send_command) do |*args|
      mutex.synchronize do
        arrived += 1
        cv.broadcast
        deadline = Process.clock_gettime(Process::CLOCK_MONOTONIC) + 5
        while arrived < 2
          remaining = deadline - Process.clock_gettime(Process::CLOCK_MONOTONIC)
          raise 'image groups did not dispatch concurrently' if remaining <= 0

          cv.wait(mutex, remaining)
        end
      end
      args[1] == 'clip' ? ['embedding', blob(1.0)] : ['embedding', blob(2.0)]
    end

    clip = client[:clip].image('a'.b)
    dino = client[:dino].image('b'.b)

    expect(clip['embedding']).to eq([1.0])
    expect(dino['embedding']).to eq([2.0])
  end

  it 'rejects an invalid per-client max_image_batch_bytes at construction' do
    expect { Emb::Client.new(max_image_batch_bytes: 0) }
      .to raise_error(ArgumentError, /max_image_batch_bytes/)
  end
end
