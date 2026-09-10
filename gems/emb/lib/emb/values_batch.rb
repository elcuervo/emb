# frozen_string_literal: true

require_relative 'values_reply'

module Emb
  # VALUES-format batch resolution for the deferred path. Split out of
  # BatchDispatch so both modules stay under the Metrics rubric.
  module ValuesBatch
    module_function

    # Resolves a VALUES reply onto the slice's items. Single-model slices get
    # ONE envelope reply carrying every text's flattened values (row-major),
    # so the rows are re-sliced per item; mixed-model slices get per-pair
    # envelopes and map one row each. Failures stay nil, mirroring the BLOB
    # path's MGET semantics.
    def resolve(loader, slice, results)
      if single_model?(slice)
        rows = envelope_rows(results)
        expect_rows!(slice, rows.size)
        assign_rows(loader, slice, rows)
      else
        resolve_entries(loader, slice, results)
      end
    end

    def single_model?(slice)
      slice.map { |item| item[1] }.uniq.size == 1
    end

    def envelope_rows(results)
      envelope = Emb::ValuesReply.parse(rehydrate_envelope(results))
      Emb::ValuesReply.rows(envelope[:shape], envelope[:values])
    end

    # Under RESP3, redis-client decodes the server's single-model VALUES map
    # into a Ruby Hash, but dispatch_slice wraps every reply in Array(), which
    # turns that Hash into an array of [key, value] pairs. Rehydrate those
    # pairs back into a Hash so parse sees a real envelope; RESP2 flat pair
    # arrays (top-level strings) pass through untouched.
    def rehydrate_envelope(results)
      return results.to_h if results.all? { |e| e.is_a?(Array) && e.size == 2 }

      results
    end

    def expect_rows!(slice, got)
      expected = slice.sum { |_, _, text, _| Array(text).size }
      return if got >= expected

      raise ShortReplyError, "expected #{expected} VALUES rows, got #{got}"
    end

    def assign_rows(loader, slice, rows)
      offset = 0
      slice.each do |item|
        _, _, text, = item
        texts = Array(text)
        values = rows[offset, texts.size]
        offset += texts.size
        loader.call(item, values.size == 1 ? values.first : values)
      end
    end

    def resolve_entries(loader, slice, results)
      expect_entries!(slice, results.size)
      offset = 0
      slice.each do |item|
        _, _, text, = item
        texts = Array(text)
        values = entry_values(results, offset, texts)
        offset += texts.size
        loader.call(item, values.size == 1 ? values.first : values)
      end
    end

    # A short multi-model reply is a protocol violation: an explicit null slot
    # is the only legal way to signal a missing pair (see the BLOB path).
    def expect_entries!(slice, got)
      expected = slice.sum { |_, _, text, _| Array(text).size }
      return if got >= expected

      raise ShortReplyError, "expected #{expected} VALUES reply entries, got #{got}"
    end

    def entry_values(results, offset, texts)
      results[offset, texts.size].map { |entry| entry && row_of(entry) }
    end

    def row_of(entry)
      envelope = Emb::ValuesReply.parse(entry)
      Emb::ValuesReply.rows(envelope[:shape], envelope[:values]).first
    end
  end
end
