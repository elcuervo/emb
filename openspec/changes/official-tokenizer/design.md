## Context

The hand-rolled tokenizer was written early in the project when the goal was minimal dependencies. Since then, emb has accumulated CGo dependencies (ONNX Runtime via `yalue/onnxruntime_go`). A second CGo dependency (the Rust tokenizers library via `daulet/tokenizers`) doesn't add a new category of build complexity — it extends an existing one.

The `daulet/tokenizers` library:
- Wraps the official `huggingface/tokenizers` Rust library via CGo
- Provides `tokenizers.FromFile(path)` → loads any HF tokenizer.json
- Provides `Encode(text, addSpecialTokens)` → returns `[]uint32` IDs
- Provides `EncodeWithOptions` → returns IDs, attention mask, type IDs, tokens, offsets
- Requires `libtokenizers.a` — a static archive built from Rust sources

## Goals / Non-Goals

**Goals:**
- Replace hand-rolled WordPiece + BPE with the official HuggingFace tokenizers library
- Support all HuggingFace tokenizer formats (WordPiece, BPE, Unigram)
- Keep the `Tokenizer` interface unchanged
- Maintain byte-identical embedding output for existing models (minilm, siglip2)
- Add `libtokenizers.a` build/download to Nix, Docker, and justfile

**Non-Goals:**
- Changing the `Tokenizer` interface or `Encode()` signature
- Changing the registry, pipeline, or server code beyond the import path
- Adding new tokenizer features (padding, truncation options — already handled by the pipeline)
- Performance optimization of tokenization (the Rust library is faster anyway)

## Decisions

### Approach: replace, don't dual-path

Don't keep both tokenizers behind a build tag or config flag. Dual-path introduces testing burden, config surface, and the very drift this change aims to eliminate. The hand-rolled tokenizer is removed entirely.

### libtokenizers.a acquisition: pre-built binary download

Three options for getting `libtokenizers.a`:

| Option | Build time | Complexity | Reproducibility |
|--------|-----------|------------|-----------------|
| Build from Rust source (via `make build`) | Slow (Rust compile) | High (Rust toolchain) | Full |
| Download pre-built release binary | Fast (curl) | Low | Partial (pinned version) |
| Vendored in repo | Instant | Lowest | Full but bloated |

**Decision: Download pre-built binaries pinned to a `daulet/tokenizers` release version.** This matches how ONNX Runtime is handled (pre-built shared library from GitHub releases). The makefile/justfile downloads and extracts `libtokenizers.darwin-arm64.tar.gz` (or the appropriate platform variant). For Nix, the flake packages the same pre-built archive.

### Tokenizer initialization

Current flow in `registry.go`:
```go
tok, err := tokenizer.NewHFTokenizer(cfg.Tokenizer)
```

New flow:
```go
tok, err := tokenizer.NewTokenizer(cfg.Tokenizer)
```

`NewHFTokenizer` is renamed to `NewTokenizer` or replaced. The underlying implementation changes from pure Go to the CGo wrapper. The return type still satisfies the `Tokenizer` interface.

### The Encode() signature

`daulet/tokenizers.Encode(text, addSpecialTokens)` returns `[]uint32`. The current interface returns `(inputIDs, attnMask []int64, err error)`. The adapter layer handles:
1. Call `tk.Encode(text, true)` (with special tokens = CLS/SEP)
2. Convert `[]uint32` → `[]int64` for input IDs
3. Build attention mask (all 1s, matching length)
4. Truncate to `maxLength`

### Docker: static linking vs ONNX Runtime shared library

`libtokenizers.a` is a **static library** — it's linked directly into the Go binary at build time. This is simpler than ONNX Runtime (shared `.so`), which must be copied to the runtime stage and registered with `ldconfig`. The Dockerfile change is:

```dockerfile
# In builder stage, alongside the ONNX Runtime download:
RUN set -eux; \
    case ${TARGETARCH} in \
      amd64) LT_ARCH=x86_64 ;; \
      arm64) LT_ARCH=aarch64 ;; \
    esac; \
    curl -fsSL "https://github.com/daulet/tokenizers/releases/download/v1.27.0/libtokenizers.linux-${LT_ARCH}.tar.gz" \
      -o /tmp/libtokenizers.tgz; \
    mkdir -p /opt/libtokenizers; \
    tar xzf /tmp/libtokenizers.tgz -C /opt/libtokenizers; \
    rm /tmp/libtokenizers.tgz

# Updated build flags:
RUN CGO_ENABLED=1 \
    CGO_CFLAGS="-I${ORT_DIR}/include" \
    CGO_LDFLAGS="-L${ORT_DIR}/lib -lonnxruntime -L/opt/libtokenizers" \
    go build -o /emb ./cmd/emb
```

No changes to the runtime stage — `libtokenizers.a` is baked into the binary. The release workflow (`docker buildx` for linux/amd64 + linux/arm64) works unchanged since the download is per-arch inside the Dockerfile.

### CI: CGo must be enabled

Current CI (`ci.yml`) sets `CGO_ENABLED: "0"` and tests `./internal/tokenizer/...`. With `daulet/tokenizers`, the tokenizer package requires CGo. Options:

| Option | Pros | Cons |
|--------|------|------|
| Download libtokenizers.a + enable CGo for tokenizer tests | Full test coverage | CI needs the pre-built binary |
| Gate tokenizer tests behind build tag | No CI changes | Tokenizer untested in CI |
| Split: CGo-free packages tested as before, tokenizer in separate step | Clean separation | More CI config |

**Decision: Download `libtokenizers.a` in CI and enable CGo for the tokenizer test step.** The download is a single `curl` + `tar` command, same as the Dockerfile. The existing `CGO_ENABLED: "0"` env var is kept at the job level; the tokenizer test step overrides it:

```yaml
- name: Install libtokenizers
  run: |
    curl -fsSL "https://github.com/daulet/tokenizers/releases/download/v1.27.0/libtokenizers.linux-x86_64.tar.gz" \
      -o /tmp/libtokenizers.tgz
    sudo mkdir -p /opt/libtokenizers
    sudo tar xzf /tmp/libtokenizers.tgz -C /opt/libtokenizers

- name: Test tokenizer
  run: go test -short ./internal/tokenizer/...
  env:
    CGO_ENABLED: "1"
    CGO_LDFLAGS: "-L/opt/libtokenizers"
```

### Encoded output comparison

The `daulet/tokenizers` library adds [CLS]/[SEP] when `addSpecialTokens=true`. The hand-rolled tokenizer also adds CLS/SEP tokens. These should produce identical token ID sequences for BERT/WordPiece models. The Ruby PoC confirms this via the cos-sim > 0.9999 test for SigLIP2.

For BPE models (e.g., `intfloat/e5-small-v2`), the hand-rolled BPE implementation was simplified (character-level merging by vocab ID ordering) and likely differs from the official implementation. The `daulet/tokenizers` library will be the ground truth.

## Risks / Trade-offs

- [Adds a second CGo dependency] → Already have ONNX Runtime CGo. Same category of build complexity. The pre-built binary avoids requiring a Rust toolchain.
- [Platform availability of pre-built libtokenizers.a] → `daulet/tokenizers` publishes for darwin arm64/x86_64, linux arm64/amd64/s390x/ppc64le. Covers all platforms emb targets.
- [Rust library version mismatch with HF ecosystem] → Pinning to a `daulet/tokenizers` release pins the underlying Rust `tokenizers` version. Update periodically.
- [Removing the pure Go tokenizer means no fallback] → If `libtokenizers.a` is missing, emb won't start. The error message should clearly indicate the missing library. This is the same pattern as ONNX Runtime.
- [CI currently runs entirely CGo-free] → Tokenizer tests must move to a CGo-enabled step with `libtokenizers.a` downloaded. This is a one-time setup cost.
- [Docker multi-arch builds work but need a second download per arch] → Both ONNX Runtime and `libtokenizers.a` are downloaded per-arch in the Dockerfile. Two downloads instead of one, but same pattern.
