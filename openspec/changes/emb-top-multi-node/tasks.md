# Tasks

## 1. Node specifications and resolution

- [x] 1.1 Add `-node` (repeatable) and `-nodes` (comma-separated) flags, keep `-addr` as a compatibility alias, and apply the `localhost:6379` default only when no node flag is given; verify with a flag-parsing test covering each form and the combination
- [x] 1.2 Parse a specification into `host:port` (bare host takes the default port) and collect DNS names separately; verify with a table test over `host`, `host:port`, and name forms
- [x] 1.3 Expand each DNS name through a context-bound resolver call into one node per address record, deduplicating on `host:port`; verify with a fake resolver returning multiple, duplicate, and empty result sets
- [x] 1.4 Fail with an error naming the offending specification when a specification yields no address; verify the error path in a test and confirm an unresolved fleet never renders as healthy
- [x] 1.5 Re-resolve on a fixed interval and immediately after a node becomes unreachable, adding new addresses without a restart and keeping the known fleet on resolver failure; verify with a fake resolver whose answers change between ticks (no wall-clock sleeps)

## 2. RESP client: dial address versus TLS server name

- [x] 2.1 Let `internal/resp` dial one address while verifying a different TLS server name, and add a `-tls-server-name` override flag in `emb-top`; verify with a test TLS server whose certificate carries a DNS name while the client dials the address
- [x] 2.2 Confirm the change is TLS-only and leaves the non-TLS handshake, `AUTH` and `HELLO` behavior untouched; verify the existing `internal/resp` tests still pass

## 3. Per-node polling fan-out

- [x] 3.1 Introduce a fleet that holds one `resp.Client`, one `embtop.Sampler`, one `MONITOR` cursor and one last-sample timestamp per node, and poll them on independent goroutines; verify two in-process servers both report on one tick using the existing `serveEmbedded` helper
- [x] 3.2 Ensure a stalled node cannot delay another node's poll or the rendered frame, and that a node's row keeps its last values with the age of that sample; verify with a gated fake that withholds one node's replies
- [x] 3.3 Rebase rates and the `MONITOR` cursor when a node reconnects or restarts so the aggregate does not spike; verify a rejoin test asserting the first post-rejoin rate stays within the node's own counters
- [x] 3.4 Implement additive membership: keep polling a node that no longer resolves, mark it orphaned, and remove it only after it is both unresolved and unreachable for the grace period; verify with a fake resolver plus a configurable grace window
- [x] 3.5 Keep node rows in a stable first-seen order that does not follow resolution order or traffic; verify a reordering test where resolution order and load both change between ticks

## 4. Fleet rendering

- [x] 4.1 Render the aggregate band above the node rows, with shares computed against `1/N` and DNS record weights ignored; verify a golden-render test for a fixed fleet
- [x] 4.2 Render one row per node — identity, share versus expectation, verdict chips, request-activity strip — with `idle`, `orphaned` and `unreachable` as distinct states; verify each state's rendering in a test
- [x] 4.3 Report membership and traffic separately in the header (discovered, receiving traffic, idle, orphaned); verify a fleet where one node is idle, one orphaned, and one unreachable produces the expected counts
- [x] 4.4 Scroll the node rows as one unit while keeping the aggregate visible, and verify every node is reachable at narrow and wide terminal widths with no row shifting between polls
- [x] 4.5 Render the single-node dashboard unchanged when exactly one node is monitored; verify the existing single-node render tests and width tests still pass

## 5. Fleet verdict

- [x] 5.1 Derive the fleet verdict in a new pure function that reuses the existing per-node rules and thresholds, with the same `no data` guard and severity precedence; verify with table tests for healthy, degraded and critical fleets
- [x] 5.2 Drive the fleet verdict only from actionable signals — nodes unreachable or refusing auth, tail latency far above the fleet median, cold cache after a restart, load spread beyond the node count — and never from cache spread or per-node hit rate; verify a test where cache spread alone leaves the verdict healthy
- [x] 5.3 Compute each node's CPU usage against that node's own reported parallelism, falling back to the local count only when absent; verify a test where `emb-top`'s machine count differs from the node's
- [x] 5.4 Report the reason and the offending node alongside a degraded or critical verdict; verify each reason renders in the banner test

## 6. Drill-down and keybindings

- [x] 6.1 Select a node row with `j`/`k`/arrows, open that node's per-model dashboard on enter, and return to the fleet with escape, preserving row order; verify an interaction test
- [x] 6.2 Keep `q`, `p`/`space` pause and `r` reset working in both views; verify with an interaction test over the fleet view

## 7. Headless modes

- [x] 7.1 Extend `-once` to print the fleet aggregate followed by one section per node, keeping each node's per-model order stable; verify with the embedded-server test using two nodes
- [x] 7.2 Ensure `-once` exits non-zero when no node produced a sample while a partially unreachable fleet still samples the reachable nodes; verify both paths in tests
- [x] 7.3 Make `-frames` stream the fleet frame — every node's row plus the aggregate — and keep running when a node drops; verify with a frame test asserting all nodes appear and the stream continues after one node stops

## 8. Documentation

- [x] 8.1 Document the new flags, resolution and refresh behavior, the idle/orphaned semantics and the DNS-set versus client-set caveat in `README.md` and `docs/operations.md`, and verify every documented command runs as written
- [x] 8.2 Update the in-TUI help view to name the new flags and keys, and verify the help renders within the frame width

## 9. Integration verification

- [x] 9.1 Run the full suite and linter inside the dev shell (`nix develop --command bash -c 'go test ./... -count=1 && golangci-lint run ./...'`) and confirm no regressions
- [x] 9.2 Start two nodes on `:16379` and `:16380` from `test-two-models.yaml`, run `emb-top -nodes localhost:16379,localhost:16380 -interval 1s`, and confirm both rows render with traffic split between them and a load imbalance appears when one node is stopped
- [x] 9.3 Verify the existing single-node capture still works by running `just website-topviz` and confirming it produces the same single-node plate and capture as before this change

## Workflow follow-up

- Archive the change once the project's review requirements are satisfied, and verify the archived specs at `openspec/specs/emb-top-cluster-view/` and `openspec/specs/emb-top-dashboard/`.
- Re-capture the website's live `emb-top` plate (`just website-topviz`) after the fleet view is in use, if the site is to show more than one node.
