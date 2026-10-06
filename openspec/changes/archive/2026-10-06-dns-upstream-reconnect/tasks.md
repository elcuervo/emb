# Tasks

## 1. Recover a lost connection

- [x] 1.1 Mark a failure of the connection itself in `cmd/emb-dns/upstream.go` (`errTransport`), so it can be told from a reply the server sent; verify `go test ./cmd/emb-dns/ -count=1` covers a reply error that is returned unchanged
- [x] 1.2 Dial again and retry once when a call fails that way, in `Compose` and in `Embed`; verify a test with a fake emb that drops its connections recovers and answers, and that a healthy call afterwards opens no new connection
- [x] 1.3 Load the composition preset on the redialled connection; verify the recovered query composes through the preset the fake server was asked to load

## 2. One caller at a time

- [x] 2.1 Serialize the upstream calls with a mutex; verify a test runs eight concurrent `Compose` calls, all answered, over one connection

## 3. Prove it against a real emb

- [x] 3.1 Reproduce the production failure locally: run the zone's own emb with `idle_timeout: 5s` and the shipped zone config, and confirm the stale binary fails with `upstream: EOF` after the close
- [x] 3.2 Confirm the fixed binary answers the same query after the idle close, and that a conjunction still answers with its legs; verify the zone log shows no upstream failure for the recovered query

## 4. Document and integrate

- [x] 4.1 Document the recovery in `docs/dns.md` beside the zone's other operational behavior
- [x] 4.2 `just test`, `just lint` and `just verify-harness` pass
- [x] 4.3 `openspec validate dns-upstream-reconnect --strict` passes
