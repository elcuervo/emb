## 1. Before any markup edit

- [x] 1.1 Extend `website/README.md`'s console section with the direction contract for this pass — the plate is a terminal with one prompt, one transcript and a status strip; examples are the idle state; a scripted check of the panel's tab order — and verify `impeccable context --target website/index.html` has been run for both target pages, recording the contract's file path in the change's notes rather than in a new artifact
- [x] 1.2 Confirm that `openspec/changes/deploy-sandbox-repl` archives before this change (its `product-site` deltas are this change's baseline), and verify with `openspec status --change deploy-sandbox-repl --json` that its remaining tasks are deployment-only; if it cannot archive first, record the conflict and stop before editing specs

## 2. Recall in the shared client

- [x] 2.1 Add command history to `website/repl/terminal.js` — recorded on `submit`, exposed as a recall call that walks the history, stops at both ends and restores a held draft — and verify with a direct `node -e` exercise of the exported API over a short sequence of submissions
- [x] 2.2 Keep `format`, `create` and the existing state machine unchanged, and verify `website/repl`'s existing checks pass: `nix develop --command bash -c 'cd website/repl && go test ./...'`

## 3. The console becomes a REPL

- [x] 3.1 Replace the landing console's mode tablist with a status strip carrying the console's own state and the existing `RESP` selector, and verify by loading the served page: no mode control is present, `RESP 2`/`RESP 3` still changes the reply's shape
- [x] 3.2 Build the idle state's demonstration list from the two stamped digests already in the section's attributes — `EMB.HELP`, the raw `EMB` call, the `VALUES` call, the `embed` preset, the `classify` preset, `EMB.MODELS` — as operable controls, and verify one submission from the list returns the sandbox's own reply
- [x] 3.3 Wire `Enter` to submit and the up/down arrows at the prompt to recall, feature-detecting recall so a page served before the sandbox is redeployed still submits and simply has no history; verify by submitting two commands and stepping back through them with the arrow keys
- [x] 3.4 Repaint the demonstration list from the offline state as well as the idle state, and verify with the sandbox blocked that the offline state offers the retry, the commands, and no fabricated reply
- [x] 3.5 Give the prompt row the quiet `↵` submit affordance in place of the `RUN` button, keeping its target at or above the committed floor below 834px, and verify the input still submits on Enter and on the button, by touch and by keyboard
- [x] 3.6 Restyle the console from the existing tokens only — the status strip, the ruled numbered example rows, the prompt row — and verify every value is measured at the 1086px reference frame and at 390px: no text below 12px/14px, every text and control colour at or above 4.5:1 for text and 3:1 for non-text indicators, `--accent-ink` absent from the plate, and `--accent` as the focus ring
- [x] 3.7 Apply the same simplification to the standalone terminal at `website/repl/index.html`, deriving its demonstration list from the `/api/presets` manifest it already fetches, and verify both surfaces submit, recall and render the same reply for the same command

## 4. One plate module, two callers

- [x] 4.1 Extract the plate driver from `website/assets/js/main.js` into a new `website/assets/js/topviz.js` that takes its behaviour from `data-` attributes on its mount, and verify the landing page's plate still autoplays and loops exactly as before
- [x] 4.2 Re-check the plate's failure path against a copy of the page pointed at a cast that does not exist, and verify the still frame returns and the emptied mount is hidden, which is the path `website/tools/topviz/README.md` names as the one to re-check after touching the page
- [x] 4.3 Confirm `main.js` no longer carries plate or console-mode code and verify the landing page is otherwise unchanged: hero entrance, reveals, pipeline and route behave as before

## 5. The documentation surface

- [x] 5.1 Add the plate to *06 · Operations* in `website/docs/index.html` — the run's still frame as static markup, the run's caption and figures, and a play control that creates the player on demand — and verify with the served page that nothing plays until the control is used and that playback can be stopped
- [x] 5.2 Load the documentation surface with scripting disabled and verify the plate shows the still frame, the caption and the figures, and the page's content is otherwise complete
- [x] 5.3 Add the plate's rules to `website/assets/css/docs.css` from the tokens `styles.css` already declares — no new custom property — and verify the plate sits inside the reference measure with no horizontal scroll at 390px and no clipped text
- [x] 5.4 Run `python3 website/tools/published-tree.py` and verify it passes with the new module in the served set and nothing else added to the tree

## 6. Publishing and documentation

- [x] 6.1 Add `website/docs/index.html` as `publish.py`'s third stamp target, writing the documentation plate's frame, cast URL, caption and figures by marker, and verify a run with a marker removed fails loudly instead of writing half the plate
- [x] 6.2 Add `assets/js/topviz.js` to `published-tree.py`'s served set and verify `python3 website/tools/published-tree.py` passes
- [x] 6.3 Update `website/README.md`: the console's description, the two surfaces' shared client, and the retired "the documentation surface ships no JavaScript" invariant — replaced by the statement that the documentation surface's script is an enhancement whose absence leaves the page complete
- [x] 6.4 Leave the `docs/operations.md` GIF in place and verify `publish.py`'s marked block there is still written on a run, so the markdown documentation keeps a picture of the same run

## 7. Verification

- [x] 7.1 Run `just website-published`, `just website-presets` and `python3 website/tools/published-tree.py`, and verify all three pass on the changed tree
- [x] 7.2 Run `just website-ink` against both surfaces and verify every text element still renders with its glyph ink inside the viewport at all tested widths, including the rebuilt console and the new documentation plate
- [x] 7.3 Run `just website-shot` over the landing page and the documentation surface at desktop and mobile widths and verify the console reads as a terminal with one prompt, the examples are legible, and the documentation plate is composed rather than bolted on
- [x] 7.4 Run `impeccable detect --json` once over the changed HTML and CSS files after the build, and resolve or record every finding
- [x] 7.5 Run `openspec validate website-console-repl --strict` and verify the change validates with the `product-site` delta applied against the archived `deploy-sandbox-repl` baseline

## 8. Revision: the window is bounded and the menu stays

- [x] 8.1 Give `.console__screen` a floor and a ceiling with `overflow-y: auto` on both surfaces, and verify a 20-line reply leaves the panel the height it had, with the transcript scrolled and the menu, prompt and note all in place
- [x] 8.2 Move the demonstration rows out of the transcript into a band of their own, above it, built once at boot, and verify with `grep -n "idleEl\|showIdle\|appendExamples"` that the state machinery is deleted rather than adapted
- [x] 8.3 Follow the newest line only when the reader was already at the end, reading that before the transcript grows, and verify both halves: a reader at the end is carried to the newest line, and a reader scrolled back stays where they are through the next command
- [x] 8.4 Lay the rows out in two columns above 834px and one below, verify six rows occupy three ruled rows at 1440px and that the first row of each grid carries no rule, and verify no row is clipped or wraps mid-token at 390px
- [x] 8.5 Elide a long digest in a row's drawn label while the button still submits and names the full command, and verify the label is a substring of the accessible name (WCAG 2.5.3) and that clicking the row echoes the unelided digest
- [x] 8.6 At ≤834px drop the note from the drawn row and the badge from the strip, verify the strip is one row and no row wraps, and confirm the sandbox framing survives in the note under the panel
- [x] 8.7 Re-measure: the panel at rest is 467px at 1440px and 757px at 390px, both against an 853px unbounded panel before the revision, and verify the console functional suite (17 checks) and the standalone terminal suite (10 checks) pass

## 9. Revision: the documentation plate is the recording, in colour

- [x] 9.1 Have the plate's driver create the player at once and hold it where the mount carries no autoplay, with `autoplay: false` and the run's poster frame, and verify on the docs surface that the plate is the player (`.ap-player`), the plain-text `<pre>` is hidden, and the drawn text carries at least six distinct ANSI inks rather than one
- [x] 9.2 Verify the held plate does not move: sample its text over two seconds and confirm it is identical, with the player's timer at `00:00`
- [x] 9.3 Delete the `PLAY THE RECORDING` button, its `.topviz__foot` wrapper and its `docs.css` rules, and verify the plate's only control is the player's own start overlay plus its transport bar, and that pressing the overlay starts playback
- [x] 9.4 Verify the landing page's plate still autoplays and loops from the same module, and that its `data-topviz-autoplay` is the only difference between the two mounts
- [x] 9.5 Verify the two fallback paths are unchanged: a take that will not load puts the still frame back and hides the emptied mount on both surfaces, and at a width where the plate is not drawn no player is built and the recording is not fetched
- [x] 9.6 Re-check the no-scripting bytes of `website/docs/index.html`: the frame is present and complete, the mount ships empty, no inert control remains, and the player and its module are still linked

## 10. Revision: multi-model sample, no protocol switch, a CLI-shaped bridge page

- [x] 10.1 Add an `EMB.MULTI` row to the menu on both surfaces — `EMB.MULTI minilm "hello world" sst2 "this film is great"`, labelled `two models, one call` — and verify it submits the whole command and renders one slot per model
- [x] 10.2 Remove the `RESP 2|3` selector from the panel: its markup, `main.js`'s `protoEl` wiring and change listener, and its `styles.css` rules, and verify nothing on the console offers a protocol version while the request still carries `proto: 2` and `HELLO` still reaches the sandbox's own connection
- [x] 10.3 Verify the two reply forms are still both reachable with nothing set first, by running the offered `EMB` and the offered `EMB … VALUES` rows and reading the two shapes they return
- [x] 10.4 Rebuild `website/repl/index.html` as the viewport: `100dvh`, one column, the disclosure line and the sample menu, the scrolling transcript, the prompt as the terminal's last line, and no heading, card, status strip, footer or page scroll — and verify at 1440px and at 390px that the page does not scroll, the transcript scrolls instead, and the prompt stays on screen
- [x] 10.5 Keep the sandbox disclosure permanently in view on that page — it is not a banner that scrolls away — and verify it is readable and unbroken at both widths, with `emb.is` linked from the same line
- [x] 10.6 Verify the bridge page's behaviours survive the rewrite: a sample runs, `Enter` submits, the arrow keys recall and restore a draft, a long reply scrolls with the newest line in view, and the offline state states itself and offers a retry
- [x] 10.7 Verify the mobile floor on the bridge page: one column of samples, the note as the control's name, and every tappable control at or above 44px
- [x] 10.8 Verify the landing panel is otherwise unchanged by this group: seven menu rows in two columns, the strip carrying the state and the badge and nothing else, and the panel still bounded

## 11. Revision: every feature sampled, every call timed, the transcript tokenised

- [x] 11.1 Grow the menu to the whole permitted surface, in two groups — `REPLY FORMS` (the float bytes, the typed envelope, the multi-model call, both presets) and `THE SERVER` (models, info, stats, ready, help, ping, info) — and verify twelve rows numbered `01`–`12` across the groups on both surfaces, with the two preset rows still derived from the server's digests
- [x] 11.2 Drop the notes from the seven server reads, which are one word each and say what they do, and lay that group out in a single row of seven columns above 834px and two columns below, and verify no row wraps at 1440px and no cell is clipped at 390px
- [x] 11.3 Time every call in `terminal.js` from the moment of submit rather than from each retry, appending it as its own line in the form `(42 ms)` or `(23.51 s)`, and verify a reply carries it as its last line and that a waking sandbox reports the whole wait
- [x] 11.4 Add `highlight(text, command)` to `terminal.js` and use it from both pages' line builders, and verify a command line comes back with its command word and format keywords, its string literals, and its numbers marked, and that the marked-up line reassembles the original text exactly
- [x] 11.5 Verify the tokeniser's refusals: `sst2` stays a model name, a 40-character digest stays one run, a labelled `POSITIVE` is not dressed as a command, `FLOATs` is not a dtype, and dim and error lines are left whole
- [x] 11.6 Verify the site's hand-marked specimens are all still complete — every `code__body` specimen carries its token classes and no Lua keyword is left unmarked on the landing, the documentation surface or the not-found page — so the console's new treatment matches what the pages already do
- [x] 11.7 Re-measure the panel: 518px at rest at 1440px and 941px at 390px with twelve samples, against 488px and 757px with seven, and confirm the transcript stays bounded at 96–216px either way
- [x] 11.8 Re-run the site's checks and the bridge's tests, and verify the bridge page still fits the viewport at both widths with all twelve rows, no page scroll and the prompt on screen

## 12. Revision: the ledger leaves the plate, the disclaimer becomes a badge

- [x] 12.1 Move the twelve demonstrations out of the plate into their own led on the paper ground between the block's captions and the console, and verify from the rendered DOM that the plate contains no demonstration row and the ledger sits above it
- [x] 12.2 Verify the plate is three bands and nothing else — strip, transcript, prompt — and that the ledger still runs a command in it, survives the reply, and is never replaced by output
- [x] 12.3 Remove the paragraph under the plate and let the strip's `SANDBOX · MAY RESET` badge be the whole disclosure, and verify the badge is visible at every width and the strip is one line at 390px
- [x] 12.4 Give the console an empty state rather than a void: one dim line in its own voice stating how the prompt works, verified present at rest, replaced by the first command, and identical on both surfaces
- [x] 12.5 Restyle the ledger from the poster's paper vocabulary — ruled numbered rows, the muted voice for numbers and notes, hover and focus carrying three signals rather than the accent alone, and every control at the 44px floor below 834px — and verify the contrast of every state: rows, notes, numbers, hints and the accent-ink hover
- [x] 12.6 Apply the same discipline to the bridge page: the five-line paragraph becomes a one-line sentence in the terminal's voice, the group heading carries the hint, and the space it gives up goes to the transcript
- [x] 12.7 Verify the whole path again at 1440px and at 390px on both surfaces — the ledger's columns, no clipping, no horizontal scroll, the idle line giving way, a reply timed, recall and its draft, the offline state and its retry, and a long reply scrolling inside its ceiling
- [x] 12.8 Re-run `published-tree`, the preset and version checks, `openspec validate --strict`, the ink probe on both surfaces, and the detector, and record what it finds

## 13. Revision: the demo leads, on the sandbox's own ground, and says what it is

- [x] 13.1 Move the console and its ledger out of the protocol block into a block of their own placed before *THE PROTOCOL IS THE INTEGRATION*, and verify from the rendered DOM that the ledger and the plate are that block's children and the protocol block contains neither
- [x] 13.2 Give the block the sandbox's own ground (`block--dark`, #111110), invert the ledger for it (`--rule-dark` rules, paper ink, `--accent` hover) and take the plate's paper border down to a `--rule-dark` hairline, and verify at 1440px and at 390px that every label, rule and hover clears the thresholds the paper surface held and that the page does not scroll sideways
- [x] 13.3 Write the explanation the panel cannot carry about itself — what answers, what the bridge refuses, what a command costs, and how to drive the panel (`Enter`, the arrows, the timing line) — as the block's deck and a facts list, and verify every claim against `website/repl/allowlist.go`, `limits.go` and `sandbox.yaml`
- [x] 13.4 Re-index the block sequence (1–4) and the reveal order (ledger `--i: 1`, plate `--i: 2`), and verify the entrance ripples in the new order with no element waiting out another block's delay
- [x] 13.5 Re-run the checks over the changed tree: the ink probe at all 24 widths, `published-tree.py`, the preset and version stamps, and `openspec validate --strict`

## 14. Revision: the ledger clears the spine, and the window holds a reply

- [x] 14.1 Hold the ledger clear of the lane at every fold the page has — the reply forms and both group headings at `calc(var(--fold) - var(--spine-w) / 2 - var(--fold-gap))` above 1000px, the `.topviz` band's inset at 641–1000px, and the `.block__grid` width at 640px and below — and verify from the rendered DOM at 1440, 1280, 1001, 1000 and 390px that no cell, note or heading glyph overlaps the spine's rect and that nothing scrolls sideways
- [x] 14.2 Split the server's row of seven around the lane — four cells to it, `--spine-w` as the lane track, `--fold-gap` as the row's own column gap, and the fifth cell placed by `grid-column-start` — and verify all seven stay in one row with the line in the gap at 1440px and that the ≤1000px blocks still lay them two to a row
- [x] 14.3 Raise the transcript from `clamp(88px, 7vw, 110px)`/`clamp(190px, 15vw, 236px)` to `clamp(132px, 9.5vw, 158px)`/`clamp(264px, 21vw, 336px)`, and verify at 1280px that the panel at rest is 225px against 183px, that a thirty-line reply scrolls inside a 269px window against 192px instead of lengthening the panel, and that the newest line is still followed
- [x] 14.4 Re-run the checks over the changed tree: the ink probe at all 24 widths, `published-tree.py`, the version and preset stamps, and `openspec validate --strict`

## 15. Revision: the demo and cli.emb.is are one object in two frames

- [x] 15.1 Print the same prompt mark on both surfaces — `emb>` in `website/index.html`'s label and in `main.js`'s echoed caret, where the panel had `EMB ›` — and verify the prompt row and an echoed line both read `emb>` on the served page
- [x] 15.2 Draw the panel's chrome in the terminal's own hairline: the strip's rule, the prompt row's rule and the input's underline move from `--rule` (9.22:1 on #111110) to `--rule-dark`, the value the standalone page uses for the same three, and verify no internal rule on the plate is brighter than its own edge
- [x] 15.3 Drop the chrome the terminal does not have — the border box around the `↵` and the underline beneath the input — keeping the 44px target floor below 834px, and verify the row still submits by pointer and by keyboard on the served page
- [x] 15.4 Make the panel's group headings the terminal's paper bars — `--bg` ground, `--fg` label, the hint inside, at the content box's width so the bar covers the spine where it crosses — and verify the bar's hint keeps the paper's muted ink inside the dark block, where `--muted` is remapped to `--rule`
- [x] 15.5 Re-run the checks over the changed tree: the ink probe at all 24 widths, `published-tree.py`, the version and preset stamps, `openspec validate --strict`, and the detector (20 findings before this revision's files and 20 after, with the strip's entry gone and the plate's reworded)
