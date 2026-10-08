# media-demos Specification

## Purpose
Lets the gallery put a real medium — a recording, a clip — beside the model's own
reading of it, and show honestly which questions each can answer: a fingerprint
names the exact recording, an embedding describes it, and a phrase finds the
moment in a clip.

## Requirements

### Requirement: A media demo ships its media and accepts no upload

A demo whose input is a recording or a clip SHALL ship that medium in the
repository and MUST NOT accept a visitor's file, microphone, or camera. The
model receives the medium only as a picture derived from a shipped asset, and a
visitor supplies nothing but a query.

#### Scenario: The demo accepts no upload

- **WHEN** a media plate is rendered
- **THEN** it offers only media committed to the repository, with no file input, drop target, paste path, or capture device

#### Scenario: A played medium is a shipped asset

- **WHEN** a reader hears or watches a medium the plate names
- **THEN** the bytes played are a committed asset, not a visitor's recording

### Requirement: A fingerprint names the exact recording, computed live

A plate that identifies a recording SHALL compute the query's fingerprint in the
browser from the shipped excerpt — spectral peaks, paired hashes, and an offset
histogram — and MUST NOT present a stored answer as the result. The library's
hashes SHALL be built from the shipped audio by the same algorithm, and the
plate SHALL show the agreement that produced the answer.

#### Scenario: The query is fingerprinted where it runs

- **WHEN** a reader identifies an excerpt
- **THEN** the peaks, the hashes, and the winning offset are computed in the browser from that excerpt, and the count of agreeing hashes is shown

#### Scenario: A degraded excerpt still matches

- **WHEN** the library is asked to identify an excerpt degraded with noise
- **THEN** it returns the correct recording and the agreement it found, rather than failing or guessing

#### Scenario: The database ships with a verified answer

- **WHEN** the fingerprint database is built
- **THEN** every shipped query is matched against the built library and the build fails if a query does not recover its track

### Requirement: A modelled reading describes rather than identifies

Where a plate also asks a real modality model to read the same medium, it SHALL
present that reading as a description, SHALL show the similarity and the margin,
and MUST NOT present it as an identification or as evidence that the model
perceived more than it did.

#### Scenario: The two answers are distinguished

- **WHEN** the plate shows both the fingerprint's answer and the audio model's reading of one excerpt
- **THEN** the fingerprint names the recording and the model's reply is labelled a description, with its cosine and margin

#### Scenario: The caveat is load-bearing

- **WHEN** the plate's prose explains the reading
- **THEN** it states that the model was trained on general audio, that its reading of a genre is a similarity rather than a fact, and that it did not identify the recording

### Requirement: The frame index is built by the sandbox's own model

A plate that searches video SHALL rank vectors the modality model produced,
built offline from the shipped clips and committed with the gallery. The phrase
SHALL be embedded live by the same model's text tower, and a missing or
mismatched index SHALL fail legibly.

#### Scenario: The stills and their vectors are the same pictures

- **WHEN** the plate highlights a clip
- **THEN** the clip shown is a shipped file and the score beside it is that clip's committed vector against the live phrase embedding

#### Scenario: A missing index fails loudly

- **WHEN** the video index is absent or built for another model
- **THEN** the plate states the failure instead of drawing a ranking

### Requirement: A typed phrase ranks a clip's frames

A plate SHALL let the reader find a clip by a typed phrase, ranking the clip
embeddings the video tower produced from the clip's frames, and SHALL let the
reader play the winning clip.

#### Scenario: The phrase finds a frame

- **WHEN** a reader types a phrase about the footage
- **THEN** the clips are ranked by cosine, the nearest is marked, and it can be played

#### Scenario: Every shown score is from the live reply

- **WHEN** the ranking is drawn
- **THEN** each score comes from the phrase's own embedding against the shipped vectors, not from a stored ranking

### Requirement: A media plate plays the medium it names

A plate SHALL let the reader hear or watch a shipped medium it names, so the
result is grounded in the medium and not only in a picture of it. Playback MUST
NOT begin until the reader asks for it.

#### Scenario: The identified recording can be played

- **WHEN** the plate names a recording
- **THEN** the shipped audio can be played, positioned at the matched offset, and it does not autoplay

#### Scenario: The clip can be played from the moment

- **WHEN** the phrase names a clip
- **THEN** the shipped clip can be played, and it does not autoplay

### Requirement: Every displayed figure comes from the shipped data

Every number a media plate displays — the track count, the hash count, the frame
count, the dimension, the model, each score and margin, the licences — SHALL be
read from the shipped index, the build output, or the server's reply, rather
than transcribed into the page.

#### Scenario: A figure is read, not typed

- **WHEN** the plate renders its figures
- **THEN** each comes from the fingerprint database, the frame index, or the model's reply, so a rebuild cannot leave the page describing the previous build
