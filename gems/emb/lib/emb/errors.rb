# frozen_string_literal: true

module Emb
  # Raised when an EMB.MULTI batch fails — after redis-client exhausted its
  # configured re-sends (`reconnect_attempts`) on a transient error, or on the
  # first attempt for an operation error (server error reply) or the default
  # `reconnect_attempts: 0` configuration. The underlying redis error is
  # preserved as `cause`; `attempts` counts the wire sends that were made.
  class ServerError < StandardError
    attr_reader :attempts

    def initialize(message, attempts: 1)
      super(message)
      @attempts = attempts
    end
  end
end
