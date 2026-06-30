## 1. Add libtokenizers.a to build system

- [x] 1.1 Add `libtokenizers` download recipe to `justfile` — detect platform, download pre-built archive from `daulet/tokenizers` releases, extract to `./lib/libtokenizers/`
- [x] 1.2 Update `flake.nix` — add `libtokenizers` derivation that downloads and extracts the pre-built archive, set `CGO_LDFLAGS` to include it
- [x] 1.3 Update `Dockerfile`:
  - Add `libtokenizers.linux-${LT_ARCH}.tar.gz` download alongside the ONNX Runtime download (same arch mapping: amd64→x86_64, arm64→aarch64)
  - Add `-L/opt/libtokenizers` to `CGO_LDFLAGS` in the build step
  - No runtime stage changes needed (static library is linked into the binary)
- [x] 1.4 Update `.github/workflows/ci.yml`:
  - Add step to download `libtokenizers.linux-x86_64.tar.gz` and extract to `/opt/libtokenizers`
  - Remove `./internal/tokenizer/...` from the existing `CGO_ENABLED=0` test step
  - Add a new CGo-enabled test step: `CGO_ENABLED=1 CGO_LDFLAGS="-L/opt/libtokenizers" go test -short ./internal/tokenizer/...`

## 2. Add daulet/tokenizers Go dependency

- [x] 2.1 Run `go get github.com/daulet/tokenizers@latest`
- [x] 2.2 Verify build: `CGO_ENABLED=1 CGO_LDFLAGS="-L./lib/libtokenizers" go build ./...`

## 3. Implement the new tokenizer

- [x] 3.1 Create `internal/tokenizer/reference.go`
- [x] 3.2 Remove `internal/tokenizer/huggingface.go` (entire file)

## 4. Update callers

- [x] 4.1 Update `internal/registry/registry.go` — change `tokenizer.NewHFTokenizer` → `tokenizer.NewTokenizer`

## 5. Verify

- [x] 5.1 `go build ./...` compiles
- [x] 5.2 `go vet ./...` passes
- [x] 5.3 `go test ./...` — all existing tests pass
- [x] 5.4 `golangci-lint run ./...` — zero issues
- [x] 5.5 Manual: emb starts, loads minilm model, `EMB.MODELS` returns correct dim=384
- [ ] 5.6 Run the Ruby PoC (`unsplash-api/tmp/emb-poc/run.sh`) — all cos-sim > 0.9999 (requires siglip2 model + Docker)
- [x] 5.7 Verify with minilm (WordPiece) via `just verify-embeddings` — 20/20 cosine=1.000000
- [x] 5.8 Docker build: `just docker-build` — builds successfully, libtokenizers downloads and links correctly
- [x] 5.9 Docker run: `docker run emb-server:latest` — starts, PING responds +PONG, no tokenizer errors
