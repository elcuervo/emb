# frozen_string_literal: true

require_relative 'helper'

# Boots a real Rails application (see spec/integration/dummy) rather than the
# hand-rolled fakes used elsewhere. This is the regression net for the Railtie
# middleware mount: during initialization `app.middleware` is a
# Rails::Configuration::MiddlewareStackProxy, which has no `include?`, so any
# inspection-based guard raises here.
RSpec.describe 'Emb::Railtie on a real Rails application' do
  it 'boots and inserts Emb::Middleware exactly once' do
    result = RailsBoot.run

    expect(result['stack_class']).to eq('ActionDispatch::MiddlewareStack')
    expect(result['emb_count']).to eq(1)
    expect(result['emb_include']).to be(true)
  end

  it 'skips insertion when config.emb.middleware is false' do
    result = RailsBoot.run(opt_out: true)

    expect(result['emb_count']).to eq(0)
    expect(result['emb_include']).to be(false)
  end

  it 'inserts exactly once when the application opts out and mounts manually' do
    result = RailsBoot.run(opt_out: true, manual_mount: true)

    expect(result['emb_count']).to eq(1)
  end

  it 'stays functionally safe when the middleware is mounted twice' do
    result = RailsBoot.run(manual_mount: true, request: :success)

    expect(result['emb_count']).to eq(2)
    expect(result['scope_inside']).to be(true)
    expect(result['scope_after']).to be(false)
  end

  it 'clears the batch scope after a successful request' do
    result = RailsBoot.run(request: :success)

    expect(result['scope_inside']).to be(true)
    expect(result['scope_after']).to be(false)
  end

  it 'clears the batch scope when the request raises' do
    result = RailsBoot.run(request: :raise)

    expect(result['scope_inside']).to be(true)
    expect(result['scope_after']).to be(false)
  end
end
