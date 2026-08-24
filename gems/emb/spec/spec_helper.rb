# frozen_string_literal: true

require File.expand_path('../lib/emb.rb', __dir__)

EMB_PORT = 16_379

RSpec.configure do |config|
  config.before(:suite) do
    # The suite's default client stays eager (batch: false); lazy default behavior
    # is covered explicitly in emb_batch_spec / emb_configuration_spec.
    Emb.setup(port: EMB_PORT, batch: false)
    10.times do
      break if Emb.ping == 'PONG'

      sleep 1
    end
  end
end
