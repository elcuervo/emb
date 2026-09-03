# frozen_string_literal: true

require 'batch_loader'
require_relative 'batch_scope'

module Emb
  # Background-job middleware that clears the per-thread batch scope at the end
  # of each job execution, mirroring Emb::Middleware's per-request guarantee.
  # Framework-agnostic: usable as a Sidekiq/Shoryuken server middleware
  # (`call(worker, job, queue)` yielding the chain) and as the core of the
  # ActiveJob perform callback (which invokes BatchScope.wrap directly). The
  # scope is cleared even when the job raises; each job starts with a fresh
  # scope so cached embeddings and unused loaders never leak between jobs.
  class JobMiddleware
    def call(_worker, _job, _queue, &)
      BatchScope.wrap(&)
    end
  end
end
