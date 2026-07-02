# Tasks: Instance-based Emb Client

- [x] Refactor client.rb into Emb::Client class
  - Move connection pool setup into `Emb::Client#initialize`
  - Implement URL resolution: `url:` arg → `host:`+`port:` → `ENV["EMB_URL"]` → default
  - Add `send_command`, `[]`, `models`, `info`, `stats`, `help`, `ping` instance methods
  - Keep `DEFAULTS` constant
  - **Files:** `lib/emb/client.rb`

- [x] Update emb.rb module-level API
  - Add `Emb.new(url:, host:, port:, pool:)` → returns `Emb::Client.new(...)`
  - Add `Emb.setup` that creates and stores a default client
  - Add lazy default client init with Mutex
  - Delegate module-level `[]`, `models`, `info`, `stats`, `help`, `ping`, `multi` to default client
  - Keep `config` as alias for `setup`
  - **Files:** `lib/emb.rb`

- [x] Update proxy.rb to accept client reference
  - `Proxy.new(client, name)` instead of `Proxy.new(name)`
  - `Proxy#[]` calls `@client.send_command` instead of `Emb.send_command`
  - **Files:** `lib/emb/proxy.rb`

- [x] Update multi.rb to accept client reference and unpack
  - `MultiProxy.new(client)` instead of `MultiProxy.new`
  - `MultiProxy#run` calls `@client.send_command` instead of `Emb.send_command`
  - `MultiProxy#run` maps results through `unpack("e*")`
  - Fix PairCollector bracket formatting
  - **Files:** `lib/emb/multi.rb`

- [x] Update specs for instance-based client and multi unpack
  - Add tests for `Emb.new` with URL, host/port, env var, defaults
  - Add tests for independent clients (separate pools, separate proxies)
  - Add tests for `client.models`, `client.info`, `client.ping`, `client.stats`, `client.help`
  - Add tests for `client.multi` unpacking floats
  - Update existing tests if needed (proxy memoization should still work)
  - **Files:** `spec/emb_spec.rb`, `spec/spec_helper.rb`
