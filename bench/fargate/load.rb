#!/usr/bin/env ruby
# frozen_string_literal: true

# Pure-stdlib mixed-length RESP driver for the Fargate benchmark harness.
#
# Reads a corpus file (one text per line, generated deterministically by main.go),
# embeds each text with `EMB <model> <text>` over raw RESP, and writes one latency
# (ms) per request to the latencies file. Concurrency = --clients (one socket each).
# No external gems; runs from the `nix develop` shell (ruby 3.x).

require 'socket'
require 'optparse'

args = {
  port: 16_379, clients: 4, count: 400, model: 'minilm',
  corpus: nil, latencies: nil
}

OptionParser.new do |o|
  o.on('--port N', Integer) { |v| args[:port] = v }
  o.on('--clients N', Integer) { |v| args[:clients] = v }
  o.on('--count N', Integer) { |v| args[:count] = v }
  o.on('--model M') { |v| args[:model] = v }
  o.on('--corpus FILE') { |v| args[:corpus] = v }
  o.on('--latencies FILE') { |v| args[:latencies] = v }
end.parse!

texts = File.readlines(args[:corpus], chomp: true)
abort "corpus smaller than count (#{texts.size} < #{args[:count]})" if texts.size < args[:count]
texts = texts.first(args[:count]).freeze
model = args[:model]
expected = Integer(ENV.fetch('EMB_BENCH_EXPECT', '0'))
abort "corpus/expected mismatch" if expected.positive? && expected > args[:count]

def frame(model, text)
  "*3\r\n$3\r\nEMB\r\n$#{model.bytesize}\r\n#{model}\r\n$#{text.bytesize}\r\n#{text}\r\n"
end

def read_bulk(sock)
  line = sock.gets
  raise "unexpected reply #{line.inspect}" unless line&.start_with?('$')

  len = line[1..].to_i
  sock.read(len + 2)
end

start = Process.clock_gettime(Process::CLOCK_MONOTONIC)
latencies = Queue.new
errors = Queue.new

threads = args[:clients].times.map do |ci|
  Thread.new do
    begin
      sock = Socket.tcp('127.0.0.1', args[:port], connect_timeout: 10)
      chunk = (texts.size.fdiv(args[:clients])).ceil
      texts[ci * chunk, chunk].to_a.each do |t|
        t0 = Process.clock_gettime(Process::CLOCK_MONOTONIC)
        sock.write(frame(model, t))
        read_bulk(sock)
        latencies << (Process.clock_gettime(Process::CLOCK_MONOTONIC) - t0) * 1000.0
      end
      sock.close
    rescue StandardError => e
      errors << "#{ci}: #{e.class}: #{e.message}"
    end
  end
end
threads.each(&:join)
wall = Process.clock_gettime(Process::CLOCK_MONOTONIC) - start

abort "load.rb errors: #{errors.pop.inspect}" unless errors.empty?

File.open(args[:latencies], 'w') { |f| f.puts(latencies.pop) until latencies.empty? }
puts format('mixed clients=%d count=%d wall_ms=%.1f', args[:clients], texts.size, wall * 1000)