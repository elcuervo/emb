## Why

The `bin/emb` wrapper in the `emb-server` gem hardcodes a glob pattern for finding the onnxruntime shared library:

```ruby
ort_lib = Dir.glob("#{onnx_spec.gem_dir}/lib/onnxruntime.{dylib,so,bundle}").first
```

The `onnxruntime` gem v0.9+ stores its library in `vendor/`, not `lib/`, and uses a platform-qualified filename (`libonnxruntime.arm64.dylib` vs `libonnxruntime.dylib`). The glob misses entirely, so `emb-server` is broken for every user on install.

The onnxruntime gem already exposes its own library path via `OnnxRuntime.ffi_lib` — the wrapper should use that instead of replicating path resolution logic.

## How

Replace the manual glob with a `require "onnxruntime"` call and read `OnnxRuntime.ffi_lib.first`. This delegates path resolution to the gem that owns the file.

## What

| File | Change |
|------|--------|
| `gems/emb-server/bin/emb` | Replace `Gem::Specification.find_by_name("onnxruntime")` + `Dir.glob(...)` with `require "onnxruntime"; ort_lib = OnnxRuntime.ffi_lib.first` |
