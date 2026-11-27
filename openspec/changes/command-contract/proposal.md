# Proposal

## Why

The command surface is written down five times — the mux registrations in `internal/server/server.go`, the `EMB.HELP` prose, `docs/commands.md`, `website/docs/index.html`, and the Ruby gem's scattered argv builders — and nothing compares them. Two client bugs shipped through a green CI this week from exactly that gap: `Emb.multi` dropped the `OUTPUTS` keyword on `EMB.IMGMULTI`, and a deferred single-image call decoded the reply in the wrong shape. A client cannot validate either against prose. Redis solved the same problem for the same protocol by keeping command metadata in machine-readable form, generating code and docs from it, and letting clients read it; emb has no equivalent.

## What Changes

- The server gains one Go table describing every command: name, grammar, keywords, arity, flags, and — for the two image commands only — a JSON Schema for the reply. Routes are registered together with their entry, so a handler cannot exist without a contract entry.
- `commands.json` at the repository root is generated from that table. `just commands-gen` writes it; `just commands-check` regenerates in memory and fails on any byte difference, and CI runs the check (the pattern already used for `website-presets-check`).
- `reply_schema` is JSON Schema draft 2020-12 and covers only `EMB.IMG` and `EMB.IMGMULTI`, where one image returns a keyed map and several return an array of maps — the shape that broke.
- `EMB.HELP` must list every command the contract declares; a coverage test enforces it rather than generating the help prose.
- The Ruby gem ships a vendored copy (`gems/emb/lib/emb/commands.json`, already matched by `lib/**/*` in the gemspec) and validates its build against it: every command literal the gem sends is declared, every keyword a builder emits is declared for that command, and the vendored bytes equal the repository artifact.

**Non-goals:** OpenAPI (it describes HTTP, not argv dispatch or RESP types); CEL (a policy language, and the sandbox allowlist stays Go); a general Redis-style `oneof`/`block` argument tree (emb has one nested case, `EMB.SCRIPT`); reply schemas for every command; a runtime `EMB.HELP JSON`; deriving the sandbox allowlist from `flags` — the `flags` field enables that, it does not do it here.

## Capabilities

### New Capabilities

- `command-contract`: the server's machine-readable command surface — the Go table as the only writer, the generated `commands.json`, its stability under `commands-check`, the keyword and arity vocabulary, and the flags that classify commands.

### Modified Capabilities

- `emb-cmds`: adds a requirement that `EMB.HELP` lists every command the contract declares.
- `emb-ruby-client`: adds a requirement that the gem ships the contract and validates its command surface and argv against it at build time.

## Impact

- New: `internal/server/commands.go` (table, registration helper, JSON rendering), `commands.json`, a Go test for coverage and rendering.
- Changed: route registration in `internal/server/server.go`; `justfile` gains `commands-gen` / `commands-check`; the CI workflow runs the check; `docs/commands.md` and `docs/clients.md` point at the artifact.
- Ruby: new `gems/emb/lib/emb/commands.json`, a `commands:sync` rake task, three specs. The gemspec needs no change.
- No wire-protocol change, no new dependency in either the server or the gem.
