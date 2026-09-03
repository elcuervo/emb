# frozen_string_literal: true

require 'batch_loader'
require_relative 'batch_scope'

module Emb
  # Rack middleware: clears the per-thread batch scope after each request,
  # even when the app raises.
  class Middleware
    def initialize(app)
      @app = app
    end

    def call(env)
      BatchScope.wrap { @app.call(env) }
    end
  end
end
