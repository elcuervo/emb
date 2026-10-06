# Tasks

## 1. The conjunction operator in the grammar

- [x] 1.1 Add `*` to the operator set and record on the query that it is a conjunction (`internal/emoji/grammar.go`); verify `go test ./internal/emoji/ -count=1` covers `🍕*🦅` parsing into two terms and `🍕*🦅*🍔` into three
- [x] 1.2 Make the glyph form and the word form the same conjunction — `pizza*eagle` spells to the same descriptions as `🍕*🦅`; verify with a grammar test asserting both spelled queries are equal
- [x] 1.3 Refuse a mixed name (`🍕*🦅+🍔`) as unparseable and a lone `*` for having no term; verify tests assert both errors and that no result is produced
- [x] 1.4 Assert a conjunction is not a sentence, beside the existing composition scenario; verify a grammar test covers `🍕*🦅`
- [x] 1.5 Document `*` in `docs/dns.md` beside `+`/`-`, including what each operator asks; verify the documented `pizza*eagle` query answers over the HTTP route

## 2. Joint neighbourhood ranking

- [x] 2.1 Add a multi-term joint rank to `internal/emoji` (product of the terms' cosines, the query's own rows excluded, equal products keeping vocabulary order); verify index tests construct a vocabulary where a third entry wins, where an operand would otherwise win but is excluded, where two entries tie, and where a term is a zero vector
- [x] 2.2 Return each result's similarity to each term beside the joint score; verify a test asserts the legs for a two-term conjunction and that they are the cosines it was ranked from
- [x] 2.3 Leave the single-vector `Rank` and its ordering untouched; verify the existing `internal/emoji` tests pass unchanged

## 3. The zone's conjunction path

- [x] 3.1 Route a conjunction to the joint rank in `cmd/emb-dns` — embed the spelled terms through `Embedder.Embed`, rank jointly, never the preset; verify a service test answers a conjunction with a third entry and never with an operand
- [x] 3.2 Carry the joint score and the legs to the JSON document only for a conjunction, leaving the arithmetic document unchanged; verify an HTTP test asserts the legs are present for `🍕*🦅`, absent for `🦈-🐟+🐦`, and that the TXT record still carries only a glyph
- [x] 3.3 Keep the arithmetic path on the preset and prove it did not move; verify `just verify-emoji` still reports `🦈-🐟+🐦` at 0.731/0.689/0.654
- [x] 3.4 Confirm `scripts/emoji.lua` is byte-identical (`git diff --exit-code scripts/emoji.lua`) and the plate's inline listing of it still matches the file

## 4. Flags in the vocabulary

- [x] 4.1 Read `common/annotationsDerived/en.xml` at the pinned ref in `dns/tools/build-emoji-vocab.py`, merge with `annotations/en.xml`, sort by slug, and record both sources with a digest each; verify `just emoji-vocab` regenerates and `just emoji-vocab-check` passes
- [x] 4.2 Rebuild `dns/emoji-vocab.json` (1961 → 2223) and verify the loader still refuses a duplicate slug and that a flag glyph resolves to its own entry
- [x] 4.3 Verify the service reports the new entry count from the running process, not from a page, after a restart

## 5. Discovery and re-pinning

- [x] 5.1 Add `dns/tools/emoji-jokes.py`: for every pair whose terms are dissimilar below a ceiling, compute the best joint third entry and report both legs, printing the floors used; verify it reports the measured triples (panda, dango, stadium, om, fox) and prints identical output on a repeat run
- [x] 5.2 Verify the search excludes near-duplicate pairs — a pair of harpoon barbs or quotation marks is not reported as a composition example
- [x] 5.3 Add `just emoji-jokes` beside `just verify-emoji` and document what it measures
- [x] 5.4 Re-pin `dns/examples.json`: every score that moved, the new `verified_against` (model and 2223 entries), the discovered `*` examples with both legs, and the ceiling example — each carrying the label and mode a surface needs to offer it
- [x] 5.5 Record the ceiling: `🍕*🦅` answering 🐧 penguin, with 🇺🇸's legs and the winner's legs written beside it, and copy that states the flag is unreachable
- [x] 5.6 Verify the gate: `just verify-emoji` against a freshly started zone reports every pinned example matching, including the ceiling example

## 6. The play surface

- [x] 6.1 Add the `website/tools/` build step that inlines `dns/examples.json` into the plate as its starter controls, grouped by family and carrying each example's mode; verify by rebuilding — a newly pinned example appears as a control, a removed one disappears, and the plate's own content was not edited
- [x] 6.2 Make each ranked result addable to the query as a term, with an operator toggle (`*`, `+`, `-`) that defaults to `*`; verify in a browser that the built query, the displayed name, the reported query, and the `dig` line all carry the same composition, and that no reachable control builds a name the zone refuses
- [x] 6.3 Render the legs when the answer is a conjunction, beside the joint score; verify a conjunction shows each result's similarity to each term and that a low joint score is stated as the honest reading
- [x] 6.4 State what each operator asks in the plate's copy, and confirm the ceiling example's control carries the copy that the flag is unreachable; verify both against a local zone
- [x] 6.5 Enable the local browser check without widening the zone's production origins: extend `website/tools/dev-server.py` to proxy a same-origin path to the local zone and point the plate's existing `?zone=` override at it; verify with `just website-dev` (plus the zone) that tap-to-compose works end to end and that a refused name renders as a stated refusal, not an empty result
- [x] 6.6 Verify the plate still explains itself without scripts and that its figures still come from the zone's metadata route

## 7. Integration

- [x] 7.1 `just test`, `just lint`, `just verify-harness` (which includes `./internal/emoji/...`) and `just deadcode` all pass
- [x] 7.2 Start the zone with `just dns-dev` and verify `just verify-emoji` exits 0 with no drift, then that an unreachable zone exits with the distinct status the spec requires
- [x] 7.3 `openspec validate emoji-knn-composition --strict` passes
