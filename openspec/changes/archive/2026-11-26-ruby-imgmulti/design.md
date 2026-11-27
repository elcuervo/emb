# Design

## Context

See `proposal.md` and `specs/emb-ruby-client/spec.md`. The server already accepts `EMB.IMGMULTI`; `Emb::Proxy#image` already handles byte normalization and BLOB/VALUES output decoding for one model. `Emb.multi` is the existing eager block that collects `m[:model][text]` pairs and sends `EMB.MULTI`.

## Goals / Non-Goals

**Goals:** expose the existing cross-model image command through the same eager block API as text, with predictable Ruby return shapes and the current binary-safety guarantees.

**Non-Goals:** add image URL fetching, change the server protocol, defer this explicit block (it is eager by construction, like the text block), or invent a second pairs-list API beside `multi`.

## Decisions

- Add `image(bytes, *more, format:, outputs:)` to the block's `PairCollector` with the same arguments as `Proxy#image`; a multi-image call contributes one image pair per image, so the block result holds one Hash per image in submission order. This makes single sends (`Emb[:model].image`) and multi sends behave identically apart from transport.
- Text pairs keep `EMB.MULTI`; image pairs are grouped by reply format and output projection (the command's `VALUES`/`OUTPUTS` keywords precede all pairs, so incompatible calls cannot share one command) and chunked by `batch_size` like text. A block mixing both kinds sends one command per kind and reassembles results by submission index.
- Validate the pair structure and format when the call is recorded (before the block's command is sent), and normalize image bytes through the same binary-safe path as `Proxy#image`.
- Reuse image output decoding for every non-null result. BLOB results become maps of output names to float arrays; VALUES results retain their typed envelopes and the pair's `model` tag the server sends. Malformed response shape and transport/server errors remain errors.

## Risks / Trade-offs

- [Risk] One very large explicit block can exceed the server's configured request limit → document that server-side image and command caps apply; preserve null truncation slots rather than hiding them.
- [Risk] Per-model output names vary → decode each returned image independently instead of assuming a shared output schema.
- [Risk] Mixed text/image blocks need more than one round trip → the block is an explicit eager API, and each command is still one round trip per kind.

## Migration Plan

This is additive and eager. No existing client call changes. Ship the block support with focused fake-RESP tests and examples; rollback by removing the `image` collector method, with no server or data migration.
