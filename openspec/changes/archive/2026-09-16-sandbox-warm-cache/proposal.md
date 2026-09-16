## Why

The sandbox's demo repeats itself and every restart discards the warm cache, so
a returning visitor and the fixed examples re-run inference on the machine's one
shared vCPU. `emb` already has a persistent snapshot; the sandbox names no
`cache_file`, so it starts cold by choice. The volume the models live on is
already there, so the cost of a warm restart is one config line and one Fly
shutdown knob.

## What Changes

- **The sandbox cache is persisted on the volume and restored on restart.**
  `website/repl/sandbox.yaml` names `cache_file: /data/cache.embcache` and a
  `cache_save` interval. `cache_load` and `cache_save_on_shutdown` already
  default to true, so the file is restored before ready and flushed on a
  graceful stop; the interval covers a restart that never got a signal.
- **The machine's graceful-exit window outlasts the flush.** `kill_timeout` is
  raised above Fly's 5s default, because `emb`'s shutdown path allows up to 30s
  and a SIGKILL mid-save would leave the previous snapshot in place and lose the
  restart.
- **`just sandbox-deploy` builds first.** The target depends on
  `sandbox-build`, so a bridge that does not compile fails locally before the
  image upload. (The image is rebuilt from the same context by Docker either
  way; this is the earlier gate, not a different artifact.)
- **The `EMB.SAVE` refusal stops claiming the sandbox persists nothing.** A
  visitor still cannot trigger a write, but the string would otherwise be false
  once the lifecycle writes the snapshot itself.
- **Local runs stay cold.** `just website-dev` and `just website-demos` rewrite
  the snapshot path off `/data`, so a developer machine neither reads nor writes
  the deployment's volume path.

## Capabilities

### New Capabilities

None.

### Modified Capabilities

- `sandbox-service`: the sandbox's cache gains a lifecycle — a snapshot on the
  mounted volume, restored before ready and flushed on a graceful stop, with a
  periodic save as the backstop for a restart that got no signal.

## Impact

- **Modified:** `website/repl/sandbox.yaml` (cache snapshot path and interval,
  comment), `website/repl/fly.toml` (`kill_timeout`), `website/repl/allowlist.go`
  (the `EMB.SAVE` refusal reason), `justfile` (`sandbox-deploy` depends on
  `sandbox-build`; the two local config generators rewrite the snapshot path).
- **Unchanged:** the server binary, its cache-snapshot implementation, the
  gems, the published site, the bridge's permitted surface, and the Fly
  volume/machine/health-check configuration. No new mount, app, or DNS name.
- **Operational:** at most 64 MB of snapshot data beside the models on the
  existing volume, and one bounded periodic write that is skipped when the
  cache is clean.
