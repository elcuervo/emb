#!/usr/bin/env ruby
# frozen_string_literal: true

# Pool head-of-line-blocking gate.
#
# Drives the real emb client through the shared repro mock
# (bench/repro/client-timeout/mock_server.rb), which answers a MOCK_SLOW_P
# fraction of commands slowly, from more threads than the pool has connections,
# and asserts the p90 stays near the fast path. A pool that blocks a waiting
# command on a specific busy connection while another is free turns the slow
# replies into p90 waits and fails the gate. The deterministic RSpec guard
# (gems/emb/spec/emb/round_robin_pool_spec.rb) independently pins the selection
# behavior.
#
# Run from gems/emb so bundler resolves the gem:
#
#   cd gems/emb && bundle exec ruby ../../bench/repro/pool-hol/gate.rb
#
# Env: MOCK_BASE, MOCK_SLOW, MOCK_SLOW_P, POOL, THREADS, ITERS, MAX_P90_MS.

require 'emb'

MOCK    = File.expand_path('../client-timeout/mock_server.rb', __dir__)
POOL    = Integer(ENV.fetch('POOL', 5))
THREADS = Integer(ENV.fetch('THREADS', 10))
ITERS   = Integer(ENV.fetch('ITERS', 300))
MAX_P90 = Float(ENV.fetch('MAX_P90_MS', 50.0))

def spawn_mock
  out_r, out_w = IO.pipe
  pid = Process.spawn(
    { 'MOCK_BASE' => ENV.fetch('MOCK_BASE', '0.005'),
      'MOCK_SLOW' => ENV.fetch('MOCK_SLOW', '0.15'),
      'MOCK_SLOW_P' => ENV.fetch('MOCK_SLOW_P', '0.05') },
    RbConfig.ruby, MOCK, out: out_w, err: :out
  )
  out_w.close
  port = out_r.gets&.match(/MOCK_PORT=(\d+)/)&.captures&.first&.to_i
  abort 'mock server failed to start' unless port

  [pid, port]
end

def percentile(sorted, percent)
  sorted[((percent / 100.0) * (sorted.size - 1)).round]
end

pid, port = spawn_mock
client = Emb::Client.new(url: "redis://127.0.0.1:#{port}", pool: POOL, lazy: false)
begin
  latencies = Queue.new
  started = Process.clock_gettime(Process::CLOCK_MONOTONIC)
  THREADS.times.map do |t|
    Thread.new do
      ITERS.times do |i|
        t0 = Process.clock_gettime(Process::CLOCK_MONOTONIC)
        client[:minilm]["gate text #{t}-#{i}"]
        latencies << (Process.clock_gettime(Process::CLOCK_MONOTONIC) - t0) * 1000
      end
    end
  end.each(&:join)
  wall = Process.clock_gettime(Process::CLOCK_MONOTONIC) - started

  samples = []
  samples << latencies.pop until latencies.empty?
  sorted = samples.sort
  p50 = percentile(sorted, 50)
  p90 = percentile(sorted, 90)
  p99 = percentile(sorted, 99)

  puts format('pool=%d threads=%d calls=%d slow=%.3fs@%.0f%% wall=%.2fs',
              POOL, THREADS, samples.size, Float(ENV.fetch('MOCK_SLOW', '0.15')),
              Float(ENV.fetch('MOCK_SLOW_P', '0.05')) * 100, wall)
  puts format('p50=%.2fms  p90=%.2fms  p99=%.2fms  (threshold p90 <= %.2fms)',
              p50, p90, p99, MAX_P90)

  if p90 <= MAX_P90
    puts 'PASS: waiting commands were served from freed connections (no head-of-line blocking)'
  else
    puts 'FAIL: p90 reached the slow-reply path — a waiting command blocked behind a busy connection'
    exit 1
  end
ensure
  Process.kill('TERM', pid)
  Process.wait(pid)
end
