# frozen_string_literal: true

require 'spec_helper'
require 'json'

# The gem ships the server's contract (lib/emb/commands.json) and compares its
# own command surface to it, so a renamed command or keyword fails the build
# instead of shipping. This is a drift alarm, not an argv parser: the rule is
# "every uppercase token a builder writes in argv position 1..n is a keyword the
# contract declares for that command", the model name and the payloads being the
# only uppercase tokens a builder is allowed to invent.
RSpec.describe Emb do
  let(:vendored_path) { File.expand_path('../lib/emb/commands.json', __dir__) }
  let(:repo_path) { File.expand_path('../../../commands.json', __dir__) }
  let(:contract) { JSON.parse(File.read(vendored_path)) }
  let(:declared) { contract.fetch('commands').to_h { |command| [command.fetch('name'), command] } }

  after { BatchLoader::Executor.clear_current }

  describe 'command literals in lib/' do
    it 'declares every command name and keyword literal it finds' do
      vocabulary = declared.keys + declared.values.flat_map { |command| command.fetch('keywords') }
      offenders = command_literals.reject { |_, literal| vocabulary.include?(literal) }

      expect(offenders).to be_empty, <<~MESSAGE
        lib/ names command vocabulary the contract does not declare (run `rake commands:sync` after a server rename):
        #{offenders.map { |file, literal| "  #{literal} (#{file})" }.join("\n")}
      MESSAGE
    end
  end

  describe 'builder keywords' do
    it 'drives every keyword-bearing command the gem sends' do
      sent = command_literals.map(&:last).uniq & declared.keys
      keyword_bearing = sent.select { |name| declared.fetch(name).fetch('keywords').any? }
      missing = keyword_bearing - builder_runs.map(&:first).uniq

      expect(missing).to be_empty, "no driver for #{missing}; a keyword path would go unchecked"
    end

    it 'declares every keyword token a builder writes for its command' do
      builder_runs.each do |command, run|
        keywords = declared.fetch(command).fetch('keywords')
        argvs_for(run).each do |label, argv|
          expect(argv.first).to eq(command), "#{command} (#{label}) drove #{argv.first.inspect} instead"
          undeclared = keyword_tokens(argv) - keywords
          expect(undeclared).to be_empty, "#{command} (#{label}): undeclared #{undeclared.join(', ')}"
        end
      end
    end

    # An option must write the keyword that carries it. Each variant is driven
    # alone against the default wire form, so an empty difference means the
    # option silently stopped being emitted (the shipped EMB.IMGMULTI-without-
    # OUTPUTS bug).
    it 'writes a declared keyword for every option it accepts' do
      builder_runs.each do |command, run|
        keywords = declared.fetch(command).fetch('keywords')
        base = keyword_tokens(run.fetch(:base))

        run.fetch(:variants).each do |label, argv|
          added = keyword_tokens(argv) - base
          expect(added).not_to be_empty, "#{command} (#{label}) emitted no keyword: #{argv.inspect}"
          expect(added - keywords).to be_empty, "#{command} (#{label}): undeclared #{added.join(', ')}"
        end
      end
    end
  end

  describe 'vendored copy' do
    it 'is byte-identical to the repository commands.json' do
      skip "repository commands.json is unreachable at #{repo_path}" unless File.exist?(repo_path)

      stale = 'lib/emb/commands.json is stale: run `rake commands:sync`'
      expect(File.binread(vendored_path)).to eq(File.binread(repo_path)), stale
    end
  end

  # Each run is [command, {base:, variants: {label => argv}}]. The base is the
  # default wire form; every variant is that call plus one option, so the
  # differential above is what proves the option reaches the wire.
  def builder_runs
    sent = []
    eager = recording_client(sent)
    lazy = recording_client(sent, lazy: :multi)

    [emb_run(sent, eager, lazy), emb_multi_run(sent, lazy), image_run(sent, eager),
     image_multi_run(sent, eager), script_run(sent, eager), config_run(sent, eager)]
  end

  def emb_run(sent, eager, lazy)
    ['EMB', { base: capture(sent) { eager[:clip]['hi'] },
              variants: { values: capture(sent) { eager[:clip]['hi', format: :values] },
                          deferred: capture(sent) { lazy[:clip]['hi', format: :values].to_a } } }]
  end

  def emb_multi_run(sent, lazy)
    ['EMB.MULTI', { base: capture(sent) { lazy_mixed(lazy) },
                    variants: { values: capture(sent) { lazy_mixed(lazy, format: :values) } } }]
  end

  def image_run(sent, eager)
    ['EMB.IMG', { base: capture(sent) { eager[:clip].image('img'.b) },
                  variants: { values: capture(sent) { eager[:clip].image('img'.b, format: :values) },
                              projection: capture(sent) { eager[:clip].image('img'.b, outputs: ['patches']) } } }]
  end

  def image_multi_run(sent, eager)
    ['EMB.IMGMULTI', { base: capture(sent) { multi_image(eager) },
                       variants: { values: capture(sent) { multi_image(eager, format: :values) },
                                   projection: capture(sent) { multi_image(eager, outputs: ['patches']) } } }]
  end

  def script_run(sent, eager)
    ['EMB.SCRIPT', { base: capture(sent) { eager.script.load('clip', 'return 1') },
                     variants: { exists: capture(sent) { eager.script.exists('clip', 'ab') },
                                 flush: capture(sent) { eager.script.flush } } }]
  end

  def config_run(sent, eager)
    ['CONFIG', { base: capture(sent) { eager.config.to_h },
                 variants: { set: capture(sent) { eager.config['max_concurrent_requests'] = '4' } } }]
  end

  def multi_image(client, **options)
    client.multi { |m| m[:clip].image('img'.b, **options) }
  end

  # Creating both loaders before forcing either is what puts two models in one
  # batch scope, which is what makes the dispatcher send EMB.MULTI.
  def lazy_mixed(client, format: :binary)
    first = client[:clip]['hi', format: format]
    second = client[:dino]['ho', format: format]
    first.to_a
    second.to_a
  end

  def argvs_for(run)
    run.fetch(:variants).merge(base: run.fetch(:base))
  end

  def capture(sent)
    sent.clear
    yield
    sent.last
  end

  def recording_client(sent, **options)
    client = Emb::Client.new(**options)
    allow(client).to receive(:send_command) do |*args|
      sent << args
      reply_for(args)
    end
    client
  end

  # Just enough of each reply shape for the call to return: the specs assert on
  # the recorded argv, never on these values.
  def reply_for(argv)
    values = argv.include?('VALUES')
    case argv.first
    when 'EMB', 'EMB.MULTI' then text_reply(argv.first, values)
    when 'EMB.IMG', 'EMB.IMGMULTI' then image_reply(argv.first, values)
    else []
    end
  end

  # EMB's VALUES reply is one envelope carrying every text; EMB.MULTI's is one
  # entry per pair.
  def text_reply(command, values)
    return values ? envelope : [blob] if command == 'EMB'

    values ? [envelope, envelope] : [blob, blob]
  end

  def image_reply(command, values)
    return [] unless values

    command == 'EMB.IMG' ? ['embedding', envelope] : [['embedding', envelope]]
  end

  def envelope
    ['dtype', 'FLOAT', 'shape', [1, 1], 'values', ['1.0']]
  end

  def blob
    [1.0].pack('e*')
  end

  # Quoted uppercase literals are the gem's command names and the keywords it
  # writes. Comment lines are skipped so prose cannot trip the alarm; names with
  # underscores (ENV keys such as EMB_URL) do not match the vocabulary shape.
  def command_literals
    root = File.expand_path('..', __dir__)
    Dir.glob(File.join(root, 'lib/**/*.rb')).flat_map do |path|
      code = File.readlines(path).reject { |line| line.lstrip.start_with?('#') }.join
      code.scan(/['"]([A-Z][A-Z0-9.]{2,})['"]/)
          .flatten
          .map { |literal| [path.delete_prefix("#{root}/"), literal] }
    end
  end

  # Model names are lowercase at every call site and payloads are binary
  # strings, so an uppercase token in argv position 1..n is a keyword candidate.
  def keyword_tokens(argv)
    Array(argv).drop(1).grep(/\A[A-Z][A-Z0-9_]{2,}\z/)
  end
end
