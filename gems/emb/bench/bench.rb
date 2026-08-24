# frozen_string_literal: true

# End-to-end benchmark harness for the emb Ruby client (gems/emb).
#
# Plain Ruby, no test framework. Run via `just bench-ruby` or `rake bench`.
# The emb server must be running and reachable at EMB_BENCH_PORT (default 16379,
# matching test-two-models.yaml).
#
# Scenarios measure end-to-end embedding workloads against a live server and
# report requests/sec, p50/p99, and an overhead ratio against a warm
# single-embed inference baseline:
#   overhead = (per-embed e2e − inference baseline) / inference baseline

require 'emb'
require 'hiredis-client' if ENV['EMB_BENCH_DRIVER'] == 'hiredis'

EMB_PORT = Integer(ENV.fetch('EMB_BENCH_PORT', 16_379))
TEXTS    = Integer(ENV.fetch('EMB_BENCH_TEXTS', 200))    # texts per round
ROUNDS   = Integer(ENV.fetch('EMB_BENCH_ROUNDS', 4))     # rounds per scenario
THREADS  = Integer(ENV.fetch('EMB_BENCH_THREADS', 4))    # concurrency (threaded)
BASELINE = Integer(ENV.fetch('EMB_BENCH_BASELINE', 10))  # warm single-embed samples
POOL     = Integer(ENV.fetch('EMB_BENCH_POOL', 5))
DRIVER   = ENV['EMB_BENCH_DRIVER']&.to_sym
MODEL    = 'minilm'
STABILITY_ENABLED = ENV.fetch('EMB_BENCH_STABILITY', '1') == '1'

# CPU partition under which this run was measured (set by `just bench-ruby`).
# app = CPUs reserved for the server; bench = CPUs reserved for the tooling.
APP_CPUS   = ENV.fetch('EMB_BENCH_APP_CPUS', '6')
BENCH_CPUS = ENV.fetch('EMB_BENCH_BENCH_CPUS', '4')

def distinct_texts(count, seed)
  Array.new(count) { |i| "benchmark text #{seed} #{i}" }
end

def percentile(sorted, percent)
  return sorted.first if sorted.empty?

  sorted[((percent / 100.0) * (sorted.size - 1)).round]
end

def ms
  Process.clock_gettime(Process::CLOCK_MONOTONIC) * 1000
end

def new_client
  Emb::Client.new(pool: POOL, port: EMB_PORT, driver: DRIVER)
end

# Counts EMB vs EMB.MULTI commands at the client boundary, delegating to a
# real client — used to assert round-trip reduction (lazy = one MULTI).
class CountingClient
  attr_reader :counts

  def initialize(client)
    @client = client
    @counts = Hash.new(0)
  end

  def send_command(*args)
    @counts[args.first] += 1
    @client.send_command(*args)
  end

  def batch? = false
end

# Warm inference baseline: median latency of a few sequential eager embeds of
# distinct texts (server cache-cold for these texts).
def inference_baseline_ms
  latencies = distinct_texts(BASELINE, "baseline-#{Process.pid}").map do |t|
    started = ms
    Emb::Proxy.new(new_client, MODEL)[t]
    ms - started
  end
  percentile(latencies.sort, 50)
end

def check_roundtrips
  assert_roundtrips(lazy_client_counts, { 'EMB.MULTI' => 1 }, 'lazy')
  assert_roundtrips(eager_client_counts, { 'EMB' => 5 }, 'eager')
end

def assert_roundtrips(actual, expected, label)
  return if actual == expected

  detail = actual.sort_by(&:first).map { |command, count| "#{command}=#{count}" }.join(' ')
  abort "#{label} round trips: #{detail} (expected #{expected.inspect})"
end

def lazy_client_counts
  client = CountingClient.new(new_client)
  distinct_texts(5, 'lazy').map { |t| Emb::BatchProxy.new(client)[MODEL][t] }.each(&:first)
  client.counts
end

def eager_client_counts
  client = CountingClient.new(new_client)
  distinct_texts(5, 'eager').each { |t| Emb::Proxy.new(client, MODEL)[t] }
  client.counts
end

def timed_call
  started = ms
  yield
  ms - started
end

def scenario_eager
  samples = ROUNDS.times.flat_map { |round| eager_round(round) }
  Result.new(TEXTS * ROUNDS, samples, 1, 0.0)
end

def eager_round(round)
  distinct_texts(TEXTS, round).map do |t|
    timed_call { Emb::Proxy.new(new_client, MODEL)[t] }
  end
end

def scenario_lazy
  samples = ROUNDS.times.map { |round| lazy_round(round) }
  Result.new(TEXTS * ROUNDS, samples, TEXTS, 0.0)
end

def lazy_round(round)
  timed_call do
    loaders = distinct_texts(TEXTS, round).map { |t| Emb::BatchProxy.new(new_client)[MODEL][t] }
    loaders.each(&:first)
  end
end

def scenario_pipelined
  samples = ROUNDS.times.map { |round| pipelined_round(round) }
  Result.new(TEXTS * ROUNDS, samples, TEXTS, 0.0)
end

def pipelined_round(round)
  started = ms
  enqueue = ->(pipe) { distinct_texts(TEXTS, round).each { |t| pipe.call('EMB', MODEL, t) } }
  replies = new_client.pool.with { |conn| conn.pipelined(&enqueue) }
  replies.each { |r| r.unpack('e*') }
  ms - started
end

def scenario_threaded
  started = ms
  THREADS.times.map { |thread| worker(thread) }.each(&:join)
  Result.new(TEXTS * THREADS, drain_sample_queue, 1, ms - started)
end

def worker(thread)
  Thread.new do
    distinct_texts(TEXTS, thread).each do |t|
      timed_call { Emb::Proxy.new(new_client, MODEL)[t] }.then { SAMPLE_QUEUE << _1 }
    end
  end
end

SAMPLE_QUEUE = Queue.new

def drain_sample_queue
  samples = []
  samples << SAMPLE_QUEUE.pop until SAMPLE_QUEUE.empty?
  samples
end

COLS = %w[scenario embeds total(ms) per-embed req/s p50 p99 overhead].freeze

Result = Struct.new(:embeds, :samples, :per_sample, :total_ms)

def print_header
  puts COLS.join('  ')
end

def print_row(name, result, baseline)
  total = result.total_ms.positive? ? result.total_ms : result.samples.sum
  by_embed = result.samples.map { |sample| sample / result.per_sample }.sort
  puts row_columns(name, result, total, by_embed, baseline).join('  ')
end

def row_columns(name, result, total, by_embed, baseline)
  per_embed = total / result.embeds
  [
    name,
    result.embeds.to_s,
    format('%.1f', total),
    format('%.3f', per_embed),
    format('%.1f', result.embeds / (total / 1000.0)),
    format('%.3f', percentile(by_embed, 50)),
    format('%.3f', percentile(by_embed, 99)),
    format('%+.1f%%', overhead_pct(per_embed, baseline))
  ]
end

def overhead_pct(per_embed, baseline)
  (per_embed - baseline) / baseline * 100
end

def server_ready?
  Emb.setup(port: EMB_PORT).ping == 'PONG'
rescue StandardError
  false
end

def run_scenarios(baseline)
  print_header
  {
    eager: -> { scenario_eager },
    lazy: -> { scenario_lazy },
    pipelined: -> { scenario_pipelined },
    threaded: -> { scenario_threaded }
  }.each do |name, scenario|
    print_row(name.to_s, scenario.call, baseline)
  end
end

STABILITY_THRESHOLD = Float(ENV.fetch('EMB_BENCH_STABILITY_RATIO', 1.5))
STORM_THRESHOLD = Float(ENV.fetch('EMB_BENCH_STORM_RATIO', 1.75))
STABILITY_ROUNDS = Integer(ENV.fetch('EMB_BENCH_STABILITY_ROUNDS', 100))
STABILITY_BATCH  = Integer(ENV.fetch('EMB_BENCH_STABILITY_BATCH', 50))
LOAD_WORKERS     = Integer(ENV.fetch('EMB_BENCH_LOAD_WORKERS', 1))
LOAD_PAIRS       = Integer(ENV.fetch('EMB_BENCH_LOAD_PAIRS', 100))
STORM_WORKERS    = Integer(ENV.fetch('EMB_BENCH_STORM_WORKERS', 2))
STORM_PAIRS      = Integer(ENV.fetch('EMB_BENCH_STORM_PAIRS', 400))

# Inference p50/p99 of lazy batches, under a constant parse-heavy load and a request
# storm (many-arg EMB.MULTI commands with unknown models: heavy parse + goroutine
# churn, no inference), plus an idle reference. The storm mode reproduces the measured
# server fan-out fail — many workers x large EMB.MULTI pairs. Many short rounds keep
# p99 a real percentile, not the max.
def stability_scenario
  {
    idle: sample_inference,
    constant: with_parse_load(LOAD_WORKERS, LOAD_PAIRS) { sample_inference },
    storm: with_parse_load(STORM_WORKERS, STORM_PAIRS) { sample_inference }
  }
end

def sample_inference
  latencies = STABILITY_ROUNDS.times.map { |round| lazy_batch_ms(round) }.sort
  { p50: percentile(latencies, 50), p99: percentile(latencies, 99) }
end

def lazy_batch_ms(round)
  started = ms
  loaders = distinct_texts(STABILITY_BATCH, "stab-#{round}").map { |t| Emb::BatchProxy.new(new_client)[MODEL][t] }
  loaders.each(&:first)
  ms - started
end

def with_parse_load(workers, pairs)
  stop = [false]
  threads = workers.times.map { Thread.new { parse_load_worker(stop, pairs) } }
  result = yield
  stop[0] = true
  threads.each(&:join)
  result
end

def parse_load_worker(stop, pairs)
  client = new_client
  until stop[0]
    begin
      args = ['EMB.MULTI']
      pairs.times { |i| args.push('ghost', "noise #{i}") }
      args.push('ghost', 'x' * 10_000)
      client.send_command(*args)
    rescue StandardError
      nil
    end
  end
end

def main
  abort "emb server not reachable on :#{EMB_PORT} (start it via `just bench-ruby`)" unless server_ready?

  baseline = inference_baseline_ms
  puts format('inference baseline (warm single embed): %.3f ms', baseline)
  puts format('cpu partition: app(server)=%<app>s bench(tooling)=%<bench>s', app: APP_CPUS, bench: BENCH_CPUS)
  check_roundtrips
  puts 'round-trip check: eager = 5 EMB/0 MULTI, lazy = 1 MULTI/0 EMB ✓'
  puts ''

  run_scenarios(baseline)
  puts ''
  run_stability_gate if STABILITY_ENABLED
end

def run_stability_gate
  stability = stability_scenario
  constant_ratio = stability[:constant][:p99] / stability[:idle][:p99]
  storm_ratio = stability[:storm][:p99] / stability[:idle][:p99]
  ok = constant_ratio <= STABILITY_THRESHOLD && storm_ratio <= STORM_THRESHOLD
  puts stability_line(stability, constant_ratio, storm_ratio, ok ? 'PASS' : 'FAIL')
  abort 'stability gate failed' unless ok
end

def stability_line(stability, constant_ratio, storm_ratio, status)
  idle = stability[:idle]
  storm = stability[:storm]
  [
    "stability: idle p50=#{ms3(idle[:p50])} p99=#{ms3(idle[:p99])}",
    load_line('constant load', stability[:constant], constant_ratio, ms3(STABILITY_THRESHOLD)),
    load_line("storm (#{STORM_WORKERS}w x #{STORM_PAIRS}p)", storm, storm_ratio, ms3(STORM_THRESHOLD)),
    status
  ].join(' | ')
end

def load_line(label, data, ratio, threshold)
  "#{label} p50=#{ms3(data[:p50])} p99=#{ms3(data[:p99])} ratio=#{format('%.2f', ratio)} (threshold #{threshold})"
end

def ms3(value)
  format('%.3f', value)
end

main
