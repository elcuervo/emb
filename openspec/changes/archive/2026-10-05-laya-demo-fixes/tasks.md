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

## 6. Inbox decisions, variety, and aligned bars

- [x] 6.1 Expand the Inbox example set so it covers the plate's question vocabulary with distinct tickets; verify the plate lists the added tickets
- [x] 6.2 Lead the typed-question demos with a `decided` strip (argmax per question, matching the marked leaf), mirroring the loops' DECIDED line; verify every question names its chosen option
- [x] 6.3 Share one grid for the readout bars so `ghost near` / `routes clear` (and Snake's pair) start at the same x; verify the bars align in the browser

## 7. Answer semantics and decision-only highlight

- [x] 7.1 Derive each typed-question answer to match the checkpoint's own semantics — choice argmax, score label from the rounded expected value, noul true only above 0.5 — and mark the leaf that answer selects; verify against `ruby-laya`'s `Answer` semantics and in the browser
- [x] 7.2 State each answer with its value (probability / expected score / noul probability) so a near-coin-flip reads as one; verify the Inbox strip shows the probability beside every answer
- [x] 7.3 Stop highlighting readout bars that are not the decision: keep the neutral tone for the readout bars and reserve the accent for the executed move; verify in the browser that only the decided move's bar is accented

## 8. The decision DAG

- [x] 8.1 Replace the bar-tree with a decision DAG: the state fans into one node per question, each edge continuing to the answer the reply selected; verify the figure is generated from the reply and every typed-question example renders it
- [x] 8.2 State each answer as a node (the option plus its value) instead of a probability bar, and mark the node as the decision; verify the Inbox and Quickstart figures show one decision per question with no bars
- [x] 8.3 Stylize with the impeccable pass in the site's tokens — paper card, hairline nodes, accent reserved for a decision, one authored reveal — and confirm no new colour/font/token with the detector; verify with desktop screenshots
- [x] 8.4 Re-verify the Inbox re-selection regression (one list/form), the no-console-error check, and narrow-width legibility after the rework
