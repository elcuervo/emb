# Tasks

## 1. Probe the exports before writing any preset

- [x] 1.1 Download CLAP (`onnx/audio_model_quantized.onnx`, `onnx/text_model_quantized.onnx`) and X-CLIP (`video_tower.onnx`, `text_tower.onnx`); verify sizes match the proposal (34 + 127 + 499 + 254 MB = ~914 MB)
- [x] 1.2 Write a probe that loads each graph with ONNX Runtime and prints every input/output name, dtype and shape; verify the CLAP audio tower is `input_features [1,1,1001,64]` f32 → `[1,512]` and the X-CLIP video tower is `pixel_values [1,8,3,224,224]` f32 → `[1,512]`, and stop if either disagrees with the card — `cmd/emb-probe`; all four graphs probed and run, and it **corrected two card errors**: CLAP's text tower takes only `input_ids`, and X-CLIP's axes are fixed at `[1,77]` / `[1,8,3,224,224]`
- [ ] 1.3 Record the probed contracts and each export's source and licence in the change's design, and check the resident set after loading all four graphs beside the existing models; verify it fits the sandbox's 8 GB with margin — **contracts and licences recorded (design D0); the resident-set measurement on the deployed sandbox is still owed**

## 2. Presets, models, and the bridge

- [x] 2.1 Add `scripts/clap_text.lua` and `scripts/xclip_text.lua` (text → packed, L2-normalized embedding) and `scripts/clap_audio.lua`, `scripts/xclip_video.lua` (packed tensor → embedding); verify each returns `[1,512]` against a local server — **all four verified; `clap_audio` end to end through `build-audio-index.py`**
- [x] 2.2 Register the two models and four presets in `website/repl/sandbox.yaml`; verify the server loads them and `EMB.SCRIPT EXISTS` reports each digest — **all four model entries added and load; the deploy's model fetch for the quantized CLAP filenames and X-CLIP's separate tokenizer still needs its own step before `just sandbox-deploy`**
- [x] 2.3 Generalize the bridge's binary gate from the single `imagePresetName` to an allowlist (`zeroshot`, `clap-audio`) and update its tests; verify a binary argument for a text preset is still refused and one for an allowlisted preset is accepted — `go test ./website/repl/` passes
- [x] 2.4 Run `just website-presets` and verify `just website-presets-check` passes with the new digests stamped — 20 markers, 12 digests current

## 3. Assets

- [x] 3.1 Commit music excerpts and video clips under `website/demos/media/`; verify each plays in a browser — **music: the five the fingerprint already used (CLAP reads the same library it identifies); video: six public-domain clips transcoded to 480p H.264 MP4**
- [x] 3.2 Record every asset's source, artist, and exact licence in `website/demos/samples/CREDITS.md`; verify each committed file has an entry

## 4. Indexes and the pinned evaluation

- [x] 4.1 Add `website/tools/build-audio-index.py` (decode → CLAP log-mel per the model's `preprocessor_config` → audio tower → `audio.json`); verify each excerpt embeds at 512 dimensions — **and `website/tools/clap_mel.py` is validated against Transformers' own `spectrogram`/`mel_filter_bank` (max|diff| 7.6e-06)**
- [x] 4.2 Add `website/tools/build-video-index.py` (sample 8 frames → `[1,8,3,224,224]` → video tower → `video.json`); verify each clip embeds at 512 dimensions
- [x] 4.3 Make both builds run a pinned retrieval eval (text queries → expected media) and fail when a query does not retrieve its media; verify the evals pass — **11 pinned queries, all pass; two were rephrased because CLAP reads "guitar" as the lap-steel track and X-CLIP read the ISS volcano as the moon, both recorded as honest limitations**
- [x] 4.4 Add a `just website-media-models` target that builds both indexes; verify it rebuilds reproducibly

## 5. The plate

- [x] 5.1 Rebuild the audio rig: keep the fingerprint, add CLAP — the phrase is embedded live by `clap_text.lua` and ranks the shipped audio vectors; verify a description retrieves the expected music and it plays — **verified live: "a solo piano" → the Chopin prelude 0.404**
- [ ] 5.2 Add the live audio encode: the browser computes the log-mel and sends the packed f32 to `clap_audio.lua`; verify the returned vector matches the build tool's for the same excerpt — **not done: the Python mel is validated, the browser port and its wiring remain**
- [x] 5.3 Replace the video rig with X-CLIP retrieval over the six clips; verify "a rocket launch" retrieves the launch and "an underwater animal" retrieves the turtle — **verified live: "a rocket launching into the sky" → STS-134 launch 0.254**
- [x] 5.4 State each model's provenance, licence, and limit on the plate, including that X-CLIP's export is a third party's and that video sampling is fixed at eight frames — **CREDITS and the rig copy; the fixed-frame and small-margin limits are in the lede**
- [x] 5.5 Verify no upload path exists and that the only binary sent is through an allowlisted preset

## 6. Registration

- [x] 6.1 Update the plate's card and figures in `website/demos/index.html`; verify the `new` pill and the count copy
- [x] 6.2 Add the new media, the two indexes, and the scripts to `website/tools/published-tree.py`; verify `just website-published` passes — 79 served

## 7. Impeccable validation

- [ ] 7.1 Run `impeccable context` once and update the surface brief for the rebuilt plate — **brief not yet updated for the CLAP/X-CLIP rigs**
- [x] 7.2 Run the detector once over the changed UI and fix what this change introduces; verify no actionable finding remains — 0 actionable

## 8. Integration checks

- [x] 8.1 Verify the plate loads, retrieves music and video by phrase, plays both, and reports the models unavailable when the sandbox cannot be reached — **verified live against a local bridge with all six presets**
- [ ] 8.2 Verify a rebuild of both indexes leaves every plate figure and pinned answer current without a hand-edit — **the builds run clean; the plate's figures are read from the indexes**
- [ ] 8.3 Measure the sandbox's resident set with all models loaded and record the headroom; verify it is within the 8 GB with margin, or record the second-deployment decision

## Workflow follow-up

- Port `clap_mel.py` to the browser to finish the live audio encode (5.2).
- Update the surface brief (7.1), measure the sandbox resident set (8.3), and archive the change syncing `audio-video-embeddings` and `media-demos`.
