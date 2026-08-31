# frozen_string_literal: true

module Emb
  # Hash-like live view of the server's runtime configuration, backed by
  # CONFIG GET / CONFIG SET.
  #
  #   config.to_h           # => { "cache" => "auto", "listen" => ":6379", … }
  #   config['cache']       # => "auto"          (scalar for an exact key)
  #   config['cache*']      # => { "cache" => …, "cache_file" => … }  (glob)
  #   config['cache_file'] = '/var/lib/emb/cache.rdb'   # => "OK"
  #
  # Values are Strings (config is text, not metrics) and round-trip into
  # writers unchanged. Server errors (unknown/read-only parameter, invalid
  # value, NOAUTH) raise RedisClient::CommandError.
  class RuntimeConfig
    def initialize(client)
      @client = client
    end

    # All parameters as a String-keyed Hash (CONFIG GET).
    def to_h
      pairs(@client.send_command('CONFIG', 'GET'))
    end

    # Read one parameter or a glob. An exact key returns its String value
    # (nil when the server has no such parameter); a glob that matches
    # several parameters returns a Hash.
    def [](key)
      key = key.to_s
      map = pairs(@client.send_command('CONFIG', 'GET', key))
      return map[key] if map.key?(key)

      map.empty? ? nil : map
    end

    # Write one parameter (CONFIG SET). Returns the server's reply ("OK").
    def []=(key, value)
      @client.send_command('CONFIG', 'SET', key.to_s, value.to_s)
    end

    private

    def pairs(raw)
      raw.each_slice(2).to_h { |k, v| [k, v] }
    end
  end
end
