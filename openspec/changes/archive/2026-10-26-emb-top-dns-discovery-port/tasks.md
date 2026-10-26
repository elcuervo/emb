# Tasks

## 1. A specification's host decides whether it is a name

- [x] 1.1 In `internal/embtop/node.go`, express the loopback alias once and use it on both sides:
  `ParseNodeSpec` reports `isName: net.ParseIP(host) == nil && !isLoopbackAlias(host)`, and
  `IsLoopback` reuses the same predicate. Update both doc comments (`ParseNodeSpec`,
  `DefaultAddr`) so they no longer say the port decides. Verify: `go test ./internal/embtop/ -run
  TestParseNodeSpecTable` passes after the table is flipped — `db-a:16379` and
  `emb.internal:16379` are names, `localhost:6379`, `10.0.3.7` and `[::1]:16379` stay literal.
- [x] 1.2 Cover the new resolution in `internal/embtop/node_test.go`: a fake resolver answering
  three addresses for `emb.internal`, `BuildSpecs("", nil, "emb.internal:6399")` →
  `ResolveSpecs` yields three nodes whose `Addr` is each address on port `6399`, `Name` empty and
  `Key` the address; and a name resolving to one address keeps `Key` and `Name` as the
  specification with the port carried. Verify: `go test ./internal/embtop/ -count=1`.

## 2. A resolved row verifies TLS against the name it was resolved from

- [x] 2.1 Give `ResolvedNode` a `TLSName` (the specification's host; empty for a literal row), set
  it in `ResolveSpecs`, and pass it from `fleet.addNode` to `newClient` next to `Addr`. Verify:
  the `node_test.go` assertion from 1.2 checks `TLSName` is `emb.internal` for expanded rows and
  empty for literals, and a `cmd/emb-top` test using the fake client factory checks the client is
  built with that name while `-tls-server-name` still overrides it.
- [x] 2.2 Exercise the user-visible fleet shape: specs from `BuildSpecs("", nil,
  "emb.internal:6399")`, a fake resolver with three addresses and the `fakeFleet` dashboard
  clients → three rows labelled `<address>:6399`, each polled once; adding an address to the
  resolver's answer on the next refresh adds a fourth row without restarting. Verify:
  `go test ./cmd/emb-top/ -run Fleet -count=1`.

## 3. Document the port-carrying form

- [x] 3.1 In `docs/operations.md` (fleet section), add `emb-top -nodes emb.internal:6399` beside
  the bare-name example, state that the port joins every address record, that `localhost` and IP
  literals stay a single row, and that TLS still verifies the resolved name. Verify: with a node
  running on a non-default port (`./bin/emb -config test-two-models.yaml -listen 127.0.0.1:16399`
  inside `nix develop`, model downloaded), `emb-top -nodes "$(scutil --get LocalHostName).local:16399"
  -once -samples 1` shows the resolved row on `:16399`, and `-nodes localhost:16399` still shows
  one row.

## 4. Integration check

- [x] 4.1 Run the affected packages and the regression watch: `go test ./internal/embtop/
  ./cmd/emb-top/ ./internal/embstress/ -count=1`, `golangci-lint run ./internal/embtop/
  ./cmd/emb-top/`, and `just build` followed by `./bin/emb-top -addr localhost:6379` against a
  local node — one row, unchanged. Verify: every command passes and the default invocation renders
  a single row.

## Workflow follow-up

- Archive the change once the implementation and the fleet capture requirements for any changed
  readout are satisfied.
