# Tasks

## 1. Reveal on the fragment-naming plates

- [x] 1.1 In `website/demos/laya.html`, scroll `#try-it-h` into view when the page loads with a fragment naming an example and when a known fragment arrives, after the example is mounted. Verify by opening `demos/laya.html#pacman` (the page lands on the TRY IT heading with Pac-Man selected) and by setting a known fragment on the loaded page (it selects and reveals).
- [x] 1.2 In `website/demos/atlas.html`, reveal `#try-it-h` when the page loads with `#meaning`/`#year` and on a known `hashchange`, after the order is set. Verify `demos/atlas.html#year` lands on TRY IT with the year order selected.
- [x] 1.3 In `website/demos/lens.html`, reveal `#try-it-h` when the page loads with `#minilm`/`#bge-small` and on a known `hashchange`. Verify `demos/lens.html#bge-small` lands on TRY IT with bge-small selected.
- [x] 1.4 In `website/demos/image.html`, reveal `#try-it-h` when the page loads with a sample fragment and on a known `hashchange`. Verify `demos/image.html#storm` lands on TRY IT with the storm sample selected.

## 2. Verification

- [x] 2.1 Confirm a click does not scroll: on a loaded plate, note the scroll position, click a different control, and confirm the page keeps its position (only `hashchange` reveals, and `replaceState` fires none).
- [x] 2.2 Confirm the default load does not jump: open each plate with no fragment and with an unknown fragment, and confirm the page opens at the top and the default choice renders.
- [x] 2.3 Confirm the sticky masthead does not cover the revealed heading: after a naming fragment lands, `#try-it-h` is visible below the masthead, not behind it.
