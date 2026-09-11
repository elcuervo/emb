## Why

Clients currently must `EMB.SCRIPT LOAD` every script before they can `EMB.EVSHA` it. This forces a bootstrapping round-trip per script and means the first `EVSHA` pays the Lua parse/compile cost. For deployments with known scripts (e.g. GLiNER extraction pipelines, classification scripts), the server should accept script file paths in model config and preload them at boot so their SHAs are immediately available and the compiled prototypes are already warm.

## What Changes

- Add `scripts: []string` to `ModelConfig` in the YAML config, each value a file path to a Lua script.
- Resolve relative script paths against the config file's directory; absolute paths are used as-is.
- At boot, read each script file, validate it (syntax, size), load it into the per-model script cache, and precompile the Lua prototype.
- A bad script (missing file, invalid Lua, oversized) is fatal at boot — the server refuses to start half-configured, matching existing model validation semantics.
- Preloaded scripts are indistinguishable from dynamically loaded ones: `EMB.SCRIPT EXISTS` reports 1, `EMB.EVSHA` works immediately, and `EMB.SCRIPT FLUSH` drops them like any other cache entry.

## Capabilities

### New Capabilities
- `script-preload`: Server preloads Lua scripts from file paths declared in model config at boot time, validates them, and warms the source and compiled-prototype caches so `EMB.EVSHA` is ready immediately.

### Modified Capabilities
- `script-eval`: The script cache may be pre-populated at boot by the server (not just by client `EMB.SCRIPT LOAD`). `EMB.SCRIPT EXISTS` and `EMB.EVSHA` behave identically regardless of how a script entered the cache.

## Impact

- `internal/config/config.go` — new `Scripts []string` field, path resolution in `Load()`.
- `internal/server/server.go` — new `PreloadScript(model, src) (string, error)` method.
- `internal/server/script.go` — refactor `handleScriptLoad` to delegate to `PreloadScript`.
- `internal/script/compiler.go` — new `Precompile(model, source) error` for boot-time prototype warming.
- `cmd/emb/main.go` — read script files and call `PreloadScript` before `SetReady()`.
- `internal/server/script_test.go` — tests for boot-time preload, EXISTS, EVSHA, invalid script fatal.
