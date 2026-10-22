# frozen_string_literal: true

require 'spec_helper'
require_relative '../support/telemetry_helpers'

RSpec.describe Emb::Telemetry do
  include TelemetryHelpers

  around do |example|
    tracer_provider = OpenTelemetry.tracer_provider
    meter_provider = OpenTelemetry.meter_provider
    example.run
  ensure
    Thread.current[:emb_telemetry_deferrals]&.clear
    BatchLoader::Executor.clear_current
    restore_providers(tracer_provider, meter_provider)
  end

  # The gem has no runtime dependency on the OpenTelemetry gems. This is the one
  # seam that can stand in for their absence in-process; if any path touched an
  # OpenTelemetry constant while unavailable, it would raise here.
  describe 'without the OpenTelemetry gems' do
    before { allow(described_class).to receive(:available?).and_return(false) }

    it 'embeds normally and emits nothing' do
      client = TelemetryHelpers::FakeClient.new

      expect(Emb::Proxy.new(client, :minilm)['hello']).to eq([1.0, 2.0])
      expect(client.commands).to eq([%w[EMB minilm hello]])
      expect(described_class.start_span('emb.embed')).to be_nil
    end

    it 'forces a deferred batch without raising' do
      client = TelemetryHelpers::FakeClient.new(lazy_mode: :batch)
      loader = Emb.build_batch_loader(client, :minilm, 'hello')

      expect(force(loader)).to eq([1.0, 2.0])
      expect(Emb::BatchScope.wrap { nil }).to be_nil
    end
  end

  describe 'without an SDK' do
    it 'is inert: the same result, no raise, no export' do
      install_noop_providers
      client = TelemetryHelpers::FakeClient.new

      expect(Emb::Proxy.new(client, :minilm)['hello']).to eq([1.0, 2.0])
      expect(client.commands).to eq([%w[EMB minilm hello]])
    end
  end

  describe 'eager calls' do
    before { install_telemetry_sdk }

    it 'emits one emb.embed span with the call attributes' do
      client = TelemetryHelpers::FakeClient.new
      result = Emb::Proxy.new(client, :minilm)['hello', 'world']

      expect(result.size).to eq(2)
      span = span_named('emb.embed')
      expect(span.kind).to eq(:client)
      expect(span.attributes).to include(
        'emb.operation' => 'embed',
        'emb.model' => 'minilm',
        'emb.format' => 'binary',
        'emb.texts' => 2,
        'emb.mode' => 'eager'
      )
      point = histogram('emb.client.embed.duration', attributes: { 'emb.model' => 'minilm' })
      expect(point.count).to eq(1)
    end

    it 'spans an image call' do
      client = TelemetryHelpers::FakeClient.new

      expect(Emb::Proxy.new(client, :clip).image('image-bytes')).to eq([1.0, 2.0])

      span = span_named('emb.embed')
      expect(span.attributes).to include(
        'emb.operation' => 'image',
        'emb.model' => 'clip',
        'emb.texts' => 1
      )
    end

    it 'spans a script evaluation with its operation and texts' do
      client = TelemetryHelpers::FakeClient.new

      expect(client.eval(:minilm, 'return 1', %w[hello])).to include('OK')
      span = span_named('emb.embed')
      expect(span.attributes).to include(
        'emb.operation' => 'script',
        'emb.model' => 'minilm',
        'emb.texts' => 1,
        'emb.mode' => 'eager'
      )
    end

    it 'returns the value when the span processor raises' do
      provider = OpenTelemetry::SDK::Trace::TracerProvider.new
      provider.add_span_processor(TelemetryHelpers::RaisingProcessor.new)
      OpenTelemetry.tracer_provider = provider
      client = TelemetryHelpers::FakeClient.new

      expect(Emb::Proxy.new(client, :minilm)['hello']).to eq([1.0, 2.0])
      expect(client.commands.size).to eq(1)
    end

    it 'applies per-unit boundaries through the documented views' do
      hist = OpenTelemetry::SDK::Metrics::Aggregation::ExplicitBucketHistogram
      { 's' => Emb::Telemetry::BOUNDARIES,
        '{share}' => Emb::Telemetry::COUNT_BOUNDARIES,
        '1' => Emb::Telemetry::RATIO_BOUNDARIES }.each do |unit, boundaries|
        OpenTelemetry.meter_provider.add_view('emb.client.*', type: :histogram, unit: unit,
                                                              aggregation: hist.new(boundaries: boundaries))
      end
      client = TelemetryHelpers::FakeClient.new(lazy_mode: :batch)
      minilm = Emb.build_batch_loader(client, :minilm, 'one')
      clip = Emb.build_batch_loader(client, :clip, 'two')

      force(minilm)
      force(clip)

      expect(histogram('emb.client.embed.duration').explicit_bounds).to eq(Emb::Telemetry::BOUNDARIES.sort)
      expect(histogram('emb.client.batch.shares').explicit_bounds).to eq(Emb::Telemetry::COUNT_BOUNDARIES.sort)
      expect(histogram('emb.client.batch.parallelism').explicit_bounds).to eq(Emb::Telemetry::RATIO_BOUNDARIES.sort)
    end
  end

  describe 'deferred calls' do
    before { install_telemetry_sdk }

    it 'collapses duplicate deferrals into one item and keeps the first timestamp' do
      client = TelemetryHelpers::FakeClient.new(lazy_mode: :multi)
      first = Emb.build_batch_loader(client, :minilm, 'hello')
      sleep 0.02
      Emb.build_batch_loader(client, :minilm, 'hello')

      force(first)

      expect(client.commands).to eq([%w[EMB minilm hello]])
      spans = spans_named('emb.embed')
      expect(spans.size).to eq(1)
      expect(spans.first.attributes['emb.queue']).to be >= 0.02
    end

    it 'records the queue from deferral to dispatch and leaves nothing pending' do
      client = TelemetryHelpers::FakeClient.new(lazy_mode: :batch)
      loader = Emb.build_batch_loader(client, :minilm, 'hello')
      sleep 0.02

      force(loader)

      point = histogram('emb.client.queue.duration', attributes: { 'emb.mode' => 'batch' })
      expect(point.sum).to be >= 0.02
      expect(span_named('emb.embed').attributes['emb.dispatched']).to be(true)

      Emb::BatchScope.wrap { nil }
      expect(spans_named('emb.embed').size).to eq(1)
    end

    it 'reports an item that never reached a command as not dispatched' do
      client = TelemetryHelpers::FakeClient.new(lazy_mode: :multi)
      Emb.build_batch_loader(client, :minilm, 'hello')

      Emb::BatchScope.wrap { nil }

      span = span_named('emb.embed')
      expect(span.attributes['emb.dispatched']).to be(false)
      expect(spans_named('emb.share')).to be_empty
      expect(client.commands).to be_empty
    end
  end

  describe 'batches' do
    before { install_telemetry_sdk }

    it 'emits one emb.batch span with the batch shape and one share per command' do
      client = TelemetryHelpers::FakeClient.new(lazy_mode: :batch)
      minilm = Emb.build_batch_loader(client, :minilm, 'one')
      clip = Emb.build_batch_loader(client, :clip, 'two')

      force(minilm)
      force(clip)

      batch = span_named('emb.batch')
      expect(batch.attributes).to include(
        'emb.mode' => 'batch',
        'emb.shares' => 2,
        'emb.texts' => 2,
        'emb.workers' => 2
      )
      expect(spans_named('emb.share').size).to eq(2)
      expect(spans_named('emb.embed').size).to eq(2)
      # The text count stays on the span: as a metric attribute it would create
      # one series per count.
      expect(histogram('emb.client.share.duration').attributes).not_to have_key('emb.texts')
      expect(span_named('emb.share').attributes).to include('emb.texts')
    end

    it 'records overlapping shares and a parallelism near the share count' do
      latch = TelemetryHelpers::Latch.new(2)
      # Both shares must be inside the command before either returns, and each
      # takes measurable time so parallelism has something to measure.
      hook = lambda do |args|
        latch.call(args)
        sleep 0.02
      end
      client = TelemetryHelpers::FakeClient.new(lazy_mode: :batch, &hook)
      minilm = Emb.build_batch_loader(client, :minilm, 'one')
      clip = Emb.build_batch_loader(client, :clip, 'two')

      force(minilm)
      force(clip)

      shares = spans_named('emb.share').sort_by(&:start_timestamp)
      expect(shares.size).to eq(2)
      expect(shares.last.start_timestamp).to be < shares.first.end_timestamp

      point = histogram('emb.client.batch.parallelism', attributes: { 'emb.mode' => 'batch' })
      expect(point.sum).to be > 1.5
      expect(histogram('emb.client.batch.shares').sum).to eq(2)
      expect(counter_value('emb.client.shares.in_flight')).to eq(0)
    end

    it 'parents each share to its batch span' do
      client = TelemetryHelpers::FakeClient.new(lazy_mode: :batch)
      minilm = Emb.build_batch_loader(client, :minilm, 'one')
      clip = Emb.build_batch_loader(client, :clip, 'two')

      force(minilm)
      force(clip)

      batch = span_named('emb.batch')
      shares = spans_named('emb.share')
      expect(shares.size).to eq(2)
      expect(shares.map(&:parent_span_id).uniq).to eq([batch.span_id])
      expect(shares.map(&:trace_id).uniq).to eq([batch.trace_id])
    end

    it 'reports a serial batch as parallel near 1' do
      slow = ->(_args) { sleep 0.01 }
      client = TelemetryHelpers::FakeClient.new(lazy_mode: :multi, batch_size: 1, &slow)
      first = Emb.build_batch_loader(client, :minilm, 'one')
      Emb.build_batch_loader(client, :minilm, 'two')

      force(first)

      expect(client.commands.size).to eq(2)
      point = histogram('emb.client.batch.parallelism', attributes: { 'emb.mode' => 'multi' })
      expect(point.sum).to be < 1.2
    end

    it 'closes deferred items on the VALUES path' do
      client = TelemetryHelpers::FakeClient.new(
        lazy_mode: :batch,
        replies: { %w[EMB minilm VALUES a b] => ['dtype', 'FLOAT', 'shape', [2, 2], 'values', %w[1.0 2.0 3.0 4.0]] }
      )
      first = Emb.build_batch_loader(client, :minilm, 'a', format: :values)
      second = Emb.build_batch_loader(client, :minilm, 'b', format: :values)

      force(first)
      force(second)

      expect(client.commands).to eq([%w[EMB minilm VALUES a b]])
      expect(spans_named('emb.embed').map { |span| span.attributes['emb.dispatched'] }).to eq([true, true])
      Emb::BatchScope.wrap { nil }
      expect(spans_named('emb.embed').size).to eq(2)
    end

    it 'keeps a password out of the instance attribute when the URL is malformed' do
      stub_const('RedisClient', TelemetryHelpers::StubRedis)
      client = Emb::Client.new(url: 'redis://user:pa/ss@host:6379/0?password=secret', lazy: :batch)
      loader = Emb.build_batch_loader(client, :minilm, 'hello')

      force(loader)

      attributes = span_named('emb.share').attributes
      expect(attributes['emb.instance']).to eq('host:6379')
      expect(attributes.inspect).not_to include('pa/ss')
      expect(attributes.inspect).not_to include('secret')
    end

    it 'attributes each share to the instance that served it, without credentials' do
      stub_const('RedisClient', TelemetryHelpers::StubRedis)
      client = Emb::Client.new(url: ['redis://user:secret@a.example:6379/0', 'redis://b.example:6379/1'],
                               lazy: :batch)
      loader = Emb.build_batch_loader(client, :minilm, 'hello')

      force(loader)

      expect(span_named('emb.share').attributes['emb.instance']).to eq('a.example:6379')
      expect(finished_spans.map { |span| span.attributes.inspect }.join).not_to include('secret')
    end
  end

  describe 'failures' do
    before { install_telemetry_sdk }

    it 'marks the failed share and its item, and still raises' do
      failing = ->(_args) { raise RedisClient::CommandError, 'rejected' }
      client = TelemetryHelpers::FakeClient.new(lazy_mode: :multi, &failing)
      loader = Emb.build_batch_loader(client, :minilm, 'hello')

      expect { force(loader) }.to raise_error(Emb::ServerError)

      share = span_named('emb.share')
      expect(share.status.code).to eq(OpenTelemetry::Trace::Status::ERROR)
      expect(span_named('emb.embed').status.code).to eq(OpenTelemetry::Trace::Status::ERROR)
    end

    it 'reports the unsent remainder as not dispatched and records one share duration' do
      failing = ->(_args) { raise RedisClient::CommandError, 'rejected' }
      client = TelemetryHelpers::FakeClient.new(lazy_mode: :multi, batch_size: 1, &failing)
      first = Emb.build_batch_loader(client, :minilm, 'one')
      Emb.build_batch_loader(client, :minilm, 'two')

      expect { force(first) }.to raise_error(Emb::ServerError)

      expect(client.commands.size).to eq(1)
      dispatched = spans_named('emb.embed').map { |span| span.attributes['emb.dispatched'] }
      expect(dispatched).to contain_exactly(true, false)
      expect(histogram('emb.client.share.duration').count).to eq(1)
      # Only the attempted share reports a queue; the unsent one never does.
      expect(histogram('emb.client.queue.duration').count).to eq(1)
    end
  end
end
