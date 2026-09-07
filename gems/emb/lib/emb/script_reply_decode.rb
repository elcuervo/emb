# frozen_string_literal: true

module Emb
  # Opt-in float decoding for scripted replies — the decode: keyword on
  # Emb::Commands#eval / #evalsha. Kept out of the Commands module so the
  # command surface stays small. decode: is validated (normalize_decode) before
  # any command is sent, then applied after reply parsing.
  module ScriptReplyDecode
    private

    # Validates an (un-normalized) decode option and canonicalizes it. Raises
    # ArgumentError for unknown modes so callers fail before any command is
    # sent. nil and an empty hash both mean "no decoding".
    def normalize_decode(decode)
      case decode
      when nil, :f32
        decode
      when Hash
        schema = decode.each_with_object({}) do |(field, mode), acc|
          unless mode == :f32
            raise ArgumentError, "unsupported decode mode #{mode.inspect} for field #{field.inspect} (supported: :f32)"
          end

          acc[field.to_s] = :f32
        end
        schema.empty? ? nil : schema
      else
        raise ArgumentError, "unsupported decode mode #{decode.inspect} (supported: :f32 or {field => :f32})"
      end
    end

    # Applies a normalized decode to the parsed (script_hash'ed) reply array.
    def apply_decode(parsed, decode)
      return parsed if decode.nil?

      case decode
      when :f32
        parsed.each_with_index.map { |v, i| decode_f32(v, position(i)) }
      when Hash
        parsed.each_with_index.map { |v, i| decode_hash_fields(v, decode, position(i)) }
      end
    end

    def position(index)
      index.zero? ? 'the reply' : "element #{index + 1}"
    end

    # decode: :f32 — a float32-packed bulk unpacks to floats; a numeric array
    # (the pre-float32_bytes reply style) passes through unchanged; anything
    # else is a contract mismatch and raises.
    def decode_f32(value, where)
      case value
      when String
        unless (value.bytesize % 4).zero?
          raise ArgumentError,
                "decode :f32: #{where} is a #{value.bytesize}-byte bulk, not a multiple of 4 — not a float32 vector"
        end

        value.unpack('e*')
      when Array
        return value if value.all?(Numeric)

        raise ArgumentError, "decode :f32: #{where} is an array of non-numbers, not a float vector"
      else
        raise ArgumentError,
              "decode :f32: #{where} is a #{value.class}, expected a float32-packed bulk or a numeric array"
      end
    end

    # decode: {field => :f32} — the reply (or each per-text reply) must be a
    # hash; present fields decode as :f32, absent fields stay untouched.
    def decode_hash_fields(value, schema, where)
      unless value.is_a?(Hash)
        raise ArgumentError, "decode #{schema.inspect}: #{where} is a #{value.class}, expected a hash reply"
      end

      schema.each_key do |field|
        next unless value.key?(field)

        value[field] = decode_f32(value[field], "field #{field.inspect} of #{where}")
      end
      value
    end
  end
end
