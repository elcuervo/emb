# frozen_string_literal: true

module Emb
  # Decoding for EMB VALUES envelopes (the RedisAI META+VALUES shape). The
  # server replies with flat alternating key/value pairs under RESP2 — dtype,
  # shape, values — where values are decimal bulk strings carrying the float64
  # widening of each float32 dimension.
  module ValuesReply
    module_function

    # Parses one envelope (a length-6 pair array) into a Hash. values are
    # converted to Ruby Floats; an unparsable entry raises ArgumentError
    # naming the offending value.
    def parse(reply)
      pairs = reply.each_slice(2).to_h
      shape = pairs.fetch('shape')
      values = Array(pairs.fetch('values')).map { |v| float_value(v) }
      { dtype: pairs.fetch('dtype'), shape: shape, values: values }
    end

    # rows reshapes flat values into per-text rows using shape [m, dim].
    def rows(shape, values)
      dim = shape[1].to_i
      values.each_slice(dim).to_a
    end

    def float_value(v)
      Float(v)
    rescue ArgumentError, TypeError
      raise ArgumentError, "EMB VALUES: unparsable decimal value #{v.inspect} (expected a numeric bulk string)"
    end
  end
end