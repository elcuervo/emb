# The DNS zone

`dns.emb.is` is an authoritative DNS zone where every name under the apex is a
natural-language query and the answer is the emoji whose description the model
places nearest to it. `i.lost.my.job.dns.emb.is` is a sentence; the reply is the
emoji that fits, as a `TXT` record.

The zone is a second deployment of the product, not a second process in the
site's sandbox: it runs its own `emb` on loopback and `cmd/emb-dns` in front of
it, holds no ONNX Runtime of its own, and is reachable on port 53 (UDP and TCP)
and over HTTP for the view `dig` cannot print.

- **Apex** — `dns.emb.is` itself is reserved for `SOA`, `NS`, and a usage `TXT`;
  queries live strictly beneath it.
- **Authority contact** — `ns1.emb.is` / `hostmaster.emb.is`, deliberately
  outside the zone, because a name inside it would be a query.

## The grammar

A name's labels are the words of a sentence. A `.` is a word break; `+` and `-`
separate the terms a query composes, evaluated left to right with no precedence,
and `*` joins terms into a conjunction — the entries closest to *all* of them at
once. A name composes or conjoins, never both: mixing `*` with `+` or `-` is
refused, because each operator means one thing and no precedence decides between
them.

| Name | Treated as |
|---|---|
| `i.lost.my.job.dns.emb.is` | the sentence `i lost my job` |
| `🦈.dns.emb.is` | the glyph's vocabulary entry, `shark` |
| `shark.dns.emb.is` | the same query — a glyph is a way of writing its entry |
| `🦈-🐟+🐦.dns.emb.is` | `shark − fish + bird`, composed as vectors |
| `🐉*🥟.dns.emb.is` | the entry closest to both a dragon and a dumpling |

A composition of unit vectors is an average, so it scores its own terms equally
and answers with one of them (`pizza + eagle` returns pizza or eagle). A
conjunction is a different question: it ranks every entry by the product of its
similarity to each term and answers with the best entry the query did not name,
so it can answer with a third thing entirely — `🐉*🥟` → 🐼, `🍣*🌸` → 🍡,
`🗽*🏈` → 🏟. The HTTP reply carries each result's similarity to every term
beside its joint score, so a joke can be told from a stretch.

A glyph expands to its vocabulary entry's **description** before embedding, which
is why `🦈` and `shark` are the same query by construction: the model never sees
a raw glyph, because BERT's vocabulary contains almost none of them.

**A sentence is answered with an emoji sentence.** When one term was written as
more than one word, the first record is the whole ranking read as one pictogram —
`my.mom.is.in.the.hospital` → `"🏥🧑‍⚕️👩‍⚕️"` — followed by the ranked records it
came from. Ask in a glyph or compose terms and the answer is a single emoji,
because the shape of the question, not a flag, decides.

A name the grammar cannot read is refused rather than truncated: an empty name,
one that is only operators or opens with `-`, one that mixes `*` with `+`/`-`, a
label over 63 bytes, or a name over 253 bytes. DNS answers `NXDOMAIN`; HTTP
answers `400`.

## The reply

One `TXT` record per ranked result (`top_k`, default 3), each carrying only its
emoji — no score, no vocabulary name — and the owner is always the name that was
queried. A record is a value a resolver caches for its TTL and answers other
people's questions from, so it holds the answer, not the working behind it.

```
$ dig +short TXT the.server.is.on.fire.dns.emb.is
"\240\159\148\165\240\159\154\146\240\159\148\166"
"\240\159\148\165"
"\240\159\154\146"
"\240\159\148\166"
```

Per qtype: `TXT` and `ANY` are answered; every other type gets NODATA with the
`SOA` in authority (which is what lets a resolver cache the absence); `AXFR` and
`IXFR` are refused; a name outside the zone is `REFUSED`, which is also what
keeps the zone from being usable as an open resolver. Answers carry a positive
TTL (default 300s) and refusals and NODATA a shorter negative TTL (default 60s),
both from `dns/config.yaml`.

## Why `dig` prints bytes, and the HTTP surface

Every resolver tool renders non-ASCII `TXT` data escaped, so `dig` shows
`\240\159\145\184` where the record carries `👍`. That is the honest wire truth,
and it is why the deployment also serves the same ranking over HTTP in the form
a reader can see.

```
$ curl http://127.0.0.1:8099/the.server.is.on.fire
# the.server.is.on.fire.dns.emb.is.	300	IN	TXT
"\240\159\148\165\240\159\154\146\240\159\148\166"	🔥🚒🔦	the sentence	
"\240\159\148\165"	🔥	fire	0.771
"\240\159\154\146"	🚒	fire engine	0.422
"\240\159\148\166"	🔦	flashlight	0.398
```

The routes are read-only and derived from the same ranking the `TXT` records are:

| Route | Body |
|---|---|
| `GET /<name>` | the records, one per line: escaped data, glyph, vocabulary name, score; the sentence first when there is one |
| `GET /?q=<name>` | the same as JSON (`name`, `type`, `ttl`, `model`, `records[]` with `escaped`/`glyph`/`name`/`slug`/`score`, and `sentence.glyphs` when there is one) |
| `GET /healthz` | `{"ready":true,"zone":"dns.emb.is."}`; `503` until the vocabulary index exists |
| `GET /meta` | model, dimensions, vocabulary entry count, default `top_k`, TTLs, readiness, and the vocabulary's source and licence |
| `GET /stats` | served, unparseable, rate-limited, upstream errors/calls and mean latency |

Only `GET`/`HEAD` are served, and only the site's own origins (`origins` in the
config) may read the routes from a browser; every other origin is denied, and a
preflight for a writing method is refused.

## Running it locally

The whole zone runs on loopback with non-privileged ports. It needs the model,
which `just download-model` fetches:

```sh
just download-model            # fp32 MiniLM, the weights the examples were pinned against
just dns-dev                   # emb :16389, DNS :5354, HTTP :8099
```

Then, from another shell inside `nix develop`:

```sh
dig @127.0.0.1 -p 5354 +short TXT the.server.is.on.fire.dns.emb.is
curl http://127.0.0.1:8099/the.server.is.on.fire
just verify-emoji              # checks the zone against dns/examples.json
```

`dns/emoji-vocab.json` is committed, so the zone builds with no tooling and no
network. It is rebuilt and checked from the pinned CLDR annotations:

```sh
just emoji-vocab               # rebuild from CLDR
just emoji-vocab-check         # fail on any drift
```

The vocabulary is one entry per fully-qualified emoji (2223 of them), each with
a glyph, a kebab-case slug, and a description; skin-tone and gender variants are
folded onto one canonical glyph. It is derived from Unicode CLDR — the emoji
annotations, plus the flag entries CLDR keeps with its derived annotations — and
carries the Unicode licence (`Unicode-3.0`) and every source it was built from,
with a digest each, beside the asset.

## Deploying

The zone is a Fly app of its own (`dns/fly.toml`): one machine, one volume for
the model and the cache snapshot, and a dedicated IPv4 because that is the only
way the platform answers UDP.

```sh
just dns-image                                    # docker build -f dns/Dockerfile
fly apps create emb-dns
fly volumes create emb_dns_data --region iad --size 1 -a emb-dns
just dns-deploy                                   # fly deploy . -c dns/fly.toml
```

UDP binds the platform's own address (`fly-global-services:53`) and TCP binds the
wildcard (`0.0.0.0:53`), both set by `dns/run.sh`; UDP is answered only on a
dedicated IPv4, so allocate one (without `--shared`, which cannot answer UDP),
and a shared-less IPv6 for the TCP path:

```sh
fly ips allocate-v4 -a emb-dns --yes            # dedicated by default
fly ips allocate-v6 -a emb-dns                  # TCP only; never the UDP endpoint
```

Set the zone's apex addresses and request the certificate for the readable
surface:

```sh
fly secrets set APEX_ADDRESS=<the dedicated IPv4> -a emb-dns
fly secrets set APEX_ADDRESS_V6=<the dedicated IPv6> -a emb-dns
fly certs add zone.emb.is -a emb-dns
```

The zone's own name (`dns.emb.is`) does not carry TLS. It is a delegation, and
the platform's certificate checker does not follow one; the readable HTTP
surface therefore lives at `zone.emb.is`, a sibling the parent zone serves
directly, so validation reaches it (see "Certificate validation" below).

### The delegation at `emb.is`

`dns.emb.is` is delegated to the zone, and `zone.emb.is` is the readable HTTP
surface. The parent `emb.is` zone carries four records and nothing else:

```text
; the delegation: every query name lives here
dns.emb.is.    NS    ns1.emb.is.
ns1.emb.is.    A     <the dedicated IPv4>       ; DNS-only: never proxied

; the readable surface, served by the app but validated through the parent
zone.emb.is.   A     <the dedicated IPv4>       ; DNS-only: never proxied
zone.emb.is.   AAAA  <the dedicated IPv6>       ; DNS-only: never proxied
```

- The `ns1.emb.is` A record is the nameserver's own address. `ns1.emb.is` sits
  inside the parent zone, so the parent answers it; no glue record is needed.
  Keep it **DNS-only** (grey cloud at Cloudflare): a proxied nameserver address
  cannot answer queries.
- Do **not** add an `A`/`AAAA` for `dns.emb.is` at the parent. The delegation
  makes the zone authoritative, and Cloudflare shadows any record at a
  delegated name; the zone answers its own apex from `APEX_ADDRESS`.
- Do **not** add an `AAAA` for `ns1.emb.is`. The IPv6 answers TCP, but UDP (what
  a resolver needs first) is answered only on the dedicated IPv4, so a resolver
  that preferred the AAAA would fail.

At Cloudflare that reads as `NS  dns  ns1.emb.is`, `A  ns1  <ipv4>`,
`A  zone  <ipv4>`, `AAAA  zone  <ipv6>`, all unproxied.

Once delegated, verify from outside before anything references the zone:

```sh
dig +short TXT the.server.is.on.fire.dns.emb.is
curl https://zone.emb.is/the.server.is.on.fire
curl https://zone.emb.is/healthz
dig @204.10.79.246 +short TXT example.com        # REFUSED: not this zone's business
```

## Cost

The only new line items are the machine, its volume, and the dedicated IPv4 that
UDP requires. Fly bills machine time, not queries, so query volume does not move
the bill; the app runs one small always-on `shared-cpu-1x`. `emb-sandbox`, the
site's machine, is neither resized nor reconfigured: the zone's `emb` is its own,
on the zone's machine.

## Certificate validation

A delegated name cannot carry a platform-managed certificate: the checker reads
the **parent** zone, returns the parent's `SOA` and empty record lists, and never
follows the `NS` delegation into the zone; and Cloudflare, as the parent, either
refuses or shadows any record at or below a delegation. That is why the readable
HTTP surface is `zone.emb.is` rather than `dns.emb.is`: `zone.emb.is` is served
by the parent zone itself, so `fly certs add zone.emb.is` validates normally.

The zone's apex `A`/`AAAA` (`APEX_ADDRESS`/`APEX_ADDRESS_V6`) still make
`dns.emb.is` resolve to the app, but they are for reference, not for TLS: there
is no managed certificate for the delegated name.
