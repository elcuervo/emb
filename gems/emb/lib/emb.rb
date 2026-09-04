# frozen_string_literal: true

require_relative 'emb/version'
require_relative 'emb/configuration'
require_relative 'emb/commands'
require_relative 'emb/runtime_config'
require_relative 'emb/client'
require_relative 'emb/proxy'
require_relative 'emb/batch_scope'
require_relative 'emb/middleware'
require_relative 'emb/job_middleware'
require_relative 'emb/multi'
require_relative 'emb/batch'
require_relative 'emb/railtie' if defined?(Rails::Railtie)

module Emb
  class << self
    def new(...) = Client.new(...)

    def setup(...)
      @default_client = Client.new(...)
    end

    def configuration
      @configuration ||= Configuration.new
    end

    def configure
      yield configuration if block_given?
      configuration
    end

    def [](name)    = default_client[name]
    def models      = default_client.models
    def info(name)  = default_client.info(name)
    def stats       = default_client.stats
    def server_info(*sections)  = default_client.server_info(*sections)
    def config                  = default_client.config
    def help        = default_client.help
    def ping        = default_client.ping
    def ready       = default_client.ready
    def ready?      = default_client.ready?
    def multi(&)    = default_client.multi(&)
    def reset_registry! = default_client.reset_registry!
    def debug? = @debug
    def send_command(*) = default_client.send_command(*)

    def debug!
      @debug = true
    end

    private

    def default_client
      @default_client ||= Client.new
    end
  end
end
