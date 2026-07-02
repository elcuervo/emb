# frozen_string_literal: true

module EmbServer
  VERSION = Gem.loaded_specs['emb-server']&.version&.to_s || '0.0.0'
end
