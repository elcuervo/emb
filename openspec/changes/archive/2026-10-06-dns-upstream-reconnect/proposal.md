# Proposal

## Why

The zone holds one RESP connection to the emb server beside it. emb closes a
connection that has been idle for `idle_timeout` (15 minutes by default), and the
zone's client nils its connection the moment a flush or a read fails, so that one
close is permanent: every later query fails with `upstream: resp: not connected`
until the machine is restarted. Measured in production on 2026-10-06 — the
machine booted at 16:49, the connection was reaped at 17:12 after a quiet spell,
and the zone answered nothing from then on. The gallery's plate showed the
failure as `unreachable — upstream: resp: not connected`.

The zone also serves queries concurrently, and it shares that one connection: a
RESP exchange is a write, a flush, and a read on a socket, so two callers on it
can interleave their replies.

## What Changes

- The zone redials when its connection to emb is lost, and retries the query
  once. A connection lost to an idle close, or to an emb restart, no longer ends
  the zone's service.
- A redial reloads the composition preset, because the server that answers the
  new connection may be a restarted one that has forgotten it.
- Only a failure of the connection itself is retried. A reply the server sent —
  a refusal, a bad model — is returned as it is.
- One caller at a time uses the connection, so concurrent queries cannot
  interleave on one socket.

## Capabilities

### New Capabilities

None.

### Modified Capabilities

- `dns-service`: the zone recovers a lost upstream connection, and serializes its
  upstream calls.

## Impact

- `cmd/emb-dns/upstream.go`: a transport sentinel, `reconnect`, an `exchange`
  helper, a mutex, and the retry in `Compose` and `Embed`.
- `cmd/emb-dns/upstream_test.go`: a fake emb that drops connections the way emb's
  idle timeout does.
- `docs/dns.md`: the recovery, beside the zone's other operational behavior.
