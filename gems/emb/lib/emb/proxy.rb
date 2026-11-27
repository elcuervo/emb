# frozen_string_literal: true

require_relative 'image_command'
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
      names = Array(outputs).map { |name| name.to_s.dup.freeze }
      images = coerce_images([image_bytes, *more])
      return Emb.build_image_loader(@client, @name, images, format: format, names: names) if deferred?

      Emb::Telemetry.embed_call(operation: 'image', model: @name, texts: images.size,
                                format: format, mode: 'eager') do
        image_reply(images, format, names)
      end
    end

    # coerce_image_bytes normalizes an image argument to a fresh binary String
    # without transcoding: a String's bytes are copied exactly (encoding label
    # set to ASCII-8BIT), and an IO is read fully — or, with a limit, read only
    # up to it so an oversized stream is never materialized. The caller's
    # object is never touched, so a later mutation cannot change a deferred
    # request that captured this value.
    def self.coerce_image_bytes(bytes, limit: nil)
      return bytes.to_s.b unless bytes.respond_to?(:read)

      data = read_image_bytes(bytes, limit)
      data ? data.force_encoding(Encoding::BINARY) : ''.b
    end

    def self.read_image_bytes(io, limit)
      return io.read if limit.nil?

      data = io.read(limit + 1)
      return data if data.nil? || data.bytesize <= limit

      raise ImageBatchLimitError, "image bytes would exceed max_image_batch_bytes (#{limit} bytes)"
    end

    private

    # A deferred call reads each IO against what its pending scope can still
    # hold, so an oversized stream raises instead of being materialized.
    def coerce_images(inputs)
      budget = deferred? ? ImageBatch.remaining_bytes(@client) : nil
      inputs.map do |bytes|
        data = self.class.coerce_image_bytes(bytes, limit: budget)
        budget = [budget - data.bytesize, 0].max if budget
        data
      end
    end

    def deferred?
      @client.respond_to?(:lazy?) && @client.lazy?
    end

    def image_reply(images, format, names)
      reply = @client.send_command(*ImageCommand.args(@name, images, format: format, names: names))
      return nil if reply.nil? && images.size == 1
      return ImageCommand.decode(reply, format) if images.size == 1

      Array(reply).map { |entry| entry.nil? ? nil : ImageCommand.decode(entry, format) }
    end

    # One packed vector for a single text, an array for several; a null slot
    # (failed/truncated position) maps to nil.
    def unpack_set(raw)
      return nil if raw.nil?

      result = Array(raw).map { |entry| entry&.unpack('e*') }
      result.size == 1 ? result.first : result
    end

    def normalize_format(format)
      ImageCommand.validate_format!(format)
    end

    def values_query(text, texts)
      reply = @client.send_command('EMB', @name.to_s, 'VALUES', text, *texts)
      envelope = Emb::ValuesReply.parse(reply)
      return envelope if texts.empty?

      envelope.merge(values: Emb::ValuesReply.rows(envelope[:shape], envelope[:values]))
    end
  end
end
