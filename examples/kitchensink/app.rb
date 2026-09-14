#!/usr/bin/env ruby
# frozen_string_literal: true

# Kitchensink — the smallest end-to-end vector application on top of emb.
#
#   emb   : text   -> vector    (compute; knows nothing about search)
#   redis : vector -> hits      (stores and searches; knows nothing about text)
#
# One client library per server, one protocol between them. Everything hard
# lives in a server, so this file only ingests, queries, and reports.
#
#   run.sh index README.md DESIGN.md
#   run.sh search "how does batching work" 3
#   run.sh stats

require 'emb'            # Emb::Client → the emb instance
require 'redis-client'   # RedisClient → the index
require 'digest'
require 'json'

MODEL = 'minilm'
DIM   = 384
KEY   = 'kitchensink:docs'
BATCH = 512   # chunks per EMB.MULTI (the server's default max_pairs is 4096)
CHUNK = 900   # target characters per chunk, roughly 200 tokens
MIN   = 80    # below this a paragraph is a heading or a stub

EMB   = Emb::Client.new(url: ENV.fetch('EMB_URL',   'redis://127.0.0.1:16400'))
# Pinned to RESP3 because the reply shape depends on it: VSIM comes back as a
# map of element => [score, attributes] rather than a flat array of strings.
REDIS = RedisClient.new(url: ENV.fetch('REDIS_URL', 'redis://127.0.0.1:6399'), protocol: 3)

Hit = Data.define(:id, :score, :src, :body)

# ── ingest ───────────────────────────────────────────────────────────────────

# A paragraph is a good chunk: long enough to answer with, short enough to embed
# sharply. Oversized ones are wrapped on a word boundary. Ids are a digest of
# source and text, so re-indexing is idempotent and an edit replaces exactly the
# chunks it touched.
def chunks(path)
  File.read(path)
      .split(/\n{2,}/)
      .flat_map { |para| para.strip.scan(/.{1,#{CHUNK}}(?:\s|\z)/m) }
      .map(&:strip)
      .reject { |body| body.length < MIN }
      .map { |body| { id: Digest::SHA1.hexdigest("#{path}:#{body}"), body:, src: path } }
end

def index(paths)
  work = paths.flat_map { |path| chunks(path) }
  warn "embedding #{work.size} chunks from #{paths.size} files"
  REDIS.call('DEL', KEY)                          # full rebuild; see README

  skipped = 0
  work.each_slice(BATCH).with_index(1) do |slice, n|
    # One round trip for the whole slice, not one per chunk.
    vectors = EMB.multi { |m| slice.each { |chunk| m[MODEL][chunk[:body]] } }

    REDIS.pipelined do |pipeline|
      slice.zip(vectors).each do |chunk, vector|
        # EMB.MULTI answers MGET-style: one null per failed pair, the rest
        # intact. Skip it — an absent element beats a fabricated zero vector,
        # which would match nothing while looking indexed.
        if vector.nil?
          skipped += 1
          next
        end
        pipeline.call('VADD',     KEY, 'VALUES', DIM, *vector, chunk[:id])
        # The payload rides in the index's own attributes, so search returns the
        # text itself and there is no second store to keep in sync.
        pipeline.call('VSETATTR', KEY, chunk[:id], JSON.generate(chunk.slice(:src, :body)))
      end
    end
    warn "  batch #{n}: #{[n * BATCH, work.size].min}/#{work.size}"
  end

  warn "skipped #{skipped} chunks that failed to embed" if skipped.positive?
  warn "indexed #{REDIS.call('VCARD', KEY)} elements at dim #{REDIS.call('VDIM', KEY)}"
end

# ── query ────────────────────────────────────────────────────────────────────

def search(query, k: 5, filter: nil)
  # The query must be embedded by the same model, with the same normalization,
  # as everything in the index. Change either and ranking degrades silently.
  args = ['VSIM', KEY, 'VALUES', DIM, *EMB[MODEL][query],
          'COUNT', k, 'WITHSCORES', 'WITHATTRIBS']
  args.push('FILTER', filter) if filter

  # VSIM answers as a map of element => [score, attributes]. The text comes back
  # out of the index itself, so there is no second store to drift. A key that
  # does not exist yields an empty reply, not an error.
  #
  # The score is NOT a raw cosine: Redis rescales it to (1 + cos) / 2, so an
  # orthogonal pair scores 0.5, not 0. Ordering is unaffected — the map is
  # monotonic — but the number does not mean what it looks like. See the README.
  REDIS.call(*args).map do |id, (score, attrs)|
    meta = attrs.to_s.empty? ? {} : JSON.parse(attrs)
    Hit.new(id:, score:, src: meta['src'], body: meta['body'])
  end
end

def render(hits)
  if hits.empty?
    puts '  no results — have you run `index`?'
    return
  end
  hits.each_with_index do |hit, i|
    printf("  %2d.  %.3f  %-24s %s\n", i + 1, hit.score,
           File.basename(hit.src.to_s), hit.body.to_s[0, 64].gsub(/\s+/, ' '))
  end
end

# ── observe ──────────────────────────────────────────────────────────────────

def index_shape
  return [0, '—'] unless REDIS.call('EXISTS', KEY) == 1

  [REDIS.call('VCARD', KEY), REDIS.call('VDIM', KEY)]
end

def stats
  s = EMB.stats
  c = EMB.server_info('cache')[:Cache] || {}
  elements, dim = index_shape
  puts "  emb    requests=#{s[:total_requests]} tokens=#{s[:total_tokens]} errors=#{s[:total_errors]}"
  puts "  model  #{s[:per_model]}"
  puts "  cache  hits=#{c[:cache_hits]} misses=#{c[:cache_misses]} rate=#{c[:cache_hit_rate]}"
  puts "  index  elements=#{elements} dim=#{dim}"
end

# ── cli ──────────────────────────────────────────────────────────────────────

USAGE = 'usage: app.rb index FILE... | search QUERY [K] [--filter EXPR] | stats'

case ARGV.shift
when 'index'
  abort USAGE if ARGV.empty?
  index(ARGV)
when 'search'
  abort USAGE unless ARGV[0]
  query = ARGV.shift
  filter = nil
  if (at = ARGV.index('--filter'))
    filter = ARGV[at + 1] || abort(USAGE)
    ARGV.slice!(at, 2)
  end
  render(search(query, k: (ARGV[0] || 5).to_i, filter:))
when 'stats'
  stats
else
  abort USAGE
end
