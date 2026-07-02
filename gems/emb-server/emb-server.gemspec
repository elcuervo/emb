# frozen_string_literal: true

Gem::Specification.new do |spec|
  spec.name        = 'emb-server'
  spec.version     = File.read(File.expand_path('../../VERSION', __dir__)).strip
  spec.license     = 'MIT'
  spec.summary     = 'emb: Redis-compatible embedding server'
  spec.description = 'Precompiled emb server binary.'
  spec.authors     = ['elcuervo']
  spec.email       = ['elcuervo@elcuervo.net']
  spec.homepage    = 'https://github.com/elcuervo/emb'

  spec.required_ruby_version = '>= 3.3'

  spec.files = Dir[
    'lib/emb-server.rb',
    'lib/emb-server/*',
    'lib/emb-server/emb-binary-*',
    'LICENSE',
    'README.md'
  ]

  spec.bindir = 'bin'
  spec.executables = ['emb']
  spec.require_paths = ['lib']

  spec.add_dependency 'onnxruntime', '~> 0.11'
  spec.metadata['rubygems_mfa_required'] = 'true'
end
