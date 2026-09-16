## MODIFIED Requirements

### Requirement: The published tree is the site and nothing else

The deployed origin SHALL serve exactly one landing page, one documentation
surface, one not-found page, and the assets those three reference. Files that
exist to author, measure, or critique the site MUST NOT be reachable at any
path on the deployed origin, and neither MUST the source, configuration, or
presets of a service runtime that happens to live under the site directory.
Because those files may live beside the pages they describe, their absence MUST
NOT rest on convention: the exclusion MUST be declared in the repository,
applied by the deploy tooling, and asserted against the resulting tree rather
than trusted.

#### Scenario: An authoring file is not served

- **WHEN** any file that documents, generates, measures, or critiques the site is requested at its path on the deployed origin
- **THEN** the response is not that file's content

#### Scenario: Service source is not served

- **WHEN** a file that belongs to the sandbox's implementation, its configuration, or its preset scripts is requested at its path on the deployed origin
- **THEN** the response is not that file's content

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
- **THEN** both surfaces render completely, because nothing in the tree requires the deploy tooling to have run

### Requirement: The gate runs only the suites a change can affect

Continuous integration SHALL classify a change before running the repository's
suites. A change confined to the site directory, excluding the sandbox service's
own code, MUST NOT run the server's or the gems' jobs; a change touching any
other file MUST run them. A change to the sandbox service's code MUST run the
tests for that code even though it lives under the site directory, because that
code is a program and not a page. The site's own checks MUST run for both kinds
of change, because the version file reaches the pages as well as the binary.

When the changed files cannot be determined, the suites MUST run rather than be
skipped, and a skipped suite MUST be reported as a result of the change rather
than by the change never triggering a workflow, so a required check is never
left waiting on a status that will not report.

#### Scenario: A site-only change skips the suites

- **WHEN** a change touches only files under the site directory that are not sandbox service code
- **THEN** the site's own checks run, and the server's and the gems' jobs are skipped

#### Scenario: A service change runs the service's tests

- **WHEN** a change touches the sandbox service's code, its presets, or its deployment configuration
- **THEN** the tests for the service run, whether or not the other suites run

#### Scenario: A change outside the site runs the suites

- **WHEN** a change touches any file outside the site directory
- **THEN** the server's and the gems' jobs run, whatever else the change touches

#### Scenario: An undeterminable diff is not treated as site-only

- **WHEN** the previous revision is absent — a new branch, a forced push, a dispatch — or the diff cannot be computed
- **THEN** the suites run, because skipping on an unknown diff is the failure mode that hides a defect

#### Scenario: The skip blocks nothing

- **WHEN** the suites are skipped for a change and those jobs are required by branch protection
- **THEN** the change is not left waiting on a status that will not report
