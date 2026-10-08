# Design

## Context

See `proposal.md` — Why. What shapes the approach:

- `emb` loads ONNX graphs and drives them through Lua presets with named-tensor
  inference; preprocessing happens **outside** the graph and packed float32 bytes
  are fed in. A scripted model's tokenizer is only needed if the script calls
  `emb.tokenize`, but `ModelConfig.Validate` stats a tokenizer file when
  `model_repo` is empty, so each new model declares one it may never use.
- The bridge admits binary only for one hardcoded preset (`imagePresetName`), and
  the sandbox caps a request at **2 images, 256 KiB each, 1 MP decoded**.
- The sandbox is `shared-cpu-4x` / **8 GB**; the existing models occupy well
  under a gigabyte.
- The gallery's established pattern is **index offline, query live**.

## Goals / Non-Goals

**Goals:**
- Real audio and video models, over genuinely good media, with a text query.
- Every claim measured: a pinned retrieval eval, a probed export, stated
  provenance and licences.

**Non-Goals:**
- No audio or video **upload**; no live **video** encode (a 4.8 MB tensor against
  a 256 KiB cap).
- No new protocol surface; `EMB.IMG`/`EMB.EVAL` stay refused.
- No training or fine-tuning; shipped exports only.

## Decisions

### D0. Probe results — the contracts, as measured

`cmd/emb-probe` loads each graph with ONNX Runtime, prints the real inputs and
outputs, and then runs it on zeros with the assumed dtype and shape. Run against
the two downloads on 2026-10-08:

| graph | inputs (measured) | output | ran |
|---|---|---|---|
| `clap/audio_model_quantized.onnx` | `input_features` rank 4, all axes dynamic; `[1,1,1001,64]` f32 accepted | `audio_embeds` `[1,512]` | ok |
| `clap/text_model_quantized.onnx` | **`input_ids` only — no `attention_mask`** | `text_embeds` `[1,512]` | ok |
| `xclip/text_tower.onnx` | `input_ids`, `attention_mask` both fixed `[1,77]` i64 | `text_embeds` `[1,512]` | ok |
| `xclip/video_tower.onnx` | `pixel_values` fixed `[1,8,3,224,224]` f32 | `video_embeds` `[1,512]` | ok |

Two things the probe corrected, which is why it is a prerequisite and not a
formality: **CLAP's text tower has no `attention_mask` input** (its card listed
one, and passing it changes nothing), and **X-CLIP's towers are fixed-shape**, so
the text tower must be padded to exactly 77 and the video tower fed exactly 8
frames. All four embeddings come back **unnormalized**, so each preset
L2-normalizes before returning.

### D1. CLAP for audio, X-CLIP for video — the only exports that exist

CLAP (`Xenova/clap-htsat-unfused`, Apache-2.0) is the maintained optimum export
of `laion/clap-htsat-unfused`: separate audio and text towers sharing a 512-d
space, and **publisher-quantized** — int8 audio **34.3 MB** + int8 text
**126.6 MB** (fp16 and fp32 also exist; fused int8 161 MB). One caveat:
`resolveQuantize` auto-detects only the literal `model_quantized.onnx`, and
CLAP's files are named `audio_model_quantized.onnx`/`text_model_quantized.onnx`,
so each model's `onnx:` points **directly** at the quantized file. X-CLIP
(`imbcmdth/xclip-onnx`, MIT) is **the only text↔video ONNX export that exists**:
`video_tower.onnx` (499 MB) + `text_tower.onnx` (254 MB), 512-d, **fp32 only**.
Wav2CLIP and AudioCLIP have no ONNX export; VideoPrism, InternVideo, ViCLIP,
ViViT and TimeSformer have none; `onnx-community` publishes **no video model at
all**; the only other video ONNX (a VideoMAE embedder) is **CC-BY-NC** and the
remaining VideoMAE exports are classification heads. The choice is therefore
forced, and the proposal says so rather than implying a curation.

### D2. Index offline; text live; audio may also encode live, video may not

Both media towers run in build tools and their vectors ship. The text towers run
live through preloaded presets. Audio additionally supports a **live encode**
because its tensor fits: the mel is `1001 × 64 × 4 = 256,256 B`, just under the
256 KiB cap. Video does not: `1 × 8 × 3 × 224 × 224 × 4 = 4.8 MB`. The asymmetry
is stated on the plate, not hidden.

### D3. Mel parity is the whole audio risk, and it is quoted from the model

The browser computes the log-mel and the build tool computes it, and a hash
match requires both to be the same function. The spec is taken from the model's
own `preprocessor_config.json` — **48 kHz mono, Hann 1024, hop 480, 64 mels,
fmin 50, fmax 14000, power 2.0, dB, no top-db clip, centre + reflect pad, 1000
frames → `[1001, 64]`** — including the reference path's **×32768** waveform
scaling. The build tool and the browser share the constants, and the pinned eval
is what proves they agree end to end.

### D4. Video sampling is fixed and said so

X-CLIP's export declares **no dynamic axes**: `[1, 8, 3, 224, 224]`, RGB, scale
`[0,1]`, mean/std inside the graph. Eight evenly-spaced frames is therefore the
contract, not a tuning knob, and the plate states that the count and the
sampling are fixed. A clip's score is a retrieval similarity, not a description
of its motion.

### D5. The bridge's binary gate becomes an allowlist

`imagePresetName` becomes a small set (`zeroshot`, `clap-audio`). Bytes still
reach only a preloaded preset called by digest, still within the caps; a text
preset still cannot carry bytes. The existing refusal tests are updated to
assert the boundary rather than removed.

### D6. Assets are chosen for quality and licence, and recorded

Eight music excerpts, all CC0 or public domain (US Army Blues jazz, Kimiko
Ishizaka's CC0 Goldberg Variations, FMA CC0 electronic, a live jazz-duo field
recording, three 1920s LOC/MMA public-domain 78s), and six public-domain video
clips (Sarychev Peak from the ISS, a moon transit, the Arecibo collapse, an
olive ridley turtle, STS-134, a GOES storm). Each asset's source and licence is
recorded in `CREDITS.md`; the three 78s are historically genuine rather than
hi-fi and are described as such.

### D7. A pinned evaluation ships with the index

Each index carries queries and the media they must retrieve; the build fails if
one stops matching. This is `emoji-joke-gallery`'s discipline applied to media,
and it is what keeps a silent model or preprocessing change from shipping a demo
that no longer works.

### D8. The fingerprint stays

The plate keeps its exact-match fingerprint (browser DSP, no model) and gains
CLAP beside it. The contrast — an exact matcher names the recording, a semantic
model retrieves by meaning — is the plate's argument, and it gets stronger when
the semantic half is a real audio model rather than a picture trick.

## Risks / Trade-offs

- **X-CLIP's export is a third party's side project** (opset 17, no dynamic axes,
  validated on a random tensor) → probe it with ONNX Runtime before any preset,
  fail the build on a mismatch, and state its provenance on the plate.
- **~914 MB of new weights (~1.2–1.3 GB resident) on an 8 GB sandbox** → fits
  beside today's sub-gigabyte set; task 1.3 measures the real number. If the
  margin is ever thin, the documented levers are, in order: quantize X-CLIP's
  towers locally with `onnxruntime.quantization.quantize_dynamic` (transformer
  MatMuls go int8; the Conv3d patch embedding stays fp32, so expect roughly a
  halving, not a 4×), keeping only the video tower fp32, or moving the demo to a
  second deployment (the DNS zone's precedent).
- **A locally quantized X-CLIP is untested** → it is only ever adopted if the
  pinned retrieval eval still passes on it, so an accuracy loss shows up as a
  failed build rather than a quietly worse demo.
- **Mel parity between browser and build** → one shared constants block, one
  spec source, and the pinned eval.
- **CLAP on music** → CLAP is trained on general audio; its music-genre reading
  is a similarity, stated on the plate.
- **Asset size** → excerpts are trimmed; the six videos are transcoded.

## Migration Plan

1. Probe both exports with ONNX Runtime; record the real tensor contracts.
2. Add the four presets and the two model entries to `sandbox.yaml`; generalize
   the bridge's binary gate; `just website-presets`.
3. Land the assets and CREDITS; build both indexes with their pinned evals.
4. Rebuild the plate's audio and video rigs; run the Impeccable pass.
5. Gate: `just website-published`, `just website-presets-check`, and the two
   index builds; then `just sandbox-deploy`.
6. Rollback: revert the presets and the model entries; the plate reports the
   models as unavailable rather than showing a stale ranking.

## Open Questions

- Whether to keep the current CLIP-picture demo as a separate "the limit" plate
  once the real models ship — deferred; it is a gallery-composition decision,
  not a design one.
- Whether a quantized X-CLIP is worth the export work — deferred until the
  memory measurement in step 1.
