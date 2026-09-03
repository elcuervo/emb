#!/usr/bin/env ruby
# frozen_string_literal: true

# Repro (post-fix regression guard): the batch-loader leak loop is DEAD.
#
# History: production showed sustained server CPU/network with no matching
# client success, dropping "back to normal" only when the web app restarted.
# Root cause (validated before this fix): the gem's embedding calls carried
# redis-client's silent 1.0s read timeout; a slower reply timed out and, since
# ReadTimeoutError is a ConnectionError subclass, redis-client auto-re-sent the
# whole EMB.MULTI up to reconnect_attempts+1 times (default 3 → 4× server
# work); then the error propagated into Emb::BATCH_BLOCK BEFORE batch-loader's
# delete(), so every deferred item stayed in the thread-local executor and any
# app retry re-forced the whole batch — a self-sustaining loop. An app restart
# only wiped the thread-local state, masking the loop.
#
# After secure-client-batch-defaults the gem: ships 10s read/write timeouts,
# reconnect_attempts 0 (no auto re-send), and a fail-closed BATCH_BLOCK
# (errors surface once; the pending set is cleared; retries are inert and
# resolve to []; later batches exclude the failed items).
#
# Run from gems/emb so bundler resolves the emb gem and its deps:
#
#   cd gems/emb && bundle exec ruby ../bench/repro/client-timeout/repro.rb
#
# Phases:
#   A: WORST-CASE config (1s timeout + 3 auto-resends) — the error surfaces
#      once, the resend amplifier is visible but CONTAINED, and fail-closed
#      keeps retries inert, the pending set empty, and stale items out of
#      later batches.
#   B: adequate timeout (10s) — one send, batch completes, no loop.
#
# Exits 0 when every guard holds, 1 otherwise. `MOCK_DELAY` (default 1.5)
# overrides the mock inference time; keep it > 1s for Phase A.

require 'open3'
require 'socket'
require 'json'
require 'tempfile'
require 'time'

require 'emb'
require 'batch_loader'
require 'redis_client'

REPRO_DIR = File.expand_path(__dir__)
MOCK = File.join(REPRO_DIR, 'mock_server.rb')

$failures = []

def check(cond, label, detail = nil)
  status = cond ? 'PASS' : 'FAIL'
  puts "  [#{status}] #{label}#{detail ? " (#{detail})" : ''}"
  $failures << label unless cond
  cond
end

def pending_count
  BatchLoader::Executor.current&.items_by_block&.values&.sum(&:size) || 0
end

def fresh_scope
  BatchLoader::Executor.clear_current
end

# ---- mock server harness ---------------------------------------------------

def spawn_mock
  log = Tempfile.new('emb-mock-log')
  out_r, out_w = IO.pipe
  pid = Process.spawn(
    { 'MOCK_LOG' => log.path,
      'MOCK_DELAY' => ENV.fetch('MOCK_DELAY', '1.5'),
      'MOCK_CLOSE' => '1' },
    RbConfig.ruby, MOCK, out: out_w, err: :out
  )
  out_w.close
  port = out_r.gets&.match(/MOCK_PORT=(\d+)/)&.captures&.first&.to_i
  abort 'mock server failed to start' unless port

  [pid, port, log.path, out_r]
end

def server_work(log_path)
  items = 0
  cmds = 0
  if File.exist?(log_path)
    File.foreach(log_path) do |line|
      next if line.strip.empty?

      row = JSON.parse(line)
      items += row['items']
      cmds += 1
    end
  end
  [items, cmds]
end

def kill_mock(pid, out_r)
  Process.kill('TERM', pid)
  Process.wait(pid)
  out_r.close
rescue Errno::ESRCH, Errno::ECHILD
  nil
end

# ---- loaders ----------------------------------------------------------------

def make_loaders(client, count, prefix)
  (1..count).map { |i| Emb.build_batch_loader(client, :minilm, "#{prefix}-#{i}") }
end

def force(l)
  # __send__ is a kept method on BatchLoader (they undef `send`), so this
  # dispatches straight to the private __sync (which triggers the batch) without
  # the method_missing → __sync! replacement path.
  l.__send__(:__sync)
end

def try_force(l)
  begin
    force(l)
    nil
  rescue StandardError => e
    e
  end
end

def client(port, read_timeout: nil, reconnect_attempts: nil)
  opts = { pool: 1, url: "redis://127.0.0.1:#{port}" }
  opts[:read_timeout] = read_timeout unless read_timeout.nil?
  opts[:reconnect_attempts] = reconnect_attempts unless reconnect_attempts.nil?
  Emb::Client.new(**opts)
end

# ---- the repro --------------------------------------------------------------

puts '=' * 76
puts 'emb gem batch leak-loop guard (fail-closed + secure defaults)'
puts '=' * 76

puts "\n[setup] gem defaults: read_timeout=#{Emb.configuration.read_timeout}s, " \
     "write_timeout=#{Emb.configuration.write_timeout}s, reconnect_attempts=#{Emb.configuration.reconnect_attempts}"
pid, port, mock_log, out_r = spawn_mock
puts "[setup] mock emb server on :#{port} (inference #{ENV.fetch('MOCK_DELAY', '1.5')}s per command)"

puts <<~PHASE_A

  ── Phase A: worst-case config (1s timeout + 3 auto-resends) is CONTAINED ──
PHASE_A

fresh_scope
# The pre-fix combination that caused the leak: 1s timeout + auto re-sends.
c_a = client(port, read_timeout: 1, reconnect_attempts: 3)
items0, cmds0 = server_work(mock_log)

loaders = make_loaders(c_a, 6, 'A')
puts "  built 6 loaders, forced none (pending=#{pending_count})"

err = try_force(loaders.first)
items1, cmds1 = server_work(mock_log)
check(!err.nil?, 'the batch still errors under the 1s timeout (with auto-resends)', err.class.name)
check(cmds1 - cmds0 >= 2, 'resend amplifier still exists at the redis-client layer (pre-fix behavior we no longer default to)', "#{cmds0}→#{cmds1} commands")
check(pending_count.zero?, 'FAIL-CLOSED: pending items cleared despite the error — the retention loop cannot form', "pending=#{pending_count}")

# The app retries the failed batch (the old re-run storm driver): must be inert.
3.times do |i|
  try_force(loaders.first)
  sleep 0.1
end
items2, cmds2 = server_work(mock_log)
check(items2 == items1 && cmds2 == cmds1, 'retries send NOTHING — no re-run storm', "server flat at #{items2} items / #{cmds2} cmds")
values = loaders.map { |l| begin; force(l); rescue StandardError => e; e; end }
check(values.all? { |v| v == [] }, 'failed items resolve to [] (no error, no network)', values.inspect)

# Stale-exclusion: a fresh batch in the SAME scope must carry ONLY its own
# items. Under the vulnerable client (auto-resends) each send is retried 4×,
# so assert content, not count: every command in this window holds exactly the
# 2 new pairs — never a 6-item stale re-send.
fresh = make_loaders(c_a, 2, 'A2')
items_before2, cmds_before2 = items2, cmds2
err2 = try_force(fresh.first)
items3, cmds3 = server_work(mock_log)
d_cmds = cmds3 - cmds_before2
d_items = items3 - items_before2
check(d_cmds >= 1 && d_items == 2 * d_cmds, 'subsequent batch(es) carry ONLY the 2 new items each — stale batch excluded', "#{items_before2}→#{items3} items, #{cmds_before2}→#{cmds3} cmds")
check(!err2.nil?, 'that batch fails again under the 1s timeout (expected)', err2.class.name)
check(pending_count.zero?, 'pending stays empty after repeated failures', "pending=#{pending_count}")

puts <<~PHASE_B

  ── Phase B: adequate timeout (10s, the new default) — one send, no loop ──
PHASE_B

fresh_scope
c_b = client(port, read_timeout: 10)
items_b0, cmds_b0 = server_work(mock_log)
loaders_b = make_loaders(c_b, 6, 'B')
err_b = try_force(loaders_b.first)
items_b1, cmds_b1 = server_work(mock_log)
check(err_b.nil?, 'batch completes without error under the 10s default timeout')
check(cmds_b1 - cmds_b0 == 1, 'server received the batch exactly ONCE (no resends)', "#{cmds_b0}→#{cmds_b1} commands")
check(items_b1 - items_b0 == 6, 'server processed the 6 items exactly once', "#{items_b0}→#{items_b1} items")
check(pending_count.zero?, 'pending items cleared after a successful batch', "pending=#{pending_count}")
try_force(loaders_b.first)
items_b2, = server_work(mock_log)
check(items_b2 == items_b1, 're-forcing is a memoized hit — no new network traffic', "server flat at #{items_b2}")
check(loaders_b.all? { |l| try_force(l).nil? }, 'all 6 loaders resolved')

kill_mock(pid, out_r)

puts "\n" + '=' * 76
if $failures.empty?
  puts 'GUARD HOLDS: the batch leak loop is dead under every configuration.'
  puts '  • worst-case 1s timeout + 3 auto-resends: error surfaces once, resend'
  puts '    amplifier contained, pending cleared, retries inert, [] resolution'
  puts '  • stale failed items excluded from later batches; pending stays 0'
  puts '  • 10s default: one send, exact completion, memoized re-use'
else
  puts "GUARD BROKEN: #{$failures.length} check(s) failed:"
  $failures.each { |f| puts "  • #{f}" }
end
puts '=' * 76
exit($failures.empty? ? 0 : 1)