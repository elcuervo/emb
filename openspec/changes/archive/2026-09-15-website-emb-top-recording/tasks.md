## 1. Prove the toolchain is shippable

- [x] 1.1 Verify `asciinema`, `asciinema-agg` and `libwebp` introduce no source builds into the website shell (`nix-store -qR` over the devShell shows no `-vendor` path built locally), and confirm by watching `nix develop .#website` copy them from the cache rather than build them.
- [x] 1.2 Add `asciinema`, `asciinema-agg` and `libwebp` to `websiteDeps` in `flake.nix` and verify `nix develop .#website --command bash -c 'asciinema --version && agg --version'`.

## 2. The recorded node and the traffic scenario

- [x] 2.1 Write `website/tools/topviz/models.yaml`: four models by local `onnx`/`tokenizer` path, cache on, `listen: 127.0.0.1:16379`, aliases at most 13 characters, one model of a different width, `intra_op_threads: 3`. Verified with `EMB.MODELS` that every model loads and with a frame that no name is truncated.
- [x] 2.2 Fetch the models the config names, and verify the rig's missing-model path names the file and the exact `just download-model` command.
- [x] 2.3 Write `website/tools/topviz/traffic.sh`: staggered `redis-benchmark` bands, scheduled by the parent with `sleep` alone and killed per model, mixed text lengths, bounded text pools. Verified against a live node with `emb-top -once`: at least three models non-zero, a rising cache hit ratio, percentiles that respond to text length, and a 32-second run with the full ramp and drain.
- [x] 2.4 Write `website/tools/topviz/run.sh`: the dashboard in the foreground of the pty, the scenario behind it with its output logged. Verified that the recording contains nothing but the dashboard.

## 3. The rig

- [x] 3.1 Add the `website-topviz` target to the `justfile`: refuse to run without the server half, build the binaries, start the node, wait on `EMB.READY`, record headless, trim, render, publish, verify. Verified end to end, including the missing-model and not-ready failure paths.
- [x] 3.2 Record at `--window-size 120x32` and verify no dashboard row wraps or is clipped, by inspecting the recorded frames and the header surviving to the last one.
- [x] 3.3 Add `--until-pct` to `trim.py` and derive the page's frame from the cast with `asciinema convert -f txt`; verify the frame lands in the sustained phase (every model non-zero, the window largely filled) rather than on the idle opening or the drain.
- [x] 3.4 Render the documented capture with `agg` as a GIF at a geometry legible at the documentation's measure, themed from the site's tokens; verify the accent carries the request-rate stream and record the file size.

## 4. Publish

- [x] 4.1 Rewrite `publish.py` for the two artifacts: stamp the frame between the plate's `<pre>` markers and the run's provenance into the caption, write the capture into `docs/assets/` under a content-hashed name, replace the block between the markers in `docs/operations.md`, and delete the previous take in both places. Verify by running the rig twice: the second run writes different names that both surfaces reference, and no orphan is left.
- [x] 4.2 Remove the site's take assets and their two lines from `published-tree.py`'s served set, and verify `just website-published` passes with the served set back to its previous size.
- [x] 4.3 Update `website/README.md`'s tools inventory, the rig's `README.md` (the pipeline, the frame, the capture), and the surface brief's amendment; verify `just website-version-check` still passes.

## 5. The plate and the docs render

- [x] 5.1 Replace the image plate in `website/index.html` with a `<pre>` carrying the captured frame, the provenance caption beside it, and the run's figures as a sentence; verify the page carries no image element for the dashboard.
- [x] 5.2 Return the plate's rules in `website/assets/css/styles.css` to the text treatment (the dark plate, the mono measure, the scrollbar suppression, the print rule) and delete the image-specific rules; verify no rule selects a class the page no longer uses.
- [x] 5.3 Verify at 390px, 768px and 1440px: below the narrow breakpoint the plate is hidden, the figures sentence is visible at the type floor, and the region does not scroll sideways; above it the frame renders and the sentence is not shown twice.
- [x] 5.4 Replace the hand-drawn ASCII render in `docs/operations.md` with the capture block, verified by opening the rendered markdown and confirming the image resolves and the block sits where the render did.

## 6. Integration

- [x] 6.1 Run `just website-published`, `just website-ink` and `just website-version-check`; all pass.
- [x] 6.2 Run `impeccable detect --json` in place over the changed HTML and CSS and compare against the same run at `HEAD`; resolve or record every finding.
- [x] 6.3 Serve the site and confirm in a browser that the plate renders as text, the caption matches the take, the narrow layout shows the figures, and nothing in the panel animates.
- [x] 6.4 Confirm nothing outside the site and docs changed: `just lint` and `just test` pass, and no file under `cmd/`, `internal/` or `gems/` is modified.

## 7. The plate plays the take (amendment)

- [x] 7.1 Vendor the player: pin the release tarball by hash in `flake.nix`, add a `website-player` target that copies `asciinema-player.min.js` and `asciinema-player.css` into `website/assets/{js,css}/` under names carrying the release version, and assert their digests. Verify a re-vendor is byte-identical and that `nix-store -qR` over the website shell still shows no locally built source.
- [x] 7.2 Teach `publish.py` the take: `--cast runs/take.cast` writes `website/assets/cast/emb-top-<sha8>.cast`, deletes the previous cast, stamps the plate's cast path, and rewrites the cast's entry in `published-tree.py`'s served set. Leave the GIF path exactly as it is. Verify by running the rig twice: both names change, both surfaces reference the second, and no orphan is left.
- [x] 7.3 `website/index.html`: keep the captured frame in the plate as its still state, add the plate's mount point and the stamped cast path, load the player's stylesheet in the head and its script `defer` on the landing only. Verify the frame renders before any script runs and the page carries no image element for the dashboard.
- [x] 7.4 `website/assets/js/main.js`: create the player from the plate's stamped cast path with `autoplay` under the existing `prefers-reduced-motion` gate, `loop`, and `speed` matching the GIF's pacing, a poster inside the take, and `controls` on. Verify with scripting disabled, with `reduce` set, and confirm the control stops and resumes it.
- [x] 7.5 `website/assets/css/styles.css`: size the player to the plate's measure, theme it from the site's own tokens (plate, paper, the accent on the request-rate stream), and keep the plate's one-cell leading. Verify no rule selects a class the page no longer uses and that the player's chrome does not widen the region.
- [x] 7.6 `website/_headers`: add the content-hashed `/assets/cast/*` immutable rule; verify `just website-published` still passes, including its assertion that no `.js` or `.css` path falls under an immutable rule.
- [x] 7.7 Update the rig's `README.md` (the page plays the cast; why the frame stays as the still state; why pacing is a player option and not a second file), `website/README.md` (the tools inventory and the player's vendored provenance) and the surface brief's amendment; verify `just website-version-check` still passes.
- [x] 7.8 Verify in a browser at 390px, 768px and 1440px: the plate plays, pauses on the control, holds the frame with reduced motion and with scripting off, makes no third-party request, and does not scroll sideways; below the narrow breakpoint the figures sentence still stands in for the plate (and the take is not fetched at all).
- [x] 7.9 Run `just website-published`, `just website-ink`, `just website-version-check` and `just website-player-check`; run `impeccable detect --json` in place over the changed HTML and CSS and resolve or record every finding.
- [x] 7.10 Confirm nothing outside the site and docs changed: `just lint` and `just test` pass, and no file under `cmd/`, `internal/` or `gems/` is modified.
