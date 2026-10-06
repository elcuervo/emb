# Proposal

## Why

`emb` is invisible in the way that matters: it is a server you point a client at,
and every demonstration of it so far has looked like a demonstration of a
client. A DNS zone inverts that. The query is the program, the reply is one
record, and the model behind it is doing real semantic retrieval on a sentence
someone typed into `dig`.

`dns.emb.is` is that zone: every name under it is a natural-language query, and
the answer is the emoji that best matches it. It is a stupid joke with a serious
payload — semantic retrieval over thousands of emoji descriptions, reached
over the most boring protocol there is, with the whole trick readable as one Lua
preset that the public script surface already knows how to load, cache, and
invalidate.

## What Changes

- **A new public surface: an authoritative DNS zone at `dns.emb.is`.** Every
  name under the apex is a query, answered from a `TXT` record. The apex itself
  is reserved for `SOA`/`NS` and a usage `TXT`; queries live strictly beneath it.
- **The query grammar.** A name's labels are the words of a sentence
  (`i.lost.my.job.dns.emb.is`); `+` and `-` separate the terms a name composes,
  evaluated left to right with no precedence. Emoji glyphs expand to their
  vocabulary entry's description before embedding, so `🦈.dns.emb.is` and
  `shark.dns.emb.is` are the same query, and `🦈-🐟+🐦.dns.emb.is` is the algebra
  the joke started as.
- **A sentence is answered with an emoji sentence.** Ask in words and the reply's
  first record is the whole ranking read as one pictogram — `my.mom.is.in.the.hospital`
  → `"🏥🧑‍⚕️👩‍⚕️"` — followed by the ranked records it came from. Ask in a glyph or
  compose terms and the answer is one emoji, because the shape of the question is
  the joke rather than a setting.
- **Retrieval, not lookup.** The reply is a ranked list (`top_k`, default 3): one
  `TXT` record per result, each carrying only its emoji — no score, no vocabulary
  name. The first record is the answer; the working behind it rides the readable
  surface instead.
- **A vocabulary with one source of truth.** Glyph, slug, and description per
  emoji, deduplicated across skin-tone and gender variants, loaded once at boot
  and embedded in one batched pass through the same `emb` model that serves the
  queries.
- **The joke lives in Lua.** `scripts/emoji.lua` turns a parsed name into a
  query vector using `emb.embed` and `emb.math.add`/`scale`, preloaded by model
  config exactly as `script-preload` already specifies.
- **A DNS process that holds no model.** `cmd/emb-dns` speaks DNS on 53 and RESP
  on loopback to a co-located `emb`; it contains no ONNX, no tokenizer, and no
  CGo.
- **HTTP on the same app** for the view `dig` cannot print: non-ASCII `TXT`
  rdata is escaped as `\DDD` by every `dig` (`"\240\159\145\184"`), so the
  readable form is `GET /i.lost.my.job`, and `GET /?q=i.lost.my.job` returns the
  same ranking as JSON. Health and query statistics are HTTP endpoints too.
- **A deployment of its own**, not a second process on the site's machine: one
  Fly app, one volume, `emb` on loopback, DNS public, one dedicated IPv4 for UDP.
- **A plate in the demos gallery that needs no `dig`.** `emb.is/demos/dns` runs
  the same queries in the browser against the service's HTTP surface, and shows
  the `dig` invocation it mirrors, the DNS question it would have sent, and the
  `TXT` rdata as a resolver actually renders it — `"\240\159\145\184"` beside
  `🏥` — because that escaping is the honest wire truth and the reason the HTTP
  surface exists. It is the gallery's one plate served by a second deployment,
  and it says so, reporting that deployment's own model, dimension, and
  vocabulary size rather than the sandbox's.

## Capabilities

### New Capabilities

- `emoji-operations`: the expression grammar (word labels, `+`, `-`, glyph
  expansion), the vocabulary's shape and deduplication, and the ranking contract
  — top-k with scores, deterministic order, and the documented behavior for an
  unparseable name, an unknown glyph, or an over-long query.
- `dns-service`: the authoritative zone at `dns.emb.is` — reserved apex,
  synthetic answers, per-qtype behavior (`TXT` answered, `A`/`MX` NODATA,
  `ANY` answered, `AXFR` refused), TTLs, EDNS0, the HTTP surface, the readiness
  gate and rate limiting, and the platform constraints that decide whether a
  packet is answered at all (UDP bound to `fly-global-services`, TCP to
  `0.0.0.0`, dedicated IPv4 required).

### Modified Capabilities

- `embedding-demos`: the requirement that every demo runs against the models the
  sandbox reports is widened to admit a plate served by another deployed surface
  of the product — the DNS zone — provided the plate names the surface that
  answered it and reports that surface's own model and index metadata. The
  gallery's existing curriculum rules (five sections, a figure from the live
  reply, commands that unfold, honest degradation, claims read not typed) then
  apply to it unchanged.

## Impact

- Code: new `cmd/emb-dns`, new `internal/emoji` (vocabulary, grammar, ranking —
  pure Go, `CGO_ENABLED=0`, testable without a model), new preset
  `scripts/emoji.lua`, new deployment directory `dns/` (`config.yaml`,
  `fly.toml`, vocabulary).
- Dependencies: `github.com/miekg/dns` — the one new module. DNS message parsing
  is a trust boundary and is not hand-rolled.
- Build: the DNS binary joins the existing image build; the runtime image
  already carries ONNX Runtime and libtokenizers for the co-located `emb`.
- Site: a new plate at `website/demos/dns.html`, its entry in the gallery
  index's reading order with the `new` mark, and the cross-origin allowance the
  plate needs on the service's HTTP surface. No change to the published-tree
  boundary, no new stylesheet token, no change to `wrangler.jsonc`.
- Corpus: the emoji vocabulary becomes a committed gallery corpus, so it carries
  its licence and attribution (Unicode CLDR, per-entry names) the way the Poe
  corpus does, and the site's own check fails if the page's numbers drift from
  the shipped vocabulary.
- Docs: a `docs/dns.md` and a README section; the emb reference docs are
  unchanged.
- Deployment: a new Fly app with its own machine, volume, and dedicated IPv4. No
  change to `emb-sandbox`, so no re-measure or resize of the site's machine.
- Not affected: the Redis protocol, reply formats, `INFO`/`CONFIG`, the script
  API version, the site, and the published tree.
