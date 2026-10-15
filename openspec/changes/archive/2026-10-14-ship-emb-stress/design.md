# Design

## Context

- `emb-stress` lives in `cmd/emb-stress/` and is a pure RESP client: `CGO_ENABLED=0`,
  no model, no ONNX Runtime, 8.1 MB on darwin/arm64 (measured). `just build`
  already builds it; `just stress` runs it.
- `emb-top` is the precedent for distribution, and `emb-top` is already a member
  of every artifact: a tarball member on all three platforms, `/usr/local/bin/emb-top`
  in the Docker image, and `emb-top-binary-<platform>` plus a `bin/emb-top`
  wrapper in the `emb-server` gem.
- The gem release job (`release-emb-server`) downloads all three platform
  tarballs and builds **one** gem: the gemspec sets no `spec.platform` and the
  job has no matrix, so a single published gem carries every platform's copy of
  each binary. (The `gems-release-lifecycle` wording about per-platform variants
  does not describe this job; that drift is pre-existing and out of scope here.)
- `standalone-distribution` already drifted: its tarball requirement describes
  `emb` and `libonnxruntime` and does not mention `emb-top`, which has shipped
  in the tarballs for several releases. The gem's own README does not mention
  `emb-top` either.

## Goals / Non-Goals

**Goals:**

- Every artifact that carries `emb-top` also carries `emb-stress`, built the same
  way (`CGO_ENABLED=0`) and stamped with the same release version.
- The release itself produces evidence that each platform's tarball carries all
  three binaries, so a missed build step fails the job rather than the user.
- `docs/operations.md`'s existing claim that `emb-stress` "runs from anywhere
  `emb-top` does" becomes true, rather than being edited down.

**Non-Goals:**

- The sandbox image under `website/repl/`: it has no execution path for a load
  generator, and adding one is a feature, not distribution.
- `emb-stress` behavior, flags, or output.
- New install flags in `install.sh`; the tarball is already extracted whole.
- Restructuring the gem into platform-specific builds.
- A Homebrew formula or any new artifact.

## Decisions

**D1 — Membership in the existing artifacts, not a new artifact.** Alternatives:
a separate `emb-tools` tarball (a new release asset, a second thing to keep in
sync, and a new download for the operator), or Docker-only (leaves exactly the
users `install.sh` serves without the tool). Adding a third member to the three
existing artifacts is the smallest surface that makes the documentation true.

**D2 — The gem takes the third binary and the size.** Because the gem is
universal (Context), `emb-server` grows by ~24 MB (8.1 MB × 3 platforms) on top
of ~51 MB of binaries today. Alternative considered: split the gem per platform,
which would both shrink every download and make `gems-release-lifecycle`'s claim
true — rejected as a separate change, since it rewrites the gem release job and
the published gem's identity and has nothing to do with shipping a binary.

**D3 — A third thin wrapper, not a shared loader.** `spec.executables` requires
one file per command in `bin/`, so a shared helper would live in `lib/` and be
required by each wrapper: more moving parts than the ~20 lines it removes.
Cost accepted knowingly: the platform→binary map now appears in three wrappers,
the gemspec, the release workflow and the justfile.

**D4 — Version stamping stays per-artifact.** Each tarball job and the Dockerfile
passes `-X main.version`; `emb-stress` gains the same flag value as `emb-top` in
the same build step. No new version source is introduced.

**D5 — Release-time evidence is presence, not a load run.** Each package step
asserts the tarball's member list contains all three binaries, and
`just validate-gems` runs the installed `emb-stress -version` through the new
wrapper (the wrapper is the only new logic in the change). A real load run in
the release path would need a model and a server, and `just bench-smoke` already
exercises `emb-stress` against `testdata/laya`.

**D6 — `install.sh` reports what it extracted, not a fixed list.** Extraction is
`tar xz` of the whole member list, so the binaries land without a code change.
The report line, though, is derived from the members that exist in the install
directory afterwards: while the latest release precedes this change, a fixed
list naming `emb-stress` would claim a binary the download did not contain, and
deriving it stays true for every release without a version check.

**D7 — The stale tarball requirement is corrected here.** The member list is the
line being edited, so the requirement that documents it is updated in the same
change rather than left describing a tarball that has not existed for releases.

## Risks / Trade-offs

- [Re-releasing a tag whose tarball predates `emb-stress`: the gem job's copy
  fails] → no guard. Failing loudly beats publishing a gem silently missing a
  binary, and such a re-run could not publish anyway (the gem version already
  exists on rubygems).
- [The universal gem's growth] → accepted under D2; the fix is a platform-split
  gem in its own change, and nothing here blocks it.
- [~8 MB per tarball] → static and CGo-free; a second download would cost the
  operator more than the bytes.
- [Three wrappers drifting] → each is exercised by `just validate-gems` locally
  and copied into the gem by the release job; the gemspec's `executables` keeps
  the set discoverable.
- [A platform job that forgets the build step ships a tarball without the tool]
  → the package-step member assertion fails that job before the asset is
  uploaded.

## Migration Plan

Additive: no file is removed, no command changes meaning, and nothing that
already works stops working. Published tarballs, images and gems keep resolving;
users gain a binary on the next release.

Rollback is a revert; the release workflow is the only writer of these assets,
and no on-disk or shared state is migrated.
