## 1. Ruby gem: forward redis-client options

- [x] 1.1 Refactor `Client#initialize` to accept `**rest`, merge defaults
- [x] 1.2 Forward `**rest` through `Emb.setup` / `Emb.config` / `Emb.new` module methods (already uses `(...)` forwarding)
- [x] 1.3 Update `gems/emb/README.md` with examples of forwarded options (timeouts, SSL, driver, inherit_socket)
- [x] 1.4 Add tests for forwarded options (custom connect_timeout, read_timeout, reconnect_attempts, write_timeout)
- [x] 1.5 Run `bundle exec rubocop` — no new offenses (only pre-existing)
- [x] 1.6 Run `bundle exec rspec` — all tests pass

## 2. Go server: TLS support

- [x] 2.1 Add `TLSCert`/`TLSKey` fields to `Config`, validate mutual requirement
- [x] 2.2 Add `-tls-cert` / `-tls-key` flags to `ParseFlags`
- [x] 2.3 Load `tls.Config` in `cmd/emb/main.go` and pass to `server.New`
- [x] 2.4 Accept `*tls.Config` in `server.New`, store on `Server`
- [x] 2.5 Swap `ListenAndServe` to use `tls.Listen` when config present
- [x] 2.6 Add TLS edge case tests (no cert, no key, bad paths, plain TCP fallback)
- [x] 2.7 Update root `README.md` — add `tls_cert`/`tls_key` to config example
- [x] 2.8 Update `config.yaml` — add commented-out `tls_cert`/`tls_key` fields

## 3. Verify

- [x] 3.1 Run Go tests: `go test -race ./...`
- [x] 3.2 Run `golangci-lint run` — 0 issues
- [x] 3.3 Run Ruby tests: `bundle exec rspec` from `gems/emb/`
- [x] 3.4 Run `bundle exec rubocop` from `gems/emb/` — no new offenses
- [x] 3.5 Manual TLS smoke test: automated via `TestTLSConnection` (generates cert at runtime, connects via tls.Dial, sends PING)
