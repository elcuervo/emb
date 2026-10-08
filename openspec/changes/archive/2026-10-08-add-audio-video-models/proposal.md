# Proposal

## Why

The gallery's media plate is conceptually honest but deliberately weak: its
"semantic" half asks the **image** model to read a **spectrogram** (audio) and
its video half ranks **stills** with the same image model. Neither is a real
audio or video model, and the measured result says so — CLIP calls a dizi
"electronic music", and a frame's vector carries no motion. Real models exist,
are small enough to run, and are licence-clean. This change upgrades the plate
from "what a picture trick cannot do" to the real thing: a text↔audio model and
a text↔video model, over genuinely good music and striking footage.

## What Changes

### Models — the two that actually exist as ONNX

**Decision: the smallest audio pair that runs in the current setup.** CLAP int8
(161 MB) over fp16 or fp32, so the whole change stays inside the existing
`shared-cpu-4x` / 8 GB sandbox with no new deployment.

- **Audio: CLAP** — `Xenova/clap-htsat-unfused` (Apache-2.0), the canonical
  optimum export of `laion/clap-htsat-unfused`. Two separate graphs that share a
  512-d space:
  - `onnx/audio_model_quantized.onnx` (**34.3 MB**) — `input_features` f32 `[1, 1, 1001, 64]`,
    a **pre-computed log-mel**, out `audio_embeds` `[1, 512]`;
  - `onnx/text_model_quantized.onnx` (**126.6 MB**) — `input_ids` + `attention_mask`,
    out `text_embeds` `[1, 512]`.
  Both publisher-quantized (fp16 and fp32 variants also exist; the fused
  `model_quantized.onnx` is 161 MB). **~161 MB total.**
  Outputs are **not** L2-normalized (normalization lives outside the exported
  towers), so the preset normalizes.
  Mel spec, from the repo's own `preprocessor_config.json`: **48 kHz mono,
  n_fft/win 1024 Hann, hop 480, 64 mels, fmin 50, fmax 14000, power = 2.0,
  dB, no top-dB clipping, centre + reflect pad, 1000 frames → `[1001, 64]`**,
  with the reference path scaling the waveform by 32768 before the STFT.
- **Video: X-CLIP** — `imbcmdth/xclip-onnx` (**MIT**), the only text↔video ONNX
  export that exists (base `microsoft/xclip-base-patch16-kinetics-600`):
  - `video_tower.onnx` (499 MB) — `pixel_values` f32 `[1, 8, 3, 224, 224]`
    (8 frames, 224², RGB, scale `[0,1]`; mean/std **inside** the graph), out
    `video_embeds` `[1, 512]`;
  - `text_tower.onnx` (254 MB) — `input_ids` + `attention_mask`, out
    `text_embeds` `[1, 512]`.
  This is a genuine temporal model: its video tower runs a multi-frame
  integration transformer, so a clip is one tensor with motion in it, unlike
  CLIP's per-frame encode. **fp32 only — no quantized export exists** (verified
  against the publisher's file list, the `onnx-community` org, and an HF-wide
  search; the sole other video ONNX is a VideoMAE classification head, wrong
  task and partly non-commercial). The 753 MB is the cost, and the proposal
  keeps a self-quantized variant as an optional path rather than a dependency.
  **Provenance risk, stated up front:** a single author's side project, opset 17,
  no `dynamic_axes`, validated only against a random tensor. The change MUST
  probe the graph's real input/output names, dtypes and shapes with ONNX Runtime
  before any preset is written, and MUST fail the build loudly if the probe
  disagrees with the card above.

### Where each model runs

- **Media embeddings are built offline** and shipped: a build tool decodes each
  music excerpt to a CLAP log-mel and each video to 8 frames, runs the **media
  tower**, and writes a committed index.
- **The query is text, embedded live** by the same model's **text tower**,
  through a preloaded preset. Cross-modal ranking is a cosine over the shipped
  index — the gallery's established pattern (index offline, query live).
- **Audio additionally gets a live media encode**, because it fits: the mel is
  `1001 × 64 × 4 = 256,256` bytes, just under the sandbox's 256 KiB per-image
  cap. The browser computes the log-mel with its own FFT and sends the packed
  f32; a preloaded preset runs CLAP's audio tower and returns the embedding.
  This needs the bridge's binary gate to admit more than one preset (see below).
- **Video does not get a live media encode**: `[1,8,3,224,224]` f32 is 4.8 MB,
  far past the cap. Stated as a non-goal, not smuggled.

### Bridge

The bridge admits binary only for the preset hardcoded as `zeroshot`
(`imagePresetName`). Generalize that to a small **allowlist of preloaded
preset names** (`zeroshot`, `clap-audio`), so a packaged tensor can reach the
audio tower while text presets still cannot carry bytes. The existing refusal
tests are updated to assert the new boundary, not deleted.

### Assets — fantastic music, great footage

- **Music, 8 excerpts**, all CC0 or public domain: US Army Blues (Duke Ellington
  "Main Stem", "Stardust"), Kimiko Ishizaka's CC0 Goldberg Variations (Aria,
  Variatio 13), FMA CC0 electronic (Loyalty Freak Music, Anonymous420), a live
  jazz-duo field recording, and three 1920s LOC/MMA public-domain 78s (blues,
  Romanian and Ukrainian folk). Trimmed to 15–25 s, mono.
- **Video, 6 clips**, all public domain (NASA/NSF/USFWS/NOAA): Sarychev Peak
  eruption from the ISS, a moon transit of the sun, the Arecibo collapse, an
  olive ridley turtle, the STS-134 launch, and a GOES storm sequence. Transcoded
  to a browser-playable size.

### The plate

One plate, upgraded in place (`demos/medium.html`), keeping the working
fingerprint as the exact-match contrast:

- **Name that tune** — the fingerprint (exceptionally exact, no model) and now
  **CLAP** (semantic, real audio) on the same excerpt. The contrast the plate
  already teaches becomes sharper: fingerprint names the recording, CLAP
  retrieves by meaning, and neither is the other.
- **Describe the shot** — replaces the frame strip with **X-CLIP text↔video**
  retrieval over the six clips: type "a rocket launch", get the launch; type
  "an underwater animal", get the turtle.
- A **pinned retrieval eval** (queries → expected media) runs at build time and
  ships with the index, so a rebuild that breaks the model or the mel/frame
  preprocessing fails the build instead of shipping a demo that quietly stopped
  working.

### Out of scope

- No audio/video **upload**; the media ship with the page (unchanged rule).
- No live video encode (4.8 MB tensor vs the 256 KiB cap).
- No new protocol surface; `EMB.IMG`/`EMB.EVAL` stay refused.

## Capabilities

### New Capabilities

- `audio-video-embeddings`: media embeddings produced by real audio and video
  models, index built offline and committed, the query embedded live by the same
  model's text tower, a probe and a pinned eval that keep the shipped index from
  drifting from the model, and a stated provenance and licence for each export.

### Modified Capabilities

- `media-demos`: the plate's modelled reading is now a real media model rather
  than the image model reading a picture, and the phrase ranks the media library
  rather than a clip's stills.

## Impact

**Models downloaded (one-time, onto the volume):** CLAP **int8** audio 34.3 MB +
int8 text 126.6 MB = **161 MB** (the smallest published pair); X-CLIP **fp32**
video 499 MB + text 254 MB = **753 MB** (no quantized export exists). Total
**~914 MB**, roughly 1.2–1.3 GB resident with activations, which fits the
sandbox's 8 GB beside the existing sub-gigabyte model set. Task 1.3 measures the
real resident set and records the headroom; a locally int8-quantized X-CLIP is a
documented optional follow-up (task 1.4), not a prerequisite.

**New files:** `scripts/clap_text.lua`, `scripts/clap_audio.lua`,
`scripts/xclip_text.lua`, `scripts/xclip_video.lua`,
`website/tools/build-audio-index.py`, `website/tools/build-video-index.py`,
committed music and video under `website/demos/media/`, and the built
`website/assets/demo/{audio,video}.json`.

**Modified files:** `website/repl/bridge.go` (+ its tests) for the binary
allowlist, `website/repl/sandbox.yaml` (two new models, new presets),
`website/assets/js/fingerprint.js` (CLAP mel), `website/demos/medium.html`,
`website/demos/index.html`, `website/tools/{published-tree,stamp-presets}.py`,
`website/demos/samples/CREDITS.md`, `justfile`, `website/assets/css/styles.css`.

**Deploy:** `just website-presets` + `just sandbox-deploy`.
