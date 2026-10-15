# Tasks

## 1. The bounded wait in `internal/embtop`

- [x] 1.1 Add the wait sibling to `ResolveSpecs` in `internal/embtop/node.go`: it retries the resolution until every specification yields an address or the budget expires, with attempts at least a second apart and each attempt bounded by the package's resolve timeout, returning the same `[]ResolvedNode` or the same wrapped error. Verify with a fake `Resolver` in the style `node_test.go` already uses: a fake that fails twice then succeeds returns the nodes with a nil error and records exactly three calls.
- [x] 1.2 Make the wait all-or-nothing and verify it: with one specification that never resolves and one that does, the call returns the wrapped error naming the failing specification, returns no nodes, and `errors.As` still reaches the underlying `*net.DNSError`; with a resolver that always fails, the wait stops at the budget (a tiny budget in the test) rather than looping. Assert call counts and the returned error only — no wall-clock assertion.

## 2. `emb-top` startup uses the wait

- [x] 2.1 Point the startup resolution in `cmd/emb-top/main.go` at the wait, with the production resolver and the built-in budget, leaving the TUI / `-once` / `-frames` dispatch below it and the in-watch refresh (`fleet.go`, still one-shot `ResolveSpecs`) unchanged. Verify: `nix develop --command bash -c 'go test ./cmd/emb-top/ -count=1'` passes, and `./bin/emb-top -nodes does-not-exist.internal` exits 1 with the same message shape and text as before the change, after the budget.
- [x] 2.2 Smoke the case the change exists for: with a local `emb` node listening on 16379 and a name absent from `/etc/hosts`, start `./bin/emb-top -nodes late.internal`, then add `127.0.0.1 late.internal` to `/etc/hosts`; the node is polled and rendered as a row without restarting the tool. Record the observed row in the change's notes.

## 3. `emb-stress` target expansion uses the same wait

- [x] 3.1 Point the target expansion in `cmd/emb-stress/main.go` at the same wait, so a run and a watch started before their fleet agree about the name. Verify the package tests pass and that `./bin/emb-stress -config test-two-models.yaml -target does-not-exist.internal -levels 1` still exits non-zero with the resolution error, after the budget.

## 4. Docs: the discovery prerequisites

- [x] 4.1 State, inside `docs/operations.md`'s existing monitoring section and next to the `-nodes <name>` examples, what makes a discovery name resolvable in a Fargate fleet: the namespace's private hosted zone has to be associated with the VPC the watching task runs in; per-task address records come from Cloud Map DNS service discovery while a Service Connect client alias resolves to the proxy in the watching task's own task, so it is one monitored node and not the fleet; and the `dig` that tells an absent name from a zone this VPC cannot see. Introduce no new top-level section. Verify by reading the section against both new scenarios in the change's `product-docs` delta and by running `just website-published` unchanged.

## 5. Integration

- [x] 5.1 Prove the fleet case end to end: two local `emb` nodes on 16379 and 16380, both address records behind one name that does not resolve at start; the fleet view renders one row per address once the records appear, `emb-top -nodes <resolvable-name>` keeps its existing single-node and `-once`/`-frames` output, and `emb-stress -target <same name>` drives both nodes.
- [x] 5.2 Run the standing gates inside `nix develop` and report their output: `just test`, `just lint`, `just deadcode`, `just build`.

## Workflow follow-up

- Archive the change with `/openspec-archive-change` once the review requirements are satisfied; it syncs the two delta specs into `openspec/specs/` and moves the change under `openspec/changes/archive/`.
- No `BENCHMARK.md` figure, preset digest or `VERSION` change is involved, so `just website-presets-check` and `just website-version-check` need no re-stamp for this change.
