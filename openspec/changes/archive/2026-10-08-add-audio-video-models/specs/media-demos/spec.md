# Spec Delta

## MODIFIED Requirements

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
