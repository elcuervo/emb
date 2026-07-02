## Why

Two bugs prevent `emb-server` from working when installed as a gem. Both are "embedding a build-time file path into a runtime artifact" — one in Go, one in Ruby.

**Bug 1: Go binary dyld dependency on ORT**

The build includes `-lonnxruntime` in `CGO_LDFLAGS`, which creates a compile-time link against ONNX Runtime. The linker embeds the ORT library's install name as an `LC_LOAD_DYLIB` entry — `/opt/homebrew/opt/onnxruntime/lib/libonnxruntime.1.dylib` on macOS CI.

On the user's machine, `dyld` tries to resolve this path before `main()` runs. The path doesn't exist, so the process crashes. The `-ort-lib` flag (which correctly points to the onnxruntime gem's bundled library) never gets a chance to run.

The fix: `yalue/onnxruntime_go` already loads ORT entirely via `dlopen` at runtime (see `setup_env.go:68` — `C.dlopen(cName, C.RTLD_LAZY)`). The compile-time `-lonnxruntime` link is unnecessary and harmful. Removing it from `CGO_LDFLAGS` eliminates the dyld dependency entirely. The binary starts up, parses `-ort-lib`, and loads ORT at the right moment.

**Bug 2: emb gem version.rb reads VERSION by relative path**

`gems/emb/lib/emb/version.rb` does:

```ruby
VERSION = File.read(File.expand_path('../../../../VERSION', __dir__)).strip
```

In the repo, `__dir__` resolves to `gems/emb/lib/emb/` and 4 levels up reaches the repo root. In an installed gem, `__dir__` is the gem's installation directory — 4 levels up goes outside the gem entirely into an unrelated directory. The `VERSION` file is not part of the gem's files list, so it doesn't exist there.

Fix: use `Gem.loaded_specs['emb'].version.to_s` instead. No file I/O, resolves from the gem's own specification, works everywhere.

## How

1. Remove `-lonnxruntime`, the ORT `-L` path, and `-Wl,-rpath` for ORT from all build commands. The tokenizer's `-l` and `-L` flags remain.
2. Replace `File.read(File.expand_path(...))` with `Gem.loaded_specs['emb'].version.to_s` in both `version.rb` files.

## What

| File | Change |
|------|--------|
| `.github/workflows/release.yml` (darwin build) | Remove `-lonnxruntime -L/opt/homebrew/opt/onnxruntime/lib -Wl,-rpath,@loader_path` from `CGO_LDFLAGS` |
| `.github/workflows/release.yml` (linux build) | Remove `-lonnxruntime -L/opt/onnxruntime/lib -Wl,-rpath,\$ORIGIN` from `CGO_LDFLAGS` |
| `justfile` | Remove `-lonnxruntime` and ORT `-L` from build recipe |
| `gems/emb/lib/emb/version.rb` | Replace `File.read(...)` with `Gem.loaded_specs['emb'].version.to_s` |
| `gems/emb-server/lib/emb-server/version.rb` | Replace `File.read(...)` with `Gem.loaded_specs['emb-server'].version.to_s` |
