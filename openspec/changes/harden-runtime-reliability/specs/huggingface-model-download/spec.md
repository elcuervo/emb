## ADDED Requirements

### Requirement: Atomic download publication
The downloader SHALL write a new artifact to a unique temporary sibling file and publish its final path only after copying and closing succeed. Any failed transfer or publication SHALL return an error and clean up its temporary file without publishing partial bytes.

#### Scenario: Interrupted transfer followed by retry
- **WHEN** an HTTP body fails after some bytes have been written
- **THEN** that attempt SHALL return an error without creating a final cache entry
- **AND** a later retry SHALL perform a new transfer and return the complete artifact

#### Scenario: Close or publication failure
- **WHEN** closing the downloaded file or publishing it to the final path fails
- **THEN** the downloader SHALL return an error and remove its temporary file

#### Scenario: Completed artifact already exists
- **WHEN** a completed local artifact exists at the requested final path
- **THEN** existing reuse behavior SHALL be preserved
