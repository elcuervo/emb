# Tasks

## 1. Inbox re-selection

- [x] 1.1 Make `inboxMount()` clear the stage before it appends, so the ticket click handler's direct call cannot stack a second list/form; verify in the browser that switching tickets leaves exactly one `.tickets` and one form
- [x] 1.2 Add a regression check that repeatedly switching tickets keeps the control count constant; verify by counting `.tickets__btn` after several switches (stays 4)

## 2. Readout geometry

- [x] 2.1 Restructure the move row into marker / direction / bar / value columns and give the readout label column `max-content`, with `nowrap`, so no value wraps or misaligns; verify with a proposed `RIGHT`/`LEFT` that every row stays on one aligned line
- [x] 2.2 Confirm the fix at narrow width (the `.hud` breakpoint) and with the readout-bar labels; verify desktop and narrow screenshots show no wrapped label or shifted value

## 3. Executed decision over legal moves

- [x] 3.1 In `scripts/snake.lua`, build the move question's criteria from the legal directions only (mirroring `pacman.lua`) and index the probabilities by the same marked list; verify `go test ./internal/server/ -run TestLayaSnakeEpisode -count=1` inside `nix develop`
- [x] 3.2 Render one loop figure for both presets: a `DECIDED` line with the executed move and the intervention badge, a `model proposed …` line, and the probability table with the executed move marked and blocked directions shown as unavailable; verify both loops show the same structure and a veto shows the proposal it replaced
- [x] 3.3 Update the readout copy (labels, badge words) so the decision is the headline; verify the readout text names the executed move first

## 4. Copy and impeccable pass

- [x] 4.1 Shorten the four example blurbs and the honesty caption to decisional copy; verify each names what runs, where, and what the safety layer does
- [x] 4.2 Run the `impeccable` pass over the readout and confirm no new colour, font, or token; verify with the mechanical detector and desktop + narrow screenshots

## 5. Integration

- [x] 5.1 Restamp the preset digests with `python3 website/tools/stamp-presets.py`; verify `--check` exits 0
- [x] 5.2 Run `just lint`, `just verify-harness`, and the focused Go suites inside `nix develop`; verify all pass
- [x] 5.3 Drive the local rig end to end — step and play Snake, step and play Pac-Man, switch every Inbox ticket and Quickstart — and verify no duplicate controls, no wrapped rows, and no console errors
