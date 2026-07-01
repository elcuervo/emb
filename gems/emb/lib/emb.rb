# frozen_string_literal: true

require_relative "emb/version"
require_relative "emb/client"
require_relative "emb/proxy"
require_relative "emb/multi"

module Emb
  class << self
    def models
      raw = send_command("EMB.MODELS")

      return [] if raw.nil?

      raw.map do |name, dim, status|
        { name: name, dim: dim.to_i, status: status }
      end
    end

    def info(name)
      raw = send_command("EMB.INFO", name.to_s)

      return {} if raw.nil?

      raw
        .each_slice(2)
        .to_h { |k, v| [k.to_sym, v] }
    end

    def stats = send_command("EMB.STATS")

    def help = send_command("EMB.HELP")

    def ping = send_command("PING")
  end
end
