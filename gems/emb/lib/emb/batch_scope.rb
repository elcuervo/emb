# frozen_string_literal: true

require 'batch_loader'

module Emb
  # Runs a block of work and clears the per-thread batch scope afterwards,
  # even when the block raises.
  module BatchScope
    def self.wrap
      yield
    ensure
      BatchLoader::Executor.clear_current
    end
  end
end
