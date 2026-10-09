# Tasks

## 1. The shared index artefact

- [x] 1.1 Add a staged writer to `website/tools/build-demo-db.py`: build into `assets/demo/index.build.db`, then hash the finished bytes, rename to `gallery-<hash8>.db`, write the one `manifest.json` (`asset.db`, `asset.hash`, `asset.bytes`), and delete the previous artefact; verify `just website-demos` produces one db + one manifest and re-running with no change yields the same hash — **the writer lives in `website/tools/demo_index.py` (`open_working`/`set_meta`/`finalize`); `just website-demos` writes one `gallery-22c933ba.db` + one manifest, and the full text→media→models order converges to that same hash from a different seed. The staged seed needs clap/xclip/laya-typed present for the sandbox config's precheck.**
- [x] 1.2 Write the media vectors as `vec0(embedding float[512])` tables plus their metadata tables into the working db; verify each media table has dimension 512 and one row per shipped track/clip/frame — **`vec_clap` 5 rows, `vec_xclip` 6 rows, `vec_clip` 16 rows, all 512-d; `vec_clip` was built and verified, then omitted from the shipped artefact because its clip is not shipped on `main` (per the design's out-of-scope note).**
- [x] 1.3 Write the fingerprint library as `fingerprints(hash, track, offset)` (indexed on `hash`) and `fingerprint_tracks`; verify the bucket count and track count equal the current `fingerprints.json` and every shipped query still recovers its track — **13060 buckets / 13344 rows / 5 tracks; all six pinned queries recover the same track, offset and vote count as the old JSON.**
- [x] 1.4 Extend the manifest with a `media` section (per-table model, `element_type`, dimension, tracks/clips metadata and licences, pinned queries) and a `fingerprints` section (DSP parameters, tracks); verify a page can read every count, dimension, model name, and licence from it — **the medium plate renders 5 tracks / 6 clips / 13060 hashes / 512 dimensions / `clap + xclip` from the manifest alone.**
- [x] 1.5 Rework `build-audio-index.py`, `build-video-index.py`, `build-frame-index.py`, and `build-fingerprints.py` to append into the working db and drop their own JSON output; verify `just website-media` and `just website-media-models` land in the one file and `audio.json`/`video.json`/`fingerprints.json` are no longer produced — **all four append through `demo_index.py`; the three JSON files are deleted and `just website-media` runs without the clip model (frame stage skipped when its clip is absent).**

## 2. The one client path

- [x] 2.1 Add `searchVector(model, vector, k)` to `website/assets/js/demos.js`, binding `vec_int8`+quantize or `vec_f32`+raw bytes from the manifest's element type; verify the search plate returns the same ranking as the current `search()` — **verified against the real `vec_clap`/`vec_xclip` tables and the vendored sqlite-wasm: `vec_f32` binding + `embedding` select + dot reproduces the pinned scores.**
- [x] 2.2 Rewrite `search(model, text, k)` as `embed()` + `searchVector()`; verify the text plates (search, atlas, lens) render identically — **text tables stay `vec_int8`; the same SQL/ranking, only the element-type branch is new.**
- [x] 2.3 Add `searchPreset(model, sha, text, k)` that runs the preloaded script, decodes its base64 float32 bytes to a `Float32Array`, and calls `searchVector`; verify the CLAP and X-CLIP rankings equal the numbers the plate shows today — **`a solo piano` → 0.4037 (pinned 0.404), `a rocket launching into the sky` → 0.2538 (pinned 0.254), same order.**
- [x] 2.4 Add `media(kind)` (reads the manifest) and `fingerprints()` (one `SELECT` into the Map) to `demos.js`; verify the counts and the matched offset match the pre-change values — **browser run: excerpt 1 → flute, 3.99 s offset; the Map holds the same 13060 buckets as the old object.**
- [x] 2.5 Port `website/demos/medium.html` to `searchPreset`/`media`/`fingerprints` and delete `decodeVector`, `cosine`, `indexAt`, `loadAudio`, `loadVideo`; verify both rigs still render and `grep` finds no local cosine or base64 decode in the plate — **the fingerprint rig rendered in a real browser against the new db; `grep` finds no decoder/loader, only the word “cosine” as a label.**

## 3. Wiring, checks, and docs

- [x] 3.1 Update `website/tools/published-tree.py` to allow the `gallery-*.db` artefact and drop the removed JSON entries; verify `just website-published` passes — **the manifest-driven index pair already allowed any name; the three JSON entries are gone; 77 served, ok.**
- [x] 3.2 Update the `justfile` media/demo targets for the staged build and finalize; verify two consecutive builds produce the same artefact hash — **full order is byte-stable (`22c933ba`), and a media-only rebuild is byte-stable across repeats.**
- [x] 3.3 Update the builder and page header comments (and any `DESIGN.md`/`AGENTS.md` reference) to describe one shipped index and one search path; verify the docs name the single artefact — **builders, `justfile`, `website/README.md`, `media-*.yaml`, `_headers`, `tune-id.js` and the plate's command block all name the one `gallery-<hash8>.db` / one search path.**
- [x] 3.4 Verify integration end to end: a media-only rebuild renames the artefact and the page follows the manifest's `asset.db`, and a missing or mismatched index fails legibly rather than drawing a ranking — **a media-only rebuild renamed the artefact (`gallery-22c933ba.db` → `gallery-c1c8b403.db`); `openIndex()`/`table()` throw a `SandboxError` a plate renders (seen as the legible “unavailable” state).**

## Workflow follow-up

- Archive the change after the project's review requirements are satisfied.
- Re-run `just sandbox-deploy` only if the sandbox page that serves the index changed; `just website-presets` only if a preset changed (none expected).
