# visual-embeddings Specification

## Purpose

Demonstrates image embedding end to end on the public site: samples committed to
the repository rather than visitor uploads, an image bounded before it leaves the
page, binary carried explicitly and admitted for the sandbox's own image preset
only, per-request bounds enforced before decode, and a preset that runs the
model's image branch and reads its embedding.

## Requirements

### Requirement: An image ships as a sample and is bounded before it is sent

A demo that embeds an image SHALL ship its images in the repository and MUST NOT
accept a visitor's file, so the sandbox never receives an arbitrary upload from a
stranger. The page SHALL still bound whatever image it sends — capping the long
edge and re-encoding it before any byte leaves the page — and SHALL report the
bytes it sent. The size bound is the first limit in the chain, and it is the one
the client can enforce without trust.

#### Scenario: The demo accepts no upload

- **WHEN** the image plate is rendered
- **THEN** it offers only samples committed to the repository, with no file input, drop target, or paste path

#### Scenario: A sent image is bounded before sending

- **WHEN** the page sends a sample
- **THEN** it caps the long edge and re-encodes the image first, sends the reduced bytes, and reports how many bytes it sent

#### Scenario: The sent bytes are the shown bytes

- **WHEN** the page displays an image and the size it sent
- **THEN** the preview is the reduced image that was sent

### Requirement: Binary is carried explicitly and admitted for one preset only

The transport SHALL carry an image as base64 in an argument the request names as
binary, and the bridge SHALL decode exactly those arguments back to bytes. Binary
MUST be refused unless the command is the sandbox's own preloaded image preset
called by digest, and MUST be refused for a raw image command or any other
command. A request that names an out-of-range index, a malformed base64 value, or
an image above the byte cap SHALL be refused before the server is reached.

#### Scenario: Only the image preset accepts binary

- **WHEN** a request names a binary argument for anything but the sandbox's preloaded image preset
- **THEN** the bridge refuses it and the server receives no such command

#### Scenario: The decoded bytes are the sent bytes

- **WHEN** the bridge accepts a binary argument
- **THEN** the server receives the original bytes, because the bridge base64-decodes them rather than passing the encoding through

#### Scenario: A malformed or oversized argument is refused before the server

- **WHEN** a binary argument is not valid base64, names an out-of-range index, or decodes above the per-image byte cap
- **THEN** the bridge refuses the request legibly and the server is not contacted

#### Scenario: Raw image commands stay outside the surface

- **WHEN** a visitor sends `EMB.IMG` or `EMB.IMGMULTI`
- **THEN** the bridge refuses it and names the permitted surface, as it does for every other out-of-surface command

### Requirement: The sandbox bounds images before decode

The server SHALL enforce a per-request image count, a per-image byte cap, and a
decoded-pixel cap, and MUST refuse an image that exceeds them before decoding it.
The bounds SHALL be small enough that one visitor cannot spend the machine's
memory or CPU with a single request, and the bridge SHALL enforce the same count
and byte bounds so a refusal happens at the nearest edge.

#### Scenario: Too many images are refused

- **WHEN** a request carries more images than the sandbox admits
- **THEN** it is refused as a capacity bound, and the images are not decoded

#### Scenario: An oversized image is refused before decode

- **WHEN** an image exceeds the byte or pixel cap
- **THEN** the request fails legibly and no decode or inference runs for it

#### Scenario: The image is used for one request and not stored

- **WHEN** a visitor embeds an image
- **THEN** the bytes answer that request and are not persisted, indexed into any corpus, or shown to another visitor

### Requirement: The image preset runs the model's image branch

The image demo SHALL call the sandbox's preloaded image preset by digest, and the
script SHALL preprocess the raw bytes with the model's own plan and read the
model's image embedding. Where the model fuses text and image branches in one
graph, the script SHALL supply the unused branch as a host-built constant rather
than materializing it element by element, and SHALL request only the outputs it
reads.

#### Scenario: The script sees bytes, not a URL

- **WHEN** the image preset runs
- **THEN** it receives the image as the request's bytes, and the server fetches nothing from the network

#### Scenario: The branch not in use is a constant

- **WHEN** a fused model needs every input for either branch
- **THEN** the script builds the unused branch host-side as a constant tensor and asks for one output per run

#### Scenario: The labels are text, not training

- **WHEN** the image demo scores candidate labels
- **THEN** the labels are embedded as text in the same space and ranked by cosine, and the model was not trained on them
