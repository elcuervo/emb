# Public Release Pipeline

## Overview

```
                   ┌─────────────────┐
                   │  Create tag v*   │
                   │  (manual / CI)   │
                   └────────┬────────┘
                            │
                            ▼
                   ┌─────────────────┐
                   │  GitHub Release  │
                   │  (draft/prerelease)│
                   └────────┬────────┘
                            │
                   ┌────────▼────────┐
                   │  Release CI     │
                   │  (triggered)    │
                   └────────┬────────┘
                            │
               ┌────────────┼────────────┐
               ▼            ▼            ▼
        ┌──────────┐ ┌──────────┐ ┌──────────┐
        │  Test     │ │ Docker   │ │ GoRel    │
        │  (CGo)    │ │ Build    │ │ (macOS)  │
        └──────────┘ └──────────┘ └──────────┘
                            │            │
                            ▼            ▼
                     ┌──────────┐ ┌──────────┐
                     │  Smoke   │ │  Smoke   │
                     │  Test    │ │  Test    │
                     └──────────┘ └──────────┘
                            │            │
                            └─────┬──────┘
                                  ▼
                         ┌─────────────────┐
                         │  Publish to GH  │
                         │  Release / DH   │
                         └─────────────────┘
                                  │
                                  ▼
                         ┌─────────────────┐
                         │  Release notes  │
                         │  auto-generated │
                         └─────────────────┘
```

## Single Source of Truth for Versions

**Problem**: ONNX Runtime version and libtokenizers version are duplicated across:
- `release.yml` (env: `ORT_VERSION`)
- `Dockerfile` (hardcoded `v1.27.0` in URL)
- `justfile` (hardcoded `libtokenizers-version`)

**Solution**: One `.github/versions.env` file, sourced by all workflows and the justfile:

```
ORT_VERSION=v1.27.0
TOKENIZERS_VERSION=v1.27.0
```

Each consumer reads from this file. The justfile can `include` or `source` it.

## Release Notes Automation

GoReleaser has a built-in changelog generator. Supplement it with:

- A "Changes included" section listing OpenSpec changes active since last release
- A "Known issues" section from the milestone

This can be done via a `gh` script in a workflow step that queries `openspec list --json` and formats the output.

## Smoke Test Protocol

After building artifacts for each platform:

1. Extract/pull the compiled artifact
2. Start the server on a random port
3. Download a known model (all-MiniLM-L6-v2) via HF hub or pre-packaged
4. Run `EMB.HELLO` → expect pong
5. Run `EMB.INFER` with a test sentence → expect valid embedding
6. Verify embedding dimension matches model config
7. If `normalize: true`, verify output is unit length
8. Shut down

## Dependency URL Mapping

| Upstream | Arch | ONNX asset name | tokenizers asset name |
|----------|------|-----------------|-----------------------|
| amd64 | x86_64 | `onnxruntime-linux-x64-{ver}.tgz` | `libtokenizers.linux-x86_64.tar.gz` |
| arm64 | aarch64 | `onnxruntime-linux-aarch64-{ver}.tgz` | `libtokenizers.linux-aarch64.tar.gz` |
| macOS arm64 | arm64 | `onnxruntime-osx-arm64-{ver}.tgz` | `libtokenizers.darwin-arm64.tar.gz` |
| macOS amd64 | x86_64 | `onnxruntime-osx-x86_64-{ver}.tgz` via brew | `libtokenizers.darwin-x86_64.tar.gz` |
