# Production Hardening

## Why

Two gaps surfaced during exploration. The Ruby client hardcodes a few redis-client options
and swallows the rest — users can't configure timeouts, SSL, reconnect backoff, or custom
drivers without patching the gem. On the server side, emb speaks plain TCP with no TLS
option — anyone deploying across a network boundary either accepts plaintext or adds a
proxy (stunnel, Envoy, nginx) to terminate TLS. Both are paper cuts that push users
toward operational complexity for straightforward needs.

## What Changes

### emb Ruby gem: forward redis-client options

The `Client` constructor currently accepts `url:`, `host:`, `port:`, and `pool:` as
named kwargs. All other `RedisClient` options (`connect_timeout`, `read_timeout`,
`ssl`, `reconnect_attempts`, `driver`, `inherit_socket`, etc.) are inaccessible.

Change the constructor to accept `**rest` and merge defaults with user-provided values.
Only `pool:` remains a gem-level concern; everything else passes through to redis-client.

### emb Go server: TLS support

Add optional `tls_cert` and `tls_key` fields to the YAML config and corresponding
`-tls-cert` / `-tls-key` CLI flags. When both are provided, the server wraps the
listener with `tls.Listen` instead of `net.Listen`. Redcon's `Serve(net.Listener)`
accepts either — no library changes needed.

When neither is set, behavior is identical to today.

## Capabilities

### New Capabilities

- `server-tls`: TLS support for the emb server via cert/key paths in config or flags
- `gem-redis-client-config`: Forward arbitrary redis-client options through the gem

### Modified Capabilities

(none)

## Impact

**Code changes:**

- `gems/emb/lib/emb/client.rb` — replace named kwargs with `**rest`, merge defaults
- `gems/emb/lib/emb.rb` — forward `**rest` through `setup`/`config` module methods
- `gems/emb/spec/emb_spec.rb` — add tests for forwarded options
- `internal/config/config.go` — add `TLSCert`/`TLSKey` fields to `Config`, `-tls-cert`/`-tls-key` flags
- `internal/server/server.go` — accept `*tls.Config` in `New`, swap listener in `ListenAndServe`
- `internal/server/server_test.go` — test TLS edge cases
- `cmd/emb/main.go` — load cert/key and pass TLS config to `server.New`

**Documentation:**

- `README.md` — add `tls_cert`/`tls_key` to the config YAML example in the Configuration section
- `gems/emb/README.md` — document forwarded redis-client options with examples (timeouts, SSL, driver, inherit_socket)
- `config.yaml` — add commented-out `tls_cert`/`tls_key` fields
