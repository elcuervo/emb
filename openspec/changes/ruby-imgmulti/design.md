# Design

## Context

See `proposal.md` and `specs/emb-ruby-client/spec.md`. The server already accepts `EMB.IMGMULTI`; `Emb::Proxy#image` already handles byte normalization and BLOB/VALUES output decoding for one model. The module and client APIs delegate to the same default/instance client.

## Goals / Non-Goals

**Goals:** expose the existing cross-model image command with predictable Ruby return shapes and the current binary-safety guarantees.

**Non-Goals:** add image URL fetching, change the server protocol, defer this explicit call, or add per-pair output-projection options.

## Decisions

- Use `image_multi(pairs, format: :binary)` on both `Emb` and `Emb::Client`; each pair is `[model, bytes]`. This keeps the method explicit and avoids introducing another block DSL beside `multi`.
- An empty pair list returns `[]` without I/O. Validate pair structure and format before sending; normalize image bytes through the same binary-safe path as `Proxy#image`.
- Build one `EMB.IMGMULTI` command with optional leading `VALUES`, then ordered model/byte pairs. Do not chunk implicitly: the server already owns image-count truncation semantics and returns null slots visibly.
- Reuse image output decoding for each non-null pair. BLOB results become maps of output names to float arrays; VALUES results retain their typed envelopes. A null slot remains `nil`; malformed response shape and transport/server errors remain errors.

## Risks / Trade-offs

- [Risk] One very large explicit pair list can exceed the server's configured request limit → document that server-side image and command caps apply; preserve null truncation slots rather than hiding them.
- [Risk] Per-model output names vary → decode maps from each returned pair independently instead of assuming a shared output schema.

## Migration Plan

This is additive and eager. No existing client call changes. Ship the client method with focused fake-RESP tests and examples; rollback by removing the new methods, with no server or data migration.
