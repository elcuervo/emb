## MODIFIED Requirements

### Requirement: The published tree is the site and nothing else

The deployed origin SHALL serve exactly one landing page, one documentation
surface, one demos gallery, one client surface, one not-found page, and the
assets those five reference. Files that exist to author, measure, or critique
the site MUST NOT be reachable at any path on the deployed origin, and neither
MUST the source, configuration, or presets of a service runtime that happens to
live under the site directory. Because those files may live beside the pages
they describe, their absence MUST NOT rest on convention: the exclusion MUST be
declared in the repository, applied by the deploy tooling, and asserted against
the resulting tree rather than trusted.

The gallery's committed vector indexes are site artifacts and MUST be served;
the corpus sources, the embedding tool, and any intermediate build output MUST
NOT be.

#### Scenario: An authoring file is not served

- **WHEN** any file that documents, generates, measures, or critiques the site is requested at its path on the deployed origin
- **THEN** the response is not that file's content

#### Scenario: Service source is not served

- **WHEN** a file that belongs to the sandbox's implementation, its configuration, or its preset scripts is requested at its path on the deployed origin
- **THEN** the response is not that file's content

#### Scenario: A built index is served and its source is not

- **WHEN** a demo's vector index and the corpus or tool that produced it are requested at their paths
- **THEN** the index is served and the corpus source and build tool are not

#### Scenario: A new surface is asserted rather than assumed to ship

- **WHEN** a page is added to the site directory
- **THEN** the site's own check reports it as an unexpected file until it is named in the served set and in the pages that must declare their address, so the page cannot ship because someone forgot to exclude it

#### Scenario: The exclusion is declared, not remembered

- **WHEN** a file is added under the site directory
- **THEN** whether it ships is determined by a checked-in exclusion list, and a file not named there ships

#### Scenario: The declared set is asserted, not assumed

- **WHEN** a file is present in the site directory and absent from the exclusion list
- **THEN** the site's own check reports it before any deployment, rather than the deploy uploading it silently

#### Scenario: Working locally and serving publicly differ on purpose

- **WHEN** one site directory is both the working tree and the publish source
- **THEN** the local server serves it exactly as authored, including the files the origin must not serve, so measuring and critiquing the site requires no second location

#### Scenario: A same-origin harness measures without shipping

- **WHEN** a browser-measured check requires loading the site in a same-origin frame
- **THEN** that harness is reachable at a local path and is absent from the deployed origin

#### Scenario: The served tree is renderable without the deploy tooling

- **WHEN** the site directory is served by any static file server from the filesystem
- **THEN** every surface renders completely, because nothing in the tree requires the deploy tooling to have run
