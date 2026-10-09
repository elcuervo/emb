# frozen_string_literal: true

require_relative 'script_reply_decode'

module Emb
  # Server-status and runtime-config command wrappers. They only depend on
  # #send_command, so they live in this module (included into Emb::Client)
  # rather than growing the client class.
  module Commands
    include ScriptReplyDecode

    # EMB.STATS as a Symbol-keyed Hash. No client-side type layer: values are
    # exactly what the RESP decoder returned (Integer where the server sends
    # RESP integers, String otherwise).
    def stats
      raw = send_command('EMB.STATS')
      raw.each_slice(2).to_h { |key, value| [key.to_sym, value] }
    end

    # Server-wide INFO (the Redis-style sectioned command). No sections =
    # all sections; any number of sections filter the reply.
    def server_info(*sections)
      parse_info(send_command('INFO', *sections.map(&:to_s)))
    end

    # Evaluate a script against a model (EMB.EVAL). texts become the script's
    # KEYS, args its ARGV. Replies are typed per the reply grammar: hash
    # replies (flat field/value pair arrays under RESP2) become Ruby Hashes,
    # nested values recurse. decode: opt-in float decoding — see
    # parse_script_reply (decode: :f32 or decode: {field => :f32}).
    #
    # Honors the client's lazy mode: eager under false, deferred under :multi
    # and :batch (see Emb::Commands#run_script_command).
    def eval(model, script, texts, args = [], decode: nil)
      texts = Array(texts)
      run_script_command(script_argv('EMB.EVAL', model, script, texts, args), decode)
    end

    # Evaluate a previously loaded script by SHA1 (EMB.EVSHA). See #eval.
    def evalsha(model, sha, texts, args = [], decode: nil)
      texts = Array(texts)
      run_script_command(script_argv('EMB.EVSHA', model, sha, texts, args), decode)
    end

    # EMB.SCRIPT subcommands: a small command object so the surface reads
    # naturally (script.load / script.exists / script.flush).
    def script
      @script ||= ScriptCommands.new(self)
    end

    # EMB.SCRIPT LOAD/EXISTS/FLUSH.
    class ScriptCommands
      def initialize(client)
        @client = client
      end

      # Compile, cache, and return the script's SHA1 for the model.
      def load(model, source)
        @client.send_command('EMB.SCRIPT', 'LOAD', model.to_s, source)
      end

      # Whether each SHA is cached for the model.
      def exists(model, *shas)
        @client.send_command('EMB.SCRIPT', 'EXISTS', model.to_s, *shas.map(&:to_s)).map { |flag| flag == 1 }
      end

      # Clear the model's cached scripts, or all models when omitted.
      def flush(model = nil)
        args = ['EMB.SCRIPT', 'FLUSH']
        args << model.to_s if model
        @client.send_command(*args)
      end
    end

    # Remove all cached embeddings, or only entries belonging to +model+.
    # Returns the server's integer removed-entry count.
    def cache_flush(model = nil)
      args = ['EMB.CACHE.FLUSH']
      args << model.to_s unless model.nil?
      send_command(*args)
    end

    # Ask the server to start an asynchronous cache snapshot. Completion and
    # failures are observable through #stats or #server_info(:cache).
    def save_cache
      send_command('EMB.SAVE')
    end

    private

    # EMB.EVAL/EMB.EVSHA argv: command, model, script|SHA, numtexts, KEYS, ARGV.
    def script_argv(cmd, model, target, texts, args)
      [cmd, model.to_s, target, texts.size, *texts.map(&:to_s), *args.map(&:to_s)]
    end

    # Sends the built argv (eager) or defers it into the thread's batch scope
    # (Emb.build_script_loader). decode: is normalized here so an invalid mode
    # raises before anything is sent; the reply is parsed only on resolution
    # (ScriptReplyDecode#parse_script_reply), with multi: from the argv count.
    def run_script_command(argv, decode)
      schema = normalize_decode(decode)
      return Emb.build_script_loader(self, argv, schema) if lazy?

      parse_script_reply(send_command(*argv), multi: argv[3].to_i > 1, decode: schema)
    end

    # Parse Redis INFO section text into a nested Hash:
    #   {Server: {redis_version: "0.2.4", uptime_secs: "7"}, Cache: {…}, …}
    # Section names and keys are Symbols; values pass through as the server
    # sent them. Lines that don't fit the grammar are ignored.
    def parse_info(text)
      sections = {}
      current = nil

      text.split("\r\n").each do |line|
        if line.start_with?('# ')
          current = line[2..].to_sym
          sections[current] ||= {}
        elsif current && line.include?(':')
          key, value = line.split(':', 2)
          sections[current][key.to_sym] = value
        end
      end

      sections
    end
  end
end
