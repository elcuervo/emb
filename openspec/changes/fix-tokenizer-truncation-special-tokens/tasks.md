# Tasks

## 1. Shared truncation helper

- [x] 1.1 Add `truncatePreservingSpecialTokens(ids []int64, specialMask []uint32, maxLength int) []int64` (package-private in `internal/tokenizer`): keeps special tokens in place, truncates content to the remaining budget, no-op when `len(ids) <= maxLength`. Verify: unit-test the helper directly with synthetic ids/masks covering CLS/SEP (BERT), trailing-`<eos>` (siglip2), and `maxLength < specials` degenerate cases.
- [x] 1.2 Request `tokenizers.WithReturnSpecialTokensMask()` alongside `WithReturnAttentionMask()` in the encode call sites that need it, and thread the mask (compacted to real tokens, same window as `trimByMask`) into the helper. Verify: `go test ./internal/tokenizer/` passes; a test asserts the specials mask aligns with the attention-mask compaction for a padded encode.

## 2. Fix the embedding path

- [x] 2.1 `RefTokenizer.Encode`: replace the `ids[:maxLength]` cap (both `pad_output` branches) with the special-token-preserving truncation, so `realLen > maxLength` yields specials kept in place. Verify: new boundary unit test — encode `"7"*127` with a siglip2-style tokenizer at `maxLength=64` and assert the result is exactly `content[:63]` + `<eos>` (id `1`) at position 63, len 64; existing `reference_test.go` suite still green.

## 3. Fix the offsets path

- [x] 3.1 `slicesFromEncoding` (used by `EncodeOffsets`): apply the same special-token-preserving truncation via `truncatePairPart`-compatible slice semantics so offsets stay cardinally aligned with the truncated ids. Verify: an offsets test at the boundary returns ids whose tails match `Encode`'s truncated ids 1:1, and `go test ./internal/tokenizer/ ./internal/script/` passes.
- [x] 3.2 Confirm `EncodePairOffsets` and `EncodePretokenized` are intentionally unchanged (pair template already budgets specials; word-aligned path disables specials). Verify: no diff to those paths; existing pair/pretokenized tests pass unchanged.

## 4. Parity validation

- [x] 4.1 Re-run the 10,808-entry Ruby-vs-emb corpus parity harness and record results: expect tokenizer id parity 10,808/10,808 and the `cos < 0.99` count to drop from 127 to 0 (boundary entries move to ≥ 0.999 band). Verify: the harness report shows those counts for the `long_dense`/`numbers` classes and zero regression in realistic classes.
- [x] 4.2 Run `just test` (or `go test ./...` in `nix develop`) and confirm no regression in pipeline/server/image packages. Verify: full suite green with `-count=1`.