## Context

The `emb` Go server requires Go, CGo, ONNX Runtime headers, and libtokenizers to build. A precompiled binary avoids all of that. Distributing it as a multi-arch Ruby gem gives users a familiar install path (`gem install emb-server`) and piggybacks on the `onnxruntime` gem for the shared library dependency.

## Goals / Non-Goals

**Goals:**
- `gem install emb-server` puts `emb` on PATH
- Server runs identically to a source-built `emb`
- Precompiled binaries for arm64/x86_64 on darwin and linux
- Auto-detect platform at gem-build time from `RUBY_PLATFORM`
- Resolve the ONNX Runtime shared library from the `onnxruntime` gem

**Non-Goals:**
- Wrapping or extending `emb`'s command set (thin distribution only)
- Windows support
- Replacing the Go build pipeline

## Decisions

### Decision: Platform-specific gems

Each platform gets its own gem variant. Platform is auto-detected at build time:

```ruby
spec.platform = Gem::Platform.new(
  case RUBY_PLATFORM
  when /arm64.*darwin/  then "arm64-darwin"
  when /x86_64.*darwin/ then "x86_64-darwin"
  when /aarch64.*linux/ then "aarch64-linux"
  when /x86_64.*linux/  then "x86_64-linux"
  else raise "unsupported platform: #{RUBY_PLATFORM}"
  end
)
```

### Decision: File structure

```
gems/emb-server/
  emb-server.gemspec
  Gemfile
  VERSION
  lib/
    emb-server/
      version.rb
      emb-binary      ← precompiled Go binary (platform-specific)
  bin/
    emb               ← tiny Ruby wrapper, exec's emb-binary
```

### Decision: bin/emb wrapper

```ruby
#!/usr/bin/env ruby
require "emb-server/version"
spec = Gem::Specification.find_by_name("emb-server")
binary = File.join(spec.gem_dir, "lib", "emb-server", "emb-binary")
onnx_spec = Gem::Specification.find_by_name("onnxruntime")
ort_lib = Dir.glob("#{onnx_spec.gem_dir}/lib/onnxruntime.{dylib,so,bundle}").first
exec binary, "-ort-lib", ort_lib, *ARGV
```

This is the only runtime Ruby code. It finds both gems' install paths, resolves the ORT shared library, and `exec`'s the binary.

### Decision: onnxruntime gem dependency

The `emb` binary is compiled against ONNX Runtime v1.27.0. The gemspec pins the dependency:

```ruby
spec.add_runtime_dependency "onnxruntime", "~> 0.9"
```

The `onnxruntime` gem ships its own platform-specific shared library. At runtime, the wrapper finds it via `Gem::Specification`.

### Decision: Release pipeline integration

As part of the existing release workflow, after `build-linux-amd64`, `build-linux-arm64`, and `build-darwin-arm64` produce tarballs, a new `release-gem` job:

1. Extracts the `emb` binary from each tarball
2. Copies it into `gems/emb-server/lib/emb-server/emb-binary`
3. Sets `EMB_PLATFORM` env var to the target platform
4. Runs `gem build emb-server.gemspec`
5. Runs `gem push` to rubygems.org

This requires a `RUBYGEMS_API_KEY` secret.

## Risks / Trade-offs

- **ORT version coupling** — binary compiled against ORT v1.27.0; `onnxruntime` gem needs to provide a compatible version. Pin to gem version that ships matching ORT.
- **Double arm64 for darwin** — both `arm64` and `x86_64` darwin gems exist. Recent macOS can run both, but `arm64` is native for Apple Silicon.
- **Gem size** — each gem bundles a ~20MB Go binary. Acceptable for distribution, but users notice the download size.
