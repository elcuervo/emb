# Proposal

## Why

Every scripted model that needs checkpoint constants receives them per call:

```bash
EMB.EVSHA laya <sha> 1 '<state>' '<questions>' '{"max_len":64,"head_max_len":32,…}'
```

That envelope is a property of the **checkpoint**, not the **request**: the same
bytes on every call, known by every client, re-decoded by the preset on every
evaluation. The first design put it in a sibling `script_config:` key on the
model entry — but that fragments one script's declaration across two places
(`scripts:` and `script_config:`) that must be kept in sync, and needs its own
namespacing once a model mounts more than one preset.

Folding the constants into the `scripts` entry keeps **one declaration per
script**, needs no namespacing (the config belongs to exactly the script that
declared it), and gives the host a natural place to hand them to Lua.

## What Changes

- **`scripts` entries become structured.** Each entry is either a string (the
  existing shorthand for a path with no config) or a mapping with `path` and an
  optional `config` mapping. Existing configs keep working unchanged.
- **`emb.script.config`.** The host exposes the running script's config as a Lua
  table under a namespace, scoped per script, an empty table when absent. The
  host validates only the shape (a mapping, finite numbers, bounded size) and
  never interprets a key — that is what keeps it general across models.
- **Config joins the reply-cache identity.** Today the envelope travels in ARGV
  and is therefore implicitly cached; moving it to the model entry means a
  restored cache snapshot could otherwise serve replies computed under an older
  envelope. The script's config hash is folded into the cache key.
- **First consumer: `scripts/laya.lua`.** Its budgets and temperatures come from
  `emb.script.config`; `ARGV[2]` remains as a per-call override. The wire call
  loses the envelope.
- **Documented generality.** The sandbox's other presets (`gliner2.lua`,
  `classify.lua`, `zeroshot.lua`) are re-expressed with their model literals in
  config, as the acceptance check that nothing here is Laya-shaped.

## Capabilities

### New Capabilities
<!-- none -->

### Modified Capabilities
- `script-preload`: a `scripts` entry may carry a per-script config, exposed to
  that script as `emb.script.config` and folded into its reply-cache identity.

## Impact

- `internal/config/config.go` — `Scripts []string` → `[]ScriptEntry` with a
  string/mapping `UnmarshalYAML`.
- `internal/server` — preload resolves entries and binds each config into
  `script.Hosts`.
- `internal/script/host.go` — `emb.script` table; `internal/script/cache.go` —
  config digest in the key.
- `scripts/laya.lua` — reads `emb.script.config`; `sandbox.yaml`, `test-laya.yaml`
  — envelope moves onto the model entry.
- Non-goal: no new command, no wire-format change, no per-request behavior change
  when a caller still passes `ARGV[2]`.
- Deliberately **not** a performance change: the config is ~210 B and one
  decode; this is a clarity/ergonomics change (see design D7).
