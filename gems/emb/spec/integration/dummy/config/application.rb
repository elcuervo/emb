# frozen_string_literal: true

# Minimal Rails application used by the Railtie integration suite. It pulls in
# only what the Railtie touches (no Active Record, no routes) so a real boot is
# fast, and it never talks to an emb server.
require 'logger'
require 'rails'
require 'action_controller/railtie'
require 'emb'

module Dummy
  class Application < Rails::Application
    config.eager_load = false
    config.secret_key_base = 'emb-railtie-integration-secret-key-base'
    config.public_file_server.enabled = false
    config.hosts.clear
    config.log_level = :error
    config.logger = Logger.new(IO::NULL)
    config.active_support.deprecation = :silence
  end
end
