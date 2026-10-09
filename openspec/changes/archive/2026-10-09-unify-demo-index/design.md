# Design

See `proposal.md` for motivation and `specs/` for the behavior contract.

## Context

Today one SQLite + `sqlite-vec` file (`poe-<hash8>.db`) and one `manifest.json`
back every text plate, opened through `openIndex()` in `website/assets/js/demos.js`.
Three bespoke JSON files back the medium plate: `audio.json` and `video.json`
(base64 float32, already L2-normalized at build) and `fingerprints.json` (an
inverted index). `medium.html` carries its own `decodeVector()` and `cosine()`
loop. The media builders each run against a different model server (CLAP,
X-CLIP, CLIP) on its own port, and the frame builder's output is not shipped on
`main`.

Constraints that shape this: the sandbox and bridge are unchanged; the db is
loaded fully into browser memory with `sqlite3_deserialize`; every displayed
number must be read from the shipped data; nothing may be quantized in a way
that changes a demo's teaching.

## Goals / Non-Goals

**Goals**

- One content-hashed index file + one manifest holding every retrieval the
  gallery performs.
- One client opener and one client vector-search verb, reused by every plate.
- Media vectors and the fingerprint library exposed through that same artefact.
- Preserve each dataset's existing numerics exactly (float32 media, int8 text).

**Non-Goals**

- No change to the fingerprint DSP, its matcher, or its verified queries.
- No change to the zero-shot reading (spectrogram → labels); it is a live
  preset call with no index.
- Wiring the medium plate's frame search is out of scope — only the *placement*
  of the frame vectors is covered here.
- No new model, no re-embedding, no serving-surface change.

## Decisions

### D1. One artefact, assembled in stages, content-addressed at finalize

Each media builder opens the same working file (`assets/demo/index.build.db`),
creates its tables, fills them, and closes; the text builder does the same. A
finalize step then hashes the completed file, renames it to
`gallery-<hash8>.db`, writes the single `manifest.json` (`asset.db`,
`asset.hash`, `asset.bytes`), and deletes the previous artefact.

*Why staged:* each stage needs a different model server on a different port, and
requiring all four simultaneously makes `just` brittle. Appending tables keeps
the existing per-stage `just` targets nearly as they are.

*Alternative rejected:* one mega build with every model server up at once —
heavier orchestration for no benefit. *Alternative rejected:* keep per-demo
files — fails the whole point.

### D2. Text int8, media float32, precision declared per table

Text keeps `vec0(embedding int8[384])` with its measured `recall@k`. Media keeps
`vec0(embedding float[512])`. The manifest states each table's `element_type`,
`dimension`, and model.

*Why:* int8 exists for the text corpus's size (6 MB) and is justified by a
measured recall. Quantizing a 5–16 vector media table saves nothing and risks
flipping the medium's near-tie margins — the very thing that plate teaches.
A `vec0` table declaring its element type is itself the honest lesson.

*Alternative rejected:* int8 everywhere — no benefit, real risk to the medium's
claim.

### D3. The fingerprint library is a table, loaded into a Map at open

The inverted index ships as `fingerprints(hash INTEGER, track INTEGER, offset
INTEGER)` (indexed on `hash`) plus a `fingerprint_tracks` metadata table. The
client runs one `SELECT` at open and builds the same `Map` the page uses today.

*Why:* the algorithm stays in JS and unchanged; the data still lives in the one
file, so "the gallery's index is one file" is literally true. A `Map.get` per
query hash is preserved.

*Alternative rejected:* one SQL query per hash — pushes the matcher into SQL for
no gain. *Alternative rejected:* keep `fingerprints.json` — two loaders, two
data shapes.

### D4. One client verb, three entry points, all in `demos.js`

- `searchVector(model, vector, k)` — the one search implementation: reads the
  model's table from the manifest, binds `vec_int8`/`vec_f32` per element type,
  returns ranked rowids.
- `search(model, text, k)` — `embed()` then `searchVector()` (text plates).
- `searchPreset(model, sha, text, k)` — runs the preloaded script, decodes its
  base64 float32 bytes to a `Float32Array`, then `searchVector()` (media plates).
- `media(kind)` and `fingerprints()` — read the shared manifest / run the one
  `SELECT`.

`medium.html` deletes `decodeVector`, `cosine`, `indexAt`, `loadAudio`, and
`loadVideo`; it calls `g.searchPreset(...)` and renders the returned rows.

*Alternative rejected:* let the plate obtain a vector and call `searchVector`
directly — viable, but the preset's byte decode belongs with the rest of the
wire handling, and one `searchPreset` keeps plates from re-implementing it.

### D5. Manifest grows media and fingerprint sections

`models[]` keeps the text models and gains the media models (`clap`, `xclip`,
`clip`) with `element_type` and `dimension`; a `media` section carries each
medium's tracks/clips metadata (titles, artists/sources, licences, durations)
and the pinned query checks; a `fingerprints` section carries the DSP parameters
and track list. The page reads every count, dimension, model name, and licence
from here.

## Risks / Trade-offs

- **Build coupling across model servers** → staged table appends keep each
  stage independent; the finalize step is the only new orchestration.
- **Content-addressing the whole file renames it on any stage rebuild** → the
  page reads `asset.db`, never a hard-coded name; the finalize step deletes the
  previous artefact; `published-tree.py` must allow the `gallery-*.db` glob.
- **A per-table element-type branch** → accepted; it is the honest encoding of
  "a `vec0` table declares its type," not duplicated search logic.
- **Whole-db load grows** (text db + media + fingerprint tables) → still small;
  `sqlite3_deserialize` cost is bounded by tens of MB, far under the current
  ceiling.
- **Frame search is data-only here** → the frame vectors get a table and a
  manifest entry; the plate wiring stays with the existing media work.

## Migration Plan

1. Add the media/fingerprint stages and the finalize step to the index writer;
   build `gallery-<hash8>.db` + the one manifest.
2. Move the media and fingerprint readers onto `demos.js`; delete the plate's
   own decode/cosine/loaders.
3. Remove `audio.json`, `video.json`, `fingerprints.json`; update
   `published-tree.py` and `justfile`; keep the old JSON available only while a
   stage is un-migrated.
4. Run the published-tree check and the media-pinned query checks; verify each
   medium's ranking and the fingerprint match is unchanged.

Rollback: revert the commit — the previous db and JSON files are in git, and the
page reads whatever the manifest names.

## Open Questions

- The artefact's filename prefix (`gallery-` proposed) — cosmetic, settled at
  build time, no spec or task impact.
