# Design

## Context

See proposal.md — Why. The pieces that decide the shape:

- `internal/resp.Client` has no mutex, and `Flush` and `ReadReply` call
  `Close()` on error, which nils `conn`. So one failure of the connection turns
  every later `WriteArgv` into `resp: not connected` — the error the plate
  relayed. `Dial` closes any previous connection, so a redial needs no cleanup.
- emb's `idle_timeout` defaults to 15m (`config.yaml` documents it; `dns/emb.yaml`
  leaves it alone). The zone is idle whenever the zone is quiet, so the default
  guarantees this happens on a quiet deployment.
- `Compose` already retries once for a `NOSCRIPT` reply, because an emb restart
  loses the preloaded preset. The zone therefore already expected a restart, and
  simply had no path for the connection a restart or an idle close takes away.
- The zone serves HTTP and DNS concurrently, and both paths call `Compose` or
  `Embed` on the one `Upstream`.

## Goals / Non-Goals

**Goals:**
- A lost connection is recovered inside the query that notices it, so no caller
  sees the failure.
- A reply the server sent is never mistaken for a lost connection.

**Non-Goals:**
- Changing emb's `idle_timeout`. The zone must survive an emb restart too, so a
  client that cannot redial is the wrong thing to build regardless of the
  timeout.
- Keeping the connection warm with a ping loop. That is a second goroutine and a
  timer to avoid a redial that costs milliseconds.

## Decisions

### D1. Retry only a failure of the connection, marked by a sentinel

`exchange` wraps write, flush, and read failures with `errTransport`
(`upstream: connection lost`), and `SetDeadline`'s failure too, since it fails
when the client has no connection. `Compose` and `Embed` retry once when
`errors.Is(err, errTransport)`, and return anything else unchanged. A server
error reply (`reply.Err()`) is not marked, so a bad model or a refusal is
reported rather than retried.

*Alternative rejected:* retry every error once. It would hide a server's refusal
behind a redial, and it would double the cost of a genuinely bad query.

### D2. A redial reloads the preset

`reconnect` dials and then loads the preset, so a redial to a *restarted* server
leaves the zone able to compose. Without it, a redial after a restart would
answer with `NOSCRIPT` — the case `Compose` already retried, now handled once,
at the point where the connection is known to be new.

### D3. One mutex over the connection

`Upstream` holds a `sync.Mutex` for the length of `Compose` and `Embed`. A RESP
exchange is three steps on one socket, so concurrent callers on it interleave
replies; the mutex also makes a redial safe, since no other caller can be
mid-read while the connection is replaced.

*Alternative rejected:* a connection per query. It would trade a lock for a dial
per query and would lose the pipelining the index build depends on.

## Risks / Trade-offs

- **A redial storm when emb is down** → one redial per call, not a loop, and the
  failure is returned to the caller; the zone's own refusal path is unchanged.
- **The retry doubles the work of a call that lost its connection** → the retry
  is one query's worth of round trips, and only after a connection was lost.
- **A reply error after a redial is reported with the retry's error, not the
  original** → accepted: the caller needs the reason the retry failed, and the
  original failure (a lost connection) is not actionable.

## Migration Plan

Deploy the zone (`just dns-deploy`). Nothing else changes: the config, the
vocabulary, and the model are untouched.

## Open Questions

None.
