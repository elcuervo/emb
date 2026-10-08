# audio-video-embeddings Specification

## Purpose
Puts real audio and video models in the gallery: media embedded by a model that
was trained on that modality, an index built offline and shipped, a text query
embedded live by the same model's text tower, and the provenance and measurable
quality of each export stated rather than assumed.

## Requirements

### Requirement: Media embeddings come from a real modality model

A demo that retrieves audio or video SHALL embed the media with a model trained
on that modality, and MUST NOT present an image model reading a picture of the
medium as that modality's model. The model SHALL be named on the plate.

#### Scenario: The audio model is an audio model

- **WHEN** the plate retrieves music by a phrase
- **THEN** the media vectors come from a text-audio model's audio tower and the query vector from its text tower, and the plate names that model

#### Scenario: The video model is a temporal model

- **WHEN** the plate retrieves a clip by a phrase
- **THEN** the clip is embedded by a model whose video tower consumes the clip's frames together, and the plate names that model

### Requirement: The index is built offline and the query is embedded live

The media index SHALL be built offline from the shipped media and committed with
the gallery. The query SHALL be a text query embedded live by the same model the
index was built with, and a missing or mismatched index SHALL fail legibly.

#### Scenario: The query uses the index's model

- **WHEN** a reader types a phrase
- **THEN** the phrase is embedded live by the same model the shipped index was built with, and a disagreement fails loudly rather than returning meaningless neighbours

#### Scenario: A missing index fails loudly

- **WHEN** an index is absent or built for another model
- **THEN** the plate states the failure instead of drawing a ranking

### Requirement: An export's provenance is stated and probed before use

For each model export the change SHALL record its source, license, and whether
it is a vendor-published export or a third-party one. Where the export's tensor
names, dtypes, or shapes are not documented by its publisher, the build SHALL
probe the graph on the real runtime and SHALL fail when the probe disagrees with
what the code assumes.

#### Scenario: An unverified export is probed

- **WHEN** a model's input or output contract comes from a third-party card rather than the publisher
- **THEN** a build step loads the graph and prints its real tensors, and a mismatch stops the build

#### Scenario: The provenance is on the plate

- **WHEN** a reader asks what is running
- **THEN** the plate states each model's source and license, including that one export is a third party's

### Requirement: A pinned retrieval evaluation guards the index

Each shipped index SHALL carry a pinned evaluation — a set of text queries and
the media each is expected to retrieve — and the build SHALL fail when a query
no longer retrieves its expected media, so a changed model, mel, or frame
preprocessing is a decision rather than a silent regression.

#### Scenario: A broken pipeline fails the build

- **WHEN** the mel, the frame sampling, or the model changes enough that a pinned query retrieves the wrong media
- **THEN** the build fails and names the query

#### Scenario: The evaluation ships with the index

- **WHEN** the index is built
- **THEN** its pinned queries and their measured answers are written into the shipped data

### Requirement: Live media encoding is bounded to an allowlisted preset

Where a plate sends a preprocessed medium to the server for live encoding, the
transport SHALL admit bytes only for a preset named on an allowlist, within the
sandbox's per-request byte and count bounds, and MUST refuse bytes for any other
command. The plate MUST NOT send a tensor larger than the bound.

#### Scenario: Only an allowlisted preset receives bytes

- **WHEN** a request names binary arguments for a preset that is not on the allowlist
- **THEN** the bridge refuses it and the server receives no such command

#### Scenario: An oversized tensor is not sent

- **WHEN** a medium's packed tensor would exceed the per-request byte bound
- **THEN** the plate does not offer a live encode for that medium and states why

### Requirement: A media demo accepts no upload

A demo whose input is a recording or a clip SHALL ship that medium in the
repository and MUST NOT accept a visitor's file, microphone, or camera. A
visitor supplies nothing but a query.

#### Scenario: The demo accepts no upload

- **WHEN** a media plate is rendered
- **THEN** it offers only media committed to the repository, with no file input, drop target, paste path, or capture device

### Requirement: The plate states each model's limit

A plate SHALL state, for each model, what it does not do — for a video model,
that the frame count and sampling are fixed and that a description is a
retrieval score; for an audio model, that it is trained on general audio and its
reading of a specific genre is a similarity rather than a fact.

#### Scenario: The limits are on the plate

- **WHEN** the plate presents a model's ranking
- **THEN** its prose states the fixed sampling and that the score is a retrieval similarity, not a judgment
