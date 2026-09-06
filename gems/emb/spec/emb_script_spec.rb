# frozen_string_literal: true

require 'spec_helper'

RSpec.describe Emb do
  describe 'script evaluation' do
    before(:all) do
      described_class.setup(port: EMB_PORT, batch: false)
      described_class.script.flush
    end

    after(:all) do
      described_class.script.flush
      described_class.reset_registry!
    end

    let(:join_script) { 'return KEYS[1] .. "|" .. ARGV[1]' }

    describe '.eval' do
      it 'evaluates an inline script with KEYS/ARGV' do
        expect(described_class.eval(:minilm, join_script, ['hello world'], ['PERSON'])).to eq('hello world|PERSON')
      end

      it 'returns a hash for flat field/value pair replies' do
        result = described_class.eval(:minilm, 'return {PERSON = {KEYS[1]}, score = 98}', ['Tim Cook'])
        expect(result).to eq('PERSON' => ['Tim Cook'], 'score' => 98)
      end

      it 'returns an array of hashes for multiple texts' do
        script = 'return {PERSON = {KEYS[1]}}'
        result = described_class.eval(:minilm, script, %w[a b])
        expect(result).to eq([{ 'PERSON' => ['a'] }, { 'PERSON' => ['b'] }])
      end

      it 'raises for script-reported errors' do
        expect { described_class.eval(:minilm, 'return {err = "bad labels"}', ['x']) }
          .to raise_error(RedisClient::CommandError, /bad labels/)
      end

      it 'raises wrong-arity errors' do
        expect { described_class.eval(:minilm, join_script, []) }
          .to raise_error(RedisClient::CommandError, /numtexts/)
      end
    end

    describe '.evalsha' do
      it 'runs a previously loaded script by SHA1' do
        sha = described_class.script.load(:minilm, join_script)
        expect(sha).to match(/\A[0-9a-f]{40}\z/)

        expect(described_class.evalsha(:minilm, sha, ['x'], ['ORG'])).to eq('x|ORG')
      end

      it 'raises for an unknown SHA' do
        expect { described_class.evalsha(:minilm, 'deadbeef', ['x']) }
          .to raise_error(RedisClient::CommandError, /no such script/)
      end
    end

    describe '.script' do
      it 'reports per-model existence and flushes' do
        sha = described_class.script.load(:minilm, join_script)
        expect(described_class.script.exists(:minilm, sha)).to eq([true])
        expect(described_class.script.exists(:bge, sha)).to eq([false])

        described_class.script.flush(:minilm)
        expect(described_class.script.exists(:minilm, sha)).to eq([false])
      end

      it 'rejects invalid scripts on load' do
        expect { described_class.script.load(:minilm, 'this is not lua (') }
          .to raise_error(RedisClient::CommandError, /compiling/)
      end
    end
  end
end
