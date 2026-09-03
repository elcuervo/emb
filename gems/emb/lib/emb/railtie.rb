# frozen_string_literal: true

if defined?(Rails::Railtie)
  require_relative '../emb'

  module Emb
    # Rails integration, loaded automatically by emb.rb when Rails is present.
    #
    #   config.emb.middleware = false      # skip Emb::Middleware in the stack
    #   config.emb.job_middleware = false  # skip job-scope protection
    #
    # This is the single wiring point (sentry-rails style): it mounts
    # Emb::Middleware in the middleware stack (idempotently, so a manual mount
    # is never duplicated) and registers Emb::JobMiddleware for every job
    # framework present — ActiveJob (an around_perform callback on
    # ActiveJob::Base, covering every adapter including SolidQueue), plus
    # explicit server middlewares for plain Sidekiq and Shoryuken workers.
    class Railtie < Rails::Railtie
      config.emb = ActiveSupport::OrderedOptions.new
      config.emb.middleware = true
      config.emb.job_middleware = true

      initializer 'emb.middleware' do |app|
        next if config.emb.middleware == false
        next if app.middleware.include?(Emb::Middleware)

        app.middleware.use Emb::Middleware
      end

      config.after_initialize do
        next if config.emb.job_middleware == false

        ActiveSupport.on_load(:active_job) do |base|
          base.around_perform { |_job, block| Emb::BatchScope.wrap { block.call } }
        end

        if defined?(Sidekiq)
          Sidekiq.configure_server do |sidekiq_config|
            sidekiq_config.server_middleware { |chain| chain.add Emb::JobMiddleware }
          end
        end

        if defined?(Shoryuken)
          Shoryuken.configure_server do |shoryuken_config|
            shoryuken_config.server_middleware { |chain| chain.add Emb::JobMiddleware }
          end
        end
      end
    end
  end
end
