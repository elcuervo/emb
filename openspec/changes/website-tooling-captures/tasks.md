# Tasks

## 1. Re-record the committed takes

- [x] 1.1 Fetch the three models `website/tools/topviz/models.yaml` names and this checkout lacks (`just download-model Xenova/bge-small-en-v1.5 ./models/bge-small`, the same for `jina-small` and `e5-small`) and verify all four `onnx:` paths in the config exist, because both rigs refuse to start without them
- [x] 1.2 Re-run `just website-topviz` inside the dev shell and verify both pages carry a new cast name, the caption names the new run's date, commit and captured version, the previous cast and capture are gone, and `website/tools/topviz/runs/frame.txt` shows the dashboard's current readouts including the `SIZING` row
- [x] 1.3 Inspect the new still frame and confirm `trim.py --until-pct 65` still lands in the sustained phase (all four models active, not the idle opening or the drain), re-deriving the frame with a different percentage if it no longer does; verify by reading `runs/frame.txt` against the middle of `runs/take.cast`
- [x] 1.4 Re-run `just website-clusterviz` and verify `docs/operations.md` references the new capture's name and exactly one cluster GIF exists; record in the change's notes whether that run rendered the fleet sizing line, which appears only while a node's headroom is spent
- [x] 1.5 Verify `just website-published` and `just website-version-check` pass after both re-records, so the served set and every version-bearing element still agree with `VERSION`

## 2. The brief, and the fleet take on the documentation surface

- [x] 2.1 Run `impeccable context` for `website/docs/index.html` and amend `website/.impeccable/surfaces/website-docs-index-html.md` with the direction contract for the two new figures before any markup edit; verify the amendment exists and states the constraints the specs carry — the run's own text rather than a picture, no unsolicited motion, one accent signal, and every drawn value in the caption
- [x] 2.2 Fix `publish.py`'s cast cleanup so a single-node publish cannot delete the fleet take, using the cluster-prefix guard its GIF cleanup already has; verify by placing `assets/cast/emb-top-cluster-deadbeef.cast` and confirming a publish leaves it in place while still replacing the previous single-node cast
- [x] 2.3 Add the region-scoped stamp to `publish.py` (the fleet block between `<!-- fleetviz:begin -->` and `<!-- fleetviz:end -->`) and the fleet publish path — the take, its still frame, its caption, its figures, and its own anchored entry in `published-tree.py`'s served set; verify a run writes `assets/cast/emb-top-cluster-<sha8>.cast`, the fleet region points at it spelled as that page needs it, and `python3 website/tools/published-tree.py` passes
- [x] 2.4 Extend `website-clusterviz` so its one fleet run also trims the take, derives the still frame and publishes both forms — the cast into the served tree, the capture into `docs/assets/`; verify both come from the same `runs/cluster-take.cast` and that a second run leaves exactly one cast and one capture
- [x] 2.5 Add the fleet plate's markup to `website/docs/index.html` (mount, still frame, caption, screen-reader figures) inside the region markers, styled from the existing atoms in `website/assets/css/docs.css`; verify in the browser that scripting off shows the frame and complete figures, `prefers-reduced-motion: reduce` plays nothing, and the player's own control starts the recording

## 3. The `emb-stress` capture rig and its figure

- [x] 3.1 Write `website/tools/stressviz/`'s scenario: two nodes from `topviz`'s `models.yaml` on one port, one on `127.0.0.1` and one on `[::1]`, cache-enabled and ready-checked before the run; verify by running the bring-up alone, and by holding one address with a listener to watch it refuse
- [x] 3.2 Record the `emb-stress` run once at the scenario's stated levels and dwell, keep the run's own output, and run the `emb-top -node … -frames` sidecar beside it; verify the output parses into per-level and per-endpoint values, every endpoint shows non-zero completed work, nothing is reported under `NOT EXERCISED`, and the record names both endpoints' resolutions
- [x] 3.3 Draw the plate from that run — the level curve, the per-endpoint offered-versus-observed shares, and the sizing lines the frames produced — and publish its stamped values, its caption and its figure text into `website/docs/index.html`; verify every drawn value appears in the caption, a second publish over the same run rewrites the same text (so no value is hand-typed), and the drawing adds no hue, gradient, radius or shadow
- [x] 3.4 Render the run's own output to the documentation's capture and publish it: `agg` to `docs/assets/stressviz-<sha8>.gif`, its reference inside `docs/operations.md`'s `Load: emb-stress` section, and the run's record between markers in `BENCHMARK.md` (machine, fleet, method, invocation, and the values the figure draws); verify a re-run replaces the capture and the record and leaves exactly one of each
- [x] 3.5 Add `just website-stressviz` — build, model check, port guard, run, publish, served-tree check — and verify it end to end inside the dev shell: exit 0, one figure's worth of stamped values, one capture, one record, and `python3 website/tools/published-tree.py` passing

## 4. The surfaces' prose and the repository's source of truth

- [x] 4.1 Document the fleet pairing in the docs surface's operations section: what the run takes as input, what it offers, what it reports per endpoint and for the fleet, and that the two tools together separate the offered share from the served share, with the invocation that reproduces the figure; verify every command and flag it states exists in the shipped tools, and that the section order and anchors are unchanged
- [x] 4.2 Update `README.md`'s tooling entry and its Operations link so the site's fleet claims are sourced there; verify every statement the two surfaces make about the tools appears in `README.md` or `docs/operations.md` and contradicts neither
- [x] 4.3 Verify each value the figure draws appears in `BENCHMARK.md` with the machine, the fleet and the invocation, and that the caption states the values themselves rather than a summary; verify by reading the record against the plate's own stamp, not against the design

## 5. Integration

- [x] 5.1 Run `impeccable detect --json` once over the changed HTML and CSS and compare it finding by finding against the same run in a detached worktree at `HEAD`; resolve or record every difference in the surface brief
- [x] 5.2 Verify both surfaces from `file://` and from `just website`: no third-party request, both tooling plates complete with scripting off, each recorded run named by its own caption and figures, and no image of a dashboard anywhere in the served tree
- [x] 5.3 Measure the added served bytes (the fleet take and the stress capture, raw and compressed) and record them in the change's notes, confirming `_headers` already pins `/assets/cast/*` and declares its type
- [x] 5.4 Run the repository's checks inside the dev shell — `just website-published`, `just website-version-check`, `just website-presets-check`, `just lint`, `just test` — and verify all green

## Workflow follow-up

- Archive the change once the review requirements are satisfied, syncing the four deltas into `openspec/specs/`.
- Verify the archived result and that `openspec list` no longer shows the change.
