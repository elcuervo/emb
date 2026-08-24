# frozen_string_literal: true

module Emb
  class Configuration
    OPTIONS = %i[
      host port url pool batch driver protocol
      connect_timeout read_timeout write_timeout reconnect_attempts
    ].freeze

    attr_accessor(*OPTIONS)

    def initialize
      self.host = 'localhost'
      self.port = 6379
      self.url = nil
      self.pool = 5
      self.batch = true
      self.driver = nil
      self.protocol = 2
      self.connect_timeout = nil
      self.read_timeout = nil
      self.write_timeout = nil
      self.reconnect_attempts = 3
    end

    def to_h
      OPTIONS.to_h { |key| [key, public_send(key)] }
    end
  end
end
