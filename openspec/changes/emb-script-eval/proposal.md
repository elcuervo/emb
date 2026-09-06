## Why

`emb` can only run text-embedding ONNX models through the BERT-shaped pipeline (tokenize → encode → pool → float32 bytes). Non-embedding models — first target: GLiNER-style extractive models like `cuerbot/gliner2-multi-v1`, which need 7 named input tensors (schema baked into the sequence) and return structured spans instead of vectors — cannot be mounted, so `emb` cannot serve entity extraction alongside embeddings. Users want a Redis-style script surface to bridge arbitrary ONNX graphs: pass the script and its parameters per request, keep the RESP2 signature, and get Redis-native reply constructs (hash, array, bulk) back.

## What Changes

- **New `EMB.EVAL` / `EMB.EVSHA` / `EMB.SCRIPT` command family**, Redis-shaped: script identity by SHA1, cached per model; `numtexts` disambiguates texts (KEYS) from args (ARGV) exactly like Redis `numkeys`.
- **Sandboxed Lua (gopher-lua, Lua 5.1 semantics)** as the scripting engine; fresh interpreter per evaluation (no shared state, no global blocking — a busy script fails only its own request); wall-clock + step budgets, script-size cap.
- **Whitelisted host functions**: `emb.run`, `emb.tokenize.pretokenized`, `emb.tokenize.words`, `json`, safe stdlib subset (no `io`/`os`/network/FFI/`math.random`) — scripts are pure compute, so replies are deterministic and cacheable. Cross-model composition (`emb.embed` of another model) is deferred.
- **Lua→RESP2 conversion contract**: string→bulk (byte-safe, covers the float32 embedding path), number→integer, list table→array, string-keyed table→**hash** (flat field/value pairs, HGETALL-shaped), nil→null, `{err=…}`→error, arbitrary nesting.
- **Content-addressed caching**: `model:sha1(script):args:text` → converted RESP bytes, extending the existing LRU cache; schema/param changes are just new cache keys.
- **Generic tensor plumbing behind the scenes**: `onnx.RunNamed` (arbitrary named int64/float32 tensors in/out, additive — the fast BERT path untouched), `tokenizer.EncodePretokenized` (word-level encode + word_ids), and a scripted execution path alongside the BERT pool.
- **Zero config changes** — the same `models:` YAML mounts the ONNX binary as today; scripts and labels are request-time only.
- **Generic building blocks, not maintained adapters**: the server ships reusable host primitives (`emb.tokenize.words`, `emb.tokenize.pretokenized`, `emb.run`, `json`); model-specific glue stays in user scripts. `examples/scripts/gliner2.lua` demonstrates the pattern.
- **Testbed**: `cuerbot/gliner2-multi-v1` served end-to-end via an example `gliner2.lua` script built from the blocks, with dynamic labels in ARGV, validated by golden fixtures generated from the model itself (Go golden-file pattern; the Python fixture generator requires torch and is not run in CI).

## Capabilities

### New Capabilities
- `script-eval`: Redis-style scripted model evaluation — `EMB.EVAL`/`EMB.EVSHA`/`EMB.SCRIPT` command surface, per-model SHA1 script cache, KEYS/ARGV semantics, sandbox and execution budgets, Lua→RESP2 reply conversion, content-addressed caching, and the GLiNER extraction reference behavior.

### Modified Capabilities
(none — config shape, existing commands, and the embedding path are unchanged; the new capability is purely additive)

## Impact

- **`internal/server`**: new command handlers (`handleEMBEVAL`, `handleEVSHA`, `handleSCRIPT`), script cache ownership, Lua→RESP writer.
- **`internal/onnx`**: `RunNamed` generic multi-tensor session API (additive; `Session.Run`/`RuntimeSession` unchanged).
- **`internal/tokenizer`**: `EncodePretokenized` capability (word-level encode with `word_ids` alignment) on the existing tokenizers wrapper.
- **new `internal/script`** package: engine wrapper (gopher-lua), sandbox/whitelist/budgets, per-model script cache, return-value→RESP conversion, content-addressed cache keys.
- **`internal/registry`**: `ModelEntry` executes via an interface (embed pool or scripted path); scripted runs reference config-mounted models by name.
- **`cmd/emb` / `EMB.HELP`**: document the new command family and reply grammar.
- **`gems/emb`**: `eval`/`evalsha`/`script.load` client methods; typed decoding of hash replies (embed batch decoding untouched).
- **Dependencies**: `github.com/yuin/gopher-lua` (new); testbed model `cuerbot/gliner2-multi-v1` (download at test time).