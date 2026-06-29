## Context

The current `downloadModel` in `registry.go` uses `exec.Command("optimum-cli", "export", "onnx", ...)` which requires Python, PyTorch, and the `optimum` library. This is a heavy dependency for a simple file download, fails in minimal environments, and adds ~2-3 seconds of overhead on every model load.

## Goals / Non-Goals

**Goals:**
- Download ONNX models from HuggingFace Hub using pure Go, no external dependencies
- Auto-detect available ONNX files via the Hub API (`/api/models/{repo}`)
- Download `model.onnx`, `tokenizer.json`, `config.json`, and tokenizer support files
- Works in the minimal Docker image (no Python required)
- Backward compatible with existing `model_repo` configs
- Clear error messages when repo has no ONNX files

**Non-Goals:**
- Converting PyTorch models to ONNX (that's what optimum-cli is for, and it's out of scope)
- Downloading every quantized variant (just the standard `model.onnx`)
- Authentication (public models only)
- Partial downloads / resume support (the models are 50-200MB, one-shot download is fine)

## Decisions

### HuggingFace Hub API

Two endpoints:

| Endpoint | Purpose |
|---|---|
| `GET /api/models/{repo}` | List files in the repo (returns siblings array with `rfilename`) |
| `GET /{repo}/resolve/main/{file}` | Download a specific file |

No authentication needed for public models. Rate limiting is generous enough for server startup downloads.

### ONNX file discovery

The API response lists all files. The downloader filters for `*.onnx` files. Priority:
1. `model.onnx` — standard name
2. `onnx/model.onnx` — optimum export subdirectory
3. First `.onnx` file found (sorted alphabetically)

If no `.onnx` files exist, return a clear error: "No ONNX files in repo X. Use optimum-cli to export manually."

### File caching

Downloaded files are saved to the directory derived from `onnx` path (already handled by existing `downloadModel` structure). The `resolveModelConfig` step that infers `dim`, `max_length`, and `tokenizer` path runs after download — no changes needed there.

### Justfile update

`just download-model` currently creates a venv and installs optimum. Simplified to just run the emb server's download logic, or use a simple curl-based approach.

### Implementation structure

New package: `internal/hfhub/`

```go
type Client struct {
    HTTPClient *http.Client
    BaseURL    string // for testing: can point to test server
}

func (c *Client) ListFiles(repo string) ([]FileInfo, error)
func (c *Client) Download(repo, filePath, dest string) error
```

`registry.downloadModel` is rewritten to use `hfhub.Client`.

## Risks / Trade-offs

- [Not all models have ONNX files] → Clear error message pointing to manual optimum-cli usage. Users of non-converted models can still download and export manually.
- [Rate limiting on HuggingFace API] → Unlikely for server startup downloads (1-2 model fetches). If it becomes an issue, users can pre-download models.
- [Large download delays server startup] → Models are 50-200MB. Download time is comparable to optimum-cli export time (which also downloads from HuggingFace first).
