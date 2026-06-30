## Why

The Go `emb` server currently strips trailing [PAD] tokens from tokenizer output and passes the correct attention mask to ONNX Runtime. This produces mathematically correct embeddings. However, the existing Ruby `Siglip2Text` implementation (`unsplash-api/lib/siglip2_text.rb`) was written without an attention mask — it passes padded token sequences to ONNX with no mask, defaulting to all-1s attention.

This means emb's correct output doesn't match the Ruby production output. All vectors previously indexed by `Siglip2Text.encode` would need re-indexing if emb were deployed with the current correct behavior.

The solution: a per-model `pad_output` config option. When `true`, the tokenizer leaves trailing [PAD] tokens in place (matching the Ruby behavior), and the pipeline's `PadEncodings` sets attention mask to all-1s for all positions — producing identical (equally slightly-wrong) output. When `false` (default), emb uses the correct behavior: strip padding and pass proper attention mask.

## What Changes

- Add `PadOutput bool` to `ModelConfig` (default: `false`)
- Thread the flag into the tokenizer: `tokenizer.NewTokenizer(path, padOutput)`
- Add condition to `reference.go` — when `padOutput` is `true`, don't strip padding, pad to `maxLength`
- Update `registry.go` to pass `cfg.PadOutput` when creating the tokenizer
- Update `README.md` with the new config option

## Modified Capabilities

- `config`: new `pad_output` field
- `tokenizer`: `NewTokenizer` signature change, conditional padding in `Encode`
- `model-loading`: tokenizer initialization passes config value

## Impact

| File | Change |
|------|--------|
| `internal/config/config.go` | Add `PadOutput bool` to `ModelConfig` |
| `internal/tokenizer/tokenizer.go` | No change (interface stays) |
| `internal/tokenizer/reference.go` | Add `padOutput` field to `RefTokenizer`, conditional logic in `Encode` |
| `internal/registry/registry.go` | Pass `cfg.PadOutput` to `tokenizer.NewTokenizer` |
| `README.md` | Document `pad_output` config option |
