## 1. Prove the toolchain is shippable

- [x] 1.1 Verify `asciinema`, `asciinema-agg` and `libwebp` introduce no source builds into the website shell: `nix-store -qR $(nix eval --raw .#devShells.aarch64-darwin.website.drvPath) | grep '\.source.*\.drv$'` prints nothing. If it does, record the fallback (`nix shell nixpkgs#asciinema-agg`) in `design.md` before continuing.
- [x] 1.2 Add `asciinema`, `asciinema-agg` and `libwebp` to `websiteDeps` in `flake.nix` and verify `nix develop .#website --command bash -c 'asciinema --version && agg --version'` succeeds.

## 2. The recorded node and the traffic scenario

- [x] 2.1 Write `website/tools/topviz/models.yaml`: three or four models by local `onnx`/`tokenizer` path, cache enabled, `listen: 127.0.0.1:16379`, every model alias at most 13 characters, at least one model whose dimension differs from the others. Verify with `./bin/emb -config website/tools/topviz/models.yaml` plus `redis-cli -p 16379 EMB.MODELS` that every model loads and the names are not truncated in `emb-top`.
- [x] 2.2 Fetch the small models the config names (via `just download-model <repo> <dir>`), and verify the target's missing-model failure path names the absent file and that exact command.
- [x] 2.3 Write `website/tools/topviz/traffic.sh`: a `redis-cli`-driven scenario that staggers per-model start and stop so the heatmap shows bands, mixes short and long texts so percentiles move, and repeats texts so the cache ratio moves. It sends no request intended to fail. Verify against a live node: `emb-top -once -samples 30` shows at least three models with non-zero rates, a rising cache hit ratio and latency percentiles that respond to text length.
- [x] 2.4 Write `website/tools/topviz/run.sh`: start `traffic.sh` in the background with stdout and stderr redirected to the run's log, then `exec` `emb-top` in the foreground at the chosen size. Verify by recording once and confirming the cast contains nothing but the dashboard.

## 3. The rig

- [x] 3.1 Add the `website-topviz` target to the `justfile`: refuse to run without the server half (`go`, `redis-cli`, a non-empty `ort_lib`) with a message naming `nix develop`; build `emb` and `emb-top`; start the node; wait on `EMB.READY` with a deadline; warm the cache; record; render; publish; print artifact names and sizes. Verify the whole run from a clean tree exits 0 and the failure paths (no models, node never ready) exit non-zero with a diagnostic.
- [x] 3.2 Record at `--window-size 120x40` and verify no dashboard row wraps or is clipped in the cast, by comparing the recorded frame against `emb-top`'s own width behaviour at the same size.

## 4. Render, theme and edit

- [x] 4.1 Render with `agg --theme` built from the site's tokens (plate `#111110`, paper `#F3F0E8`, accent `#FF5A1F` on the request-rate stream) and verify against the plate's own background and text colours.
- [x] 4.2 Tune `--speed`, `--idle-time-limit`, `--fps-cap` and `--select` so the shipped clip opens idle, shows the ramp and the drain, and holds its final frame; verify by watching the shipped asset once end to end.
- [x] 4.3 Produce the animated WebP and the poster frame from the same take and verify the file size and that both frames come from one cast (no second recording).

## 5. Publish the take

- [x] 5.1 Write `website/tools/topviz/publish.py`: name the artifacts from their own bytes (`emb-top-<sha8>.webp` / `.png`), write the names and the run's provenance into `website/index.html`, update `published-tree.py`'s `SERVED` set, delete the previous take's files, and print the sizes. Verify by running the target twice: the second run writes a different name, the page references it, and `just website-published` passes with no orphaned file.
- [x] 5.2 Update the tools inventory in `website/README.md` and the stale example in `stamp-version.py`'s docstring, and verify `just website-version-check` still passes.
- [x] 5.3 Record in `website/tools/topviz/README.md` why the capture contains no errors (per-model `err/s` needs a real pipeline failure; scripted failures are not reported by `EMB.INFO`), so the omission is not re-attempted as an oversight.

## 6. The plate

- [x] 6.1 Run `impeccable context` for the landing page and write the surface brief with its direction contract before touching markup, as `product-site` requires.
- [x] 6.2 Replace `.topviz__screen` in `website/index.html`: a `<picture>` whose animated source is gated on `(prefers-reduced-motion: no-preference)`, the poster as the fallback `<img>` with explicit dimensions, an `.sr-only` text rendering of the dashboard's rows and figures, and a provenance caption written from the take rather than by hand. Verify with scripting disabled that the plate is complete and still.
- [x] 6.3 Rewrite the plate's rules in `website/assets/css/styles.css` and delete the `.tf`, `.tf--bar` and narrow-viewport rules the `<pre>` needed. Verify no rule in the stylesheet selects a class the page no longer uses.
- [x] 6.4 Verify the plate at 390px, 768px and 1440px: the still frame is not clipped, no horizontal scroll appears, and the caption wraps without splitting a value from its unit.
- [x] 6.5 Run `impeccable detect --json` once over the changed HTML and CSS and resolve or record every finding.

## 7. Integration

- [x] 7.1 Run `just website-published`, `just website-ink` and `just website-version-check`; all pass.
- [x] 7.2 Serve the site (`just website`) and confirm in a browser that the plate animates when motion is allowed and shows the still frame when reduced motion is requested.
- [x] 7.3 Confirm nothing outside the site changed: `just lint` and `just test` pass, and no file under `cmd/`, `internal/` or `gems/` is modified.
