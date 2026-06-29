## 1. HuggingFace Hub Client

- [x] 1.1 Create `internal/hfhub/hfhub.go` with `Client` struct, `ListFiles`, `Download`, `DownloadModel`, `FindONNX`
- [x] 1.2 Implement `ListFiles` using `GET /api/models/{repo}` — parse siblings for `.onnx` files
- [x] 1.3 Implement `Download` using `GET /{repo}/resolve/main/{file}` — stream to destination file
- [x] 1.4 Add ONNX file discovery: try `model.onnx`, `onnx/model.onnx`, then first `.onnx` sibling

## 2. Registry Rewrite

- [x] 2.1 Rewrite `registry.downloadModel` to use `hfhub.Client` instead of `exec.Command("optimum-cli")`
- [x] 2.2 Remove `"os/exec"` import from `registry.go`
- [x] 2.3 Add `"github.com/elcuervo/emb/internal/hfhub"` import

## 3. Justfile Update

- [x] 3.1 Update `just download-model` recipe to use curl directly (no Python/optimum)
- [x] 3.2 Default repo changed to `Xenova/all-MiniLM-L6-v2` (has pre-converted ONNX)

## 4. Verification

- [x] 4.1 Build and vet: `go build ./...` and `go vet ./...` pass
- [x] 4.2 Run `just test` — all tests pass
- [x] 4.3 Run `just lint` — zero issues
- [x] 4.4 Test with `model_repo: Xenova/all-MiniLM-L6-v2` — full download + EMB works
