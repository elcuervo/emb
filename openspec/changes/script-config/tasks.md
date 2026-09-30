# Tasks

## 1. Config schema

- [x] 1.1 Replace `ModelConfig.Scripts []string` with `Scripts []ScriptEntry`, `ScriptEntry{Path string; Config map[string]any}` and a `UnmarshalYAML` that accepts a bare scalar as `{path: <value>}`; resolve `Path` against the config directory as today.
- [x] 1.2 Validate at boot: `config` must be a mapping, its numbers finite, and the encoded size ≤ 64 KiB; a violation is fatal and names the script path.
- [x] 1.3 Add config parse tests covering the shorthand, the mapping form, the resolution rules, and each rejection.

## 2. Host exposure

- [x] 2.1 Add `Config map[string]any` to `script.Hosts`; `registerHosts` builds `emb.script.config` (empty table when absent) and adds the `emb.script` namespace.
- [x] 2.2 Confirm `emb.script` is reachable from `EMB.EVAL` (client-supplied scripts get an empty config) and from preloaded scripts (their own config), with a host test for each.

## 3. Cache identity

- [x] 3.1 Fold a digest of the entry's config (computed once at load) into `script.CacheKey`, so editing a config misses rather than serving a stale reply.
- [x] 3.2 Test: same request, changed config → miss; unchanged config → hit; no-config models keep byte-identical keys to today.

## 4. First consumer and generality

- [x] 4.1 Update `scripts/laya.lua` to read `emb.script.config` as its defaults and treat `ARGV[2]` as an override; the parity corpus (`internal/server/laya_parity_test.go`) still passes with the envelope moved out of the call.
- [x] 4.2 Move the envelope onto the model entry in `test-laya.yaml`, `website/repl/sandbox.yaml`, and the website-dev derivation; boot + `EMB.EVSHA laya <sha> 1 <state> <questions>` smoke test.
- [ ] 4.3 Re-express `gliner2.lua`, `classify.lua`, and `zeroshot.lua` with their model literals in config, proving the mechanism is not Laya-shaped. **Not done** — the mechanism is exercised by two presets (`laya.lua`, `snake.lua`) and the shape tests; the other three still take their constants as ARGV/hardcoded. Doing this moves those presets' digests and the plates that call them, so it belongs with the plate that owns each one.
- [x] 4.4 Document `emb.script.config` (the rule for config vs ARGV, the size/shape bounds) in `website/docs/index.html` section 10 and the plate's THE API section. (The README's Laya section still shows the `ARGV[2]` form; update it with the next README touch.)

## 5. Validation

- [x] 5.1 `just lint` (0 issues), `go test ./...`, `openspec validate script-config`; `just website-presets-check` passes (laya's digest moved, snake's is new, and both are stamped).
