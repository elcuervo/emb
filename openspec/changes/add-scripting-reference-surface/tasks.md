# Tasks

## 1. Unify the version

- [x] 1.1 `internal/script/version.go`: replace the `APIVersion` constant and the
      1.x history with the server-version variable, `DefaultVersion`, and
      `SetVersion`.
- [x] 1.2 `internal/server/server.go`: `SetVersion` moves `script.APIVersion`
      with `s.version`.
- [x] 1.3 `internal/script/host.go` / `cache.go`: report and fold the one value.
- [x] 1.4 Update `compat_test.go` and `cache_test.go`, including the pinned
      reply-cache key, for the unified version.
- [x] 1.5 `go vet` + `go test ./internal/script/ ./internal/server/` pass.

## 2. Build the reference surface

- [x] 2.1 `website/scripting/index.html`: the eight sections, composed from the
      shared stylesheet and `docs.css` atoms.
- [x] 2.2 Version stamped from `VERSION` via `data-emb-version`.
- [x] 2.3 Reads with JavaScript off; `.tldr` summary lines present.
- [x] 2.4 Masthead nav adds **Scripting** on the new page and all four existing
      pages (desktop + mobile disclosure).

## 3. Reconcile the existing surfaces

- [x] 3.1 `website/docs/index.html` `05 · Lua scripting` links to the reference
      for the complete form.
- [x] 3.2 `website/tools/published-tree.py` carries `scripting/index.html`.
- [x] 3.3 `website/tools/stamp-version.py` carries `scripting/index.html`.

## 4. Record the design

- [x] 4.1 Impeccable surface brief at
      `website/.impeccable/surfaces/website-scripting-index-html.md`.

## 5. Verify

- [x] 5.1 `just website-published` / `python3 website/tools/published-tree.py`.
- [x] 5.2 `python3 website/tools/stamp-version.py --check`.
- [x] 5.3 Render the page at 1086px and 390px and confirm the floors and the
      contrast pairs hold.
