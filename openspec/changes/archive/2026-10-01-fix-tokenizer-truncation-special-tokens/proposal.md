# Proposal

## Why

When a sanitized input tokenizes to more than `maxLength` real tokens, `RefTokenizer.Encode` truncates with a plain Go-side slice (`ids[:maxLength]`), which drops the trailing special tokens the tokenizer's post-processor appends (e.g. siglip2's `<eos>`). The reference stacks keep those specials and truncate content only: Ruby's `enable_truncation(64)` and the `huggingface/tokenizers` Python library both reserve the special-token slots. For siglip2 the pooled vector is `Gather(-1)` — position 63 — so Ruby pools the `<eos>` token while emb pools a content token. Measured over a 10,808-entry diverse corpus: 127 entries (1.2%, all in the >64-real-token class) diverge with cosine down to 0.968 and maxAbsDiff 3.3e-2 — the single deterministic parity gap between emb and another production embedding runtime, and the same mechanism applies to any prepooled model at its `max_length` boundary (e5/hyperclusters at 512).

## What Changes

- `internal/tokenizer` truncation becomes special-token-preserving: when `realLen > maxLength`, keep all special tokens (via the tokenizer's `special_tokens_mask`) in place and truncate content only — replicating the official library's truncation-with-special-tokens semantics.
- Applied to the embedding path (`Encode`, both `pad_output` branches) and the single-sequence script-offsets path (`slicesFromEncoding`/`truncatePairPart`), keeping offsets aligned with the truncated ids.
- `EncodePairOffsets` is unchanged (it already budgets the `[CLS] A [SEP] B [SEP]` template explicitly); `EncodePretokenized` (word-aligned, specials disabled) is unchanged.
- No behavior change for inputs at or under `maxLength` — the common case is byte-for-byte identical to today.
- Adds boundary parity tests and re-runs the corpus validation (expects id parity 10,808/10,808 and the `cos < 0.99` set to drop from 127 to 0).

## Capabilities

### New Capabilities

None.

### Modified Capabilities

- `tokenizer`: the truncation requirement changes from "sequences SHALL be truncated to `maxLength`" (slice semantics) to special-token-preserving truncation matching the official library.

## Impact

- Code: `internal/tokenizer/reference.go` (`Encode`), `internal/tokenizer/pairs.go` (`slicesFromEncoding`, `truncatePairPart`), plus shared helper and unit tests in `internal/tokenizer/`.
- Behavior: only inputs that exceed `maxLength` real tokens change output; every other input is unaffected (verified by the corpus run).
- Dependents: embedding pool, script `emb.tokenize.encode` surface (via `EncodeOffsets`), batch padding (unchanged: real token count still ≤ `maxLength`, pads/masks unchanged).
- Reference parity: closes the measured 0.968–0.972 divergence class against Ruby and the Python reference; does not change int8 kernel-tolerance drift (~4.2e-3), which is orthogonal.