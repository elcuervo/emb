# frozen_string_literal: true

Gem::Specification.new do |spec|
  spec.name     = 'emb'
  spec.version  = File.read(File.expand_path('../../VERSION', __dir__)).strip
  spec.license  = 'MIT'
  spec.summary  = 'Client for emb: a Redis-compatible embedding server'
  spec.authors  = ['elcuervo']
  spec.email    = ['elcuervo@elcuervo.net']
  spec.homepage = 'https://github.com/elcuervo/emb'

  spec.required_ruby_version = '>= 3.3'

  spec.files = Dir[
    'lib/**/*',
    'LICENSE',
    'README.md',
    'Gemfile',
  ]

  spec.require_paths = ['lib']

  spec.add_dependency 'connection_pool', '~> 2.5'
  spec.add_dependency 'redis-client', '~> 0.24'

  spec.add_development_dependency 'rspec', '~> 3.13'
  spec.metadata['rubygems_mfa_required'] = 'true'
end
