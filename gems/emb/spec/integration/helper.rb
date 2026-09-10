# frozen_string_literal: true

require 'json'
require 'open3'
require 'rbconfig'

require_relative '../../lib/emb'

# Orchestrates the real-Rails boot probe. Deliberately does NOT load the
# server-backed `spec_helper`: these examples never talk to an emb server, so
# `rake spec:rails` can run them without one.
module RailsBoot
  PROBE = File.expand_path('boot_probe.rb', __dir__)

  module_function

  # Boots the dummy app in a fresh subprocess and returns the parsed fact sheet.
  # A new process per boot is required because a Rails application can only be
  # initialized once, and the fake-based railtie spec rewrites Emb::Railtie.
  def run(opt_out: false, manual_mount: false, request: nil)
    env = { 'RAILS_ENV' => 'test' }
    env['EMB_OPT_OUT'] = '1' if opt_out
    env['EMB_MANUAL_MOUNT'] = '1' if manual_mount
    env['EMB_REQUEST'] = request.to_s if request

    stdout, stderr, status = Open3.capture3(env, RbConfig.ruby, PROBE)
    raise "Rails boot probe failed (exit #{status.exitstatus}):\n#{stderr}\n#{stdout}" unless status.success?

    JSON.parse(stdout.lines.last)
  end
end
