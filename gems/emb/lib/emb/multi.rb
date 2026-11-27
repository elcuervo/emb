# frozen_string_literal: true

require_relative 'image_command'

module Emb
  # Eager multi-model batching: text and image calls collected in one block and
  # sent as EMB.MULTI / EMB.IMGMULTI round trips. The explicit block is the
  # opt-in, so it never defers, even under a lazy client (same as text).
  class MultiProxy
    def initialize(client)
      @client = client
      @pairs = []
    end

    def [](name)
      PairCollector.new(@pairs, name.to_sym)
    end

    # Results come back in submission order, one per call (one per image for a
    # multi-image call): a text vector, or an image output-name Hash, with nil
    # for a failed or truncated image pair.
    def run
      chunk = @client.respond_to?(:batch_size) && @client.batch_size ? @client.batch_size : Emb.configuration.batch_size
      results = Array.new(@pairs.size)
      text, images = @pairs.each_with_index.to_a.partition { |pair, _| pair[:kind] == :text }
      send_texts(text, results, chunk)
      send_images(images, results, chunk)
      results
    end

    private

    def send_texts(indexed, results, chunk)
      indexed.each_slice(chunk) do |slice|
        args = slice.flat_map { |pair, _| [pair[:model].to_s, pair[:text]] }
        replies = @client.send_command('EMB.MULTI', *args)
        slice.each_with_index { |(_, index), i| results[index] = replies[i]&.unpack('e*') }
      end
    end

    # One EMB.IMGMULTI per compatible group (format and projection), chunked by
    # batch_size like the text path.
    def send_images(indexed, results, chunk)
      indexed.group_by { |pair, _| [pair[:format], pair[:names]] }.each_value do |group|
        send_image_group(group, results, chunk)
      end
    end

    def send_image_group(group, results, chunk)
      format = group.first.first[:format]
      group.each_slice(chunk) do |slice|
        replies = @client.send_command(*image_args(slice))
        slice.each_with_index { |(_, index), i| results[index] = image_value(replies[i], format) }
      end
    end

    # The group's projection rides on every pair's argv: a share is built only
    # from pairs that asked for the same outputs.
    def image_args(slice)
      first = slice.first.first
      pairs = slice.map { |pair, _| [pair[:model], pair[:bytes]] }
      ImageCommand.multi_args(pairs, format: first[:format], names: first[:names])
    end

    def image_value(entry, format)
      entry.nil? ? nil : ImageCommand.decode(entry, format)
    end

    class PairCollector
      def initialize(pairs, model)
        @pairs = pairs
        @model = model
      end

      def [](text)
        @pairs << { kind: :text, model: @model, text: text.dup }
      end

      # The same arguments as Proxy#image. Each image becomes its own pair, so
      # the block's result holds one Hash per image in submission order.
      def image(bytes, *more, format: :binary, outputs: nil)
        format = ImageCommand.validate_format!(format)
        names = Array(outputs).map { |name| name.to_s.dup.freeze }
        [bytes, *more].each do |payload|
          @pairs << { kind: :image, model: @model, bytes: Proxy.coerce_image_bytes(payload),
                      format: format, names: names }
        end
      end
    end
  end
end
