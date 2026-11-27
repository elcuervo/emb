# frozen_string_literal: true

require File.expand_path('../lib/emb.rb', __dir__)

EMB_PORT = 16_379

RSpec.configure do |config|
  config.before(:suite) do
    # The suite's default client stays eager (the `lazy: false` default); lazy
    # mode behavior is covered explicitly in emb_batch_spec / configuration_spec.
    Emb.setup(port: EMB_PORT)
    begin
      10.times do
        break if Emb.ping == 'PONG'

        sleep 1
      end
    rescue RedisClient::Error
      # The contract specs stub send_command and never touch the network;
      # EMB_NO_SERVER lets them run where no server exists. A normal run still
      # aborts here, so a missing server is one line, not a hundred failures.
      raise unless ENV['EMB_NO_SERVER']

      warn "no emb server on port #{EMB_PORT}; running the server-free specs only"
    end
  end
end
