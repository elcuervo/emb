# frozen_string_literal: true

require 'batch_loader'
require_relative 'image_command'

module Emb
  # Deferred image batching: the item layout, scope bounds, compatible-group
  # shares, and ordered reply resolution. Kept out of BatchDispatch (which
  # stays under its Metrics::ModuleLength budget) and out of BATCH_BLOCK, which
  # only partitions items and hands image work here.
  module ImageBatch
    # Identity sentinel in slot 1 of an image batch item. Items are
    # [client, ITEM, model, images, format, output_names]; a unique object
    # matched with #equal? cannot collide with a model name.
    ITEM = Object.new

    module_function

    # Registers one deferred image call, refusing it before queue insertion
    # when the pending scope would exceed either bound.
    def build(client, model, images, format:, names:)
      check_limits!(client, images)
      item = [client, ITEM, model, images, format, names]
      Telemetry::Deferred.record(
        item, Telemetry.entry_for(client, operation: 'image', model: model, texts: images.size, format: format)
      )
      BatchLoader.for(item).batch(default_value: [], key: Emb::BATCH_KEY, &Emb::BATCH_BLOCK)
    end

    # Groups only compatible calls (model, format, projection). The scope bound
    # already caps the pending scope at batch_size images, so one group is one
    # EMB.IMG share; there is no chunk to split.
    def shares(items)
      items.group_by { |item| [item[2], item[4], item[5]] }.values
    end

    # One EMB.IMG carrying every image of the slice in deferral order.
    def args(slice)
      first = slice.first
      ImageCommand.args(first[2], slice.flat_map { |item| item[3] }, format: first[4], names: first[5])
    end

    # Maps the reply's per-image entries back onto each item, preserving the
    # eager shape: a Hash for a one-image call, an Array of Hashes for several,
    # nil for a failed or truncated image.
    def resolve(loader, slice, results)
      expected = slice.sum { |item| item[3].size }
      raise ShortReplyError, "expected #{expected} image reply entries, got #{results.size}" if results.size < expected

      offset = 0
      slice.each do |item|
        value, offset = decode_item(item, results, offset)
        loader.call(item, value)
        Telemetry::Deferred.resolve(item)
      end
    end

    # One item's slice of the reply, decoded per image, in the eager shape.
    def decode_item(item, results, offset)
      images = item[3]
      decoded = results[offset, images.size].map do |entry|
        entry.nil? ? nil : ImageCommand.decode(entry, item[4])
      end
      [images.size == 1 ? decoded.first : decoded, offset + images.size]
    end

    # Enforces both bounds against the items already pending in this thread's
    # batch scope. Deriving the usage from batch-loader's own pending set (and
    # not a second counter) means the existing scope cleanup releases it,
    # including when a request or job raises.
    def check_limits!(client, images)
      count, bytes = pending_usage(client)
      max_count, max_bytes = limits(client)
      if count + images.size > max_count
        raise ImageBatchLimitError,
              "deferred image scope would exceed batch_size (#{max_count} images)"
      end
      return unless bytes + images.sum(&:bytesize) > max_bytes

      raise ImageBatchLimitError,
            "deferred image scope would exceed max_image_batch_bytes (#{max_bytes} bytes)"
    end

    def pending_usage(client)
      items = pending_items
      return [0, 0] unless items

      items.each_with_object([0, 0]) { |item, totals| add_usage(totals, item, client) }
    end

    def pending_items
      BatchLoader::Executor.current&.items_by_block&.[]([Emb::BATCH_BLOCK.source_location, Emb::BATCH_KEY])
    end

    def add_usage(totals, item, client)
      return unless item[1].equal?(ITEM) && item[0].equal?(client)

      totals[0] += item[3].size
      totals[1] += item[3].sum(&:bytesize)
    end

    def limits(client)
      count = client.respond_to?(:batch_size) && client.batch_size
      bytes = client.respond_to?(:max_image_batch_bytes) && client.max_image_batch_bytes
      [count || Emb.configuration.batch_size, bytes || Emb.configuration.max_image_batch_bytes]
    end
  end
end
