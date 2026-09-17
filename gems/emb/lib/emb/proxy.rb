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
      return values_query(text, texts) if format == :values

      unpack_set(@client.send_command('EMB', @name.to_s, text, *texts))
    end

    def inspect
      "#<Emb::Proxy #{@name}>"
    end

    # Embeds one or more raw images (EMB.IMG). Each argument is the encoded
    # bytes of an image (JPEG/PNG/GIF/WebP) and is sent unchanged as an
    # ASCII-8BIT RESP bulk, so no base64/URL encoding is needed:
    #
    #   Emb[:clip].image(File.binread('cat.jpg'))
    #   Emb[:clip].image(io1, io2)                 # IO objects are read
    #
    # format: :binary (default) returns the float32-unpacked vectors from the
    # compact BLOB wire (nil for a failed/truncated slot); format: :values
    # returns the decoded dtype/shape/values envelope. The float32 reply decode
    # is the same `unpack('e*')` path text embeddings use.
    def image(image_bytes, *more, format: :binary)
      format = normalize_format(format)
      images = [image_bytes, *more].map { |bytes| self.class.coerce_image_bytes(bytes) }

      if format == :values
        reply = @client.send_command('EMB.IMG', @name.to_s, 'VALUES', *images)
        envelope = Emb::ValuesReply.parse(reply)
        return envelope if images.size == 1

        return envelope.merge(values: Emb::ValuesReply.rows(envelope[:shape], envelope[:values]))
      end

      reply = @client.send_command('EMB.IMG', @name.to_s, *images)
      return unpack_embedding(reply) if images.size == 1

      Array(reply).map { |entry| entry.nil? ? nil : unpack_embedding(entry) }
    end

    # coerce_image_bytes normalizes an image argument to a binary byte string
    # without transcoding: a String's bytes are preserved exactly (only the
    # encoding label is set to ASCII-8BIT), and an IO is read fully.
    def self.coerce_image_bytes(bytes)
      data = bytes.respond_to?(:read) ? bytes.read : bytes
      data = data.to_s
      data = data.dup if data.frozen?
      data.force_encoding(Encoding::BINARY)
    end

    private

    # One packed vector for a single text, an array for several; a null slot
    # (failed/truncated position) maps to nil.
    def unpack_set(raw)
      return nil if raw.nil?

      result = Array(raw).map { |entry| entry&.unpack('e*') }
      result.size == 1 ? result.first : result
    end

    def unpack_embedding(entry)
      entry&.unpack('e*')
    end

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
