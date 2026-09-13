## 1. Withhold the panel

- [x] 1.1 Add `hidden` to the console `<section>` in `website/index.html`, with the comment above it naming the withheld state, the selector, the one-attribute revert, and the seam. Verify `curl -s http://localhost:8080/index.html | grep -o 'aria-labelledby="console-h" data-reveal hidden>'` returns the attribute, and that no other attribute or inline style is used to hide it.
- [x] 1.2 Change the boot selector in `website/assets/js/main.js` to `[data-console="transcript"]:not([hidden])` and extend the section comment to state that the withheld panel does not boot. Verify `grep -n 'transcript"\]' website/assets/js/main.js` shows the `:not([hidden])` guard and `node --check website/assets/js/main.js` exits 0.
- [x] 1.3 Add `.console[hidden]{ display: none; }` beside the `.console` block in `website/assets/css/styles.css`, so the hide does not depend on the UA default remaining unoverridden by a future `display` value on `.console`. Verify `grep -n 'console\[hidden\]' website/assets/css/styles.css` shows the rule and that `.console` still declares no `display`.
- [x] 1.4 Record the rule in `website/README.md` beside the console section: the panel ships hidden, the artifact and seam are retained, and removing the single attribute restores the placeholder. Verify the new paragraph names both the withholding attribute and `window.embConsole.exec`, and that no earlier claim in the section ("it is a placeholder, and it says so") is left contradicted.

## 2. Verify the withheld state on the shipped page

- [x] 2.1 Confirm the panel is out of the rendered page and out of layout. With the site served locally, confirm the section computes `display:none`, has a zero-height rect, and adds no space between the capability grid and the block end.
  - Measured at 1280x900: `hidden: true`, `display: "none"`, `getBoundingClientRect().height: 0`.
- [x] 2.2 Confirm the panel is unreachable and its client is not running. Verify none of its controls can take focus and that the adapter global is absent from the page.
  - Measured: `#console-in.offsetParent === null`; calling `#console-in.focus()` did **not** move `document.activeElement` (still `body`); `typeof window.embConsole === "undefined"`.
- [x] 2.3 Confirm the hide is a property of the document, not of the script. Verify the served HTML carries `hidden` on the section and that no JS runs to apply it, so a browser with JavaScript blocked still withholds the panel.
  - The served bytes contain `data-reveal hidden>` on the section and the UA `[hidden]` rule is the only thing needed; the console client is gated on the attribute, not the reverse.
- [x] 2.4 Confirm the withheld panel cannot reintroduce horizontal overflow, and that the page's existing measurements are unaffected. Verify `just website-ink` reports PASS on the landing when the browser tooling is available, or, without a browser, that `document.documentElement.scrollWidth === window.innerWidth` at the reference width.
  - Measured at 1280: `scrollWidth 1280 === innerWidth 1280`.

## 3. Verify the one-attribute restore

- [x] 3.1 Copy the served tree to a scratch directory, remove only the `hidden` attribute from the console section, and load it. Verify the placeholder renders and the transcript client boots, which proves the attribute is the whole switch.
  - Measured on the scratch copy: `hidden: false`, `display: "block"`, `typeof window.embConsole === "object"`, `#console-in.disabled: false`, `#console-out` painted `Type a command. Try EMB minilm "hello world".`, `scrollWidth 1280 === innerWidth 1280`.
- [x] 3.2 Confirm the scratch copy differs from the shipped tree by exactly that one attribute, so the restore path cannot depend on an undocumented second edit.
  - `diff` between the two `index.html` files is the single attribute on the console `<section>`; `main.js` and `styles.css` are byte-identical.

## 4. The site's existing gates still pass

- [x] 4.1 Run the site's checks and confirm the change does not move the published set or the version stamps: `just website-published` and `just website-version-check`. Verify both exit 0 with the same served-path count and stamped-element count as before the change.
  - Measured: `published-tree: ok (12 served paths, one origin at https://emb.is)`; `stamp-version: ok (4 stamped element(s), all '0.4.0.pre4')`.
- [x] 4.2 Validate this change: `openspec validate hide-console-until-live-runtime --strict`. Verify it reports the change as valid, and that the MODIFIED console requirement keeps every original scenario name.
- [x] 4.3 Confirm the console's specified design contract is unbroken by the hide: the panel's modes, states, transcripts, styles and adapter seam are all still present in the tree, and only the rendering is withheld. Verify by inspection that `main.js`'s transcript map and `window.embConsole.exec` are untouched and that no console markup, style, or transcript was deleted.
