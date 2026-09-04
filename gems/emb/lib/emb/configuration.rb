# frozen_string_literal: true

module Emb
  class Configuration
    OPTIONS = %i[
      host port url pool lazy batch_size driver protocol
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

    def initialize
      self.host = 'localhost'
      self.port = 6379
      self.url = nil
      self.pool = 5
      self.lazy = false
      self.batch_size = 512
      self.driver = nil
      self.protocol = 2
      self.connect_timeout = nil
      # Read/write timeouts are explicit, NOT nil: nil forwards nothing and
      # redis-client silently applies its 1.0s default, which makes 512-pair
      # EMB.MULTI batches fail under load. 10s covers worst-case inference on
      # shared CPUs; scale up if you raise batch_size.
      self.read_timeout = 10
      self.write_timeout = 10
      # 0 = default: a failing EMB.MULTI batch fails closed after one attempt
      # and raises Emb::ServerError. Set > 0 to opt into bounded retries:
      # redis-client re-sends transient failures (timeouts, connection errors)
      # up to that many extra times — EMB.MULTI is not idempotent, so each
      # re-send duplicates inference — and the batch still terminates in
      # Emb::ServerError. An Array of per-retry delays is also accepted (one
      # retry per entry). Operation errors (server error replies) are never
      # retried.
      self.reconnect_attempts = 0
    end

    def to_h
      OPTIONS.to_h { |key| [key, public_send(key)] }
    end
  end
end
