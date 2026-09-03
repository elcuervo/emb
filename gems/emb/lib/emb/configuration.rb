# frozen_string_literal: true

module Emb
  class Configuration
    OPTIONS = %i[
      host port url pool batch batch_size driver protocol
      connect_timeout read_timeout write_timeout reconnect_attempts
    ].freeze

    attr_accessor(*OPTIONS)

    def initialize
      self.host = 'localhost'
      self.port = 6379
      self.url = nil
      self.pool = 5
      self.batch = true
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
      # 0 = never auto-send the failed command again. EMB.MULTI is not
      # idempotent: redis-client treats ReadTimeoutError as a ConnectionError
      # and would re-send the whole batch up to N+1 times (duplicate inference).
      # Recovery is the app's decision (retries now fail closed).
      self.reconnect_attempts = 0
    end

    def to_h
      OPTIONS.to_h { |key| [key, public_send(key)] }
    end
  end
end
