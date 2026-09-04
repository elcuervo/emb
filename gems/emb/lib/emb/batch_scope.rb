# frozen_string_literal: true

require 'batch_loader'

module Emb
  # Runs a block of work and clears the per-thread batch scope afterwards,
  # even when the block raises. Clearing is unconditional: batch-loader scopes
  # are thread-global and shared by every client, so a deferred-mode client
  # (global `lazy` config or a per-call override) inside the block would
  # otherwise leak cached values and pending loaders into the next request/job.
  # Under the eager default (`lazy: false`) embed calls never create a scope,
  # so the clear is a no-op — the middleware is observably inert there.
  module BatchScope
    def self.wrap
      yield
    ensure
      BatchLoader::Executor.clear_current
    end
  end
end
