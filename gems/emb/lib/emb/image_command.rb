# frozen_string_literal: true

require_relative 'values_reply'

module Emb
  # The image command surface shared by Proxy (eager and deferred EMB.IMG), the
  # deferred batch dispatcher, and MultiProxy (EMB.IMGMULTI): keyword placement,
  # format validation, and keyed reply decoding, so the callers cannot drift.
  module ImageCommand
    FORMATS = %i[binary values].freeze

    module_function

    def validate_format!(format)
      return format if FORMATS.include?(format)

      raise ArgumentError, "unknown format #{format.inspect} (expected :binary or :values)"
    end

    # EMB.IMG <model> [VALUES] [OUTPUTS name,...] <bytes>...
    def args(model, images, format:, names:)
      command = ['EMB.IMG', model.to_s]
      command << 'VALUES' if format == :values
      command << 'OUTPUTS' << names.join(',') unless names.empty?
      command.concat(images)
    end

    # EMB.IMGMULTI [VALUES] <model> <bytes> <model> <bytes>...
    def multi_args(pairs, format:)
      command = ['EMB.IMGMULTI']
      command << 'VALUES' if format == :values
      pairs.each { |model, bytes| command << model.to_s << bytes }
      command
    end

    # One image's keyed reply (a flat name/value array under RESP2, a Hash under
    # RESP3) decoded for the requested format.
    def decode(entry, format)
      format == :values ? decode_values_map(entry) : decode_blob_map(entry)
    end

    def decode_blob_map(entry)
      image_pairs(entry).to_h { |name, value| [name, value&.unpack('e*')] }
    end

    # Under VALUES an EMB.IMGMULTI pair map carries its model beside the
    # outputs (docs/commands.md); that entry is a plain string, not an envelope.
    def decode_values_map(entry)
      image_pairs(entry).to_h do |name, value|
        next [name, value] if name == 'model'

        envelope = Emb::ValuesReply.parse(value)
        [name, envelope.merge(values: Emb::ValuesReply.rows(envelope[:shape], envelope[:values]))]
      end
    end

    def image_pairs(entry)
      return entry if entry.is_a?(Hash)

      Array(entry).each_slice(2).to_h
    end
  end
end
