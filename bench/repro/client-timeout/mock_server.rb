#!/usr/bin/env ruby
# frozen_string_literal: true

# Minimal RESP2 mock of the emb server for the client-timeout leak repro.
#
# Behavior (all tuned via env):
#   MOCK_DELAY   seconds each EMB/EMB.MULTI "inference" takes (default 1.5)
#   MOCK_CPU=1   busy-loop instead of sleep — burns a real core so `top`
#                shows the server doing CPU work the client has abandoned
#   MOCK_LOG     path to append one JSON line per command (the repro reads
#                these to count server-side work independently of the client)
#   MOCK_CLOSE=1 close the client connection right after each reply, so every
#                client call reconnects cleanly (deterministic re-sends)
#
# Replies: EMB/EMB.MULTI -> RESP array of N bulks, each 16 bytes (4 float32).
# The emb gem unpacks them with `unpack("e*")`.

require 'socket'
require 'json'

DELAY  = Float(ENV.fetch('MOCK_DELAY', '1.5'))
BUSY   = ENV['MOCK_CPU'] == '1'
LOG    = ENV['MOCK_LOG']
CLOSE  = ENV['MOCK_CLOSE'] == '1'

server = TCPServer.new('127.0.0.1', 0)
puts "MOCK_PORT=#{server.addr[1]}"
$stdout.flush

log = ->(cmd:, items:) do
  File.open(LOG, 'a') do |f|
    f.puts(JSON.generate(
             t: Process.clock_gettime(Process::CLOCK_MONOTONIC).round(3),
             cmd: cmd, items: items
           ))
  end
end if LOG

# Busy-work sleeps to make CPU observable; otherwise the sleep keeps the
# repro fast and host-friendly.
def inference_wait(seconds)
  if BUSY
    deadline = Process.clock_gettime(Process::CLOCK_MONOTONIC) + seconds
    spin = 0
    loop do
      spin += 1
      break if Process.clock_gettime(Process::CLOCK_MONOTONIC) >= deadline
    end
  else
    sleep(seconds)
  end
end

# Read one RESP array command (all bulk arguments) from the socket, or nil on
# EOF/protocol error.
def read_command(io)
  line = io.gets
  return nil if line.nil?
  return nil unless line.start_with?('*')

  count = Integer(line[1..])
  args = []
  count.times do
    bl = io.gets
    return nil if bl.nil? || !bl.start_with?('$')

    len = Integer(bl[1..])
    payload = io.read(len)
    io.read(2) # trailing \r\n
    args << payload
  end
  args
end

BLOB = [0.25, 0.5, 0.75, 1.0].pack('e*')

loop do
  client = server.accept
  Thread.new(client) do |c|
    loop do
      args = read_command(c)
      break if args.nil?

      cmd = (args[0] || '').upcase
      case cmd
      when 'PING', 'EMB.READY'
        c.write("+PONG\r\n")
      when 'EMB', 'EMB.MULTI'
        n = cmd == 'EMB.MULTI' ? (args.length - 1) / 2 : args.length - 2
        n = 0 if n < 0
        log.call(cmd: cmd, items: n)
        inference_wait(DELAY)
        c.write("*#{n}\r\n")
        n.times { c.write("$16\r\n#{BLOB}\r\n") }
      else
        c.write("-ERR unknown command #{cmd}\r\n")
      end
      c.flush
      break if CLOSE # server closes after each reply
    end
  rescue Errno::EPIPE, Errno::ECONNRESET, IOError, SystemCallError
    # client went away mid-inference — fine, we already logged the work
  ensure
    c.close unless c.closed?
  end
end