# Design

## Context

See proposal.md — Why. The current surface:

- `ModelConfig.Scripts []string` (`internal/config/config.go`), resolved against
  the config file's directory, preloaded and SHA-keyed by `script-preload`.
- `internal/script/host.go` builds a fresh `emb` table per evaluation in
  `registerHosts`; the sandbox strips `require`/`package`/loaders, so presets
  cannot share Lua code — a preset's constants must live where the host can see
  them.
- `internal/script/cache.go` content-addresses replies on
  `(api_version, model, script_sha, args, numTexts, text)` — the config, when it
  travels in `ARGV[2]`, is already part of that identity.
- `scripts/laya.lua` reads `ARGV[2]` and clamps temperatures. Every call pays to
  resend and re-decode the checkpoint's envelope.

## Goals / Non-Goals

**Goals**
- One declaration per script: path and its checkpoint facts in the same entry.
- A general mechanism — the host knows nothing about any model's keys.
- Backward compatibility: existing `scripts: [path]` configs run unchanged.
- No stale replies when a config changes.

**Non-Goals**
- No new Redis command, no wire-format change.
- No per-call behavior change when a caller passes `ARGV[2]`.
- No typing or validation of specific keys (that would make the host model-aware).

## Decisions

### D1. Fold into `scripts`, not a sibling `script_config`

A separate `script_config:` key splits one script's declaration across two
lists and needs namespacing when a model mounts several presets. A structured
`scripts` entry keeps them together and makes the config's scope the script's.

### D2. `emb.script` is a namespace, `config` its first inhabitant

`emb.config` would squat on the top level next to `emb.run`/`emb.tokenize`.
`emb.script` leaves room for `path`, `sha1`, `api_version` later without another
top-level name.

### D3. Per-script scope, empty when absent

`emb.script.config` is the config of the script currently executing. A model that
mounts several presets needs no namespacing. Absent → an empty table, so
`emb.script.config.max_len or 512` reads cleanly with no guard.

### D4. Shape-validated, semantically opaque

The host checks that `config` is a mapping, that its numbers are finite, and
that it is bounded (64 KiB) — then hands it over untouched. Keys are a contract
between the operator and the script. This is the property that makes the
mechanism general across models.

### D5. Config joins the reply-cache identity

The key already folds `script_sha`; it must also fold a digest of the config, or
a cache snapshot restored after an operator edits the envelope could serve
replies computed under the old one. The digest is computed once at load, so the
per-request key input gets *smaller* (no inline envelope) rather than larger.

### D6. Table built per evaluation

`runProto` creates a fresh `lua.LState` per evaluation
(`internal/script/engine.go:101`), and Lua tables cannot cross states, so the
config table is rebuilt per call. That is acceptable **because the config is
small**; the size bound in D4 is what keeps this honest. No lazy metatable
machinery until a real config proves it necessary.

### D7. This is not a performance change

Measured/accounted impact: −~210 B on the wire per call; the decode moves from
`ARGV[2]` in Lua to the host (approximately equal work, because the LState is
fresh either way); the cache key input shrinks by the envelope and grows by a
load-time digest. The forward pass, tokenization, and round trip are untouched.
Recorded so nobody later cites this change as a speedup.

## Risks / Trade-offs

- [A large config becomes per-call work] → 64 KiB bound, and it is decoded to a
  table only when the model has a script.
- [Operators put per-request data in config] → documented rule: same-for-every-call
  stays in config, caller-chosen stays in ARGV.
- [Shorthand vs uniform entries split the parser] → one `UnmarshalYAML` on
  `ScriptEntry`; both forms produce the same struct, covered by a config parse test.
