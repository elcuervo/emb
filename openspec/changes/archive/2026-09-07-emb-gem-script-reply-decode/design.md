# Design: emb-gem-script-reply-decode

## Context

See proposal.md — Why. Current state: `Emb::Proxy#[]` auto-decodes fp32 bulks
(the embed reply type is fixed by the command), while `eval`/`evalsha` run a
schema-less parse (`parse_script_reply` → `script_hash`) that returns any
string bulk untouched — so `emb.math.float32_bytes` vectors arrive as opaque
bytes. No server or reply-shape changes are involved; this is client-side.

## Goals / Non-Goals

**Goals:**
- Scripted vector replies (single, multi-text, hash fields) decode to float
  arrays on demand, mirroring the embed path.
- Zero behavior change for calls that don't opt in (existing specs + scripts).
- Errors are descriptive, never silent corruption or cryptic `RangeError`s.

**Non-Goals:**
- Server-side reply typing/envelopes (reply grammar stays as-is).
- Auto-detecting vectors from raw bulks (impossible on the RESP2 wire — a
  3072-byte bulk could be text).
- New gem version/release mechanics (see `gems-release-lifecycle`).

## Decisions

### D1: Opt-in `decode:` keyword, not auto-detection

A raw bulk carries no type on the wire: 3072 bytes is an embedding *or* a
3 KB text reply, so decoding every 4-aligned string would corrupt text. The
embed path auto-decodes only because `EMB` has a fixed reply type. Scripts are
schema-less — the client must state intent per call. `decode: nil` (default)
keeps every existing behavior and test.

### D2: `decode:` vocabulary — `:f32` at the top level, or `{field => :f32}` in hashes

Two shapes cover the documented scenarios:
- `:f32` — the reply *is* the vector: bare `float32_bytes` bulks (siglip2),
  and the pre-`float32_bytes` numeric-array style pass through unchanged
  (decoding an Array of Numbers is identity).
- `{field => :f32}` — structured replies (`{dim = 768, embedding = <bulk>}`):
  the field is decoded after `script_hash` turns the flat pair array into a
  Ruby Hash; applied per element for multi-text replies. Field-level decoding
  stops one level deep — script replies are shallow, so a generic
  leaf-walker over arbitrary nesting is YAGNI (and harder to reason about).

Alternatives considered: a generic "decode every string leaf" mode — rejected
(magical, would silently unpack text fields); a `Vector`/typed result class —
rejected (overkill for `unpack`). Unknown modes raise `ArgumentError` before
any command is sent.

### D3: Decode runs after reply parsing, keyed off the existing `multi:` flag

The pipeline stays `parse_script_reply(reply, multi:, decode:)`: parse the
RESP value through `script_hash` first (so hashes exist), then apply decode at
the top level (single) or per element (multi). The `multi:` flag — derived
from `texts.size > 1` — resolves the only ambiguity: an Array reply is a
vector when `multi: false` and all elements are Numbers, but an array of
per-text values when `multi: true`.

### D4: Explicit error taxonomy, client-side

Decode failures raise `ArgumentError` with the offending shape in the message:
unknown mode; reply/element that is neither a String bulk nor a numeric Array
under `:f32`; a non-Hash reply under `{field => :f32}`; and a String whose
length is not a multiple of 4 (pre-checked — Ruby's `unpack('e*')` raises a
bare `RangeError` on misaligned input). Hash-mode shape rules mirror `:f32`:
absent fields are untouched, but a reply that isn't a Hash where one is
expected is a contract mismatch and errors.

### D5: Bench harness adoption proves the path

`bench_models.rb`'s siglip2 scenario calls `evalsha(..., decode: :f32)`,
dropping its hand-rolled `.unpack('e*')` and keeping the 768-dim check on the
decoded size — the change's own scenario exercised through the public API.

## Risks / Trade-offs

- **`decode:` added to an existing signature** → Ruby keyword defaults are
  backward-compatible for all current positional callers; internal callers
  (`bench_models.rb`) are updated in the same change.
- **Misaligned/foreign bulks under `:f32`** → length pre-check + type checks
  raise descriptive errors instead of `RangeError`/silent corruption.
- **Scripts change reply shape between calls** → decode is per-call intent; a
  mismatch errors loudly rather than guessing. Clients own the contract.
- **Numeric-array passthrough hides a shape change** (script switched from
  array to bytes) → both yield floats; byte replies are *faster*, so the
  ambiguity only affects a script that changed shape mid-flight — acceptable.

## Migration Plan

Pure additive client change: (1) decode layer + keyword plumbing in
`commands.rb` / `emb.rb`; (2) rspec cases for every scenario; (3) bench
adoption; (4) docs (gem README + main README script section). Rollback:
revert; default `decode: nil` means no existing caller observes a difference.

## Open Questions

(Deferrable.) Whether a `:float32` alias or additional packers
(`float64_bytes`) deserve decode modes when a second packed-reply use case
appears; whether the decode layer should live in a public helper for custom
command wrappers.