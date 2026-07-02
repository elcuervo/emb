# frozen_string_literal: true

module Emb
  VERSION = Gem.loaded_specs['emb']&.version&.to_s || File.read(File.expand_path('../../VERSION', __dir__)).strip
end
