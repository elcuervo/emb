# frozen_string_literal: true

# Lazy-execution benchmark. For each scope shape it reports, per lazy mode:
#   - wall time of one forced scope (median of ROUNDS, modes interleaved)
#   - where the commands landed across the configured instances, and how many
#     were in flight at once
#
#   cd gems/emb && EMB_LAZY_PORTS=16379,16380,16381 bundle exec ruby bench/lazy_modes.rb
#
# Instances: EMB_LAZY_PORTS (comma-separated, default 16379), served by
# test-two-models.yaml (minilm + bge). Texts are unique per round, so a server
# reply cache never short-circuits inference.
#
# Modes:
#   eager  lazy: false  — one command per call, immediately
#   multi  lazy: :multi — deferred, coalesced into EMB / EMB.MULTI, serial
#   batch  lazy: :batch — deferred, one plain EMB per model + one share per
#                         script call, dispatched concurrently
#
# The distribution column patches Emb::ConnectionRouter#perform_command, the
# one place every wire send passes through, to record (node, start, end).

require 'emb'

PORTS = (ENV['EMB_LAZY_PORTS'] || '16379').split(',').map(&:to_i).freeze
ROUNDS = Integer(ENV.fetch('EMB_LAZY_ROUNDS', 15))
POOL = 8
BAR = 26

SAME_TEXTS = 8
PER_MODEL = 6
SCRIPTS = 6
MIXED_EMBEDS = 6
MIXED_SCRIPTS = 4

MODEL = :minilm
MODEL2 = :bge
# One inference inside the script, so a script call costs about an embed and
# concurrency (not trivial round-trips) is what the numbers show.
SCRIPT = 'local v = emb.embed({KEYS[1]}, {bytes = true}); return #v'

MODES = { eager: false, multi: :multi, batch: :batch }.freeze

SCENARIOS = {
  same: "same model (#{SAME_TEXTS} minilm texts)",
  multi_model: "multi model (#{PER_MODEL} minilm + #{PER_MODEL} bge)",
  scripts: "scripts (#{SCRIPTS} EMB.EVSHA)",
  mixed: "mixed (#{MIXED_EMBEDS} embeds + #{MIXED_SCRIPTS} scripts)"
}.freeze

module RouteLog
  MUTEX = Mutex.new

  class << self
    def record(node:, cmd:, started:, finished:)
      MUTEX.synchronize { entries << { node: node, cmd: cmd, t0: started, t1: finished } }
    end

    def reset
      MUTEX.synchronize { @entries = [] }
    end

    def entries
      @entries ||= []
    end
  end

  def perform_command(pool, args)
    node = @pools.index(pool)
    t0 = Process.clock_gettime(Process::CLOCK_MONOTONIC)
    result = super
    t1 = Process.clock_gettime(Process::CLOCK_MONOTONIC)
    RouteLog.record(node: node, cmd: args.first, started: t0, finished: t1)
    result
  end
end
Emb::ConnectionRouter.prepend(RouteLog)

def ms = Process.clock_gettime(Process::CLOCK_MONOTONIC) * 1000

def median(values)
  sorted = values.sort
  mid = sorted.size / 2
  sorted.size.odd? ? sorted[mid] : (sorted[mid - 1] + sorted[mid]) / 2.0
end

def new_client(mode)
  Emb::Client.new(url: PORTS.map { |p| "redis://localhost:#{p}" }, pool: POOL, lazy: mode, batch_size: 512)
end

def embeds(client, model, count, prefix)
  return (0...count).map { |i| Emb::Proxy.new(client, model)["#{prefix}#{i}"] } unless client.lazy?

  (0...count).map { |i| Emb.build_batch_loader(client, model, "#{prefix}#{i}") }
end

def scripts(client, sha, count, nonce = '')
  (0...count).map { |i| client.evalsha(MODEL, sha, ["#{nonce}script#{i}"]) }
end

def force(values) = values.each { |v| v.__sync if v.respond_to?(:__sync) }

def scope_values(client, sha, kind, nonce)
  case kind
  when :same
    embeds(client, MODEL, SAME_TEXTS, "#{nonce}same")
  when :multi_model
    embeds(client, MODEL, PER_MODEL, "#{nonce}minilm") + embeds(client, MODEL2, PER_MODEL, "#{nonce}bge")
  when :scripts
    scripts(client, sha, SCRIPTS, nonce)
  when :mixed
    embeds(client, MODEL, MIXED_EMBEDS, "#{nonce}mix") + scripts(client, sha, MIXED_SCRIPTS, nonce)
  end
end

# Builds and forces one scope; returns the routing log for that flush.
def run_scope(client, sha, kind, nonce = '')
  RouteLog.reset
  Emb::BatchScope.wrap { force(scope_values(client, sha, kind, nonce)) }
  RouteLog.entries.dup
end

def time_rounds(clients, sha, kind)
  samples = MODES.to_h { |name, _| [name, []] }
  ROUNDS.times do |round|
    MODES.each_key do |name|
      t0 = ms
      run_scope(clients[name], sha, kind, "t#{round}-#{name}")
      samples[name] << (ms - t0)
    end
  end
  samples
end

# Wall time per mode, interleaved round by round (same server state) so drift
# does not decide the winner.
def times_by_mode(sha, kind)
  clients = MODES.to_h { |name, lazy| [name, new_client(lazy)] }
  clients.each_value { |client| run_scope(client, sha, kind, 'warm') }
  time_rounds(clients, sha, kind).transform_values { |values| { median: median(values), min: values.min } }
end

def max_in_flight(entries)
  running = 0
  best = 0
  entries.flat_map { |e| [[e[:t0], 1], [e[:t1], -1]] }.sort_by(&:first).each do |_, delta|
    running += delta
    best = running if running > best
  end
  best
end

def tally(entries)
  counts = Array.new(PORTS.size, 0)
  entries.each { |e| counts[e[:node]] += 1 }
  counts
end

def bar(value, max)
  filled = max.zero? ? 0 : ((value.to_f / max) * BAR).round
  ('█' * filled) + ('·' * (BAR - filled))
end

def describe(entries)
  entries.map { |e| e[:cmd] }.tally.map { |cmd, n| n == 1 ? cmd : "#{cmd}×#{n}" }.join(' ')
end

def node_bars(entries, max_count)
  tally(entries).each_with_index.map { |count, node| "n#{node} #{bar(count, max_count)} #{count}" }.join('  ')
end

def print_mode_row(name, time, entries, max_time, max_count)
  puts format('  %<name>-6s %<bar>s %<median>6.2fms best %<min>5.2f  |  %<nodes>s  in-flight %<flight>-2d [%<cmds>s]',
              name: name, bar: bar(time[:median], max_time), median: time[:median], min: time[:min],
              nodes: node_bars(entries, max_count), flight: max_in_flight(entries), cmds: describe(entries))
end

def winner_line(times)
  fastest = times.min_by { |_, time| time[:median] }.first
  slowest = times.max_by { |_, time| time[:median] }.first
  format('  → %<fastest>s fastest, %<ratio>.2fx vs %<slowest>s',
         fastest: fastest, ratio: times[slowest][:median] / times[fastest][:median], slowest: slowest)
end

def print_scenario(kind, label, sha)
  puts "■ #{label}"
  times = times_by_mode(sha, kind)
  entries = MODES.to_h { |name, lazy| [name, run_scope(new_client(lazy), sha, kind, 'dist')] }
  print_rows(times, entries)
  puts winner_line(times)
  puts
end

def print_rows(times, entries)
  max_time = times.values.map { |time| time[:median] }.max
  max_count = entries.values.flat_map { |entry| tally(entry) }.max || 1
  MODES.each_key { |name| print_mode_row(name, times[name], entries[name], max_time, max_count) }
end

def ready?(port)
  Emb::Client.new(port: port, lazy: false).ping == 'PONG'
rescue StandardError
  false
end

def header(sha)
  format("emb lazy-mode benchmark  ·  %<nodes>s  ·  pool %<pool>d  ·  median of %<rounds>d rounds\n" \
         'script sha %<sha>s… (each call runs emb.embed → one inference)',
         nodes: PORTS.map { |p| ":#{p}" }.join(' + '), pool: POOL, rounds: ROUNDS, sha: sha[0, 12])
end

def load_script
  PORTS.map { |port| Emb::Client.new(port: port, lazy: false).script.load(MODEL, SCRIPT) }.uniq.first
end

def main
  PORTS.each { |port| abort "no emb on :#{port}" unless ready?(port) }
  sha = load_script

  puts header(sha)
  puts
  SCENARIOS.each { |kind, label| print_scenario(kind, label, sha) }
  puts 'legend: time bars scale to the slowest mode; node bars scale to the busiest node;'
  puts '        in-flight = max commands served at once. Shares = models + script calls, not texts.'
end

main
