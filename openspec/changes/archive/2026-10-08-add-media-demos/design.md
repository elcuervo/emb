# Design

## Context

See `proposal.md` — Why. The constraints that shape this design:

- The sandbox has one non-text model: a fused CLIP ViT-B/32 with an image branch
  and no embedding `dim`. Scripts drive it with `emb.run`, and a text-only run
  supplies the image branch as a host-built constant.
- One preloaded preset accepts binary — `zeroshot.lua` — and the bridge admits
  binary only for that preset by digest. Caps: 256 KiB per image, ≤2 images,
  ≤1 MP, ≤8 labels.
- Lua has no FFT and no `io`/`os`; scripts are pure compute. Audio and video
  therefore stay in the browser or in build tools.
- The gallery already ships an offline index and embeds the query live
  (`build-demo-db.py`), and `product-site` requires the Impeccable workflow, the
  site's existing vocabulary, and no visitor upload.

## Goals / Non-Goals

**Goals:**
- One plate that shows an exact matcher and a semantic matcher on the same
  medium, with both media playable and no visitor upload.
- Every figure read from shipped data or a live reply; nothing precomputed is
  presented as a live result, and nothing live is presented as more than it is.

**Non-Goals:**
- Any audio or video model. The only model stays the fused CLIP export.
- Any new server or bridge surface, any `EMB.EVAL`, any `EMB.IMG`.
- Presenting the model's picture reading as if it heard the audio or watched the
  clip.

## Decisions

### D1. The fingerprint is browser DSP; the model is a separate, weaker reading

A Shazam-style fingerprint is cheap, exact, and needs no model: peaks → paired
hashes → offset histogram. It runs in the browser (`fingerprint.js`) on the
shipped query, and the library's hashes ship precomputed. The model is asked the
other question — describe this picture — and its answer is shown beside the
fingerprint's, with the cosine and margin. Alternatives considered: using the
embedding to identify (rejected: it cannot, and pretending it can is the
dishonesty the gallery forbids); computing the fingerprint with a build tool and
only replaying the match (rejected: the match is the live, interesting part).

### D2. Music replaces the synthesized sounds

Five open-licensed excerpts (brass, dizi, lap-steel, folk guitar, piano), 10–15 s,
mono at 96 kbps, from Wikimedia Commons; four CC0/PD, one CC BY 3.0 credited.
Six four-second query excerpts are cut from them — one degraded with pink noise —
so the plate can run a real query with no upload. The fingerprints are built from
committed audio, so the shipped hashes and the shipped audio are the same bytes.

### D3. A clip is searched frame by frame, its vectors built offline

`build-frame-index.py` cuts sixteen stills from the clip, embeds each with the
build-only `clip_embed.lua` against a local server, and writes `frames.json`. The
page embeds the phrase live with `clip_text.lua` and ranks the shipped vectors,
then moves the clip to the winning frame. The still shown is the still measured,
because the vector and the picture ship together.

### D4. `clip_text.lua` returns the value itself for one text

The server's contract is that a one-text call returns the value, and an N-text
call returns one value per text. Returning a one-element array reads as a list of
its own fields on the wire, so the script returns the table directly when
`#KEYS == 1`. The build tools' `clip_embed.lua` keeps the same host-constant
`pixel_values` seam `zeroshot.lua` uses.

### D5. Scores state their transform and show the margin

CLIP's zero-shot answer is `softmax(logit_scale · cosine)` and `logit_scale` is
trained, so a high percentage is the model's own recipe. The plate still shows
the raw cosine and the runner-up margin and names the transform, so a weak win
reads as weak — which, for a spectrogram, it is.

### D6. Eye candy is the constellation, motion is the reader's

The fingerprint's own figure — the spectrogram with its peaks lit — is the plate's
one striking image, drawn in the site's palette from the live FFT. The vote bars,
the frame strip, and the marked nearest still carry the rest. Reduced motion is
respected; the state line and the command fold follow the gallery's shared atoms.

### D7. Impeccable gates the build

`impeccable context` once for the target, a surface brief with a direction
contract, the detector once over the changed UI, and `DESIGN.md` untouched
without approval. The detector's actionable findings on this plate were fixed
(near-black text on the dark ground, sub-12px labels); the rest are the shared
stylesheet and appear on every existing plate.

## Risks / Trade-offs

- **The model's description is wrong** (a dizi is called "electronic music") →
  that is the point, not a defect: it is shown as a description with its margin,
  beside a fingerprint that named the recording exactly.
- **Fingerprints are brittle to a different performance** → stated in the plate;
  the library is exact recordings, and the noisy query proves the robustness
  that matters here.
- **Media weight** → ~1.7 MB of music, clip, stills, and indexes; each asset's
  licence is recorded in `CREDITS.md`.
- **A new preset needs a hand-run deploy** → land the preset and its stamp
  together; the plate degrades to its stated unavailable state if the digest is
  not resident.

## Migration Plan

1. Land the music, clip, stills, and `CREDITS.md`; build the fingerprint database
   and the frame index with `just website-media`.
2. Add `clip_text.lua` to `sandbox.yaml`; `just website-presets`; deploy
   (`just sandbox-deploy`).
3. Build the plate and its registration; run the Impeccable pass.
4. Gate: `just website-published`, `just website-presets-check`.
5. Rollback: revert the preset and its stamp; the static page keeps working and
   reports the sandbox as unavailable rather than showing a stale result.

## Open Questions

- Whether the fingerprint should also ship a second degraded query form (a pitch
  shift) — deferred; the shipped degraded query already exercises robustness.
