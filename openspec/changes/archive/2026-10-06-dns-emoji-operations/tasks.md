# Tasks

## 1. The vocabulary and the pure emoji layer

- [x] 1.1 Build the vocabulary asset from the CLDR emoji names: one entry per fully-qualified emoji carrying its glyph, a kebab-case ASCII slug, and its description; fold skin-tone and gender variants to one canonical glyph; record the source and the Unicode licence beside the asset. Verify: a check over the asset reports no duplicate slug, no duplicate glyph, no entry with an empty description, and every variant folded onto an existing canonical entry.
- [x] 1.2 Add `internal/emoji` with the vocabulary loader, the name grammar (labels joined as words within a term, `+`/`-` separating terms, left-to-right), and glyph-to-description expansion. Verify: table-driven tests cover a sentence term, a single-label glyph, a composition, mixed case, an unknown glyph left as text, an empty name, and an over-limit name — all with `CGO_ENABLED=0 go test ./internal/emoji/`.
- [x] 1.3 Add the ranking function: cosine over the loaded matrix, top-k with scores, ties broken on vocabulary order. Verify: `CGO_ENABLED=0 go test ./internal/emoji/` covers a known-neighbour query, a request for more results than exist, and two identical scores returning a stable order across repeated runs.
- [x] 1.4 Wire the layer into the CGO-free harness. Verify: `just verify-harness` runs `./internal/emoji/` and passes, and `just lint` is clean for the new package.

- [x] 1.5 Report whether a query is a sentence: one term of more than one word, decided from the term as the query wrote it, before any vocabulary word is spelled out, so naming an entry stays one word. Verify: `just verify-harness` covers the table (a sentence, a slug, a glyph, a composition) and the test fails if a spelled description turns a named entry into a sentence.

## 2. The query-construction preset

- [x] 2.1 Write `scripts/emoji.lua`: take the base term from `KEYS[1]`, the rest of the terms from `ARGV` as (operator, text) pairs, embed them with `emb.embed(..., {bytes = true})`, compose `+`/`-` with `emb.math.add`/`emb.math.scale` (a `-` is an add of a scaled operand), and return the packed query vector. The zone spells vocabulary words out before composing, so the vocabulary has one source of truth (`dns/emoji-vocab.json`) and the preset stays a composer. Verify: against a running `emb` with the model loaded, `shark-fish+bird` and `🦈-🐟+🐦` return the same three glyphs with the same scores (measured: 🐦 0.731, 🦆 0.689, 🐔 0.654 for both), and the socket test asserts the two forms answer identically.
- [x] 2.2 Declare the preset in the zone's own `emb` config (`dns/emb.yaml`, with `dns-dev` rewriting the container paths for a checkout) and load it at boot with `EMB.SCRIPT LOAD`, so the identity the zone uses is the digest the server computed rather than one derived here; a `NOSCRIPT` reply (a restarted server that forgot its scripts) reloads and retries once. Verify: the zone boots against a local `emb`, reports the digest it loaded, and answers a composition; editing a vocabulary description changes the served answer after a restart without touching the preset.
- [x] 2.3 Add the preset to the shipped-preset compile gate (`internal/script`'s `TestShippedPresetsCompile`, which covers `scripts/*.lua` under the real wrapper, minus the deliberately broken fixture). Verify: `just test` passes, and a preset that stops compiling fails it. Malformed names are refused before the preset is reached, by the grammar tests and the service's own refusals.

## 3. The zone service

- [x] 3.1 Add `github.com/miekg/dns` and `cmd/emb-dns` with the UDP listener on the configured platform address and the TCP listener on `0.0.0.0`; bind the upstream `emb` with `internal/resp.Client` on loopback. Verify: `CGO_ENABLED=0 go build ./cmd/emb-dns` succeeds and the binary contains no CGo dependency (`go list -deps ./cmd/emb-dns | grep -c onnxruntime` is 0).
- [x] 3.2 Serve the zone: apex `SOA`/`NS`/usage `TXT`, synthetic answers beneath it with one `TXT` record per ranked result carrying that result's glyph and nothing else, owner always the queried name, `TXT` answered, other types NODATA with `SOA`, `ANY` answered, `AXFR`/`IXFR` refused, out-of-zone `REFUSED`. Verify: loopback wire tests (no model) assert each of those replies, that no answer record's owner differs from the queried name, and that no record's data carries a score or a name.
- [x] 3.3 Enforce the TTLs, the parse refusals, and the `top_k` bound from configuration; refuse a name the transport cannot carry rather than truncating it. Verify: wire tests assert the positive TTL on answers, the shorter negative TTL on refusals and NODATA, and a refusal (not a result) for an empty name and an over-limit name.
- [x] 3.4 Implement the readiness gate, per-source rate limiting, and the statistics counters (served, unparseable, rate-limited, upstream failures, upstream latency). Verify: wire tests assert SERVFAIL and `not ready` before the index loads, `REFUSED` past the limit with the health route still answering, and that the four refusals are counted separately.
- [x] 3.5 Build the vocabulary index at boot through the upstream model in batched calls, and hold it for the process's lifetime; log no query text unless verbose logging is enabled. Verify: a boot against a local `emb` reports the entry count and the elapsed time, a query before the index is ready is refused rather than answered, and a run with default configuration logs no query name.

- [x] 3.6 Answer a sentence with an emoji sentence: the ranked glyphs joined with no separator as the first record, the ranked records after it, and nothing joined for a word, a glyph, or a composition. Verify: wire tests assert the first record equals the ranked records joined for a sentence, and that no such record appears for the three shapes that are not sentences; `dig +short TXT my.mom.is.in.the.hospital.dns.emb.is | head -1` prints one glyph string.

## 4. The HTTP surface

- [x] 4.1 Add the read-only routes: plain text for a name, JSON for `?q=`, `/healthz`, `/stats`, and a metadata route reporting the model name, dimension, vocabulary entry count, default `top_k`, and readiness — each read from the running service. Verify: `httptest` cases assert each route's body shape and that the metadata values change when the fake upstream's vocabulary count changes, with no page edit.
- [x] 4.2 Allow cross-origin reads from the site's origins only, refuse a preflight for a non-read method, and make every route reject a state-changing request. Verify: `httptest` cases assert a site origin is allowed, an unrelated origin is not, a `POST` is rejected, and a `PUT` preflight is refused.
- [x] 4.4 Carry the sentence on both read routes, escaped and readable, derived from the same ranking. Verify: `httptest` cases assert the text route prints it first, the JSON route carries `sentence.glyphs`, the escaped form decodes to it, and a word carries neither.

- [x] 4.3 Return the escaped `TXT` data, each result's vocabulary name, and each result's score on both read routes, derived from the same ranking the DNS route serves. Verify: one test asserts the escaped bytes decode to the returned glyph, that names and scores are present in rank order, and that all of it comes from a single ranking pass.

## 5. The examples, verified

- [x] 5.1 Author the fixed example set covering the three modes (a sentence, an emoji, a composition), each verified against a running service, with no example whose result depends on a name the model cannot know. Verify: `just verify-emoji` reports each example's measured top result and fails when a recorded result no longer matches.
- [x] 5.2 Record the measured results beside the examples so the plate's copy and the check read the same file. Verify: changing one recorded result makes `just verify-emoji` fail, and restoring it passes.
- [x] 5.4 Pin each example's sentence beside its ranking, and treat a sentence that appears or disappears as drift. Verify: `just verify-emoji` reports 15 of the 18 examples answered with a sentence (the three that are not are the glyph and composition ones) and fails if a sentence moves.

- [x] 5.3 Report the shape of the scores beside the pass/fail — mean, median, and how many examples land at or above 0.5 — rather than an NDCG figure, which would need graded relevance labels the corpus does not have and would mean inventing them. Verify: `just verify-emoji` prints the figure (measured: mean 0.648, median 0.694, 13/18 at or above 0.5) and repeats it across two runs.

## 6. Local development, deployment, and documentation

- [x] 6.1 Add `just dns-dev` running a local `emb` (loopback) and the zone together, mirroring `website-dev`'s shape, plus `dns/config.yaml` (the zone) and `dns/emb.yaml` (the model and the preset). Verify: `just dns-dev` answers `dig @127.0.0.1 -p 5354 +short TXT the.server.is.on.fire.dns.emb.is` with the escaped records and `curl http://127.0.0.1:8099/the.server.is.on.fire` with the readable list, from the same run.
- [x] 6.2 Add `dns/fly.toml` and the container target: one app, one machine, one volume for the model and the cache snapshot, the model the examples were pinned against (fp32), and the UDP service declared. Verify: `docker build` produces an image whose `emb-dns` runs and answers a query on loopback; `fly config validate -c dns/fly.toml` passes.
- [x] 6.3 Deploy the app, allocate the dedicated IPv4 for UDP, and add the `NS` delegation for the zone and the `zone.emb.is` host for the readable HTTP surface. Verify: from outside the machine, `dig +short TXT <example>` returns the ranked records and `curl https://zone.emb.is/<example>` returns the readable list; `dig` for a name outside the zone is refused; the `AAAA` address answers TCP but is not used as a UDP endpoint.
- [x] 6.4 Document the zone in `docs/dns.md` — the grammar, the reply shape, the escaping that makes `dig` print `\240\159\145\184`, the HTTP routes, the deployment, and the cost note (machine time plus the dedicated IPv4) — and link it from the README's contents table. Verify: the documented commands run verbatim against the deployed zone, and the README table links resolve.

## 7. The plate in the demos gallery

- [x] 7.1 Run `impeccable context` for `website/demos/dns.html` and write the surface brief with the six-block direction contract (thesis, own-world, story, first viewport, form, finish) before any markup edit. Verify: the brief exists and carries all six blocks and the seed key.
- [x] 7.2 Build `website/demos/dns.html` on the gallery's own anatomy: the five teaching sections in the established order, the dark-plate apparatus, the plate number in the title, and the caption read from the service's metadata route. Verify: the page renders all five sections in order with scripting disabled, and every figure in the caption traces to a value the service returned.
- [x] 7.3 Build the instrument: the name as it is typed becoming the name that is queried, the ranked list drawn from the reply with one accent on the top result, each mark sized by the score and labelled with the vocabulary name — the working the record itself omits — and the escaped/raw pairing drawn from the same reply. Verify: a query in the browser draws both renderings from one reply, every ranked result shows its name and score, and with reduced motion every figure is drawn in one frame.
- [x] 7.4 Build the three example controls and the five states (empty, running, answered, refused, unreachable), naming the surface that answered and reporting that surface's model and vocabulary size. Verify: each control runs its mode; an over-limit name shows the refusal the resolver would give; with the service stopped the plate states the failure, keeps its explanation and examples, and offers a retry.
- [x] 7.5 Add the plate to the gallery index in its reading order with the `new` mark on it and the mark removed from the previously new plate, and update the published-tree expectation for the new page. Verify: `just website-presets-check` passes, the published-tree check passes, and the index links the plate and states what it teaches.
- [x] 7.6 Run `impeccable detect --json` once over the changed HTML and CSS, resolve the mechanical findings, and record the rest. Verify: the detector has run once over the changed files and its findings are either fixed or written down.
- [x] 7.7 Run the finish review against the plate with the direction contract, the detector findings, and desktop and mobile captures; then apply the fixes it names in one batch. Verify: the review's verdict is recorded, and the second round finds the material fixes resolved.

## 8. Integration verification

- [x] 8.1 Walk the whole path from outside: a `dig` for each mode, the same queries over the HTTP routes, and the same queries on the plate — all three agreeing on the ranked result for the same name. Verify: the three answers match on the example set, and the daemon logs no query text during the walk.
- [x] 8.2 Confirm the Redis surface is untouched: `just test`, `just lint`, and `just verify-harness` pass, and a direct `EMB` call's reply bytes are unchanged. Verify: the full suite passes and no existing reply format differs.
- [x] 8.3 Confirm the cost posture: the DNS machine's size, the volume, and the dedicated IPv4 are the only new line items, and `emb-sandbox` was neither resized nor reconfigured. Verify: `fly machine list` for both apps shows `emb-sandbox` unchanged and one small always-on machine for the zone.

## Workflow follow-up

- Archive this change once the zone is deployed and the plate has shipped: sync the `emoji-operations`, `dns-service`, and `embedding-demos` deltas into `openspec/specs/`, then move the change to `openspec/changes/archive/`.
