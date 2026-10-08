# Tasks

## 1. Assets and the licence record

- [x] 1.1 Commit five open-licensed music excerpts (10–15 s, mono, 96 kbps) under `website/demos/media/music/`, four CC0/PD and one CC BY 3.0; verify each plays in a browser
- [x] 1.2 Cut six four-second query excerpts under `music/queries/`, one degraded with pink noise; verify each decodes in the browser
- [x] 1.3 Commit one public-domain clip under `website/demos/media/clips/` and record every asset's source, author, and licence in `website/demos/samples/CREDITS.md`; verify each committed file has an entry

## 2. The fingerprint engine

- [x] 2.1 Add `website/tools/build-fingerprints.py` (ffmpeg decode → STFT → peaks → paired hashes → offset voting) with a `--self-test`; verify the self-test recovers a noisy synthetic excerpt
- [x] 2.2 Make the build verify every shipped query against the built library and fail if one does not recover its track; verify all six queries match (103–315 votes, including the degraded one)
- [x] 2.3 Mirror the algorithm in `website/assets/js/fingerprint.js` (radix-2 FFT, peaks, hashes, matching); verify the browser and the build agree by matching a shipped query in the page

## 3. The frame index

- [x] 3.1 Add `scripts/clip_embed.lua` (a still → packed image embedding) and `scripts/clip_text.lua` (a phrase → packed text embedding), returning the value itself for a one-text call
- [x] 3.2 Add `website/tools/build-frame-index.py` (ffmpeg stills → embed each against a local server → `frames.json` with timestamps, pictures, vectors); verify sixteen stills embed at 512 dimensions
- [x] 3.3 Add a `just website-media` target that builds both indexes; verify it rebuilds reproducibly

## 4. The preset and the sandbox preload

- [x] 4.1 Register `clip_text.lua` under the `clip` model in `website/repl/sandbox.yaml` and run `just website-presets`; verify `just website-presets-check` passes and the digest is stamped into the page
- [ ] 4.2 Deploy the sandbox with `just sandbox-deploy` and verify `EMB.EVSHA clip <sha> 1 "a phrase"` returns an embedding while `EMB.EVAL` and `EMB.IMG` remain refused — **blocked: the sandbox under `website/repl/` is hand-deployed (AGENTS.md); a local bridge stands in**

## 5. The plate

- [x] 5.1 Build `website/demos/medium.html` with the five-section anatomy and two rigs; verify it draws the query buttons, the frame strip, and figures read from the shipped indexes
- [x] 5.2 Wire a picked excerpt to live fingerprinting and matching; verify the spectrogram, the hashes, the track, the offset, and the vote bars all come from that run
- [x] 5.3 Send the query's spectrogram to `zeroshot.lua` and show the model's description beside the fingerprint's answer, with cosine and margin; verify the two are labelled differently
- [x] 5.4 Play the identified track from the matched offset; verify it does not autoplay
- [x] 5.5 Wire the typed phrase to `clip_text.lua` and the shipped frame vectors; verify the nearest still is marked and the clip moves to it
- [x] 5.6 Add the caveat and the unavailable state; verify the page is complete without JavaScript and states the condition when the sandbox is unreachable

## 6. Registration

- [x] 6.1 Register the plate in `website/demos/index.html` with the `<span class="chip-new">new</span>` pill and update the gallery copy; verify the count and the pill
- [x] 6.2 Add the page, `assets/js/fingerprint.js`, the media, and the two built indexes to `website/tools/published-tree.py`; verify `just website-published` passes

## 7. Impeccable validation

- [x] 7.1 Run `impeccable context` once and write the surface brief with a direction contract
- [x] 7.2 Run the detector once and fix the findings this change introduced (near-black text on the dark ground, sub-12px labels, an image with no `src`, the portrait spectrogram aspect); verify no actionable finding remains
- [ ] 7.3 Run one bounded mobile-width pass; **owed**

## 8. Integration checks

- [ ] 8.1 Verify the plate loads, identifies a shipped excerpt, describes it, and finds a moment against a local bridge — **verified locally end to end; the deployed sandbox still awaits 4.2**
- [x] 8.2 Verify no upload path exists — no file input, drop target, paste path, microphone, or camera — and that binary is sent only as a picture through `zeroshot`
- [ ] 8.3 Rebuild both indexes with `just website-media` and verify the plate's figures update without a hand-edit

## Workflow follow-up

- Run `just sandbox-deploy` so `clip_text.lua` is resident, then finish 4.2 and 8.1.
- Run the mobile pass of 7.3 and complete 8.3.
- Archive the change after the project's review requirements are satisfied.
