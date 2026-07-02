## Why

The `emb-server` gem depends on `onnxruntime ~> 0.9`, but the Go binary is compiled against ORT 1.27.0 (API version 26). The onnxruntime gem 0.9.x and 0.10.x bundle ORT 1.23.0, which only supports API version up to 23. The result:

```
emb binary                onnxruntime gem
  │                             │
  │ compiled against API 26     │ bundles ORT 1.23.0 (API 23)
  │ (ORT 1.27.0 from CI)        │
  │                             │
  └──────────┬──────────────────┘
             ▼
  "Requested API version [26] is not available, only [1, 23]"
```

The fix: update the dependency to `~> 0.11`. The onnxruntime gem 0.11.4 bundles ORT 1.27.0, which supports API version 26.

## How

Change one line in `emb-server.gemspec`:
- `spec.add_runtime_dependency "onnxruntime", "~> 0.9"`
+ `spec.add_runtime_dependency "onnxruntime", "~> 0.11"`

## What

| File | Change |
|------|--------|
| `gems/emb-server/emb-server.gemspec` | Bump onnxruntime dependency from `~> 0.9` to `~> 0.11` |
