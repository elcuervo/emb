## 1. Add pad_output to config

- [x] 1.1 Add `PadOutput bool \`yaml:"pad_output"\`` to `ModelConfig` in `internal/config/config.go`

## 2. Thread pad_output through tokenizer

- [x] 2.1 Add `padOutput bool` field to `RefTokenizer` struct
- [x] 2.2 Update `NewTokenizer(path string, padOutput bool)` signature
- [x] 2.3 In `Encode`: when `padOutput` is `true`, skip padding stripping and pad to `maxLength` with all-1s mask
- [x] 2.4 When `padOutput` is `false` (default), keep current behavior (strip padding, correct mask)

## 3. Update registry

- [x] 3.1 In `ensurePool()`, pass `cfg.PadOutput` to `tokenizer.NewTokenizer(cfg.Tokenizer, cfg.PadOutput)`

## 4. Verify

- [x] 4.1 `go build ./...` — compiles
- [x] 4.2 `go vet ./...` — passes
- [x] 4.3 `go test ./...` — all pass
- [x] 4.4 `just verify-embeddings` — still passes 20/20 (minilm uses default `pad_output: false`)
- [ ] 4.5 Check: with `pad_output: true`, the siglip2 output matches Ruby byte-for-byte (requires Ruby PoC + siglip2 model)
