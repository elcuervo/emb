# frozen_string_literal: true

module Emb
  class Configuration
    OPTIONS = %i[
      host port url pool lazy batch_size max_image_batch_bytes driver protocol
      connect_timeout read_timeout write_timeout reconnect_attempts
    ].freeze

    # Execution modes for embed calls. false = eager (default, one EMB round
    # trip per call); :multi = defer and coalesce into EMB.MULTI, serial;
    # :batch = defer and execute chunk shares concurrently. Mutually exclusive
    # by construction.
    LAZY_MODES = [false, :multi, :batch].freeze

    attr_accessor(*OPTIONS)

    def lazy=(value)
      unless LAZY_MODES.include?(value)
        raise ArgumentError, "lazy must be false, :multi, or :batch (got #{value.inspect})"
      end

      @lazy = value
    end

    # The gem speaks RESP2 only: its models/stats/info/config decoders assume
    # flat RESP2 arrays, and RESP3 maps would silently return wrong results.
    def self.validate_protocol!(value)
      return if value == 2

      raise ArgumentError, "protocol must be 2 (RESP2 only; RESP3 is not decoded), got #{value.inspect}"
    end

    def protocol=(value)
      self.class.validate_protocol!(value)
      @protocol = value
    end

    # Deferred image calls retain their encoded bytes until the scope is
    # forced: README.md#images. Must be a positive Integer (32 MiB by default).
    def max_image_batch_bytes=(value)
      self.class.validate_image_batch_bytes!(value)
      @max_image_batch_bytes = value
    end

    def self.validate_image_batch_bytes!(value)
      return if value.is_a?(Integer) && value.positive?

      raise ArgumentError, "max_image_batch_bytes must be a positive Integer (got #{value.inspect})"
    end

    def initialize
      self.host = 'localhost'
      self.port = 6379
      self.url = nil
      self.pool = 5
      self.lazy = false
      self.batch_size = 512
      # Deferred image payloads are bounded by total encoded bytes as well as
      # by batch_size; the default matches the server's per-image byte cap.
      apply_image_defaults
      apply_network_defaults
    end

    def to_h
      OPTIONS.to_h { |key| [key, public_send(key)] }
    end

    private

    def apply_image_defaults
      self.max_image_batch_bytes = 32 * 1024 * 1024
    end

    def apply_network_defaults
      self.driver = nil
      self.protocol = 2
      self.connect_timeout = nil
      # Read/write timeouts are explicit, NOT nil: nil forwards nothing and
      # redis-client silently applies its 1.0s default, which makes 512-pair
      # EMB.MULTI batches fail under load. 10s covers worst-case inference on
      # shared CPUs; scale up if you raise batch_size.
      self.read_timeout = 10
      self.write_timeout = 10
      # 0 = default: a failing batch fails closed after one attempt and raises
      # Emb::ServerError. Set > 0 to opt into bounded re-sends: redis-client
      # retries connection/protocol failures (never read timeouts) up to that
      # many extra times — EMB.MULTI is not idempotent, so each re-send
      # duplicates inference — and the batch still terminates in
      # Emb::ServerError. An Array of per-retry delays is also accepted (one
      # retry per entry). Operation errors (server error replies) are never
      # retried.
      self.reconnect_attempts = 0
    end
  end
end
