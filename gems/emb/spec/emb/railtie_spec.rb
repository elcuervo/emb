# frozen_string_literal: true

require 'spec_helper'

# Minimal stand-ins for the Rails internals Emb::Railtie touches, so the boot
# order and wiring can be exercised without a Rails installation: a railtie
# that collects initializers and after_initialize callbacks, a per-railtie
# configuration object, an app with a middleware stack, ActiveSupport.on_load,
# and Sidekiq/Shoryuken configurators.
module RailtieFakeRails
  class Railtie
    class << self
      def initializer(name, &block)
        (@initializers ||= {})[name] = block
      end

      def initializers
        @initializers ||= {}
      end

      def config
        @config ||= Configuration.new
      end

      def inherited(subclass)
        super
        subclass.instance_variable_set(:@initializers, {})
        subclass.instance_variable_set(:@config, Configuration.new)
      end
    end
  end

  class Configuration
    attr_reader :after_initialize_blocks

    def initialize
      @options = {}
      @after_initialize_blocks = []
    end

    def after_initialize(&block)
      @after_initialize_blocks << block
    end

    def method_missing(name, *args)
      if name.to_s.end_with?('=')
        @options[name.to_s.chomp('=').to_sym] = args.first
      elsif args.empty?
        @options[name]
      else
        super
      end
    end

    def respond_to_missing?(name, include_private = false)
      name.to_s.end_with?('=') || @options.key?(name) || super
    end
  end

  class OrderedOptions
    def initialize
      @values = {}
    end

    def method_missing(name, *args)
      if name.to_s.end_with?('=')
        @values[name.to_s.chomp('=').to_sym] = args.first
      elsif args.empty?
        @values[name]
      else
        super
      end
    end

    def respond_to_missing?(name, include_private = false)
      name.to_s.end_with?('=') || @values.key?(name) || super
    end
  end

  class App
    attr_reader :middleware

    def initialize
      @middleware = MiddlewareStack.new
    end
  end

  class MiddlewareStack
    attr_reader :entries

    def initialize
      @entries = []
    end

    # use always appends; idempotency comes from the caller's include? guard.
    def use(middleware)
      @entries << middleware
    end

    def include?(middleware)
      @entries.include?(middleware)
    end
  end

  module ActiveSupport
    OrderedOptions = RailtieFakeRails::OrderedOptions

    class << self
      def on_load(hook_name, &block)
        @hooks ||= Hash.new { |h, k| h[k] = [] }
        @hooks[hook_name] << block
      end

      def hooks
        @hooks ||= Hash.new { |h, k| h[k] = [] }
      end
    end
  end

  class ActiveJobBase
    class << self
      def around_perform(&block)
        (@blocks ||= []) << block
      end

      def blocks
        @blocks ||= []
      end
    end
  end

  class JobConfigurator
    attr_reader :added

    def configure_server(&block)
      block.call(self)
    end

    def server_middleware(&block)
      block.call(self)
    end

    def add(middleware)
      (@added ||= []) << middleware
    end
  end

  # Sidekiq stand-in that also exposes a nested Testing module (mirroring
  # Sidekiq::Testing) so the testing-chain registration is exercised.
  class SidekiqWithTesting
    class << self
      attr_reader :server_added, :testing_added

      def reset!
        @server_added = nil
        @testing_added = nil
      end

      def configure_server(&block)
        block.call(self)
      end

      def server_middleware(&block)
        block.call(self)
      end

      def add(middleware)
        (@server_added ||= []) << middleware
      end
    end

    module Testing
      class << self
        attr_reader :testing_added

        def reset!
          @testing_added = nil
        end

        def server_middleware(&block)
          block.call(self)
        end

        def add(middleware)
          (@testing_added ||= []) << middleware
        end
      end
    end
  end
end

RSpec.describe 'Emb::Railtie' do
  # Rebinds Rails/ActiveSupport to the fakes (emb.rb's guarded require inside
  # spec_helper skipped the railtie because Rails was not defined there), loads
  # emb/railtie.rb, applies any config adjustments, then runs the initializer
  # and after_initialize phases against a fake app.
  def boot_railtie(app = RailtieFakeRails::App.new)
    stub_const('Rails', RailtieFakeRails)
    stub_const('ActiveSupport', RailtieFakeRails::ActiveSupport)
    load_railtie!

    yield Emb::Railtie.config if block_given?

    run_railtie(app)
    app
  end

  def load_railtie!
    RailtieFakeRails::ActiveSupport.hooks.clear
    RailtieFakeRails::ActiveJobBase.blocks.clear
    RailtieFakeRails::SidekiqWithTesting.reset!
    RailtieFakeRails::SidekiqWithTesting::Testing.reset!
    load File.expand_path('../../lib/emb/railtie.rb', __dir__)
  end

  def run_railtie(app)
    Emb::Railtie.initializers.each_value { |block| block.call(app) }
    Emb::Railtie.config.after_initialize_blocks.each(&:call)
  end

  after do
    # rubocop:disable RSpec/RemoveConst -- load re-executes emb/railtie.rb per
    # example; the constant must be truly gone or the class is reopened and
    # initializer/after_initialize state accumulates across examples.
    Emb.send(:remove_const, :Railtie) if Emb.const_defined?(:Railtie, false)
    # rubocop:enable RSpec/RemoveConst
  end

  it 'loads only when Rails::Railtie is defined and mounts the middleware' do
    app = boot_railtie

    expect(Emb::Railtie).to be < RailtieFakeRails::Railtie
    expect(app.middleware.entries).to eq([Emb::Middleware])
  end

  it 'does not duplicate a manually mounted Emb::Middleware' do
    app = RailtieFakeRails::App.new
    app.middleware.use Emb::Middleware # manual mount before boot

    boot_railtie(app)

    expect(app.middleware.entries).to eq([Emb::Middleware])
  end

  it 'is not defined when the gem loads without Rails (guarded require)' do
    lib = File.expand_path('../../lib', __dir__)
    ok = system(RbConfig.ruby, '-I', lib, '-e', "require 'emb'; exit(defined?(Emb::Railtie) ? 1 : 0)")

    expect(ok).to be true
  end

  it 'registers the ActiveJob perform callback that clears the job scope' do
    boot_railtie
    hook = RailtieFakeRails::ActiveSupport.hooks[:active_job].fetch(0)
    hook.call(RailtieFakeRails::ActiveJobBase)

    block = RailtieFakeRails::ActiveJobBase.blocks.fetch(0)
    mid_scope = nil
    block.call(:job, lambda {
      mid_scope = BatchLoader::Executor.ensure_current
      3
    })

    expect(mid_scope).to be_a(BatchLoader::Executor)
    expect(BatchLoader::Executor.current).to be_nil
  end

  it 'registers Emb::JobMiddleware for a present Sidekiq and Shoryuken' do
    sidekiq = RailtieFakeRails::JobConfigurator.new
    shoryuken = RailtieFakeRails::JobConfigurator.new
    stub_const('Sidekiq', sidekiq)
    stub_const('Shoryuken', shoryuken)

    boot_railtie

    expect(sidekiq.added).to eq([Emb::JobMiddleware])
    expect(shoryuken.added).to eq([Emb::JobMiddleware])
  end

  it 'registers Emb::JobMiddleware in the Sidekiq::Testing chain when testing is loaded' do
    sidekiq = RailtieFakeRails::SidekiqWithTesting
    stub_const('Sidekiq', sidekiq)

    boot_railtie

    expect(sidekiq.server_added).to eq([Emb::JobMiddleware])
    expect(sidekiq::Testing.testing_added).to eq([Emb::JobMiddleware])
  end

  it 'skips all job protection when config.emb.job_middleware is false' do
    sidekiq = RailtieFakeRails::JobConfigurator.new
    stub_const('Sidekiq', sidekiq)

    boot_railtie { |config| config.emb.job_middleware = false }

    expect(RailtieFakeRails::ActiveSupport.hooks).not_to have_key(:active_job)
    expect(sidekiq.added).to be_nil
  end

  it 'does not mount the middleware when config.emb.middleware is false' do
    app = boot_railtie { |config| config.emb.middleware = false }

    expect(app.middleware.entries).to be_empty
  end
end
