## Context

`emb` already implements persistent cache snapshots (`openspec/specs/cache-snapshots/`):
`cache_file` turns on a snapshot coordinator, `cache_load` defaults to true, and
`cache_save_on_shutdown` defaults to true. The sandbox config sets neither, so
the machinery is structurally dormant. `website/repl/fly.toml` already mounts
one volume at `/data` and the models download onto it, which proves the volume
survives a deploy. `run.sh` is PID 1, traps INT and TERM, forwards the signal to
`emb`, and waits for it; `emb` allows its shutdown up to 30 seconds. Fly's
default `kill_signal` is SIGINT and default `kill_timeout` is 5 seconds.

See `proposal.md` — Why for the motivation.

## Goals / Non-Goals

**Goals:**

- A restart reloads the cache the previous run wrote, using the server's own
  snapshot lifecycle, with no new component and no new Fly resource.
- A graceful stop has enough time to finish the flush.
- A restart that never got a signal still loses at most the recent window.
- A missing or invalid snapshot is a cold start, never a blocked start.

**Non-Goals:**

- Durability of every insertion, cross-instance coherence, or a second machine.
- Changing the bridge's permitted surface or the snapshot file format.
- A separate cache volume, a scheduled job, or an out-of-band backup.

## Decisions

### 1. Put the snapshot on the existing volume, named directly under `/data`

`cache_file: /data/cache.embcache` rides the volume that already holds the
models. No `[mounts]` entry changes, so no machine is recreated, and the models
already demonstrate that the volume is reattached by name on deploy.

The path is a file directly under `/data`, not a subdirectory, because
`writeSnapshotFS` creates its temporary file with `os.CreateTemp` in the
destination's directory and never calls `MkdirAll`. `/data` is the mount root
and therefore exists; a `/data/cache/` subdirectory would have to be created by
something, and nothing does.

Alternatives considered:

- A second volume for cache, isolated from a model re-download. It buys nothing:
  the cache is 64 MB against a 3 GB volume, and it adds a provisioned resource
  to a one-machine deployment.
- `/data/models/cache.embcache`, which the local `sed` would rewrite for free.
  Rejected as mixing runtime state into the directory the demo tooling treats as
  model artifacts.

### 2. Raise `kill_timeout` to outlast the shutdown, not to slow the stop

Fly's 5-second default would SIGKILL the machine while `emb` is still draining
and writing. `emb` caps its own shutdown at 30 seconds, so `kill_timeout = 40`
leaves the flush room without changing the normal case: a healthy machine drains
in about a second and exits long before the ceiling. The default SIGINT is
already trapped by `run.sh` and `emb`, so no `kill_signal` is set.

Alternatives considered:

- Lower the server's shutdown budget so the default window fits. That would
  trade a real durability improvement for a knob, and 64 MB writes in well under
  a second on a volume; the ceiling only matters if the disk is slow, which is
  exactly when it should not be preempted.
- `kill_signal = "SIGTERM"` for clarity. Unnecessary: both `run.sh` and `emb`
  install handlers for both, and the default already reaches them.

### 3. Add a periodic save as the backstop for an ungraceful restart

`cache_save: 30m` covers the stop that never delivered a signal — host
maintenance, an OOM, a forced terminate — where only the last snapshot exists.
The coordinator is single-flight and `periodicTick` skips when the mutation
generation is unchanged, so an idle sandbox writes nothing and a busy one writes
at most 64 MB per interval.

Alternatives considered:

- Shutdown-only. Simpler, and correct for deploys and machine restarts, which
  are the sandbox's normal restarts; it makes every crash or forced stop cold.
  The interval is one line for the difference.
- Rely on `EMB.SAVE` from an operator. The bridge refuses `EMB.SAVE` to
  visitors, and the deployment has no operator loop.

### 4. Keep local runs cold by rewriting the path, not by splitting the config

`just website-dev` and `just website-demos` already `sed` the sandbox config
into a generated one that points models at `./models`. They gain one more
expression that clears `cache_file`, so a developer machine never reads or
writes `/data`. The deployment and a local run differ in exactly the paths they
must differ in, and the generated configs stay gitignored.

Alternatives considered:

- Leave `/data/cache.embcache` in the local config. The server tolerates a
  missing file, so it would only log a save failure every interval and on every
  shutdown — harmless, but noise that hides a real failure.
- A dedicated local cache path and a `.gitignore` entry. More moving parts than
  the dev target needs; persistence testing belongs against the deployment or a
  hand-written config, not in the panel harness.

### 5. The `EMB.SAVE` refusal stops describing the sandbox as stateless

`allowlist.go` refuses `EMB.SAVE` with "the sandbox persists nothing". Once the
lifecycle persists, that sentence is false in a place a visitor can read. The
reason becomes a statement about who may trigger a write, which is the actual
boundary: the sandbox writes its own snapshot, a visitor cannot ask it to.

### 6. `sandbox-deploy` depends on `sandbox-build`

`just sandbox-deploy: sandbox-build` makes the local compile the first gate, so
a bridge that does not build fails before an upload. It does not change the
deployed artifact: the Dockerfile compiles `/emb` and `/repl` from the same
build context, so `fly deploy` already ships the current tree. The dependency is
preflight, and the recipe comment says so rather than implying the local binary
is uploaded.

## Risks / Trade-offs

- **[Snapshot bytes include original input text]** → The same property the
  server documents: the file is `0600`, the volume is the one that already holds
  the models, and the sandbox's inputs are public demo text. Deleting the file
  is always safe.
- **[A quantization or model change invalidates the snapshot]** → The
  fingerprint check skips incompatible entries and the first boot starts cold;
  that is one cold start after such a change, not a failure.
- **[Restore admits fewer entries than saved]** → The effective ceiling is the
  smallest of the 64 MB budget, `cache_restore_limit=auto`, and sampled
  headroom; a partial restore is visible in `INFO cache` and still warm.
- **[Periodic writes add I/O]** → Single-flight, skipped when clean, bounded at
  64 MB, and off the request goroutines by the server's own design.
- **[A long `kill_timeout` delays a stuck shutdown]** → It is a ceiling, not a
  delay; a machine that does not drain is killed at 40s instead of 5s, which is
  the price of not killing a healthy flush.
- **[A local run silently differs from the deployment]** → The difference is
  one rewritten path, and the deployment's own config is the file under
  `website/repl/`, so the shipped behavior is the readable one.

## Migration Plan

1. Deploy with `cache_file` and `cache_save` set and the raised `kill_timeout`.
   The first boot after the deploy has no file and starts cold, as today.
2. Warm the cache, restart the machine, and read the restore counters through
   `INFO cache` on the bridge to confirm a warm second boot.
3. Roll back by clearing `cache_file` and `cache_save`; the server ignores the
   disposable snapshot and resumes its previous in-memory-only behavior.
