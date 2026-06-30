## Context

The `emb` binary currently requires a YAML config file. The only CLI flags are `-config` and `-ort-lib`. A new user must write a config file before they can run anything. This adds friction to evaluation and quick-start.

The config struct already has all the fields needed; we just need to expose them as flags and make the config file optional.

## Goals / Non-Goals

**Goals:**
- Run `emb -model-repo <repo>` with no config file
- All `Config` and `ModelConfig` fields exposed as CLI flags
- `-version` flag prints build version
- Backward compatible — existing `-config` flag still works
- README shows a single-line example

**Non-Goals:**
- Hot-reload or runtime flag changes
- Windows support for the one-liner

## Decisions

### Decision: Multi-model via repeated `-model` groups

When `-config` is omitted, each `-model` flag starts a new model definition. Flags after `-model` apply to that model until the next `-model`:

```
emb \
  -model minilm -model-onnx ./minilm.onnx \
  -model bge -model-repo Xenova/bge-small-en-v1.5
```

This registers two models: `minilm` (from a local ONNX file) and `bge` (auto-downloaded from HuggingFace). The `-model` flag itself sets the model key name; if only one model, the default key is `"model"`.

### Decision: Flag prefix pattern

Model-level flags use a `-model-` prefix to namespace them from server-level flags:

| Flag | Maps to | Default |
|------|---------|---------|
| `-listen` | `Config.Listen` | `:6379` |
| `-model` | model key name | `model` |
| `-model-onnx` | `ModelConfig.ONNX` | `""` |
| `-model-repo` | `ModelConfig.ModelRepo` | `""` |
| `-model-tokenizer` | `ModelConfig.Tokenizer` | `""` |
| `-pooling` | `ModelConfig.Pooling` | `""` |
| `-normalize` | `ModelConfig.Normalize` | `false` |
| `-dim` | `ModelConfig.Dim` | `0` |
| `-max-length` | `ModelConfig.MaxLength` | `0` |
| `-output-tensor` | `ModelConfig.OutputTensor` | `""` |
| `-pad-output` | `ModelConfig.PadOutput` | `false` |
| `-workers` | `ModelConfig.Workers` | `0` |
| `-intra-op-threads` | `ModelConfig.IntraOpThreads` | `0` |
| `-inter-op-threads` | `ModelConfig.InterOpThreads` | `0` |

### Decision: Config loading logic

```
if -config is set:
    load YAML from path
    apply -listen override (if set)
else:
    collect models by splitting flags on each -model occurrence
    build Config from flag values
    require at least one model with -model-onnx or -model-repo
```

### Decision: Version via ldflags

The `version` variable in `cmd/emb/main.go` will be set at build time:
```
go build -ldflags="-X main.version=v0.1.0" ./cmd/emb
```

The binary prints the version when `-version` is passed and exits.

## Risks / Trade-offs

- **Flag explosion** — 15+ flags is a lot. Mitigation: `-config` still exists for complex setups. Flags are for quick-start only.
- **Flag/YAML divergence** — if a new field is added to `ModelConfig` but not exposed as a flag, the flag-only path won't support it. Mitigation: add flags alongside config fields.
- **Multi-model flag parsing is positional** — `-model` must come before the model-specific flags it groups. Mistaken ordering produces errors at config build time.
