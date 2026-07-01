## Why

`emb` speaks Redis protocol but has no first-party Ruby client. The only way to use it from Ruby is raw `redis-client` calls with manual RESP command construction. A thin gem that mirrors the server's command set makes `emb` feel native in Ruby projects.

## What Changes

- Create `gem/` directory with a standard Ruby gem structure (gemspec, Gemfile, lib/)
- Implement `Emb[:model]["text"]` syntax with a memoized Proxy registry
- Implement `Emb.models`, `Emb.info`, `Emb.stats`, `Emb.help`, `Emb.ping` for all server commands
- Implement `Emb.multi { |e| e[:a]["t1"]; e[:b]["t2"] }` for batch multi-model queries
- Use `redis-client` gem with `connection_pool` for connection reuse
- Update `flake.nix` devShell with `ruby_3_4`, `bundler` for development
- Wire version via ldflags in the build process

## Capabilities

### New Capabilities
- `emb-ruby-client`: Ruby gem providing a native Ruby API for all `emb` commands with connection pooling and a simple DSL (`Emb[:model]["text"]`)

### Modified Capabilities
- (none)

## Impact

| File | Change |
|------|--------|
| `gem/` | **Added** — full Ruby gem directory |
| `flake.nix` | Add `ruby_3_4`, `bundler` to devShell |
