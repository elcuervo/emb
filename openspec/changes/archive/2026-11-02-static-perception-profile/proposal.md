# Proposal

## Why

The perception plate draws an orange profile marker across both the flat still and
the 3D relief. It reads as a slider, and moving it changes nothing about the depth
answer the plate exists to show. The depth slice is already drawn in orange in the
profile figure below the stage.

## What Changes

- Remove the orange profile line and its two endpoint dots from the still and the
  relief.
- Remove the drag-across-the-still interaction and the profile figure's `from`,
  `to` and `row` range inputs.
- Fix the profile at the values it currently starts with: row 50, from 8 to 92
  percent. Keep the orange depth chart and its measured readout.
- Update the plate's copy, metadata, gallery blurb and surface brief so they no
  longer describe a draggable line or endpoints marked on the two images.
- **BREAKING** (demo interaction): the reader can no longer define a profile
  line. The chart remains, but it is one fixed slice.

## Prerequisite

The requirement that defines the draggable profile lives in the completed but
unarchived change `dinov3-parts-plate`, not in `openspec/specs/embedding-demos/`.
That change must be archived before this change's delta can modify the
requirement; otherwise the delta has no base requirement and would leave a
competing profile requirement behind.

## Capabilities

### New Capabilities

None.

### Modified Capabilities

- `embedding-demos`: the profile becomes one fixed default slice instead of a
  reader-defined line, and the plate no longer marks that line on the still or
  the relief.

## Impact

- `website/demos/perception.html`: the overlay SVG, the 3D profile group, the
  drag handlers, the three range inputs, the profile copy and the readout prompt.
- `website/demos/index.html`: the plate VII description.
- `website/tools/pages.json`: the perception title, description and social text
  stamped into the page.
- `website/.impeccable/surfaces/website-demos-perception-html.md`: the visitor
  job and the figure contract.
- `openspec/specs/embedding-demos/spec.md`: the profile requirement and the
  controls requirement, after the prerequisite archive.

No server, model, protocol or sandbox change.
