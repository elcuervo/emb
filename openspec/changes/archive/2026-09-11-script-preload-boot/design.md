## Context

The server already has a fully functional script evaluation pipeline: `EMB.SCRIPT LOAD` stores source by SHA1 in a per-model `scriptCache`, `EMB.EVSHA` looks up source and delegates to `script.Compiler.Eval` which lazily parses/compiles on first use. The compiler cache stores `*lua.FunctionProto` per (model, wrapped-source) to skip re-parsing. See proposal.md for motivation.

## Goals / Non-Goals

**Goals:**
- Accept script file paths in `ModelConfig.scripts`.
- Resolve relative paths against the config file's directory.
- At boot, read, validate, cache source, and warm compiled prototypes.
- A bad script is fatal — server refuses to start.
- Reuse existing validation and caching paths (no parallel script subsystem).

**Non-Goals:**
- Hot-reloading scripts at runtime.
- Named/aliased scripts in config (scripts are identified only by SHA1, same as today).
- CLI flags for script paths (config-file only, consistent with model config).
- Changing `EMB.SCRIPT LOAD` client semantics.
- Changing the Lua sandbox, reply conversion, or execution budgets.

## Decisions

### Path resolution anchors to config file directory

Relative script paths in YAML resolve against the directory containing the config file, not the process CWD. This matches how most tools work (e.g. `docker-compose` volume paths) and is the least surprising behavior for operators.

- **Alternative**: Resolve against CWD.
- **Why rejected**: CWD is unpredictable in systemd/docker environments; the config file is the natural anchor.

### Server exposes `PreloadScript`, called by `main.go`

Rather than passing a complex map through `server.New` options, `Server` gets a `PreloadScript(model, src) (string, error)` method. `cmd/emb/main.go` reads files and calls it after `server.New` but before `SetReady`.

- **Alternative**: A `server.WithPreloadedScripts` option.
- **Why rejected**: The option would need to accept file paths, but path resolution and file reading belong in `main.go` (which knows the config file path). Passing raw sources into the server keeps the server's contract simple.

### `handleScriptLoad` delegates to `PreloadScript`

The `EMB.SCRIPT LOAD` handler currently does inline validation and cache insertion. It will be refactored to call `PreloadScript` so both boot-time and client-time loading share exactly one validation/caching path.

### Compiler gets `Precompile(model, source) error`

A thin wrapper around the existing `compile` method that discards the returned proto (the cache already holds it). This warms the prototype cache at boot so the first `EVSHA` is fast.

- **Alternative**: Call `Compiler.Eval` with empty KEYS/ARGV at boot.
- **Why rejected**: That would require constructing dummy `Hosts`, which is unnecessary and could have side effects. `Precompile` only does parse+compile.

## Risks / Trade-offs

| Risk | Mitigation |
|---|---|
| A bad script at boot kills the server (availability risk) | Same as a bad model path — already fatal. Consistent with fail-fast config validation. |
| Preloading many large scripts increases boot time | Bound by existing `DefaultMaxScriptBytes` (64KB). Prototype cache is bounded (1024/model). |
| Preloaded scripts consume compiler cache slots | Same bound as dynamic scripts. LRU-style eviction on overflow. |
| `main.go` file-read error handling duplicates config logic | Keep it simple: `os.ReadFile` + `PreloadScript`. No special path resolution logic in `main.go` beyond the config-dir anchor. |

## Migration Plan

No migration needed — this is a purely additive config feature. Existing deployments without `scripts` are unaffected.

## Open Questions

None.
