## Context

See proposal.md — Why. The relevant current state:

- `website/repl/terminal.js` owns the only timing display: `startedAt` set at
  submit (`performance.now()`), `timing()` computed on reply receipt, rendered
  as a `k: 'time'` line. It is a client wall-clock delta.
- `website/repl/bridge.go` `send()` performs, per call: `EnsureConn` (reconnect
  only if the connection was lost), `Hello(proto)` (every call, a loopback
  round trip), then `WriteArgv` + `Flush` + `ReadReply`. `roundTrip()` takes the
  single upstream mutex and retries `send` once on a lost connection.
- `website/repl/envelope.go` defines the discriminated `Envelope` the client
  switches on. `emb` itself already records `time.Since(started)` as
  `MonitorEvent.LatencyUs`, but that never rides the `EMB` reply (a raw vector)
  and is only read by `emb-top`/stats.
- `emb` and the bridge are loopback siblings (`run.sh`, `sandbox.yaml`), so a
  timer placed on the bridge measures the server's answer time to within
  sub-millisecond loopback transit.

## Goals / Non-Goals

**Goals:**

- The trailer number is the server's answer time, measured on the bridge, never
  the visitor's round trip.
- One measurement point, carried in the reply contract that both console
  surfaces already share, so landing and standalone stay identical.
- No `emb` wire change, no RESP client change, no Ruby gem change.

**Non-Goals:**

- Reporting `emb`'s own internal `LatencyUs` (would require a reply-format
  addition).
- Showing the client's round trip anywhere, even alongside the server time.
- Measuring or attributing sandbox cold-start / wake wait in the trailer.

## Decisions

### Measure inside `send`, around the command write→read

Bracket `WriteArgv`/`Flush` through `ReadReply`, after `Hello`. This excludes
the `HELLO` negotiation round trip and the single-connection mutex wait, so the
value is the command's answer time rather than server-side queueing.

- *Alternative: bracket `roundTrip` in `Execute`.* One line, but folds in the
  mutex wait. On a single serialized upstream connection that wait is
  server-side queue, not inference — misleading under concurrent visitors.
- *Alternative: have `emb` echo its own latency.* The only exact number, but
  the `EMB` reply is a vector and carrying a scalar would change the wire format
  and the client gems. Not worth it for a demo panel, and the difference is
  sub-millisecond on loopback.

### Carry microseconds as an integer on the envelope

Add `ElapsedUs int64 \`json:"elapsed_us,omitempty"\`` to `Envelope`, matching
`emb`'s own `LatencyUs` unit convention. `omitempty` makes absence the natural
encoding for replies with no upstream call (refusals, capacity bounds), which
must render no trailer. The client formats to `ms`/`s` exactly as `timing()`
does today.

### The client renders the envelope value, nothing else

`terminal.js`'s `timing()` becomes a formatter that reads `env.elapsed_us`; if
it is absent, no `k: 'time'` line is pushed. `startedAt`, `nowMs`, and their
assignments in `submit`/`retry` are removed, so no client clock remains in the
timing path.

### Retry semantics

On a lost connection `roundTrip` retries `send`; each `send` measures its own
attempt and the successful attempt's value is the one returned. The client's
cold-start `starting` retry loop is unaffected: the final reply simply carries
the server time for the command that actually ran.

## Risks / Trade-offs

- [Loopback transit is not zero] → It is sub-millisecond on the sandbox's
  single host; the intended claim is "the server's answer time", not "pure
  matmul time", and the alternative (emb's internal clock) costs a wire change.
- [First call after a reconnect includes `EnsureConn`] → Excluded by measuring
  after `Hello` only around the command; reconnect cost is connection setup,
  not the command.
- [Refusals now show no time where they previously showed a client delta] →
  Intended: a fabricated `0 ms` or a network number is worse than none, and the
  error text already carries the reason.
- [Docs drift] → `website/README.md` wording is part of the task list.

## Migration Plan

Single-code change with the site and bridge deployed together (`just
sandbox-deploy`). The envelope field is additive and `omitempty`, so an old
client ignores it and a new client falls back to no trailer if the field is
missing; no coordination window required. Rollback is reverting the deploy.
