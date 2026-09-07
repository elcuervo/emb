# frozen_string_literal: true

# Parallel (threaded) end-to-end measurement across three model kinds:
#   siglip2  - text embeddings via the scripted path (fused CLIP export,
#              EVSHA returning a single raw float32 bulk, unpack('e*'))
#   e5       - standard embed path (Emb::Proxy#[])
#   gliner2  - scripted entity extraction (EVSHA with dynamic labels)
#
# Each scenario runs THREADS workers × TEXTS texts against a live server and
# reports per-request p50/p99 and aggregate req/s. Uses the same conventions
# as bench.rb. Run:  ruby gems/emb/bench/bench_models.rb [port]

require 'emb'

LINE = 'A flagship smartphone was announced by the company in the city last week.'

def ms
  Process.clock_gettime(Process::CLOCK_MONOTONIC) * 1000
end

def percentile(sorted, percent)
  return sorted.first if sorted.empty?

  sorted[((percent / 100.0) * (sorted.size - 1)).round]
end

def eager_client(port)
  Emb::Client.new(pool: 5, port: port, driver: nil, batch: false)
end

def median_of(arr)
  sorted = arr.sort
  sorted[sorted.size / 2]
end

def distinct_texts(count, seed)
  Array.new(count) { |i| "measurement text #{seed} #{i} — a company launched a product for people." }
end

# Runnable: one round of concurrent requests for the scenario.
# returns [per-request latencies, total_ms, total_requests]
def run_threads(threads, texts, client_factory, &work)
  queue = Queue.new
  started = ms
  workers = threads.times.map do |worker|
    Thread.new { run_worker(queue, distinct_texts(texts, worker), client_factory, worker, &work) }
  end
  workers.each(&:join)
  samples = []
  samples << queue.pop until queue.empty?
  [samples, ms - started, texts * threads]
end

# One worker's serial request loop; each request's latency is pushed to the
# shared queue for the percentile report.
def run_worker(queue, texts_for_worker, client_factory, worker, &work)
  cli = client_factory.call
  texts_for_worker.each do |t|
    t0 = ms
    work.call(cli, t, worker)
    queue << (ms - t0)
  end
end

def report(name, samples, total_ms, embeds, baseline)
  per_embed = total_ms / embeds
  req_s = embeds / (total_ms / 1000.0)
  sorted = samples.sort
  delta = baseline ? (per_embed - baseline) / baseline * 100 : 0.0
  puts format('%-18<name>s %6<embeds>d  %8<total_ms>.1f  %9<per_embed>.3f ' \
              '%9<req_s>.1f  %8<p50>.3f  %8<p99>.3f  %+6<delta>.1f%%',
              name: name, embeds: embeds, total_ms: total_ms, per_embed: per_embed,
              req_s: req_s, p50: percentile(sorted, 50), p99: percentile(sorted, 99),
              delta: delta)
end

port = Integer(ARGV[0] || ENV.fetch('EMB_BENCH_PORT', 16_379))
threads = Integer(ENV.fetch('EMB_BENCH_THREADS', 4))
texts   = Integer(ENV.fetch('EMB_BENCH_TEXTS', 50))

puts "port=#{port} threads=#{threads} texts=#{texts}/worker"
puts 'scenario              ops  total(ms) per-op(ms)      req/s      p50      p99  vs base'

# --- siglip2: scripted text embedding (EVSHA, raw float32 bulk) ---
sig_client = eager_client(port)
script = File.read(File.expand_path('../../../examples/scripts/siglip2.lua', __dir__))
sha = sig_client.script.load(:siglip2, script)
# One warm call: decode: :f32 turns the single packed float32 bulk back into
# 768 floats (dim must match the graph); the raw String would need unpack('e*').
sig_vec = sig_client.evalsha(:siglip2, sha, [LINE], ['normalize'], decode: :f32)
abort "siglip2: expected 768 dims, got #{sig_vec.size}" unless sig_vec.size == 768
sig_base = median_of([5].map do |_i|
  t0 = ms
  sig_client.evalsha(:siglip2, sha, [LINE], ['normalize'], decode: :f32)
  ms - t0
end)
samples, total, ops = run_threads(threads, texts, -> { eager_client(port) }) do |cli, t, _worker|
  cli.evalsha(:siglip2, sha, [t], ['normalize'])
end
report('siglip2 (script)', samples, total, ops, sig_base)

# --- e5: standard embed path ---
e5_base = median_of([5].map do |_i|
  t0 = ms
  Emb::Proxy.new(eager_client(port), :e5)[LINE]
  ms - t0
end)
samples, total, ops = run_threads(threads, texts, -> { eager_client(port) }) do |cli, t, _worker|
  Emb::Proxy.new(cli, :e5)[t]
end
report('e5 (embed)', samples, total, ops, e5_base)

# --- gliner2: scripted NER extraction (EVSHA, dynamic labels) ---
glin = eager_client(port)
glin_sha = glin.script.load(:gliner2, File.read(File.expand_path('../../../examples/scripts/gliner2.lua', __dir__)))
glin_base = median_of([5].map do |_i|
  t0 = ms
  glin.evalsha(:gliner2, glin_sha, [LINE], %w[PERSON ORG PRODUCT])
  ms - t0
end)
samples, total, ops = run_threads(threads, texts, -> { eager_client(port) }) do |cli, t, _worker|
  cli.evalsha(:gliner2, glin_sha, [t], %w[PERSON ORG PRODUCT])
end
report('gliner2 (script)', samples, total, ops, glin_base)
