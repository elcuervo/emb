# frozen_string_literal: true

module Emb
  # Script-reply parsing and opt-in float decoding — the decode: keyword on
  # Emb::Commands#eval / #evalsha. Kept out of the Commands module so both the
  # command surface and the deferred batch resolver can share it. module_function
  # makes every method callable as Emb::ScriptReplyDecode.foo(...) and, when
  # included, a private instance method (how Commands uses it).
  module ScriptReplyDecode
    module_function

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

    # Converts a scripted reply (single value, or an array of per-text values
    # for multi-text calls) through the hash grammar: a flat field/value pair
    # array becomes a Hash, recursively. Pure even-length string lists are
    # indistinguishable from two-field hashes on the RESP2 wire, so scripts
    # that must return them should nest them (wrap the list in a table).
    #
    # decode is the normalized (see normalize_decode) opt-in float decoding:
    #   nil                        → no decoding, parsing exactly as before
    #   :f32                       → each value position is unpack('e*')'d when
    #                                it is a packed float bulk; numeric arrays
    #                                pass through unchanged (element types kept)
    #   {field => :f32, ...}       → after hash parsing, the named field(s) of
    #                                each hash reply decode as :f32 (fields
    #                                absent from a reply are left untouched)
    def parse_script_reply(reply, multi:, decode: nil)
      parsed = (multi ? reply : [reply]).map { |v| script_hash(v) }
      decoded = apply_decode(parsed, decode)
      multi ? decoded : decoded.first
    end

    def script_hash(value)
      return value unless value.is_a?(Array) && value.size.even?

      pairs = value.each_slice(2).to_a
      return value unless pairs.all? { |field, _| field.is_a?(String) }

      pairs.to_h { |field, val| [field, script_hash(val)] }
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
