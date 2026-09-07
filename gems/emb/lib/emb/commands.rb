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
    def eval(model, script, texts, args = [], decode: nil)
      texts = Array(texts)
      schema = normalize_decode(decode)
      reply = send_command('EMB.EVAL', model.to_s, script, texts.size, *texts.map(&:to_s), *args.map(&:to_s))
      parse_script_reply(reply, multi: texts.size > 1, decode: schema)
    end

    # Evaluate a previously loaded script by SHA1 (EMB.EVSHA). See #eval.
    def evalsha(model, sha, texts, args = [], decode: nil)
      texts = Array(texts)
      schema = normalize_decode(decode)
      reply = send_command('EMB.EVSHA', model.to_s, sha, texts.size, *texts.map(&:to_s), *args.map(&:to_s))
      parse_script_reply(reply, multi: texts.size > 1, decode: schema)
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

    private

    # Converts a scripted reply (single value, or an array of per-text values
    # for multi-text calls) through the hash grammar: a flat field/value pair
    # array becomes a Hash, recursively. Pure even-length string lists are
    # indistinguishable from two-field hashes on the RESP2 wire, so scripts
    # that must return them should nest them (wrap the list in a table).
    #
    # decode is the normalized (see normalize_decode) opt-in float decoding:
    #   nil                        → no decoding, parsing exactly as before
    #   :f32                       → each value position is unpack('e*')'d when
    #                                it is a packed float bulk; numeric arrays
    #                                pass through as floats
    #   {field => :f32, ...}       → after hash parsing, the named field(s) of
    #                                each hash reply decode as :f32 (fields
    #                                absent from a reply are left untouched)
    def parse_script_reply(reply, multi:, decode: nil)
      parsed = (multi ? reply : [reply]).map { |v| script_hash(v) }
      decoded = apply_decode(parsed, decode)
      multi ? decoded : decoded.first
    end

    def script_hash(value)
      return value unless value.is_a?(Array) && value.size.even?

      pairs = value.each_slice(2).to_a
      return value unless pairs.all? { |field, _| field.is_a?(String) }

      pairs.to_h { |field, val| [field, script_hash(val)] }
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
