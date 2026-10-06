# Design

## Context

See proposal.md for motivation. The pieces this change assembles already exist,
and the shape of each one decided the architecture:

- **The embedding path is reusable and complete.** `internal/hfhub` +
  `internal/registry` download a model and auto-detect its dimension, max length,
  and pooling; `internal/pipeline.Pool.Embed` pools, normalizes, batches and
  caches; `internal/embverify.RankDocuments` already ranks a corpus by cosine.
- **The script surface is the extension point.** `script-preload` loads a Lua
  file at boot from model config, exposes it per-script as `emb.script.config`,
  and folds its digest into the scripted reply-cache key
  (`internal/server/script.go:437`). `emb.embed`, `emb.math.add`, `emb.math.scale`
  and `emb.math.topk` are already whitelisted.
- **Three host properties bound what a script can do.** `runProto` creates a
  fresh `LState` per evaluation (`internal/script/engine.go:101`), so **globals do
  not survive a call and no index can be memoized in Lua**;
  `DefaultMaxScriptBytes = 64 * 1024` (`internal/script/engine.go:20`), so neither
  the vocabulary nor a vector matrix fits in a script; and there is no matmul, so
  ranking three thousand candidates is not a Lua job.
- **`internal/resp.Client` is a complete RESP client**, CGO-free, already inside
  the `just verify-harness` set.
- **The zone's two load-bearing assumptions were measured, not assumed.** A
  throwaway resolver showed that `dig` puts raw UTF-8 emoji bytes on the wire
  (`0e f0 9f 91 91 2d …` for `👑-👨+👩`) and that it renders non-ASCII `TXT` data
  escaped:

  ```text
  $ dig @127.0.0.1 -p 5354 +short TXT "👑-👨+👩.t."
  "\240\159\145\145-\240\159\145\168+\240\159\145\169.t.|\240\159\145\184|plain-ascii"
  ```

  Names print raw; `TXT` data does not. The first fact is what makes emoji names
  work at all; the second is why this change ships an HTTP surface.
- **The platform constrains the transport.** Fly answers UDP only on a dedicated
  IPv4, only when the socket is bound to `fly-global-services`, and never
  rewrites the port; TCP cannot bind that address and belongs on `0.0.0.0`.
- **The gallery's world is fixed and authoritative.** `DESIGN.md` §The demos
  gallery defines the instrument rules (paper explains, the dark plate
  instruments; one signal spent on meaning; drawn not rendered; captions read
  from the index; numbering part of the title) and the five teaching sections.
  `embedding-demos` requires every plate to follow them.

## Goals / Non-Goals

**Goals:**

- A DNS zone that answers real semantic retrieval, with the whole trick readable
  as one shipped Lua preset.
- One deployment that carries the model, the script, and the zone, with the model
  reachable only from inside the machine.
- A gallery plate that demonstrates the same queries without a terminal, and that
  teaches the one thing `dig` cannot show: the answer as the wire carries it.
- No change to the Redis surface, the script host, the reply formats, or the
  site's visual system.

**Non-Goals:**

- No DNSSEC, no DoH/DoT, no zone transfers, no second zone.
- No new host function on the script surface, and no change to
  `internal/script`'s whitelist.
- No public RESP port: the zone is the only public protocol the model sits behind.
- No second embedding of the vocabulary per query, and no committed vector matrix.
- Not a hosted service: no account, no pricing, no uptime commitment, and the
  zone states its provenance in its own apex `TXT`.

## Decisions

**D1. The frontend is a RESP client, not a second inference server.**
`cmd/emb-dns` speaks DNS on 53 and RESP on loopback; it links `internal/resp` and
`internal/emoji`, never `internal/onnx`. Linking `internal/pipeline` would drag
CGo, ONNX Runtime, libtokenizers, a model download, and a second warm-up into a
process whose whole job is 200 lines of wire and one dot product, and would put a
second copy of the model on the machine. Rejected: a Python server (a parallel
implementation of four existing packages), and a `-dns` mode inside `cmd/emb`
(the Redis server's lifecycle, limits, and tests should not grow a second
protocol). Consequence worth keeping: `CGO_ENABLED=0 go build ./cmd/emb-dns` is a
property the tests can assert.

**D2. Lua constructs the query; Go ranks.** The script's input is the parsed
name and its output is a query vector. It does what a script is for — expanding a
glyph term to its description, embedding the terms, and composing them with
`emb.math.add`/`scale` for `+`/`-` — and it never touches the vocabulary matrix.
Three facts make this the only workable split (fresh `LState`, 64 KB source cap,
no matmul), and it keeps the ranking in Go beside `kelindar/simd`, where a
3,700-candidate cosine pass is sub-millisecond. Rejected: a Lua loop over the
index (3,700 host calls per query, and the index still has to arrive in `ARGV`);
and adding `emb.math.nearest` to the host surface (real surface area added to a
general scripting API to serve one joke).

**D3. The index is built at boot, through the model it serves.** The vocabulary's
descriptions are embedded once at startup in batched `emb.embed` calls (~58 round
trips of 100 texts), held in the proxy's memory as one 3,700 × 384 float32 matrix
(5.7 MB), and reused for every query. There is no committed matrix: a file would
be a second source of truth that drifts from the model silently, and the boot
cost is already covered by the existing cache snapshot (`cache_file`/`cache_load`),
which makes a restart a file read. Rejected: a pinned `emoji-index.bin` regenerated
by a `just` target (needs a fingerprint check or it serves stale rankings), and
embedding the descriptions per query (roughly 30 seconds of inference per query).

**D4. The vocabulary is name-first.** Each entry is a glyph, a slug, and a
description; the **description** is embedded, and the glyph is only a reply and a
way to write a term. This is what makes the three modes one code path: `🦈`
expands to `shark` before it reaches the tokenizer, so the glyph mode is the text
mode wearing a glyph and `🦈.dns.emb.is` and `shark.dns.emb.is` are the same query
by construction rather than by luck. It is also the only mode that works: BERT
uncased's vocabulary contains almost no emoji, so a glyph embedded as itself is
`[UNK]` — with a few exceptions that tokenize cleanly, which is worse than
uniformly unknown because the algebra would then be inconsistent rather than
merely noisy. Rejected: embedding glyphs (the original sketch), which would have
returned the same neighbour for nearly every query.

**D5. `github.com/miekg/dns` is the one new dependency.** DNS message parsing is
a trust boundary on a public port: name decompression accepts a pointer to
anywhere in the message, including forwards and into itself, and the classic
failure is an out-of-bounds read or an unbounded loop on attacker-chosen bytes. A
hand-rolled responder is ~150 lines of `encoding/binary` and one CVE. The library
also supplies TCP framing, EDNS0 (which `dig` sends), and correct `SOA`/rcode
handling for NODATA. Rejected: a hand-rolled UDP-only responder (fewer lines,
more ways to lose the machine). One consequence is worth writing down because it
is invisible until a name carries non-ASCII: **the library hands names and TXT
data over in presentation form**, so an emoji question arrives as
`\240\159\166\136-fish+bird.dns.emb.is.` rather than as the bytes it carried
(measured, and exactly what `dig` prints). The zone therefore splits labels on
unescaped dots and unescapes each one before the grammar sees it; the wire
itself is 8-bit clean, and the in-process tests that skipped the wire could not
see the difference.

**D6. Two surfaces, one deployment — and the HTTP surface exists because of the
spike.** The record carries the answer and the read routes carry the working. A
`TXT` record holds one glyph and nothing else: no score, no name. A record is a
value a resolver caches for its TTL and answers other people's questions from,
so it must be a property of the name rather than an artifact of one query's
ranking — and a similarity score is exactly that artifact. `GET /<expr>` returns
the ranked list as raw UTF-8 with each result's name and score, `?q=` returns the
same as JSON, `/healthz` reports readiness, `/stats` reports counters. The plate
needs the HTTP surface (a browser cannot send a DNS query), and so does every
human: `dig` escapes the very bytes the demo is about. The HTTP routes are
read-only, carry no state, and answer cross-origin requests only from the site's
own origins.

**The readable surface is a separate name, `zone.emb.is`.** The zone is a
delegation, and a platform-managed certificate cannot live on a delegated name:
the platform's checker reads the **parent** zone (`soa` is the parent's, the
record lists come back empty) and does not follow the `NS` into the zone, while
the parent — Cloudflare — shadows any record at or below a delegation, so the
`_fly-ownership` / `_acme-challenge` fallbacks are unavailable there too. The
readable HTTP surface therefore lives at `zone.emb.is`, a sibling the parent zone
serves directly, and that name carries the certificate. The zone's own apex still
answers `A`/`AAAA` for reference; it carries no TLS. The URLs in this change are
`https://zone.emb.is/<expr>` for reading and `<expr>.dns.emb.is` for the wire.

**D7. The apex is reserved; everything else is a query.** `dns.emb.is` carries
`SOA`, `NS`, and the usage `TXT`; names strictly beneath it are answered
synthetically. This keeps the zone's own infrastructure resolvable, which a rule
of "every name is an expression" cannot: an in-zone `NS` target would be an emoji
query. A leading `_` marks the reserved introspection labels (`_help`, `_stats`,
`_why.<expr>`), an alphabet no vocabulary slug uses, and anything outside the zone
is `REFUSED` — which is also what keeps the service from being useful as an open
resolver.

**D8. The deployment is its own app, on the same fp32 model the examples were
pinned against.** `emb-sandbox` is `shared-cpu-4x`/8 GB and carries a `[[vm]]`
note earned by an OOM; its config says to re-measure if the model set changes.
The zone therefore deploys as its own app with one machine, one volume, and its
own `emb` on loopback, so the site's machine — and the site's `emb` — are
untouched. Each surface owns its model and preset, which is also what keeps the
zone's answers reproducible from its own config (D9) rather than from the
site's. The weights are the fp32 MiniLM export, not a quantized one:
`dns/examples.json` records what that model answered, and an int8 export would
drift every pinned score and the plate's copy while buying nothing for a
22M-parameter model that fits the zone's small machine either way. Fly bills
machine time, not work, so query volume does not move the bill; the only new
line item is the dedicated IPv4 that UDP requires.

**D9. Determinism is enforced by the two existing cache identities.** The
script's config digest folds into the scripted reply-cache key, and the model's
fingerprint identifies the weights, so an edited vocabulary or a swapped model
invalidates cached replies instead of serving a ranking computed under the old
pair. On top of that, ranking ties break on vocabulary order, so a reply is
reproducible across restarts and a cached `TXT` record cannot disagree with a
fresh query.

**D11. The sentence is the answer to a sentence.** A query that wrote one term of
more than one word is a sentence, and a sentence's answer is the ranked glyphs
joined into one pictogram — carried as the first `TXT` record, with the ranked
records after it so the retrieval is still legible. The trigger is the shape of
the question rather than a flag or a query type: ask in words, get an emoji
sentence; ask in a glyph or compose terms, get one emoji. That keeps the rule
memorable in one line and keeps the ranked list — the actual retrieval — on the
wire. The test is made before vocabulary words are spelled out, so naming an
entry (`shark`, whose description is five words) stays one word and answers with
one emoji. Rejected: always joining the results (it destroys the single-emoji
answer the original joke was built on), an opt-in verb like `_say` (a setting
where a joke belongs), and joining on the HTTP surface only (then the joke is a
rendering choice and `dig` never shows it).

**D10. The plate inherits the gallery's world and spends its invention on the one
thing DNS makes visible.** The gallery is an established surface used as
instrumentation ("an engraved atlas of a mind"); a plate is an extension of it,
so the palette, type ladder, rules, five teaching sections, dark-plate apparatus,
and caption discipline come from `DESIGN.md` unchanged. What is new is the
subject, and it supplies exactly one argument worth drawing:

- **Thesis.** The answer is one record, and the record is a sentence someone
  typed. The plate is a **record card**: the query name drawn as its own labels
  on the atlas graticule, the ranked answer as ruled marks, and the same answer
  printed twice — the escaped `TXT` data a resolver hands back and the glyph it
  stands for.
- **First viewport.** Plate number and title in the display face (`V. The zone`,
  the gallery's group roman rather than a thirteenth ordinal), the plate's own
  `WHAT YOU ARE LOOKING AT` paragraph, and the caption line read from the service
  — model, dimensions, vocabulary entries, and the record's TTL — in the existing
  `FIG.` voice. Nothing above it but the masthead; the rail at the foot carries
  the way back, as it does on the last plate today.
- **Signature interaction.** Typing a sentence writes the name it becomes
  (`i lost my job` → `i.lost.my.job`) and the plate answers with both renderings
  in one frame. The movement is the name being cut into labels — the one motion,
  and `prefers-reduced-motion` cuts it to a frame.
- **The figure.** The ranked list is drawn from the reply: one mark per result,
  the top result the single accent, rule length the score the record omits, the
  vocabulary name in mono caps. The escaped/raw pairing is drawn from the same
  reply, so a changed answer changes both. No image file, no pasted record.
- **States.** Empty (no query yet: the shipped examples only), running, answered,
  refused (unparseable or over the transport's limit — stated as the resolver
  would state it), and unreachable (the service is down: the explanation and the
  examples stay, and the plate says which surface failed).
- **Colour.** The emoji glyphs are the only chromatic material on the plate, and
  they are the *subject's* colour, not the site's: the instrument rules spend no
  second hue on decoration, so the glyph is never tinted, framed, or shadowed to
  make it behave. The accent stays where it already is — the top result's mark.
- **Honesty.** The plate names the surface that answered and reports that
  surface's model and vocabulary size; it is the gallery's first plate served by
  a second deployment, and it says so where the commands unfold.

## Risks / Trade-offs

- **The model's judgment is the demo's content, and it is not scripted.** A
  sentence encoder will rank "my mom is in the hospital" beautifully and a proper
  noun it has never seen not at all. → The shipped examples are a fixed set
  verified against the zone before deployment, and the check fails on drift; the
  plate's copy claims the result the service actually returned.
- **A multilingual encoder is slower and larger than the English one.** → Quantized
  weights, its own machine, and a boot-time index whose cost the cache snapshot
  removes after the first start.
- **A deduplicated vocabulary loses variants.** Skin-tone and gender variants are
  folded to one canonical glyph, so a reader cannot ask for a specific modifier.
  → Accepted: the alternative is a top-three list of one emoji in three skin
  tones, which reads as a bug.
- **The vocabulary is a corpus with a licence.** It is derived from CLDR names,
  which carry the Unicode licence and an attribution obligation, and it is the
  first gallery corpus that is not literature. → It is committed like the Poe
  corpus, with its source and licence recorded and stated on the plate, and it
  goes through the same boilerplate and attribution check.
- **A public UDP socket is an abuse surface.** → Authoritative-only with
  `REFUSED` outside the zone, per-source rate limiting, a `top_k`-bounded reply,
  no query-text logging by default, and a machine that holds no secrets.
- **The zone's downtime is the plate's failure state.** → The plate's unreachable
  state is one of its five designed states, not an accident, and it is the state
  a reader is most likely to see first on a novelty service.
- **`dig` will never print the emoji.** → Stated in the plate and in the docs as
  a property of resolvers, with `curl` and the plate offered as the readable
  route; the escaped form is shown rather than hidden, because it is the truth
  about the wire.

## Migration Plan

1. Build and test the zone locally (`just dns-dev`), against a local `emb` on
   loopback, with the vocabulary index built at boot.
2. Deploy `emb-dns` as its own Fly app; allocate the dedicated IPv4; add the `NS`
   delegation for `dns.emb.is` and the apex `A` for the HTTP surface.
3. Verify from outside with `dig`, `curl`, and the health route before the site
   references it.
4. Add the plate and its gallery entry, then run the site's own checks
   (`just website-presets-check`, the published-tree check, and the example
   verification) and deploy the site.
5. Rollback: the zone is additive, so removing the `NS` delegation and the site's
   plate entry returns the site to its previous state; the app and its volume can
   be destroyed with the zone's data, which is derived at boot and reproducible
   from the committed vocabulary.

## Open Questions

- Which emoji set the vocabulary is built from — every fully-qualified emoji, or
  a curated subset — is a quality question with a measurable answer, and it can
  be settled by the example-verification check rather than by argument.
- Whether the plate joins the gallery's numbered reading order or closes it as an
  appendix is a curatorial call the gallery index can make last.
- Whether the zone should also answer over the model's own protocol on a public
  port (a second door onto the same machine) is explicitly out of scope here.
