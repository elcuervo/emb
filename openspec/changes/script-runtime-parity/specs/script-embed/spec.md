# script-embed delta

## ADDED Requirements

### Requirement: Image embedding work is shared with the native path

`emb.image.embed` SHALL execute through the same image resources and the same content-addressed image cache as the `EMB.IMG` command, mirroring the sharing that `emb.embed` already provides for text:

- Image bytes SHALL be looked up in the image cache before preprocessing, and results SHALL be written back under the same cache key `EMB.IMG` uses.
- An image embedded by either path SHALL be a cache hit for the other, so the same bytes never infer twice while caching is enabled.
- The scripted path SHALL NOT open a separate image session pool to satisfy `emb.image.embed`.

#### Scenario: Native and scripted image embeddings share a cache entry

- **WHEN** a server with caching enabled serves `EMB.IMG <model> <bytes>` and a script then evaluates `emb.image.embed(<same bytes>)`
- **THEN** the scripted call is served from the existing cache entry and performs no additional inference

#### Scenario: Repeated scripted image embeddings infer once

- **WHEN** a script evaluates `emb.image.embed` twice for the same image bytes with caching enabled
- **THEN** the image branch is invoked once

#### Scenario: Scripted image embedding is visible to the native path

- **WHEN** a script evaluates `emb.image.embed(<bytes>)` and the same bytes are then sent to `EMB.IMG`
- **THEN** the native command is served from the cache entry the script created

#### Scenario: Caching disabled still works

- **WHEN** the server runs without a cache and a script evaluates `emb.image.embed`
- **THEN** the vector is computed directly and returned, matching the `EMB.IMG` reply for the same bytes
