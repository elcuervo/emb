# frozen_string_literal: true

require 'spec_helper'

RSpec.describe Emb do
  describe 'script evaluation' do
    before(:all) do
      # Eager (non-batch) default client: `lazy: false` is the current name for
      # the pre-lazy-modes `batch: false` this script suite assumed.
      described_class.setup(port: EMB_PORT, lazy: false)
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
        script = 'local out = {} for i = 1, #KEYS do out[i] = {PERSON = {KEYS[i]}} end return out'
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
        # Observing that a script loaded for one model is absent from another's
        # cache needs a second registered model. Discover it rather than
        # hardcoding a fixture name, so the failure is explicit when the server
        # only exposes one model (the suite otherwise assumes minilm).
        names = described_class.models.map { |m| m[:name].to_s }
        other = (names - ['minilm']).first
        raise "per-model script test needs a second registered model (got: #{names.inspect})" unless other

        sha = described_class.script.load(:minilm, join_script)
        expect(described_class.script.exists(:minilm, sha)).to eq([true])
        expect(described_class.script.exists(other, sha)).to eq([false])

        described_class.script.flush(:minilm)
        expect(described_class.script.exists(:minilm, sha)).to eq([false])
      end

      it 'rejects invalid scripts on load' do
        expect { described_class.script.load(:minilm, 'this is not lua (') }
          .to raise_error(RedisClient::CommandError, /compiling/)
      end
    end

    describe 'script reply decoding' do
      # emb.math.float32_bytes is a host block available to every script, so
      # these exercise the decode layer without any model inference.
      let(:pack4) { 'return emb.math.float32_bytes({1, 2, 3, 4})' }
      let(:multi_pack) do
        <<~LUA
          local out = {}
          for i = 1, #KEYS do
            local v = {}
            for j = 1, 4 do v[j] = i + j end
            out[i] = emb.math.float32_bytes(v)
          end
          return out
        LUA
      end

      it 'decodes a packed vector reply to floats, byte-identical to manual unpack' do
        decoded = described_class.eval(:minilm, pack4, ['x'], [], decode: :f32)
        expect(decoded).to eq([1.0, 2.0, 3.0, 4.0])

        raw = described_class.eval(:minilm, pack4, ['x']) # no decode -> opaque bulk
        expect(raw.unpack('e*')).to eq(decoded)
      end

      it 'decodes each element of a multi-text reply' do
        decoded = described_class.eval(:minilm, multi_pack, %w[a b], [], decode: :f32)
        expect(decoded).to eq([[2.0, 3.0, 4.0, 5.0], [3.0, 4.0, 5.0, 6.0]])
      end

      it 'decodes a named hash field' do
        script = 'return {dim = 4, embedding = emb.math.float32_bytes({1, 2, 3, 4})}'
        decoded = described_class.eval(:minilm, script, ['x'], [], decode: { embedding: :f32 })
        expect(decoded).to eq('dim' => 4, 'embedding' => [1.0, 2.0, 3.0, 4.0])
      end

      it 'decodes the named field of each per-text hash' do
        script = <<~LUA
          local out = {}
          for i = 1, #KEYS do
            out[i] = { dim = 4, embedding = emb.math.float32_bytes({i, i + 1, i + 2, i + 3}) }
          end
          return out
        LUA
        decoded = described_class.eval(:minilm, script, %w[a b], [], decode: { embedding: :f32 })
        expect(decoded).to eq([
                                { 'dim' => 4, 'embedding' => [1.0, 2.0, 3.0, 4.0] },
                                { 'dim' => 4, 'embedding' => [2.0, 3.0, 4.0, 5.0] }
                              ])
      end

      it 'passes numeric-array replies through as floats' do
        expect(described_class.eval(:minilm, 'return {1, 2, 3, 4}', ['x'], [], decode: :f32)).to eq([1, 2, 3, 4])
      end

      it 'leaves hash fields absent from a reply untouched' do
        expect(described_class.eval(:minilm, 'return {dim = 4}', ['x'], [], decode: { embedding: :f32 }))
          .to eq('dim' => 4)
      end

      it 'keeps existing parsing when decode is omitted' do
        expect(described_class.eval(:minilm, 'return {PERSON = {KEYS[1]}, score = 98}', ['Tim Cook']))
          .to eq('PERSON' => ['Tim Cook'], 'score' => 98)
      end

      it 'raises for an unknown decode mode before any command is sent' do
        expect { described_class.eval(:minilm, 'return 1', ['x'], [], decode: :bidirectional) }
          .to raise_error(ArgumentError, /unsupported decode mode :bidirectional/)
      end

      it 'raises for :f32 on a hash reply' do
        expect { described_class.eval(:minilm, 'return {label = "x"}', ['x'], [], decode: :f32) }
          .to raise_error(ArgumentError, /expected a float32-packed bulk or a numeric array/)
      end

      it 'raises for a bulk that is not a multiple of 4 bytes' do
        expect { described_class.eval(:minilm, 'return "abc"', ['x'], [], decode: :f32) }
          .to raise_error(ArgumentError, /not a multiple of 4/)
      end

      it 'raises for {field => :f32} on a non-hash reply' do
        expect { described_class.eval(:minilm, pack4, ['x'], [], decode: { embedding: :f32 }) }
          .to raise_error(ArgumentError, /expected a hash reply/)
      end
    end
  end
end
