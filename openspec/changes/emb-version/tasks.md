## 1. Server commands

- [ ] 1.1 Add `Version` field + `SetVersion(string)` to `Server`
- [ ] 1.2 Add `handleVERSION` (bulk string, arity error) + register `emb.version` in the mux
- [ ] 1.3 Add `handleINFO` (`INFO [server]` → `# Server`/`redis_version:` bulk string) + register `info`; exempt `info` too
- [ ] 1.4 Add `emb.version` (and `info`) to `isExempt`
- [ ] 1.5 Document `EMB.VERSION` and `INFO` in `handleHELP`

## 2. Wiring & client

- [ ] 2.1 `cmd/emb/main.go`: `srv.SetVersion(version)` before serving
- [ ] 2.2 Ruby gem: `Client#version` + module `Emb.version`; README note

## 3. Tests (server + gem)

- [ ] 3.1 Server: default `dev`; set version echoed; arity error; pre-auth on password server; `INFO server` returns `redis_version:`; HELP lists both
- [ ] 3.2 Gem spec: `client.version == server build version`

## 4. Validation stage (nix develop)

- [ ] 4.1 `go test ./...`, `just lint`, `bundle exec rspec` all pass
- [ ] 4.2 Via redis-cli: `EMB.VERSION` returns `0.2.3` on a `just build` binary
- [ ] 4.3 OpenSpec change validates