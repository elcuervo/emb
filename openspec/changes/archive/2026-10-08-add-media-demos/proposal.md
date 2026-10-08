# Proposal

## Why

The gallery has thirteen plates, all text or still photos. The scripting layer
already carries any medium to the model as a picture, and the sandbox already
ships the one vision model that makes it true — but nothing on the site shows
what that can and cannot do. The interesting answer is a contrast: an exact tool
(a music fingerprint) that names a recording and a semantic tool (an embedding)
that only describes it, side by side on the same four seconds, plus a phrase
that finds the moment in a clip.

## What Changes

- Add one multimedia plate `demos/medium.html` with two rigs:
  - **Name that tune** (audio): five open-licensed music excerpts ship with the
    plate. Picking one fingerprints it in the browser — spectral peaks, paired
    hashes, an offset histogram — and matches it against a shipped fingerprint
    database, naming the exact recording and the offset. The same excerpt is
    then sent to the sandbox's preloaded script as a spectrogram picture, and
    the model's description is shown beside the fingerprint's answer, with its
    cosine and margin. The identified track streams from the matched offset.
  - **Find the moment** (video): a public-domain clip ships as sixteen stills,
    each embedded offline by the sandbox's own model. A typed phrase is embedded
    live and ranks the stills; the nearest is marked and the clip moves to it.
- Add `scripts/clip_text.lua` (phrase → packed CLIP text embedding), preloaded
  under the `clip` model; and the build-only `scripts/clip_embed.lua` (a still →
  packed image embedding).
- Add five open-licensed music excerpts, six four-second query excerpts (one
  degraded with noise), one public-domain clip, its sixteen stills, a shipped
  fingerprint database, and a shipped frame index — all committed.
- Add `build-fingerprints.py` (pure DSP, no server) and `build-frame-index.py`
  (embeds each shipped still against a local server), with a `just website-media`
  target.
- Register the plate in the gallery index, the reading order, the published-tree
  allowlist, the preset stamper, and the media credits.
- No visitor uploads anywhere. No new server surface, no `EMB.EVAL`, no
  `EMB.IMG`.

## Capabilities

### New Capabilities

- `media-demos`: how a plate puts a real medium beside the model's reading of it
  — a fingerprint computed live that names the exact recording, a model reading
  that describes rather than identifies, a phrase that finds the moment in a
  clip — with committed media, shipped indexes, and every figure read back from
  the data.

### Modified Capabilities

- `embedding-demos`: the fixed five-section order and the committed-corpus /
  exact-top-k rules are generalized from "text" to "the input", so a plate whose
  input is a recording or a clip satisfies the same contract as a text plate.
- `visual-embeddings`: the distribution a scripted image call displays must be
  the model's own score or a transform whose scale is stated, with the
  runner-up margin shown, rather than a probability that the input *is* the
  winning label.

## Impact

**New files:** `website/demos/medium.html`, `website/assets/js/fingerprint.js`,
`website/repl/presets/clip_text.lua` (plus the `scripts/clip_text.lua` symlink),
`scripts/clip_embed.lua`, `website/tools/build-fingerprints.py`,
`website/tools/build-frame-index.py`, `website/tools/media-build.yaml`, committed
media under `website/demos/media/{music,clips,frames}/`, and the built
`website/assets/demo/{fingerprints,frames}.json`.

**Modified files:** `website/repl/sandbox.yaml`, `website/demos/index.html`,
`website/tools/published-tree.py`, `website/tools/stamp-presets.py`,
`website/demos/samples/CREDITS.md`, `justfile`, and the media plate's styles in
`website/assets/css/styles.css`.

**Deploy:** `just website-presets` and `just sandbox-deploy` (one new preloaded
preset). No Go changes and no bridge change: binary is sent only as a picture
through the existing `zeroshot` preset, and the phrase carries no binary.
