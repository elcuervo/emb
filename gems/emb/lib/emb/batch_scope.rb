# frozen_string_literal: true

require 'batch_loader'

module Emb
  # Runs a block of work within a fresh per-thread batch scope: the scope that
  # existed before the wrap is cleared afterwards (even on exception), bounding
  # cache growth and dropping unused loaders at each boundary — a request, a
  # job execution, or any other unit of work.
  module BatchScope
    def self.wrap
      yield
    ensure
      BatchLoader::Executor.clear_current
    end
  end
end
