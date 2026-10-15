# Validation evidence

Commands run inside `nix develop` unless noted. Absolute figures are this
machine; the assertions are the shapes.

## The wait, unit level (`internal/embtop`)

`go test ./internal/embtop/ -run TestWaitForSpecs -count=1 -v`

| test | result | what it pins |
|---|---|---|
| `TestWaitForSpecsRetriesUntilResolved` | PASS (2.00s) | a fake that fails twice then answers → nodes, nil error, exactly 3 calls |
| `TestWaitForSpecsIsAllOrNothing` | PASS (1.00s) | one name that never resolves + one that does → no nodes, error naming `late.internal`, `errors.As` reaches the `*net.DNSError`, 2 calls |
| `TestWaitForSpecsStopsAtBudget` | PASS (0.00s) | a 100ms budget buys exactly one attempt |

Call counts and the returned error are asserted; no elapsed-time assertion
(the wait takes its budget as a parameter, so the last case is instantaneous).

## The failure is still the failure, only later

Before (the pre-change `bin/emb-top`):

```
emb-top: node "does-not-exist.internal": resolve does-not-exist.internal: lookup does-not-exist.internal: no such host
real 0m0.063s · exit 1
```

After (`./bin/emb-top -nodes does-not-exist.internal`, task 2.1):

```
emb-top: node "does-not-exist.internal": resolve does-not-exist.internal: lookup does-not-exist.internal: no such host
real 0m29.468s · exit 1
```

Byte-identical text, exit 1, reported at the budget instead of immediately.
`emb-stress -config test-two-models.yaml -target does-not-exist.internal
-levels 1` (task 3.1) reports the same line with its own prefix after
`real 0m29.469s`, exit 1. Both are within the 30s budget: the deadline is
checked before each retry, so the wait never outlives it by a whole spacing.

`go test ./cmd/emb-top/ -count=1` → ok 0.812s.
`go test ./cmd/emb-stress/ -count=1` → ok 0.530s. That package's existing
`TestUnresolvableTargetFailsNamingIt` drives `run()` against an unresolvable
target, so it now spends the whole window: it was 29.653s before the package
gained the `waitBudget` seam and is 0.530s with it (the seam defaults to
`embtop.WaitBudget`; production is unchanged).

## A record that appears while the watch runs (task 2.2)

One local node (`emb -config test-two-models.yaml -listen 127.0.0.1:6379`); the
name is absent from `/etc/hosts` (`dscacheutil -q host -a name late.internal`
printed nothing). `late.internal` is a bare name, so it takes the default port
6379 — hence the node on 6379 rather than the fixture's 16379.

```
./bin/emb-top -nodes late.internal -frames -interval 1s > /tmp/late2.jsonl &
# 4s later, no record yet:
frames-while-unresolvable=0  stderr=(empty)
printf '127.0.0.1\tlate.internal\n' | sudo tee -a /etc/hosts
# 8s after the record appeared:
frames-after-record=7
redis-cli -p 6379 EMB minilm "late row proof"
frames-after-request=12   ·  same pid throughout (63612)
```

The row the name's address produced, from the last frame (ANSI stripped):

```
emb-top vdev · 127.0.0.1:6379 · uptime 1m11s  ·  2 models · poll 1s
● HEALTHY   connected │ err   0.0% │ p95 1.5ms   │ cpu   0%
● minilm   384d·mean·fp32   0.0 r/s   0.0 t/s  p50 1.3ms  p95 1.5ms  err 0
  event minilm ·   1 texts · 1.5ms   ✓
```

Nothing is printed while the name is unresolvable, the dashboard renders
without a restart, and the requests sent afterwards are polled and counted.

## The fleet case (task 5.1)

Two nodes, both behind one name: `emb -config test-two-models.yaml -listen
127.0.0.1:6379` and `-listen 127.0.0.2:6379` (a loopback alias, removed
afterwards). One name expands to several addresses at one port, and a bare name
carries the default 6379 — so the peers share the port, not the 16379/16380 the
task assumed.

`fleet.internal` absent at start; two `A` records added 4s in; the watcher
(pgid 63708) was never restarted:

```
frames-before=0   (while unresolved)   →   records added   →   frames-after=5
```

Rows, from a frame during the run:

```
emb-top vdev · 2 nodes · poll 1s
  discovered 2 · receiving traffic 2 · idle 0 · orphaned 0
LOAD ████████████████████████████████████  busiest 127.0.0.1:6379 56% · expected 50% each
▶ ● 127.0.0.1:6379   9.0 r/s   56% █████████████████ ▁▁▁▁▁▁▁▅▅████▅  p95 123.6ms  cpu 18%  err 0.0%
  ● 127.0.0.2:6379   7.0 r/s   44% █████████████░░░░ ▁▁▁▁▁▁▁▅▅████▄  p95 122.0ms  cpu 17%  err 0.0%
IMBALANCE  ✓ all 4 pass
```

`emb-stress -config test-two-models.yaml -target fleet.internal -levels 1
-dwell 5s -warmup 2s` through the same name, while that watcher ran:

```
emb-stress: driving 2 endpoint(s) from "fleet.internal" · 2 model(s) · levels [1] · dwell 5s
FLEET        level 1 · offered 32.7/s · completed 32.7/s · p50 38.2ms · p95 120.9ms · 0 errors
127.0.0.1:6379  level 1 · offered 16.7/s · completed 16.7/s · 0 errors
127.0.0.2:6379  level 1 · offered 16.0/s · completed 16.0/s · 0 errors
```

Both halves of the documented pairing agree about the one name.

`-once` against the multi-address name emits the fleet aggregate followed by
exactly one section per address (`node:127.0.0.1:6379` ×1,
`node:127.0.0.2:6379` ×1). Against a single-address name the single-node output
is unchanged: `-nodes late.internal -once` prints the plain `t=…` lines with no
`node:` sections, and its `-frames` output is the single-node dashboard above.

## Standing gates (task 5.2)

Inside `nix develop`:

| gate | exit | output |
|---|---|---|
| `just test` | 0 | `go test ./...` — every package `ok`, no failures (`internal/embtop` 12.954s, `internal/server` 33.547s) |
| `just lint` | 0 | `golangci-lint run ./...` → `0 issues`; `go vet ./...` clean |
| `just deadcode` | 0 | `✓ no unreachable production functions outside deadcode-allow.txt` — `WaitForSpecs` has its two production callers, so nothing new is whitelisted |
| `just build` | 0 | `bin/emb`, `bin/emb-top`, `bin/emb-stress` built |

`openspec validate fargate-fleet-discovery` → `Change 'fargate-fleet-discovery' is
valid`.

## Docs (task 4.1)

`just website-published` → `published-tree: ok (78 served paths, one origin at
https://emb.is)`; the section is prose inside the existing monitoring section,
with no new heading, and both new `product-docs` scenarios are covered: the
VPC/zone association, the Cloud Map service-discovery records against the
Service Connect alias (one node, not the fleet), and the `dig` that separates
the two `NXDOMAIN` cases.
