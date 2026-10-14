# Tasks

## 1. Lift the shared pieces into `internal/embtop`

- [x] 1.1 Move node resolution out of `cmd/emb-top/nodes.go` into
      `internal/embtop` (`nodeSpec`/`parseNodeSpec`/`splitSpec`/`resolveSpecs`/
      `buildSpecs`, the `resolver` interface), pointing `cmd/emb-top` at the
      exported names. Verify: the moved tests pass and `go test ./cmd/emb-top/
      ./internal/embtop/ -count=1` is green with `emb-top` behaviour unchanged.
- [x] 1.2 Move the pure verdict functions out of `cmd/emb-top/health.go` into
      `internal/embtop` (`healthStatus`, `signal`, `healthInput`, `health`,
      `fleetNodeInput`, `fleetHealth`, `raiseSlowPeer`, `raiseColdCache`, the
      `median*` helpers and the threshold constants). Leave the TUI-coupled
      rendering (`bannerView`, `styleFor`, `healthLabel`, `modelHealth`,
      `healthDot`, `healthState`) in `cmd/emb-top`. Verify: `health_test.go`'s
      cases pass from the new home and the dashboard's banner is unchanged.

## 2. Decode the remaining signals

- [x] 2.1 Add `cpu_throttle_periods`, `cpu_throttled_periods`, `cpu_throttle_usec`,
      `cpu_steal_usec`, `cache_restore_headroom_bytes`,
      `cache_restore_limit_bytes` and `cache_restore_skipped_memory` to the
      `EMB.STATS` decode and to `Point`. Verify: a unit test decodes a captured
      reply and asserts each field, and a reply missing them decays to zero
      rather than erroring.

## 3. The recommendation

- [x] 3.1 Implement the node recommendation as a pure function over a node's
      `Point` series in `internal/embtop`, with its own fixed thresholds and no
      severity. Verify: table-driven tests cover each rule — budget binding
      (tokens at budget plus timed-out waits), host starvation (throttle/steal
      with budget unheld), cache headroom spent, unexplained growth at flat
      load, and the quiet case rendering nothing.
- [x] 3.2 Implement the fleet recommendation: name the binding resource, the
      fleet headroom, and the node to look at first, without averaging one
      node's binding resource into the fleet's. Verify: tests cover mixed
      CPU/memory binding, a fallen-behind peer outranking a node-count
      recommendation, uniform headroom recommending nodes, and a quiet fleet.
- [x] 3.3 Assert the boundary against the verdict: no recommendation, at any
      level, changes `health()`'s or `fleetHealth()`'s result. Verify: a test
      fixes a node's inputs, renders recommendations on and off, and asserts the
      status is identical.

## 4. Render it in `emb-top`

- [x] 4.1 Render the node recommendation panel in the TUI beside the health
      banner, and the fleet recommendation in the cluster view, following the
      column grid the gauges already align to. Verify: an interactive run
      against a loaded node shows the panel, and the banner chips keep their
      columns when a recommendation appears and disappears.
- [x] 4.2 Carry the recommendation into the headless modes. Verify: `-once`
      output and `-frames` frames include the recommendation, asserted by the
      existing headless tests.
- [x] 4.3 Document the recommendation in `docs/operations.md`: each rule, the
      counters it reads, and the fact that it is advisory. Verify: the
      documented commands run as written.

## 5. `internal/embstress`

- [x] 5.1 Implement the seeded text generator with length classes derived from
      the configured `max_length`, and the `-corpus` override. Verify: tests
      assert the same seed yields the same sequence, that no generated text
      exceeds the configured maximum, and that a corpus replaces generation.
- [x] 5.2 Implement the fan-out: one worker pool per resolved endpoint, and a
      level schedule with a dwell and a discarded warm-up. Verify: tests against
      a fake endpoint assert every endpoint receives load, that warm-up samples
      are excluded from the result, and that a level reports only its own dwell.
- [x] 5.3 Implement reconciliation against each endpoint's `EMB.MODELS`: the
      configured `dim` when stated, cross-endpoint agreement when it is not,
      with a failing pair dropped from the workload and named. Verify: tests
      cover a missing model, a dimension mismatch, and a clean fleet.
- [x] 5.4 Implement the result: per-endpoint numbers, the fleet aggregate, and a
      machine-readable form. Verify: tests assert errors are counted beside the
      successes, that a failed request is not counted as completed, and that the
      emitted JSON carries every endpoint plus the aggregate.

## 6. `cmd/emb-stress`

- [x] 6.1 Wire the command: `-config`, `-target`, `-seed`, `-levels`, `-dwell`,
      `-warmup`, `-corpus`, `-json`, password and TLS flags matching `emb-top`s.
      Verify: `-h` documents every flag, an unresolvable target fails naming it,
      and a missing `-config` fails before any connection is opened.
- [x] 6.2 Run it against a real server locally and record what the run produces.
      Verify: a run against an in-process `emb` server emits a per-endpoint
      section and an aggregate, and the numbers are non-zero.
- [x] 6.3 Add a `just stress` target and document the tool in
      `docs/operations.md`, including the `emb-top` pairing. Verify: the target
      runs as documented inside `nix develop` against a local fleet.

## 7. Integration and gates

- [x] 7.1 Run `emb-stress` against a two-node local fleet while `emb-top`
      watches it, and confirm the two agree: the offered share per node matches
      the observed share, and a node the stress run saturates is the node the
      fleet recommendation names. Verify: the observation is recorded in the
      change's notes.
- [x] 7.2 Add the new command to the CGo-free harness set so it cannot
      regress into a dependency on ONNX, and run the gates. Verify:
      `CGO_ENABLED=0 go build ./cmd/emb-stress/` succeeds, `just lint`,
      `just test`, `just deadcode` and `just website-presets-check` are green.

## Workflow follow-up

- Archive the change once the project's review requirements are satisfied.
- Verify the archived result: the three capabilities' requirements appear in
  `openspec/specs/` and the change has moved to `openspec/changes/archive/`.
