## 1. The trace in `website/assets/js/demos.js`

- [x] 1.1 Add a trace array and record `{args, bin}` in `exec()` before the fetch, so every command path (`embed`, `search`, `preset`, `imagePreset`, `models`, `raw`) is covered by the one choke point; verify by checking that `exec` has exactly one call per path and the record sits before `fetch`
- [x] 1.2 Add `begin()` and `trace()` to the object `gallery()` returns: `begin()` clears the array, `trace()` returns a copy of it; verify that calling `begin()` then `trace()` with no exec between returns an empty list
- [x] 1.3 Extend `argvLine(args, bin)` so an index in `bin` renders as `[N bytes]` built from the decoded argument's length, and quoting is unchanged for every other argument; verify against the image preset's argv that the base64 payload never reaches the output
- [x] 1.4 Add `unfold(node, entries)` beside `argvLine`: it fills the node's body with one `argvLine` per entry, unhides the disclosure, and hides it when there are no entries; verify that an empty trace leaves the disclosure hidden rather than empty

## 2. The disclosure's markup and styles

- [x] 2.1 Add the disclosure styles to `website/assets/css/styles.css` — a `<details>` summary row and body built from the existing rule variables and the `code` / `code__body` / `t-cmd` atoms, with no new colour, font, or panel; verify with `just website-dev` that the block is legible on a paper and on a dark plate
- [x] 2.2 Add the `[data-cmds]` disclosure markup inside each rig's `[data-live]` output on all eleven plates, hidden until a run; verify that with scripting disabled the plate reads unchanged and no disclosure appears

## 3. Wire the eleven plates

- [x] 3.1 `vector.html` and `similarity.html`: call `g.begin()` at the top of `run()` and `unfold()` on the trace when the result renders — and delete `similarity.html`'s hand-built `source.textContent` line and `vector.html`'s unused `argvLine` import; verify by running each plate twice and unfolding that the second run's own text is shown, and that neither page shows its command twice
- [x] 3.2 `search.html`, `atlas.html`, `lens.html`, `multilingual.html`: the same wiring, where a run issues a search, a preset, or both; verify each unfolded list names the commands that run actually issued, in the order issued
- [x] 3.3 `function.html` and `image.html`: the same wiring for a run with four calls and a run carrying binary; verify `function.html` unfolds all four argv including two distinct preset digests, and `image.html` marks the binary argument as bytes rather than base64
- [x] 3.4 `batch.html`, `cache.html` and `graph.html`: the same wiring for plates whose runs call `g.raw` directly, which the gallery's state machine never sees; verify that each plate's second run replaces the first run's commands, and that `cache.html` shows both the repeated call and its repeat

## 4. Confirm the contract end to end

- [x] 4.1 With `just website-dev` running, unfold every plate's disclosure after a run and confirm each lists only argv — no timing, no reply shape, and no browser-side SQL; verify by reading all eleven against the "commands and nothing else" scenario
- [x] 4.2 Confirm the failure path: point a plate at an unreachable sandbox (or run `function.html` against a digest the sandbox does not hold) and check the commands issued before the failure are still unfolded with no reply or timing invented
- [x] 4.3 Run `just website-ink http://localhost:8080 demos/similarity.html` and `just website-ink http://localhost:8080 demos/image.html` and confirm both PASS with the new block present; confirm `just website-presets-check` still passes, since the static specimen's digests are unchanged

## 5. The fold's motion and the long command

- [x] 5.1 Animate the unfold — the platform's own details content grows where the engine animates it, a keyframe runs the body in everywhere, and reduced motion shows it in one frame — and verify by opening a disclosure with motion at the default and again with reduced motion forced
- [x] 5.2 Clamp each unfolded command to two lines with the platform's ellipsis, leaving the full argv in the document, and verify on the atlas blend — whose preset command carries whole passages — that the disclosure is not a wall of text while selecting it still yields the complete command

## 6. The quoted passages and the top link

- [x] 6.1 Extend the clamp beyond the run's own command to the surfaces a plate quotes, keeping `.shape__body` out so the function plate's fixed reply is not cut, and verify on the atlas that a passage shows three lines with an ellipsis while the passage's full text stays in the document
- [x] 6.2 Add the next plate's link to the head of every plate that has a next, naming the plate and matching the foot rail, and verify on the atlas and the multilingual plate that the top link and the rail name the same plate and that the last plate carries none
