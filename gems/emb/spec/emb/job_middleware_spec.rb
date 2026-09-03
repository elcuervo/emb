# frozen_string_literal: true

require 'spec_helper'

# Minimal recording client: send_command stores the command and answers from a
# fixed response table.
class JobMiddlewareFakeClient
  attr_reader :commands

  def initialize(responses = {})
    @responses = responses
    @commands = []
  end

  def send_command(*args)
    @commands << args
    @responses.fetch(args)
  end

  def self.vec(value)
    [value].pack('e') * 2
  end
end

RSpec.describe Emb::JobMiddleware do
  after { BatchLoader::Executor.clear_current }

  it 'clears the per-thread scope after each job execution' do
    client = JobMiddlewareFakeClient.new(
      ['EMB.MULTI', 'minilm', 'hello'] => [JobMiddlewareFakeClient.vec(1.0)]
    )
    middleware = described_class.new
    job = lambda do
      Emb::BatchProxy.new(client)[:minilm]['hello'].first
    end

    middleware.call(:worker, { 'class' => 'GreetJob' }, 'default') { job.call }
    expect(client.commands.size).to eq(1)

    # A second job on the same thread starts a fresh scope: the same pair
    # must re-send EMB.MULTI rather than reuse the previous job's cache.
    middleware.call(:worker, { 'class' => 'GreetJob' }, 'default') { job.call }
    expect(client.commands.size).to eq(2)
  end

  it 'clears the scope even when the job raises' do
    middleware = described_class.new

    expect { middleware.call(:worker, {}, 'default') { raise 'boom' } }.to raise_error('boom')
    expect(BatchLoader::Executor.current).to be_nil
  end

  it 'drops loaders created but never used during a job' do
    client = JobMiddlewareFakeClient.new
    middleware = described_class.new

    middleware.call(:worker, {}, 'default') do
      Emb::BatchProxy.new(client)[:minilm]['never used']
    end

    expect(client.commands).to be_empty
    expect(BatchLoader::Executor.current).to be_nil
  end

  it 'passes the job body return value through unchanged' do
    middleware = described_class.new

    expect(middleware.call(:worker, {}, 'default') { 42 }).to eq(42)
  end
end
