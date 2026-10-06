# Tasks

## 1. Size the checkpoint's inference to the machine

- [x] 1.1 In `website/repl/sandbox.yaml`, set the `laya-real` entry's
  `intra_op_threads` to the machine's vCPU count (4 on `shared-cpu-4x`) while
  keeping `inter_op_threads: 1` and `workers: 1`; in `website/repl/fly.toml`,
  remove the `OMP_NUM_THREADS = "1"` env and correct the stale `cache:` comment
  that still calls the machine "1.92 GiB". Verify with
  `nix develop --command bash -c 'go test ./internal/config/ -count=1'` after
  extending the sandbox-config test below.
- [x] 1.2 Extend `TestSandboxConfigExampleEntries` in
  `internal/config/config_test.go` to assert `laya-real`'s `intra_op_threads` is
  greater than 1 and `inter_op_threads` is 1, so a future edit cannot silently
  re-pin the checkpoint to one core. Verify the test fails with the old value
  and passes with 1.1's value:
  `nix develop --command bash -c 'go test ./internal/config/ -run TestSandboxConfigExampleEntries -count=1'`.
- [x] 1.3 Measure the change on the deployed machine: with a cold cache (a
  payload the plate has not run), record the call's `usage.inference_ms` and
  compare against the recorded single-thread baseline (5300.5 ms for the 407-token
  Inbox call), and record `emb` RSS at load and steady state from `EMB.STATS` /
  `INFO`. Verify a cold Inbox call's `inference_ms` is materially below
  5300 ms and steady RSS stays within the 8 GB machine; if it does not, fall
  back to `cores−2` and re-measure. Record both numbers in this change's
  design.md under Context.

## 2. Boot-time warm payloads in the server

- [x] 2.1 Accept a `warm:` list on a `scripts:` config entry — each item the
  texts and arguments a client would send — with boot-time validation (a
  non-mapping item, a missing or non-string text, or a non-string argument is
  fatal and names the script path, matching the entry's `config` validation).
  Verify with table-driven tests in `internal/config/config_test.go` covering a
  valid list, an absent list, and each malformed shape.
- [x] 2.2 Run the declared payloads once after the model and scripts load,
  through the ordinary scripted-evaluation path so the reply lands under the
  client's cache key. Readiness MUST NOT wait on the warm. Verify with a test in
  the load/registry path that reports ready while the warm is blocked and that a
  subsequently cached key is answered without inference.
- [x] 2.3 Make the warm yield: capture the model's served-request count after
  each payload and stop before the next when it moved. Verify with a test that
  serves a client request between two declared payloads and asserts the second
  is not run, while still answering on demand.
- [x] 2.4 Ensure a failing warm payload is logged and does not abort startup or
  the remaining payloads. Verify with a test that declares one failing payload
  followed by a valid one and asserts the server starts, reports ready, and the
  valid one is cached.
- [x] 2.5 Document `warm` beside `script_preload` in the config reference
  (`website/docs/index.html`) with a runnable example, and note that readiness
  does not wait for it and that the warm yields to traffic. Verify the
  shipped config still loads (`go test ./internal/config/ -count=1`, which
  loads `website/repl/sandbox.yaml`), since the docs example mirrors that
  declaration.

## 3. Declare the plate's payloads and say what the wait is

- [x] 3.1 Add the plate's fixed payloads to `website/repl/sandbox.yaml`'s
  `laya-real` script entry, ordered Quickstart and the Inbox ticket the plate
  opens on first, then the remaining tickets, with a comment naming
  `website/demos/laya.html` as the source they mirror and stating that a drifted
  payload is a cache miss rather than a wrong answer. Verify the config loads
  and the declaration is visible:
  `nix develop --command bash -c 'go test ./internal/config/ -count=1'`.
- [x] 3.2 In `website/demos/laya.html`, state the real checkpoint's first-call
  cost and that the plate's fixed examples are served from the warm cache
  afterwards, so the on-screen `inference_ms` is not read as a warm latency.
  Verify the page's copy renders without layout change at mobile and desktop
  widths (`just website` and a browser check of `/demos/laya#inbox`).
- [x] 3.3 Integration check on the deployed sandbox: after `just sandbox-deploy`,
  confirm readiness is reported while the warm is still running, that a declared
  payload's first call is a cache hit (`elapsed_us` far below its stored
  `inference_ms`), and that an undeclared state still answers cold. Verify by
  issuing `EMB.EVSHA` calls through `/api/exec` for one declared and one
  undeclared state and comparing `elapsed_us` and `inference_ms`.

## Workflow follow-up

- `just sandbox-deploy` by hand — the sandbox is not deployed by CI, so a merged
  push does not reach `cli.emb.is`.
- Archive the change and sync the `sandbox-service` and `script-preload` deltas
  into `openspec/specs/` once the deployed measurements are recorded.
