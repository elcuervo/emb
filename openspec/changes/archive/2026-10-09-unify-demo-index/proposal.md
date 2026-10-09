# Proposal

## Why

The gallery ships two different local-retrieval implementations. The text plates
search a committed SQLite + `sqlite-vec` index through one client path in
`demos.js`; the medium plate decodes bespoke `audio.json` / `video.json`
base64 float32 blobs and dot-products them in its own `cosine()` loop. A reader
who learns the gallery's vector search finds a second, contradictory example one
plate over, and the gallery ships four independent index files where it claims a
corpus and a real index. There should be one example of what a vector search is.

## What Changes

- **One shipped index.** Fold the media vectors (CLAP audio, X-CLIP video, CLIP
  frames) and the fingerprint library into the same content-hashed SQLite file
  as the text corpus, described by the same `manifest.json`. Remove
  `audio.json`, `video.json`, `fingerprints.json` (and the never-shipped
  `frames.json`).
- **One client retrieval path.** `demos.js` owns the single opener and the
  single vector-search verb (`searchVector`); `search(model, text, k)` becomes a
  thin wrapper. `medium.html` loses `decodeVector`, `cosine`, `indexAt`,
  `loadAudio`, `loadVideo` and calls the shared path.
- **Per-table precision.** Text stays `vec0(embedding int8[384])` with its
  measured recall; the tiny media tables stay float32
  (`vec0(embedding float[512])`) so the medium's margin teaching is not
  distorted by a quantization nothing needs. The manifest states each table's
  element type, dimension, and model.
- **The fingerprint library rides in the same file** as a plain
  `fingerprints(hash, track, offset)` table, loaded once into an in-memory map at
  open; the exact-match algorithm and its DSP are unchanged.
- **One build story.** The index writer gains media stages and a finalize step
  that hashes and names the one artefact; the bespoke builders write into the
  shared file instead of their own JSON.
- No change to the sandbox, the bridge, RESP, the models, or the fingerprint
  algorithm. No visitor upload.

## Capabilities

### New Capabilities
<!-- none -->

### Modified Capabilities

- `embedding-demos`: one committed index and one client search implementation
  back every demo — text, media, and fingerprint — not one per demo.
- `media-demos`: the media vectors and the fingerprint library ship as tables in
  the shared index artefact, and every media figure is read from the shared
  manifest.

## Impact

**New:** a media/fingerprint stage in `website/tools/build-demo-db.py` (and/or a
shared index-writer module the existing media builders call).

**Modified:** `website/assets/js/demos.js` (opener, `searchVector`, media and
fingerprint accessors), `website/demos/medium.html` (drop its own decode, cosine
and loaders), `website/tools/{build-audio-index,build-video-index,build-frame-index,build-fingerprints}.py`
(write into the shared db), `website/tools/published-tree.py`, `justfile`
(`website-demos` / `website-media*` orchestration), the built
`website/assets/demo/{manifest.json,poe-<hash8>.db}`.

**Removed:** `website/assets/demo/{audio,video,fingerprints}.json`.

**Deploy:** `just website-presets` is unaffected; `just sandbox-deploy` is needed
only if the sandbox page that serves the index changes. No Go changes.
