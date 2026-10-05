# Tasks

## 1. Laya — the tabbed interface

- [x] 1.1 In `website/demos/laya.html`, mount the example a known fragment names on load and keep the default when it is absent or unknown, and write the fragment from the selection handler only (not from internal re-mounts). Verify by opening `demos/laya.html#pacman` (Pac-Man mounts, Snake does not), `demos/laya.html#inbox`, and `demos/laya.html` (Snake mounts, no fragment is stamped).
- [x] 1.2 Add a `hashchange` listener so a fragment change re-mounts on the already-loaded page. Verify by editing the URL to `#quickstart` without reloading (the payload replaces the surface, not appends) and by setting `#nope` (the current selection is left and no error is shown).
- [x] 1.3 Confirm selection does not add history entries. Verify that Reset (which re-mounts the same example) does not change the fragment, and that selecting a tab then pressing Back leaves the plate rather than restoring the previous tab.

## 2. The three pickers

- [x] 2.1 In `website/demos/atlas.html`, deep-link the placement order: `#meaning` and `#year`, read on load, written on selection, re-selected on `hashchange`. Verify `demos/atlas.html#year` draws the year order on load and an unknown fragment leaves the default.
- [x] 2.2 In `website/demos/lens.html`, deep-link the model lens: `#minilm` and `#bge-small`. Verify `demos/lens.html#bge-small` renders bge-small on load and the two models switch on `hashchange`.
- [x] 2.3 In `website/demos/image.html`, deep-link each sample by its basename (`data-sample="samples/storm.jpg"` → `#storm`), covering raven, storm, portrait, ship, flowers, manuscript. Verify `demos/image.html#storm` scores the storm sample on load.

## 3. Verification

- [x] 3.1 Confirm no chosen fragment collides with an existing element id on its plate: each of `snake`, `pacman`, `inbox`, `quickstart`, `meaning`, `year`, `minilm`, `bge-small`, `raven`, `storm`, `portrait`, `ship`, `flowers`, `manuscript` matches no `id=` on its page, so a shared link cannot make the browser scroll.
- [x] 3.2 Confirm the default selection still renders with scripting off on all four plates, so the fragment is an addition rather than the only path to a choice.
- [x] 3.3 Confirm the eight plates without an in-page selection and the gallery index are unchanged: no `hashchange`, `replaceState`, or fragment-writing code appears outside the four edited files.
