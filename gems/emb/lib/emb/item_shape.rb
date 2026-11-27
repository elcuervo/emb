# frozen_string_literal: true

module Emb
  # Reads the three item layouts a batch scope can hold: [client, model, text,
  # format] for embeds, [client, SCRIPT_ITEM, argv, decode] for scripts, and
  # [client, ImageBatch::ITEM, model, images, format, names] for images. Split
  # out of BatchDispatch to keep that module within its length budget.
  module ItemShape
    def script_item?(item) = item[1].equal?(SCRIPT_ITEM)

    def image_item?(item) = item[1].equal?(ImageBatch::ITEM)

    def embed_item?(item)
      !script_item?(item) && !image_item?(item)
    end

    def item_model(item)
      return item[2][1] if script_item?(item)
      return item[2] if image_item?(item)

      item[1]
    end

    def item_text_count(item)
      return item[2][3].to_i if script_item?(item)
      return item[3].size if image_item?(item)

      Array(item[2]).size
    end
  end
end
