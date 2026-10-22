# frozen_string_literal: true

module Emb
  module Telemetry
    # The per-thread deferral clock for `lazy` modes: keyed by the batch item
    # (batch-loader's dedupe key) and never stored inside the tuple, where a
    # per-call value would defeat coalescing and duplicate server inference.
    #
    # An entry is removed when its item resolves. Survivors are flushed as
    # not-dispatched when the batch scope clears, which is the only evidence
    # that a call waited and never reached a command.
    module Deferred
      KEY = :emb_telemetry_deferrals
      private_constant :KEY

      module_function

      # `||=` keeps the earliest deferral for a deduped tuple.
      def record(item, entry)
        deferrals[item] ||= entry.merge(at: Telemetry.mono, wall: Time.now)
      end

      # Records the dispatch point of a share's items: the queue ends here,
      # before the command is written. Called for failed shares too, so a failed
      # call still reports the time it waited.
      def dispatched(slice)
        now = Telemetry.mono
        slice.each do |item|
          entry = deferrals[item]
          next unless entry

          entry[:queue] ||= now - entry[:at]
          Telemetry.histogram('emb.client.queue.duration', entry[:queue], attributes: { 'emb.mode' => entry[:mode] })
        end
      end

      # Closes the deferred call's span. `dispatched: false` marks an item that
      # resolved without ever being part of a sent command, so it is never read
      # as a fast inference.
      def resolve(item, dispatched: true, error: nil)
        entry = deferrals.delete(item)
        return unless entry

        now = Telemetry.mono
        span = open_span(entry, dispatched: dispatched, queue: entry[:queue] || (now - entry[:at]))
        Telemetry.fail_span(span, error) if error
        Telemetry.end_span(span)
        Telemetry.histogram('emb.client.embed.duration', now - entry[:at], attributes: metric_attributes(entry))
      end

      # Flushes deferrals that resolved without a command, then drops the scope's
      # registry. Called when the batch scope ends and when a batch fails closed.
      def flush
        entries = Thread.current[KEY]
        return if entries.nil? || entries.empty?

        wall = Time.now
        entries.each_value { |entry| emit(entry, wall) }
        entries.clear
      end

      def span_attributes(entry, dispatched:)
        attributes = {
          'emb.operation' => entry[:operation],
          'emb.model' => entry[:model],
          'emb.texts' => entry[:texts],
          'emb.mode' => entry[:mode],
          'emb.dispatched' => dispatched
        }
        attributes['emb.format'] = entry[:format].to_s if entry[:format]
        attributes
      end

      def metric_attributes(entry)
        attributes = { 'emb.model' => entry[:model], 'emb.mode' => entry[:mode] }
        attributes['emb.format'] = entry[:format].to_s if entry[:format]
        attributes
      end

      def deferrals
        Thread.current[KEY] ||= {}
      end

      def open_span(entry, dispatched:, queue: nil)
        attributes = span_attributes(entry, dispatched: dispatched)
        attributes['emb.queue'] = queue if queue
        Telemetry.start_span(Telemetry::SPAN_EMBED, attributes: attributes, kind: :client,
                                                    start_timestamp: entry[:wall])
      end

      def emit(entry, wall)
        Telemetry.end_span(open_span(entry, dispatched: false), at: wall)
        Telemetry.histogram('emb.client.embed.duration', Telemetry.mono - entry[:at],
                            attributes: metric_attributes(entry))
      end
    end
  end
end
