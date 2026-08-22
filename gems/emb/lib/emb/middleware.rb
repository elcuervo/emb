# frozen_string_literal: true

require 'batch_loader'

module Emb
  # Rack middleware that clears the per-thread batch scope at the end of each
  # request, bounding cache growth in long-lived request threads. The scope is
  # cleared even when the app raises; a fresh scope starts with the next request.
  class Middleware
    def initialize(app)
      @app = app
    end

    def call(env)
      @app.call(env)
    ensure
      BatchLoader::Executor.clear_current
    end
  end
end
