## Why

The current tokenizer (`internal/tokenizer/huggingface.go`) is a hand-rolled implementation of WordPiece and BPE. This creates a maintenance burden: every HuggingFace tokenizer quirk, new model type, or tokenization edge case must be manually reimplemented and kept in sync with the upstream `huggingface/tokenizers` Rust library. The pure Go tokenizer only supports WordPiece and BPE — Unigram models (used by T5, ALBERT, XLNet) are not supported at all.

The Ruby PoC at `unsplash-api/tmp/emb-poc/` shows the intended validation flow: the Ruby `tokenizers` gem (wrapping the official Rust library) produces embeddings matching emb-server's output. But this only validates one model type. Every new model format requires manual scrutiny.

Using `daulet/tokenizers` (Go bindings for the official `huggingface/tokenizers` Rust library) means emb supports **any** HuggingFace tokenizer format out of the box — WordPiece, BPE, Unigram — with no manual reimplementation, no drift, no sync burden.

## What Changes

- Replace `internal/tokenizer/huggingface.go` (hand-rolled WordPiece + BPE, ~257 lines) with a thin wrapper around `daulet/tokenizers`
- The `Tokenizer` interface (`internal/tokenizer/tokenizer.go`) stays unchanged
- Add `libtokenizers.a` (Rust → C archive) as a build dependency alongside ONNX Runtime's `libonnxruntime.so`/`.dylib`
- Update Nix flake, Dockerfile, and justfile to build or download `libtokenizers.a`

## Capabilities

### New Capabilities

- `tokenizer`: Official HuggingFace tokenizers via `daulet/tokenizers` Go bindings

### Modified Capabilities

- `model-loading`: Tokenizer initialization path changes implementation (interface stays the same)

## Impact

| File | Change |
|------|--------|
| `internal/tokenizer/huggingface.go` | Removed (~257 lines) |
| `internal/tokenizer/tokenizer.go` | Unchanged (interface stays) |
| `internal/tokenizer/reference.go` | Added (thin CGo wrapper, ~40 lines) |
| `internal/registry/registry.go` | Import path update in `tokenizer.NewHFTokenizer` call |
| `flake.nix` | Add `libtokenizers` build input |
| `Dockerfile` | Multi-stage: download `libtokenizers.a` |
| `justfile` | Add `libtokenizers` download/build recipe |
| `go.mod` | Add `github.com/daulet/tokenizers` |

All existing tests must pass. The Ruby PoC should continue to show cos-sim > 0.9999.
