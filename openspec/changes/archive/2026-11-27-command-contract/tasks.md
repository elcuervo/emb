# Tasks

## 1. Server declaration table

- [x] 1.1 Add `internal/contract` (stdlib only, `CGO_ENABLED=0 go build ./internal/contract/` passes) with `Doc{Name, Grammar, Keywords, Arity, Flags, Reply, ReplySchema, Summary}`, the closed flag vocabulary, and the sorted list of declarations; declare all 22 commands with wire-case names, keywords in wire order, arity, and flags, `EMB.SCRIPT` as one entry with `LOAD|EXISTS|FLUSH`; verify the package builds without CGo and a test fails on a duplicate name.
- [x] 1.2 Add `s.route(mux, contract.Doc, handler)` in `internal/server` that registers the mux route and records the declaration; verify `go build ./internal/server/` and that every route in `newServer` goes through it.
- [x] 1.3 Add the coverage test: every registered route has exactly one declaration and every declaration has a route; verify it fails when one registration is deleted and passes on the clean tree.
- [x] 1.4 Add `reply_schema` (JSON Schema draft 2020-12) plus `wire_notes` for `EMB.IMG` and `EMB.IMGMULTI`; verify a test unmarshals both schemas and asserts the one-image branch is a map and the several-image branch an array of nullable maps.

## 2. Generated artifact and gate

- [x] 2.1 Add `cmd/emb-commands` importing only `internal/contract`, rendering deterministic JSON (entries sorted by name, `contract: 1`, two-space indent, trailing newline), and write `commands.json` with it; verify `CGO_ENABLED=0 go run ./cmd/emb-commands` twice produces byte-identical output.
- [x] 2.2 Add `just commands-gen` and `just commands-check` (check renders to a temp file, diffs, and exits non-zero naming the differing entries); verify deleting one entry from `commands.json` makes `commands-check` fail and restoring it passes.
- [x] 2.3 Wire `just commands-check` into the CI workflow ahead of model-dependent jobs; verify the workflow runs it in a job with no model download and that it needs no ONNX runtime (`CGO_ENABLED=0`).
- [x] 2.4 Point `docs/commands.md` and `docs/clients.md` at `commands.json` and state that the table is a rendering of the contract, not a second source; verify the linked path exists and `just commands-check` still passes.
- [x] 2.5 Add the `EMB.HELP` coverage test: every declared command name appears in the help body; verify it fails when one help line is removed.

## 3. Ruby gem contract validation

- [x] 3.1 Vendor the artifact at `gems/emb/lib/emb/commands.json` and add a `commands:sync` rake task that copies the repository file; verify `bundle exec rake commands:sync` leaves the copy byte-identical to the root file and that `gem build emb.gemspec` includes it.
- [x] 3.2 Add the spec that every command literal in `lib/` is declared in the shipped contract, reporting the offending command and file; verify it fails when a literal is changed to an undeclared name.
- [x] 3.3 Add the keyword spec: drive the builders (`ImageCommand.args`, `ImageCommand.multi_args`, `MultiProxy#send_image_group`, `BatchDispatch#dispatch_slice`) through a recording client and assert every emitted token is declared for that command; verify it fails when `OUTPUTS` is dropped from `multi_args`.
- [x] 3.4 Add the copy-equality spec (skipped when the repository file is unreachable) and describe the validation and the sync task in `gems/emb/README.md`; verify the spec fails after editing the vendored copy and passes after `commands:sync`.

## 4. Integration verification

- [x] 4.1 Run `just commands-check`, `go test ./internal/server/ -run 'Contract|Help' -count=1`, and the gem's contract specs together on a clean tree; verify all pass and that `just commands-check` fails on a deliberately edited declaration.
- [x] 4.2 Confirm no wire behavior changed: `go test ./internal/server/ -run 'IMGMULTI|IMG|Help' -count=1` and the gem's image specs pass unchanged.

## Workflow follow-up

- Archive the change after review; `commands.json` and the gem copy must be committed together.
