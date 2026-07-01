# frozen_string_literal: true

require_relative "../lib/emb"

EMB_PORT = 16379

RSpec.configure do |config|
  config.before(:suite) do
    Emb.setup(port: EMB_PORT)
    10.times do
      break if Emb.ping == "PONG"
      sleep 1
    end
  end
end
