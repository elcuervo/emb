# frozen_string_literal: true

require 'opentelemetry-sdk'
require 'opentelemetry-metrics-sdk'

# Test support for the telemetry specs: installs real SDK providers backed by
# in-memory exporters, and a client double that needs no server.
module TelemetryHelpers
  # Enough of a client for Proxy and BATCH_BLOCK, with BLOB-shaped replies and
  # an optional hook that can observe or fail a command.
  class FakeClient
    include Emb::Commands

    attr_reader :commands, :lazy_mode
    attr_accessor :batch_size

    def initialize(lazy_mode: false, batch_size: 512, replies: {}, &on_command)
      @lazy_mode = lazy_mode
      @batch_size = batch_size
      @replies = replies
      @on_command = on_command
      @commands = []
      @mutex = Mutex.new
    end

    def lazy? = @lazy_mode != false

    def parallel_batch? = @lazy_mode == :batch

    def pools = [Array.new(2)]

    def send_command(*args)
      @mutex.synchronize { @commands << args }
      @on_command&.call(args)
      return @replies.fetch(args) if @replies.key?(args)

      default_reply(args)
    end

    private

    def default_reply(args)
      case args.first
      when 'EMB.IMG' then [1.0, 2.0].pack('e*')
      when 'EMB' then Array.new(args.size - 2) { [1.0, 2.0].pack('e*') }
      when 'EMB.MULTI' then Array.new((args.size - 1) / 2) { [1.0, 2.0].pack('e*') }
      else 'OK'
      end
    end
  end

  # A span processor that fails: telemetry must still never change a result.
  class RaisingProcessor
    def on_start(*) = raise('processor boom')

    def on_finish(*) = raise('processor boom')

    def force_flush(*) = nil

    def shutdown(*) = nil
  end

  # Stands in for RedisClient so a real ConnectionRouter can be exercised
  # without a server.
  class StubRedis
    class << self
      def calls = (@calls ||= [])
    end

    def initialize(**); end

    def call(*)
      self.class.calls << :called
      [1.0, 2.0].pack('e*')
    end
  end

  # Blocks every command until `expected` commands are inside it, so overlap is
  # provable without sleeping.
  class Latch
    def initialize(expected)
      @expected = expected
      @arrived = 0
      @mutex = Mutex.new
      @condition = ConditionVariable.new
    end

    def call(_args)
      @mutex.synchronize do
        @arrived += 1
        @condition.broadcast
        deadline = Process.clock_gettime(Process::CLOCK_MONOTONIC) + 5
        while @arrived < @expected
          remaining = deadline - Process.clock_gettime(Process::CLOCK_MONOTONIC)
          raise 'latch timed out: dispatch was not concurrent' if remaining <= 0

          @condition.wait(@mutex, remaining)
        end
      end
    end
  end

  def install_telemetry_sdk
    @spans = OpenTelemetry::SDK::Trace::Export::InMemorySpanExporter.new
    tracer_provider = OpenTelemetry::SDK::Trace::TracerProvider.new
    tracer_provider.add_span_processor(
      OpenTelemetry::SDK::Trace::Export::SimpleSpanProcessor.new(@spans)
    )
    OpenTelemetry.tracer_provider = tracer_provider

    @metrics = OpenTelemetry::SDK::Metrics::Export::InMemoryMetricPullExporter.new
    meter_provider = OpenTelemetry::SDK::Metrics::MeterProvider.new
    meter_provider.add_metric_reader(@metrics)
    OpenTelemetry.meter_provider = meter_provider
    reset_telemetry_state
  end

  def install_noop_providers
    OpenTelemetry.tracer_provider = OpenTelemetry::Trace::TracerProvider.new
    OpenTelemetry.meter_provider = OpenTelemetry::Metrics::MeterProvider.new
    reset_telemetry_state
  end

  # The client builds its tracer and instruments once; examples swap providers,
  # so the memo has to be dropped between them.
  def reset_telemetry_state
    Emb::Telemetry.instance_variable_set(:@state, nil)
  end

  # Restores the providers an example found. Assigning the API's default proxy
  # provider back deadlocks (it delegates to itself), so the default is left in
  # place: every example that cares installs its own.
  def restore_providers(tracer_provider, meter_provider)
    restore_provider(:tracer_provider, tracer_provider)
    restore_provider(:meter_provider, meter_provider)
    reset_telemetry_state
  end

  def restore_provider(accessor, provider)
    return if provider.class.name.include?('Proxy')
    return if OpenTelemetry.public_send(accessor).equal?(provider)

    OpenTelemetry.public_send(:"#{accessor}=", provider)
  end

  def finished_spans = @spans.finished_spans

  def spans_named(name) = finished_spans.select { |span| span.name == name }

  def span_named(name) = spans_named(name).first

  # Pulls the current metrics and returns the one named `name`, if measured.
  def metric(name)
    @metrics.pull
    @metrics.metric_snapshots.find { |snapshot| snapshot.name == name }
  end

  # The first populated histogram datapoint for `name`, optionally narrowed to
  # the given attributes.
  def histogram(name, attributes: nil)
    snapshot = metric(name)
    return nil unless snapshot

    snapshot.data_points.find do |point|
      point.count&.positive? && wanted_attributes?(point.attributes, attributes)
    end
  end

  def counter_value(name)
    snapshot = metric(name)
    return nil unless snapshot

    snapshot.data_points.last&.value
  end

  def wanted_attributes?(actual, wanted)
    wanted.nil? || wanted.all? { |key, value| actual[key] == value }
  end

  def force(loader) = loader.__send__(:__sync)
end
