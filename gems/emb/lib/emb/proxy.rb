# frozen_string_literal: true

require_relative 'values_reply'

module Emb
  class Proxy
    attr_reader :name

    def initialize(client, name)
      @client = client
      @name = name
    end

    # Queries the model for one or more texts. format: :binary (default)
    # returns the float32-unpacked vectors from the compact BLOB wire;
    # format: :values sends the VALUES keyword and returns the decoded
    # envelope — a Hash with dtype:/shape:/values: (Flats, flat for a single
    # text, grouped per text when several are requested).
    def [](text, *texts, format: :binary)
      format = normalize_format(format)
      if @client.lazy?
        return Emb.build_batch_loader(@client, @name, texts.empty? ? text : [text, *texts], format: format)
      end

      if format == :values
        return values_query(text, texts)
      end

      set = Array(@client.send_command('EMB', @name.to_s, text, *texts))
      result = set.map { |entry| entry.unpack('e*') }

      return result.first if result.size == 1

      result
    end

    def inspect
      "#<Emb::Proxy #{@name}>"
    end

    private

    def normalize_format(format)
      unless %i[binary values].include?(format)
        raise ArgumentError, "unknown format #{format.inspect} (expected :binary or :values)"
      end

      format
    end

    def values_query(text, texts)
      reply = @client.send_command('EMB', @name.to_s, 'VALUES', text, *texts)
      envelope = Emb::ValuesReply.parse(reply)
      return envelope if texts.empty?

      envelope.merge(values: Emb::ValuesReply.rows(envelope[:shape], envelope[:values]))
    end
  end
end
