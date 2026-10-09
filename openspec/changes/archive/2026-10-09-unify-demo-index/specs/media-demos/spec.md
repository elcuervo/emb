# Spec Delta

## MODIFIED Requirements

### Requirement: A fingerprint names the exact recording, computed live

A plate that identifies a recording SHALL compute the query's fingerprint in the
browser from the shipped excerpt — spectral peaks, paired hashes, and an offset
histogram — and MUST NOT present a stored answer as the result. The library's
hashes SHALL be built from the shipped audio by the same algorithm, shipped as
the fingerprint table of the gallery's shared index, and the plate SHALL show
the agreement that produced the answer.

#### Scenario: The query is fingerprinted where it runs

- **WHEN** a reader identifies an excerpt
- **THEN** the peaks, the hashes, and the winning offset are computed in the browser from that excerpt, and the count of agreeing hashes is shown

#### Scenario: A degraded excerpt still matches

- **WHEN** the library is asked to identify an excerpt degraded with noise
- **THEN** it returns the correct recording and the agreement it found, rather than failing or guessing

#### Scenario: The database ships with a verified answer

- **WHEN** the fingerprint database is built
- **THEN** every shipped query is matched against the built library and the build fails if a query does not recover its track

#### Scenario: The library ships inside the shared index

- **WHEN** the gallery's index is built
- **THEN** the fingerprint library is the fingerprint table of the same content-hashed file that holds the corpus and media vectors, and no separate fingerprint file ships

### Requirement: The frame index is built by the sandbox's own model

A plate that searches video SHALL rank vectors the modality model produced,
built offline from the shipped clips and committed as a table in the gallery's
shared index. The phrase SHALL be embedded live by the same model's text tower,
and a missing or mismatched index SHALL fail legibly.

#### Scenario: The stills and their vectors are the same pictures

- **WHEN** the plate highlights a clip
- **THEN** the clip shown is a shipped file and the score beside it is that clip's committed vector against the live phrase embedding

#### Scenario: A missing index fails loudly

- **WHEN** the video index is absent or built for another model
- **THEN** the plate states the failure instead of drawing a ranking

### Requirement: Every displayed figure comes from the shipped data

Every number a media plate displays — the track count, the hash count, the frame
count, the dimension, the model, each score and margin, the licences — SHALL be
read from the gallery's shared manifest, its shared index, the build output, or
the server's reply, rather than transcribed into the page.

#### Scenario: A figure is read, not typed

- **WHEN** the plate renders its figures
- **THEN** each comes from the shared manifest, the shared index, or the model's reply, so a rebuild cannot leave the page describing the previous build
