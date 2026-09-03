# frozen_string_literal: true

require 'batch_loader'
require_relative 'batch_scope'

module Emb
  # Rack middleware that clears the per-thread batch scope at the end of each
  # request, bounding cache growth in long-lived request threads. The scope is
  # cleared even when the app raises; a fresh scope starts with the next request.
  class Middleware
    def initialize(app)
      @app = app
    end

    def call(env)
      BatchScope.wrap { @app.call(env) }
    end
  end
end
