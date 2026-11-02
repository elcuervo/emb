# Tasks

## 1. Remove the marker from both images

- [x] 1.1 Delete the still's overlay SVG (`data-profile-line`, `data-profile-a`, `data-profile-b`), the now-unused `.perception__prof` rule if nothing else uses it, and the caption "drag across it to place the profile"; verify `grep -n 'data-profile-line\|drag across it' website/demos/perception.html website/assets/css/styles.css` returns nothing.
- [x] 1.2 Delete the relief's profile line: `buildProfileLine`, `profileGroup`, `syncProfileMarkers`, `syncProfileOverlay`, their call sites (`setupStage`, `setExaggeration`, `disposeScene`) and the queries for the overlay nodes; verify no reference remains with `grep -n 'buildProfileLine\|profileGroup\|syncProfileOverlay\|syncProfileMarkers\|profileLine\|profileA\|profileB' website/demos/perception.html`.

## 2. Fix the profile to one default slice

- [x] 2.1 Remove the profile figure's `from`, `to` and `row` inputs and the prompt "drag across the photograph, or set the line here"; verify the profile figure contains no `type="range"` and the readout no longer promises a drag.
- [x] 2.2 Delete `profilePoint`, `profileDrag`, the still's pointer handlers, the three input listeners, `setProfile` and the profile input lookups; keep `S.profile` at `{ y: 0.5, x0: 0.08, x1: 0.92 }` and call `buildProfile()` when a reply arrives; verify the chart draws once after depth, and a drag across the still issues no request and moves nothing.
- [x] 2.3 Update the profile canvas `aria-label` to name a fixed horizontal slice rather than a dragged line; verify the label no longer contains "dragged".

## 3. Update the copy and metadata

- [x] 3.1 Rewrite the page's profile prose: the `WHAT JUST HAPPENED` deck, the profile caption, the stage caption and the `the index, the tolerances and the limits` paragraph; verify none of them says drag, "line you drag", or that endpoints are marked on the photograph and relief.
- [x] 3.2 Update the plate VII blurb in `website/demos/index.html` and the perception `description`, `social_description` and `twitter_description` in `website/tools/pages.json`; run `python3 website/tools/stamp-pages.py` and verify `python3 website/tools/stamp-pages.py --check` passes.
- [x] 3.3 Update `website/.impeccable/surfaces/website-demos-perception-html.md`: the visitor job, the "every figure is operable" clause and the profile gate; verify the brief no longer promises a draggable line or a marked overlay.

## 4. Verify the result

- [x] 4.1 With the sandbox reachable, load the page at 1280 px and 390 px: the still and relief show no orange line or dots, the profile chart still draws in orange at row 50 and 8–92, its readout prints the same near, far, slope and span as the pre-change default, and the console is empty.
- [x] 4.2 Run `node --check` on the extracted module, `just website-pages-check`, `just website-presets-check` and `just website-published`; verify all pass, and run the browser pass with the sandbox unavailable to confirm the page states the outage and draws no fabricated profile.

## Workflow follow-up

- The predecessor `dinov3-parts-plate` is archived; this delta now modifies a base requirement.
- Archive this change after review, then verify the archived spec keeps the fixed-slice requirement and drops the drag wording.
