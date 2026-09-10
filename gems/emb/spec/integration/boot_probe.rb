# frozen_string_literal: true

# Boots the dummy Rails application in a fresh subprocess and prints a JSON
# fact sheet on the last stdout line. Run one process per boot: a Rails
# application can only be initialized once, and the fake-based railtie spec
# rewrites the Emb::Railtie constant in-process.
#
# Environment switches:
#   EMB_OPT_OUT=1       set config.emb.middleware = false
#   EMB_MANUAL_MOUNT=1  mount Emb::Middleware manually before initialize
#   EMB_REQUEST=success|raise  run a request through the built stack
require 'json'
require 'rack/mock'

require_relative 'dummy/config/application'

Emb::Railtie.config.emb.middleware = false if ENV['EMB_OPT_OUT'] == '1'
Rails.application.config.middleware.use Emb::Middleware if ENV['EMB_MANUAL_MOUNT'] == '1'

scope_inside = nil

Rails.application.initialize!

stack = Rails.application.middleware

result = {
  'rails' => Rails::VERSION::STRING,
  'stack_class' => stack.class.name,
  'emb_count' => stack.count { |m| m.klass == Emb::Middleware },
  'emb_include' => stack.include?(Emb::Middleware)
}

if ENV['EMB_REQUEST']
  probe = lambda do |_env|
    BatchLoader::Executor.ensure_current
    scope_inside = !BatchLoader::Executor.current.nil?
    raise 'probe failure' if ENV['EMB_REQUEST'] == 'raise'

    [200, { 'content-type' => 'text/plain' }, ['ok']]
  end

  begin
    stack.build(probe).call(Rack::MockRequest.env_for('/'))
  rescue StandardError
    # Expected for the raise probe; scope assertions still apply.
  end

  result['scope_inside'] = scope_inside
  result['scope_after'] = !BatchLoader::Executor.current.nil?
end

puts result.to_json
