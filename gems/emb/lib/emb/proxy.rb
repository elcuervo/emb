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

      Emb::Telemetry.embed_call(operation: 'embed', model: @name, texts: 1 + texts.size,
                                format: format, mode: 'eager') do
        if format == :values
          values_query(text, texts)
        else
          unpack_set(@client.send_command('EMB', @name.to_s, text, *texts))
        end
      end
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
    #   Emb[:dino].image(bytes, outputs: ['patches'])
    #
    # The reply is keyed by the model's configured output names (String keys,
    # as the server sends them): one image returns a Hash of name => value,
    # several return an Array of those Hashes
    # (nil for a failed or truncated slot). format: :binary (default) unpacks
    # the BLOB wire into float32 arrays; format: :values returns the decoded
    # dtype/shape/values envelope (plus grid for a token output). outputs:
    # narrows the projection and may name any subset of the configured outputs.
    def image(image_bytes, *more, format: :binary, outputs: nil)
      format = normalize_format(format)
      names = outputs.nil? ? [] : Array(outputs).map(&:to_s)
      images = [image_bytes, *more].map { |bytes| self.class.coerce_image_bytes(bytes) }

      Emb::Telemetry.embed_call(operation: 'image', model: @name, texts: images.size,
                                format: format, mode: 'eager') do
        image_reply(images, format, names)
      end
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

    # image_args builds the command: model, the optional reply-format keyword,
    # the optional OUTPUTS projection, then the images. Both keywords sit in
    # their fixed positions, so OUTPUTS is only ever sent right after them.
    def image_args(images, format, names)
      args = ['EMB.IMG', @name.to_s]
      args << 'VALUES' if format == :values
      args << 'OUTPUTS' << names.join(',') unless names.empty?
      args.concat(images)
    end

    def image_reply(images, format, names)
      reply = @client.send_command(*image_args(images, format, names))
      decoder = format == :values ? :decode_values_map : :decode_blob_map
      return nil if reply.nil? && images.size == 1
      return send(decoder, reply) if images.size == 1

      Array(reply).map { |entry| entry.nil? ? nil : send(decoder, entry) }
    end

    # image_pairs normalizes one keyed reply (a flat name/value array under
    # RESP2, an already-decoded Hash under RESP3) into name => value pairs.
    def image_pairs(entry)
      return entry if entry.is_a?(Hash)

      Array(entry).each_slice(2).to_h
    end

    def decode_blob_map(entry)
      image_pairs(entry).to_h { |name, value| [name, unpack_embedding(value)] }
    end

    def decode_values_map(entry)
      image_pairs(entry).to_h do |name, value|
        envelope = Emb::ValuesReply.parse(value)
        [name, envelope.merge(values: Emb::ValuesReply.rows(envelope[:shape], envelope[:values]))]
      end
    end

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
