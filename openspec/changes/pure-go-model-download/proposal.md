## Why

The `model_repo` download flow currently shells out to `optimum-cli export onnx`, requiring Python, PyTorch, and the `optimum` library at runtime. This adds 2+ GB of dependencies, fails in environments without Python (e.g., minimal Docker images), and is fragile (version mismatches, network issues, silent failures). Many popular embedding models have pre-converted ONNX files on HuggingFace — downloading these directly via HTTP is simpler, faster, and has zero external dependencies.

## What Changes

- Replace `exec.Command("optimum-cli")` in `registry.downloadModel` with pure Go HTTP calls to HuggingFace Hub API
- Add `internal/hfhub` package: HuggingFace Hub API client that lists and downloads model files
- Downloader discovers ONNX files via the Hub API (`/api/models/{repo}`), downloads `model.onnx`, `tokenizer.json`, and related files
- Falls back gracefully: if the repo has no ONNX files, returns a clear error with instructions for manual export
- Removes Python + optimum dependency from the `model_repo` download path
- Updates `just download-model` target to use the new flow (no more venv + pip)

## Capabilities

### New Capabilities

- `huggingface-model-download`: Pure Go HuggingFace Hub model download via REST API — no Python, no shell, no external dependencies

### Modified Capabilities

- `model-loading`: `model_repo` field now downloads pre-converted ONNX files via HTTP instead of running optimum-cli

## Impact

Files: `internal/registry/registry.go` (rewrite `downloadModel`), new `internal/hfhub/` package, `justfile` (update `download-model` recipe), `go.mod` (no new dependencies). Backward compatible — existing configs with `model_repo` continue to work, but fail differently if the repo has no ONNX files (clear error vs. optimum-cli crash).
