# Tasks

## 1. The release tarballs carry `emb-stress`

- [x] 1.1 Add an `emb-stress` build step to each of the three platform jobs in
      `.github/workflows/release.yml` (`build-linux-amd64`, `build-linux-arm64`,
      `build-darwin-arm64`), alongside the existing `emb-top` step:
      `CGO_ENABLED=0 go build -ldflags="-X main.version=${TAG_NAME#v}" -o
      <dist>/emb-stress ./cmd/emb-stress`, with the arm64 job's path under the
      Docker builder output (`dist/linux-arm64/usr/local/bin/`). Verify: the
      same command run in the dev shell produces a binary that prints its
      version (`CGO_ENABLED=0 go build -o /tmp/emb-stress ./cmd/emb-stress &&
      /tmp/emb-stress -version`), and the three jobs' build commands run against
      a scratch `dist/` leave `emb`, `emb-top` and `emb-stress` there.
- [x] 1.2 Add `emb-stress` to each job's `tar czf` member list and assert the
      member list before the asset is uploaded, so a job that built the binary
      but packaged it wrong fails instead of publishing: after the archive is
      written, `tar tzf <tarball> | grep -qx emb-stress` (and the same for
      `emb` and `emb-top`) must succeed. Verify: run the darwin job's build and
      package commands locally against `dist/`, and confirm the resulting
      `emb_<version>_darwin_arm64.tar.gz` lists `emb`, `emb-top`, `emb-stress`
      and `libonnxruntime*.dylib`; confirm the assertion exits non-zero when
      `emb-stress` is left out of the member list.
- [x] 1.3 Make `install.sh` report the binaries it actually placed rather than a
      fixed list — after extraction, list the `emb`, `emb-top` and `emb-stress`
      members that exist in `$INSTALL_DIR`, so the message cannot claim a binary
      an older tarball does not carry. Keep the extraction and the
      unsupported-platform exit as they are. Verify: run the script with
      `EMB_INSTALL_DIR` set to a temp directory against the current latest
      release, and confirm the reported list matches
      `tar tzf <downloaded tarball>` exactly (two binaries today, three once a
      release carries `emb-stress`).

## 2. The Docker image carries `emb-stress`

- [x] 2.1 Build `/emb-stress` in `Dockerfile`'s CGo-free step beside `/emb-top`
      (same `-X main.version=${EMB_VER}`), and `COPY` it to
      `/usr/local/bin/emb-stress` in the runtime stage. Verify: `just docker`
      builds, and
      `docker run --rm --entrypoint emb-stress elcuervo/emb:<tag> -version`
      prints the built version — no ONNX library needed for it to start.
- [x] 2.2 Prove the image's binary set is the documented one. Verify:
      `docker run --rm --entrypoint ls elcuervo/emb:<tag> /usr/local/bin` lists
      `emb`, `emb-top` and `emb-stress`, and a container started against a
      running node drives load through
      `docker run --rm --entrypoint emb-stress -v <config dir>:/cfg
      elcuervo/emb:<tag> -config /cfg/server.yaml -target host.docker.internal:<port> -levels 1,2`.

## 3. The `emb-server` gem carries `emb-stress`

- [x] 3.1 Add `gems/emb-server/bin/emb-stress`: the same platform→binary map as
      `bin/emb-top`, exec'ing `emb-stress-binary-<platform>` and passing
      arguments through, with no `onnxruntime` require and no `-ort-lib`. Add
      `lib/emb-server/emb-stress-binary-*` to the gemspec's `files` and
      `'emb-stress'` to `spec.executables`. Verify: `gem build emb-server.gemspec`
      succeeds with the three binaries copied in, and
      `gem specification emb-server-<version>.gem executables` lists `emb`,
      `emb-top` and `emb-stress`.
- [x] 3.2 Extend `release-emb-server`'s copy step to place
      `emb-stress-binary-<platform>` beside each platform's `emb` and `emb-top`
      (the `${name/emb-binary/…}` substitution pattern already there). Verify:
      run the job's copy loop locally over the three tarballs packaged in 1.2
      and confirm nine binaries land in `gems/emb-server/lib/emb-server/` with
      the names the wrapper's platform map looks for.
- [x] 3.3 Extend `justfile`'s `validate-gems` to copy `bin/emb-stress` into the
      gem tree, add the `which emb-stress` warning beside the existing one, and
      run `emb-stress -version` through the installed wrapper so a broken
      platform map fails validation. Verify: `just validate-gems` prints the
      version for all three executables and cleans up after itself.
- [x] 3.4 Update `gems/emb-server/README.md`: it currently documents only `emb`,
      so name the three binaries the gem installs and what each is for. Verify:
      the README's list matches `ls gems/emb-server/lib/emb-server/` after a
      `validate-gems` run.

## 4. The operations docs and the artifacts agree

- [x] 4.1 Give `docs/operations.md`'s `emb-stress` section the completeness
      sentence its `emb-top` section already has: where it ships (the tarball,
      `/usr/local/bin/emb-stress` in the image, `bin/emb-stress` in the gem).
      The section's existing "runs from anywhere `emb-top` does" claim then
      holds against the artifacts rather than against the source tree. Verify:
      every command the section shows resolves to a shipped binary path, and no
      sentence claims a surface where the binary is absent.
- [x] 4.2 Cross-check the three surfaces name the same set. Verify: the tarball
      member list, the image's `/usr/local/bin`, the gem's `executables` and its
      `lib/emb-server/` binary names all resolve to `emb`, `emb-top`,
      `emb-stress`, and `just build` still produces all three in `bin/`.
- [x] 4.3 Prove the changed release steps by running them, not by reading them —
      no throwaway tag is dispatched and no branch is pushed; the first tagged
      release is the first real CI run. Verify: `just test` passes; each of the
      three `Package artifact` steps executed verbatim (with the CI-only ONNX
      paths pointed at stand-ins) writes a tarball listing `emb`, `emb-top` and
      `emb-stress` and exits non-zero when `emb-stress` is dropped from the
      member list; `act -l -W .github/workflows/release.yml` parses the
      workflow; `git status --short` shows only the change's own files.

## Workflow follow-up

- After the first tagged release that contains this change, verify the published
  artifacts: `gh release download <tag> --pattern 'emb_*' && tar tzf` each
  tarball for the three binaries, pull the image and check
  `/usr/local/bin/emb-stress`, and `gem install emb-server && emb-stress -version`.
- Sync the delta specs into `openspec/specs/` and archive the change once the
  artifacts above are verified.
