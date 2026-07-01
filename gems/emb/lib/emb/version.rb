# frozen_string_literal: true

module Emb
  VERSION =
    begin
      File.read(File.expand_path('../../../../VERSION', __dir__)).strip
    rescue Errno::ENOENT
      Gem::Specification.find_by_name("emb").version.to_s
    end
end
