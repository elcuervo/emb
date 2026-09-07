# frozen_string_literal: true

module Emb
  # Server-status and runtime-config command wrappers. They only depend on
  # #send_command, so they live in this module (included into Emb::Client)
  # rather than growing the client class.
  module Commands
    # EMB.STATS as a Symbol-keyed Hash. No client-side type layer: values are
    # exactly what the RESP decoder returned (Integer where the server sends
    # RESP integers, String otherwise).
    def stats
      raw = send_command('EMB.STATS')
      raw.each_slice(2).to_h { |key, value| [key.to_sym, value] }
    end

    # Server-wide INFO (the Redis-style sectioned command). No sections =
    # all sections; any number of sections filter the reply.
    def server_info(*sections)
      parse_info(send_command('INFO', *sections.map(&:to_s)))
    end

    # Remove all cached embeddings, or only entries belonging to +model+.
    # Returns the server's integer removed-entry count.
    def cache_flush(model = nil)
      args = ['EMB.CACHE.FLUSH']
      args << model.to_s unless model.nil?
      send_command(*args)
    end

    # Ask the server to start an asynchronous cache snapshot. Completion and
    # failures are observable through #stats or #server_info(:cache).
    def save_cache
      send_command('EMB.SAVE')
    end

    private

    # Parse Redis INFO section text into a nested Hash:
    #   {Server: {redis_version: "0.2.4", uptime_secs: "7"}, Cache: {…}, …}
    # Section names and keys are Symbols; values pass through as the server
    # sent them. Lines that don't fit the grammar are ignored.
    def parse_info(text)
      sections = {}
      current = nil

      text.split("\r\n").each do |line|
        if line.start_with?('# ')
          current = line[2..].to_sym
          sections[current] ||= {}
        elsif current && line.include?(':')
          key, value = line.split(':', 2)
          sections[current][key.to_sym] = value
        end
      end

      sections
    end
  end
end
