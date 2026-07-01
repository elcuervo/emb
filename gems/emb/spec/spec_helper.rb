# frozen_string_literal: true

$LOAD_PATH.unshift(File.expand_path("../lib", __dir__))
require "emb"

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
