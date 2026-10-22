# frozen_string_literal: true

module Emb
  # OpenTelemetry signals for the client itself.
  #
  # Optional: the gem does not depend on the OpenTelemetry gems. Install
  # `opentelemetry-api` (and `opentelemetry-metrics-api` for metrics) and the
  # client emits; without them every path below returns immediately. When they
  # are installed but no SDK is configured, the API's default providers are
  # no-ops. There is no gem-specific configuration; `OTEL_SDK_DISABLED=true` is
  # the standard off, and nothing here may alter a command's arguments, result,
  # or error.
  #
  # Spans carry no input text, image bytes, or vectors: model, operation, format,
  # counts, and the instance that served a command only.
  module Telemetry
    SPAN_EMBED = 'emb.embed'
    SPAN_BATCH = 'emb.batch'
    SPAN_SHARE = 'emb.share'

    # Declared as instrument advice, per unit. The SDK's default boundaries are
    # millisecond-scale, so durations need their own and counts/ratios would
    # collapse into one bucket under them.
    BOUNDARIES = [0.001, 0.0025, 0.005, 0.01, 0.025, 0.05, 0.1, 0.25, 0.5, 1.0, 2.5, 5.0, 10.0, 30.0].freeze
    COUNT_BOUNDARIES = [1, 2, 3, 4, 6, 8, 12, 16, 24, 32, 48, 64, 96, 128].freeze
    RATIO_BOUNDARIES = [1.0, 1.25, 1.5, 2.0, 2.5, 3.0, 4.0, 6.0, 8.0, 12.0, 16.0, 24.0, 32.0].freeze
    UNIT_BOUNDARIES = { 's' => BOUNDARIES, '{share}' => COUNT_BOUNDARIES, '1' => RATIO_BOUNDARIES }.freeze

    HISTOGRAMS = {
      'emb.client.embed.duration' => ['s', 'End-to-end latency of one embedding call, deferral included.'],
      'emb.client.queue.duration' => ['s', 'Time a deferred call waited before its command was dispatched.'],
      'emb.client.batch.duration' => ['s', 'Wall time of one forced batch.'],
      'emb.client.batch.shares' => ['{share}', 'Commands dispatched by one forced batch.'],
      'emb.client.batch.parallelism' => ['1', 'Share time divided by batch wall time; 1 is fully serial.'],
      'emb.client.share.duration' => ['s', 'Round trip of one command dispatched by a deferred mode.']
    }.freeze

    IN_FLIGHT = 'emb.client.shares.in_flight'
    NAME = 'emb'

    State = Struct.new(:provider, :tracer, :histograms, :in_flight)
    private_constant :State

    # Runs the block inside a span. Telemetry failures are contained: whatever
    # the block returns or raises is what crosses this boundary.
    # Loads the OpenTelemetry API on first use and remembers the answer, so an
    # app that never installs it pays one branch per signal.
    def self.available?
      @available = load_api unless defined?(@available)
      @available
    end

    # The metrics API does not define the global meter accessors on its own (it
    # is still unstable), and it is a separate gem: traces can be present while
    # metrics are not.
    def self.load_api
      require 'opentelemetry-api'
      begin
        require 'opentelemetry-metrics-api'
        require 'opentelemetry/metrics/global'
      rescue LoadError
        nil
      end
      true
    rescue LoadError
      false
    end

    # The current span context, or nil when telemetry is unavailable. Callers
    # pass it to a worker thread: a fresh Thread starts with an empty context.
    def self.current_context
      return nil unless available?

      OpenTelemetry::Context.current
    rescue StandardError
      nil
    end

    # `current: true` makes the span the parent of spans created inside the
    # block. Only a batch has children, and attaching a span costs a fiber-local
    # write, so an ordinary call does not pay for it.
    def self.in_span(name, attributes: nil, kind: :internal, current: false, &block)
      span = start_span(name, attributes: attributes, kind: kind)
      return block.call unless span
      return around_span(span, &block) unless current

      OpenTelemetry::Trace.with_span(span) { around_span(span, &block) }
    end

    def self.around_span(span, &block)
      block.call
    rescue StandardError => e
      fail_span(span, e)
      raise
    ensure
      safe { span.finish }
    end

    def self.start_span(name, attributes: nil, kind: :internal, start_timestamp: nil, parent: nil)
      tracer = state&.tracer
      return nil unless tracer

      tracer.start_span(name, kind: kind, attributes: attributes, start_timestamp: start_timestamp,
                              with_parent: parent)
    rescue StandardError
      nil
    end

    # Ends a span that was started backdated (a deferred call's span starts at
    # its deferral but is built when it resolves).
    def self.end_span(span, at: Time.now)
      safe { span&.finish(end_timestamp: at) }
    end

    def self.fail_span(span, error)
      return unless span

      safe { span.record_exception(error) }
      safe { span.status = OpenTelemetry::Trace::Status.error(error.message) }
    end

    def self.histogram(name, value, attributes: {})
      instrument = state&.histograms&.[](name)
      instrument&.record(value, attributes: attributes)
    rescue StandardError
      nil
    end

    def self.in_flight(delta)
      state&.in_flight&.add(delta)
    rescue StandardError
      nil
    end

    # One eager call: its span and its end-to-end duration, whichever way the
    # command ends.
    def self.embed_call(operation:, model:, texts:, format:, mode:, &)
      entry = { at: mono, model: model.to_s, texts: texts, format: format, mode: mode.to_s }
      attributes = { 'emb.operation' => operation, 'emb.model' => entry[:model], 'emb.texts' => texts }
      attributes['emb.format'] = format.to_s if format
      attributes['emb.mode'] = entry[:mode]
      in_span(SPAN_EMBED, attributes: attributes, kind: :client, &)
    ensure
      histogram('emb.client.embed.duration', mono - entry[:at], attributes: metric_attributes(entry)) if entry
    end

    # One deferred batch: its span, its wall time, and its parallelism.
    # `durations` is filled by the dispatcher with each share's duration, so a
    # batch that raises still reports what actually ran.
    def self.batch(mode:, shares:, texts:, workers:)
      mode = mode.to_s
      attributes = { 'emb.mode' => mode, 'emb.shares' => shares, 'emb.texts' => texts, 'emb.workers' => workers }
      started = mono
      durations = []
      begin
        in_span(SPAN_BATCH, attributes: attributes, current: true) { yield durations }
      ensure
        wall = mono - started
        histogram('emb.client.batch.duration', wall, attributes: { 'emb.mode' => mode })
        histogram('emb.client.batch.shares', shares, attributes: { 'emb.mode' => mode })
        histogram('emb.client.batch.parallelism', wall.positive? ? durations.sum / wall : 0.0,
                  attributes: { 'emb.mode' => mode })
      end
    end

    def self.mono = Process.clock_gettime(Process::CLOCK_MONOTONIC)

    # A deferral entry: what a deferred call needs to emit its span when it
    # resolves. Mode comes from the client so a per-call override is honored.
    def self.entry_for(client, operation:, model:, texts:, format:)
      { operation: operation, model: model.to_s, texts: texts, format: format,
        mode: mode_for(client) }
    end

    # The batching mode label used on spans and metrics: eager|multi|batch.
    def self.mode_for(client)
      mode = client.respond_to?(:lazy_mode) ? client.lazy_mode : false
      mode.nil? || mode == false ? 'eager' : mode.to_s
    end

    # Ends a share span and records its duration. The instance is passed in
    # because it is only known after the command ran (a pre-send retry may
    # have moved it to another instance).
    # Attributes of one dispatched share. A mixed-model chunk carries no model
    # attribute rather than a list: the model tag must stay low-cardinality.
    def self.share_attributes(models, texts)
      attributes = { 'emb.texts' => texts }
      attributes['emb.model'] = models.first.to_s if models.size == 1
      attributes
    end

    # The span keeps the text count; the histogram must not: it would create one
    # series per count (up to batch_size), which is unbounded label cardinality.
    def self.end_share(span, elapsed, attributes:, instance: nil)
      attributes = attributes.merge('emb.instance' => instance) if instance
      safe { span&.set_attribute('emb.instance', instance) } if instance
      end_span(span)
      histogram('emb.client.share.duration', elapsed, attributes: attributes.except('emb.texts'))
      elapsed
    end

    # Built once per process. The API's proxy providers forward to an SDK
    # configured later, and its provider reader takes a global mutex, so
    # re-reading one on every call would buy a lock and nothing else.
    def self.state
      return nil unless available?

      @state ||= build_state(OpenTelemetry.tracer_provider)
    rescue StandardError
      nil
    end

    def self.build_state(provider)
      meter = meter_for
      State.new(provider, provider.tracer(NAME, Emb::VERSION), histograms_for(meter), counter_for(meter))
    rescue StandardError
      nil
    end

    def self.meter_for
      return nil unless defined?(OpenTelemetry) && OpenTelemetry.respond_to?(:meter_provider)

      OpenTelemetry.meter_provider.meter(NAME, version: Emb::VERSION)
    rescue StandardError
      nil
    end

    def self.histograms_for(meter)
      return nil unless meter

      HISTOGRAMS.to_h do |name, (unit, description)|
        [name, meter.create_histogram(name, unit: unit, description: description,
                                            advisory: { explicit_bucket_boundaries: BOUNDARIES })]
      end
    rescue StandardError
      nil
    end

    def self.counter_for(meter)
      return nil unless meter

      meter.create_up_down_counter(IN_FLIGHT, unit: '{share}', description: 'Commands currently dispatched.')
    rescue StandardError
      nil
    end

    def self.metric_attributes(entry)
      attributes = { 'emb.model' => entry[:model], 'emb.mode' => entry[:mode] }
      attributes['emb.format'] = entry[:format].to_s if entry[:format]
      attributes
    end

    def self.safe
      yield
    rescue StandardError
      nil
    end

    private_class_method :state, :build_state, :meter_for, :histograms_for,
                         :counter_for, :metric_attributes, :around_span, :safe
  end
end

require_relative 'telemetry/deferred'
