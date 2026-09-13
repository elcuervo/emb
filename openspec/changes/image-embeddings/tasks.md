# Tasks

All commands run inside `nix develop` (see AGENTS.md).

## 1. Configuration foundations

- [x] 1.1 Add `golang.org/x/image` to `go.mod` and verify `go build ./...` succeeds in the dev shell.
- [x] 1.2 Add an `image:` block to `config.ModelConfig` (input tensor, size, crop, resample, rescale, mean, std) plus top-level `max_command_bytes` and per-image byte cap keys; parse from YAML and verify with new `internal/config/config_test.go` cases.
- [x] 1.3 Validate the new blocks at load: reject a partial `image:` block, an unknown crop/resample value, and a mean/std length other than 3 with descriptive errors; verify with config tests.

## 2. Image preprocessing (`internal/imageproc`)

- [x] 2.1 Implement decoding for PNG/JPEG/GIF/WebP with a `DecodeConfig` pixel cap checked before full decode; verify with unit tests including a hostile-dimension fixture that is rejected.
- [x] 2.2 Implement resize + optional center crop + rescale + per-channel normalize producing `[1,3,H,W]` float32 RGB; verify shape, dtype, and channel-order tests.
- [x] 2.3 Verify determinism: identical bytes and config produce byte-identical tensors across repeated runs; add a unit test.
- [x] 2.4 Handle grayscale/RGBA inputs by converting to RGB; add a unit test.
- [x] 2.5 Add a golden preprocessing fixture captured from a Python/PIL reference and assert the Go tensors match within a documented tolerance; test compares against the stored fixture.

## 3. Image resources and `EMB.IMG`

- [x] 3.1 Add image resources to `ModelEntry` (a pool of named sessions + an immutable preprocessing plan) analogous to `ScriptResources`, with lazy and preload paths; verify with registry tests.
- [x] 3.2 Implement `handleIMG` parsing: `<model> [BLOB|VALUES] <bytes>...` with the format keyword only at position 2 when an image argument follows, and reject a URL argument with an error telling the client to fetch it; verify with server tests for keyword position, arity, and the URL-rejection message.
- [x] 3.3 Verify binary safety end-to-end: an image whose bytes contain NUL, `0xFF`, and CR/LF sequences is received intact and embedded (include a `redis-cli -x EMB.IMG <model>` test); add a server test.
- [x] 3.4 Implement bounded batching: preprocess N images and run batched `RunNamed` calls chunked by a fixed tensor budget; verify with a fake session asserting a single run for a batch that fits and multiple bounded runs beyond it.
- [x] 3.5 Enforce `max_images` truncation (default 4096, `0` = unlimited) with one reply slot per requested image and `null` overflow, plus per-image byte and decoded-pixel caps; verify with tests for truncation and for one bad image not failing the command.
- [x] 3.6 Enforce the dual-encoder pairing rule at model load: text and image outputs must agree on dimension, pooling, and normalization (output tensors may differ per branch), else fail with an error naming both dimensions; verify with a config test.
- [x] 3.7 Wire `EMB.IMG` into `EMB.HELP`, `EMB.STATS` (image requests, truncated images), and `MONITOR`; verify with server tests.

## 4. Reply formats

- [x] 4.1 Route `EMB.IMG` through the existing `BLOB`/`VALUES` writer; verify single-bulk, array, and VALUES `[m, dim]` envelopes under RESP2 and RESP3 in tests.
- [x] 4.2 Implement `EMB.IMGMULTI [BLOB|VALUES] <model> <bytes> ...` with alternating pairs, MGET-style per-pair nulls, ordering, and `max_pairs` truncation; verify with tests for two-model success, partial failure, truncation, and keyword position.
- [x] 4.3 Verify `EMB.IMGMULTI` stats count each pair as one request and `EMB.HELP` documents both image commands.

## 5. Caching

- [x] 5.1 Add content-addressed image cache keys `img:<model>:<sha256(bytes)>` on the image embed path (lookup and store); verify identical bytes hit and different bytes miss with cache tests.
- [x] 5.2 Verify `EMB.CACHE.FLUSH <model>` removes that model's image entries and leaves other models' entries intact; add a cache test.
- [x] 5.3 Verify image entries participate in the byte budget, LRU eviction, and eviction counters; add a cache test.

## 6. Scripting integration

- [x] 6.1 Add `emb.image.preprocess(bytes)` and `emb.image.info()` host functions backed by `internal/imageproc` and the model's image plan; register them in the sandbox and verify a script preprocesses image bytes and passes the result to `emb.run`.
- [x] 6.2 Add the packed `bytes` tensor input form to `emb.run`/`emb.run_batch` (`{shape, bytes, dtype}`), charge it against the existing per-tensor and request budgets, and validate the length against the shape; verify with tests including a round-trip through `emb.math.float32_bytes` and a length-mismatch error.
- [x] 6.3 Bound `script.CacheKey` for large payloads: digest a KEYS element above a small threshold while leaving short text keys byte-identical; verify with tests for bounded key size, distinct image entries, and unchanged short-text keys.
- [x] 6.4 Verify binary KEYS round-trip through `EMB.EVAL`/`EMB.EVSHA` unchanged and that the sandbox remains network-free (no `os`/`io`/`http`); add a server test.
- [x] 6.5 Add a reference image script (zero-shot classification or cross-modal ranking) under `examples/scripts/` and verify it runs end-to-end against a real model (gated, skips when the fixture is absent).

## 7. Command-size and payload bounds

- [x] 7.1 Enforce `max_command_bytes`: refuse a command (or a declared bulk) that exceeds the cap before buffering its payload, via a bounded connection reader or a small addition to the `redcon` fork; verify with a test sending an oversized declared length.
- [x] 7.2 Expose `max_command_bytes` and the image byte cap through `CONFIG GET`/`CONFIG SET` and the YAML config, rejecting invalid values; verify with config/command tests.

## 8. Autoconfiguration

- [x] 8.1 Detect the image input tensor and target size from the ONNX graph's image-input dimensions when `image.size` is unset; verify with an autoconfig test.
- [x] 8.2 Read `preprocessor_config.json` for rescale/mean/std/crop/resample/size, with explicit config taking precedence; verify override and detection tests.
- [x] 8.3 Fail model loading with a descriptive error when a required image field cannot be determined; verify with a config test.
- [x] 8.4 Add `preprocessor_config.json` to the `hfhub` extra-files download list; verify the download test or a fixture lists it.

## 9. Ruby client binary API

- [x] 9.1 Add an image embedding method to `gems/emb` that sends bytes as an `ASCII-8BIT` string unchanged through `send_command('EMB.IMG', ...)`; verify with a binary round-trip spec against the test server.
- [x] 9.2 Verify the existing float32 reply decode path (`unpack('e*')`) is reused unchanged for `EMB.IMG` replies; add a spec.

## 10. Documentation

- [x] 10.1 Update `README.md`: add `EMB.IMG`/`EMB.IMGMULTI` to the command table, add `emb.image.preprocess`/`emb.image.info` and the `bytes` tensor form to the script-surface table, and document the bytes-only boundary.
- [x] 10.2 Document the new config in `README.md` and `config.yaml`: the `image:` block, `max_command_bytes`, and the per-image byte cap; verify the example config loads with `-config`.
- [x] 10.3 Document the preprocessing-parity caveat (Go preprocessing is within tolerance, not bit-identical, of Python/PIL; do not mix pipelines for one index) and the dual-encoder pairing requirement.
- [x] 10.4 Update `EMB.HELP` to include both image commands and the `emb.image` block, and verify with a server test asserting the output.
- [x] 10.5 Add a SigLIP2 image-model config example and update `just download-model` guidance to fetch the vision export and `preprocessor_config.json`; verify the documented commands work.

## 11. End-to-end and validation

- [x] 11.1 Add a gated end-to-end test embedding a real image with a real SigLIP2/CLIP vision export (skip when the fixture is absent, mirroring the GLiNER test) and assert dimension and normalization.
- [x] 11.2 Add a cross-modal smoke test: text `EMB` and image `EMB.IMG` on the same model produce the same dimension and comparable vectors (a known image/text pair ranks sensibly).
- [x] 11.3 Run `just test`, `just lint`, and `just build` inside `nix develop` and confirm all pass.
