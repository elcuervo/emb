# frozen_string_literal: true

require 'batch_loader'
require_relative 'batch_scope'

module Emb
  # Job middleware: clears the per-thread batch scope after each job execution,
  # even when the job raises. Registered as a Sidekiq/Shoryuken server
  # middleware (positional args differ per framework and are unused) and used by
  # the ActiveJob perform callback.
  class JobMiddleware
    def call(*_args, &)
      BatchScope.wrap(&)
    end
  end
end
