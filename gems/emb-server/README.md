# emb-server

[![emb-server gem](https://img.shields.io/gem/v/emb-server?logo=rubygems&color=red&label=emb-server)](https://rubygems.org/gems/emb-server)

Precompiled [emb](https://github.com/elcuervo/emb) server binary distributed as a Ruby gem. Runs a Redis-compatible embedding server with ONNX Runtime inference.

## Installation

Add to your Gemfile:

```ruby
gem "emb-server"
```

Or install globally:

```bash
gem install emb-server
```

## Platform support

| Platform | Status |
|----------|--------|
| macOS (Apple Silicon) | ✓ |
| Linux (x86_64) | ✓ |
| Linux (aarch64) | ✓ |

The gem ships a precompiled binary for each platform. See `bin/emb`, `bin/emb-top` and `bin/emb-stress` for platform detection logic.

## What it installs

| Binary | What it is |
|--------|------------|
| `emb` | The embedding server (needs the `onnxruntime` gem) |
| `emb-top` | Live terminal dashboard for a running node or a fleet — pure RESP client |
| `emb-stress` | Synthetic load across a fleet, from the configuration the fleet was started with: it expands a target name into its node set and reports the offered and completed rates and latency per level — pure RESP client |

## Usage

### Quick start

Auto-download a model from HuggingFace and start the server:

```bash
emb -model-repo Xenova/all-MiniLM-L6-v2
```

### With a config file

```bash
emb -config config.yaml
```

### Options

All [emb server options](https://github.com/elcuervo/emb#configuration) are available as CLI flags.

## Companion client

Use the [`emb`](https://rubygems.org/gems/emb) Ruby gem to interact with the server from Ruby code—auto-decodes float32 responses, provides proxy and multi-model support.
