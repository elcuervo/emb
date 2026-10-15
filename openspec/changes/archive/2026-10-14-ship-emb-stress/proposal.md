# Proposal

## Why

`emb-stress` shipped as a tool but not as an artifact: it is built by `just
build` and driven by `just stress`, and appears in no release tarball, no Docker
image, and no gem. An operator who installs `emb` the documented way gets the
server and `emb-top` and then cannot drive the fleet they just watched — while
`docs/operations.md` already claims `emb-stress` "runs from anywhere `emb-top`
does". The two distribution surfaces this change touches are also stale about
what they already ship: `standalone-distribution` documents the tarballs as
`emb` plus `libonnxruntime`, though `emb-top` has been a member for several
releases.

## What Changes

- The three platform tarballs (`linux_amd64`, `linux_arm64`, `darwin_arm64`)
  gain `emb-stress` as a member, built `CGO_ENABLED=0` and stamped with the
  release version like `emb-top`.
- The Docker image gains `/usr/local/bin/emb-stress` from the same CGo-free
  build step that produces `/usr/local/bin/emb-top`.
- The `emb-server` gem gains `emb-stress-binary-<platform>` for each of the
  three platforms and a `bin/emb-stress` wrapper alongside `bin/emb-top`,
  adding `emb-stress` to the gem's executables. It requires no `onnxruntime`.
- The release-tarball requirement is restated as the full member list it has
  actually had since `emb-top` was added, so the three binaries and the ORT
  shared library are named in one place.
- `install.sh` reports the binaries it actually placed, so the message stays
  true for a tarball that predates `emb-stress`. Its extraction is unchanged:
  the tarball lands wholesale.
- The release package steps assert the tarball's member list, and the local gem
  validation runs the installed `emb-stress` through its wrapper.
- No server behavior changes. No **BREAKING** changes: every addition is a new
  file in an existing artifact, and `emb-stress` is a pure RESP client with no
  runtime dependency the artifacts do not already carry.

## Capabilities

### New Capabilities

None. The behavior added is distribution of an existing tool, which the three
existing distribution capabilities already own.

### Modified Capabilities

- `standalone-distribution`: the tarball requirement enumerates the binaries a
  tarball carries (`emb`, `emb-top`, `emb-stress`) alongside `libonnxruntime`
  for each of the three platforms, gains an `emb-stress` self-containedness
  scenario, and the install-script requirement reports the binaries it places.
- `docker-build`: the image requirement names `emb-stress` beside `emb` and
  `emb-top`, with a scenario proving it runs from the image.
- `emb-server-distribution`: a requirement that the gem distributes
  `emb-stress` per platform behind a thin `bin/emb-stress` wrapper that does not
  require the `onnxruntime` gem, mirroring the existing `emb-top` requirement.

## Impact

- Modified: `.github/workflows/release.yml` (three build steps, three package
  steps, the `emb-server` binary-copy step), `Dockerfile`, `justfile`
  (`validate-gems`), `gems/emb-server/emb-server.gemspec`, `install.sh`,
  `gems/emb-server/README.md`, `docs/operations.md`.
- New: `gems/emb-server/bin/emb-stress`.
- Unchanged: the server, its RESP surface, the `emb-stress` tool itself, the
  configuration schema, and the sandbox image under `website/repl/`, which has
  no execution path for a load generator and does not get one.
- Cost: ~8 MB per tarball; the gem is a single universal gem carrying every
  platform's binaries, so it grows by ~24 MB.
