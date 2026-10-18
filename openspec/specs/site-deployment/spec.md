# site-deployment Specification

## Purpose
The `emb` product site's deployment surface: what leaves the repository and is
served at `https://emb.is`, what guarantees a visitor gets the current revision
rather than a stale or leaked one, and what a contributor sees before their
change reaches a reader.

## Requirements

### Requirement: The published tree is the site and nothing else

The deployed origin SHALL serve exactly one landing page, one documentation
surface, one demos gallery, one client surface, one not-found page, and the
assets those five reference. Files that
exist to author, measure, or critique the site MUST NOT be reachable at any
path on the deployed origin, and neither MUST the source, configuration, or
presets of a service runtime that happens to live under the site directory.
Because those files may live beside the pages they describe, their absence MUST
NOT rest on convention: the exclusion MUST be
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

### Requirement: Every page declares one absolute, agreed-upon origin

Each served page SHALL declare an absolute canonical URL, and its
social-preview image and URL metadata MUST be absolute URLs naming the same
origin as that page's canonical URL. A relative social-preview path MUST NOT
ship. The origin MUST be stated in one place per page rather than assembled from
several disagreeing literals, and every page MUST agree on the same origin, so a
pasted link cannot resolve two ways.

The origin MUST NOT be pinned to the production hostname by the repository's own
checks. One tree is served from localhost, from a per-worker address, from
per-branch preview aliases and from the production origin, and the check MUST run
against any of them rather than only against production.

#### Scenario: A page states its canonical address

- **WHEN** either surface is rendered
- **THEN** it carries a canonical link whose value is an absolute URL, and a social-preview image that is an absolute URL on the same origin

#### Scenario: A pasted link previews as a card

- **WHEN** the documentation surface's URL is pasted into a client that reads Open Graph metadata
- **THEN** it supplies an absolute image URL, title, and description, all on the origin that page declares

#### Scenario: No relative social image ships

- **WHEN** the served HTML is searched for social-preview metadata
- **THEN** every such value is an absolute URL, and none resolves against the requesting page

#### Scenario: The pages cannot disagree

- **WHEN** one page's canonical or social metadata names a different origin from another page's
- **THEN** the site's own check reports it, naming both origins and the references that carry them

#### Scenario: The local workflow is not special-cased

- **WHEN** the check is run while the site is served from a development host or a preview address
- **THEN** it asserts only that the addresses are absolute and consistent, and does not require the production hostname

#### Scenario: The document stays inert

- **WHEN** a page is opened from the filesystem
- **THEN** the canonical and social metadata cause no request to leave the filesystem

### Requirement: The production origin serves the site over HTTPS only

`https://emb.is` SHALL serve the site, and plaintext requests to that origin MUST
redirect to it rather than serving content. The canonical origin declared in the
markup MUST be the origin that actually serves the site in production. This is a
property of the deployment, not a constraint on the repository's checks: the
checks report a disagreement between the markup and the configured route rather
than refusing to run outside production.

#### Scenario: Plaintext upgrades

- **WHEN** the production origin is requested over `http`
- **THEN** the response redirects to the `https` origin

#### Scenario: The declared origin is the production serving origin

- **WHEN** the canonical URL of either surface is requested
- **THEN** it returns that page's content without a further redirect

### Requirement: `main` deploys the site and a pull request previews it

A change to the site that lands on the default branch SHALL be deployed to the
production origin automatically. A change proposed in a pull request SHALL be
made viewable at a stable preview address before it merges, and that address
MUST NOT serve production traffic. A change that does not touch the site MUST
NOT produce a deployment of it.

#### Scenario: Merging publishes

- **WHEN** a commit touching the site is pushed to the default branch
- **THEN** the production origin serves that revision

#### Scenario: Opening a pull request publishes a preview

- **WHEN** a pull request touching the site is opened or updated
- **THEN** a preview address serving that revision is reported on the pull request

#### Scenario: Revisiting a branch finds the same address

- **WHEN** further commits are pushed to a pull request's branch
- **THEN** the same preview address serves the newer revision

#### Scenario: A preview does not disturb production

- **WHEN** a preview is published
- **THEN** the production origin continues to serve the default branch's revision

#### Scenario: Unrelated work does not deploy

- **WHEN** a commit that touches no file under the site directory is pushed to the default branch
- **THEN** no deployment of the site is produced

### Requirement: Nothing is published without passing the site's checks

The published revision MUST have passed the site's version-stamp check, and the
deploy MUST verify the deployed pages are actually served before reporting
success. The version-stamp check MUST also run on pull requests, because the
repository has already shipped a version string that disagreed with the version
file and no automated check caught it.

#### Scenario: Drifted version blocks publication

- **WHEN** a page carries a version string that disagrees with the repository's version file
- **THEN** neither a deploy nor a preview is produced

#### Scenario: Publication is confirmed against the origin, not the upload

- **WHEN** a deploy completes
- **THEN** the deployed pages have been requested and confirmed to serve the expected content

#### Scenario: A pull request runs the same check

- **WHEN** a pull request touches the site
- **THEN** the version-stamp check runs and its failure is visible on the pull request

#### Scenario: The browser-measured check is not claimed as a gate

- **WHEN** the deploy's quality gates are described
- **THEN** the checks that require a browser are named as separately run, and are not implied to have passed

### Requirement: The cache policy follows the site's unhashed asset names

Because the site ships no build step, its stylesheet, script, and font filenames
carry no content hash. The caching policy MUST therefore let a corrected
stylesheet or script reach a returning visitor, and MUST NOT mark such a file
immutable. Assets whose bytes are genuinely content-stable MAY be cached
long-term, and a generated artifact whose bytes change under a stable name MUST
either carry a content hash or be revalidated. Every served response MUST carry
an explicit caching policy rather
than relying on a platform default that could change.

#### Scenario: A corrected stylesheet is not served stale

- **WHEN** a stylesheet or script is changed and deployed
- **THEN** a returning visitor receives the new bytes without clearing their cache

#### Scenario: Content-stable assets are cached long-term

- **WHEN** a font, a generated image, a vendored player, or a hashed index is requested
- **THEN** the response permits long-lived caching

#### Scenario: A rebuilt index is not served stale

- **WHEN** a demo's vector index is rebuilt with different bytes
- **THEN** a returning visitor receives the new index, because the artifact either carries a content hash in its name or is revalidated

#### Scenario: The pages themselves revalidate

- **WHEN** a page is requested
- **THEN** it is revalidated rather than cached for a fixed period

### Requirement: An unknown path returns a not-found page in the site's own vocabulary

A request for a path the site does not serve SHALL return a not-found status
with a page built from the site's existing design atoms, offering a route back
to the landing and the documentation surface. It MUST NOT introduce a visual
primitive the site does not already use, and MUST NOT present a broken or
unstyled page.

#### Scenario: A wrong URL is answered in the site's world

- **WHEN** an unknown path is requested
- **THEN** the response carries a not-found status and a page composed from existing tokens, type roles, and rules

#### Scenario: The reader can get back

- **WHEN** the not-found page is rendered
- **THEN** it offers a working route to the landing page and to the documentation surface

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

- **WHEN** a change touches only files under the site directory
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

### Requirement: The origin answers crawlers at `/robots.txt`

The deployed origin SHALL answer a request for `/robots.txt` with HTTP `200`
and a `text/plain` body. The file MUST contain an explicit `User-agent` group
carrying at least one `Allow` or `Disallow` rule, so that a crawler reads a
stated policy instead of its own default, and it SHALL name the site's sitemap
by absolute URL.

The directives MUST be about this origin's own tree: every path a `Disallow`
rule names SHALL be a path this origin serves. A directive about a path that is
not this site's — a boilerplate exclusion carried over from another tree — is a
statement about some other origin, and MUST NOT be published, because a crawler
reading it learns nothing about what is here. A group with no rule at all is
not a policy either, which is why the rule is required rather than optional.

#### Scenario: A crawler asks for the directives

- **WHEN** any client fetches `/robots.txt` from the deployed origin
- **THEN** the response is HTTP `200` with a `text/plain` content type, not the site's not-found page and not an HTML body

#### Scenario: The policy is stated, not implied

- **WHEN** the served `robots.txt` is read
- **THEN** it carries a `User-agent` group with at least one `Allow` or `Disallow` rule, and its rules describe the tree the origin actually serves

#### Scenario: The directives name the sitemap

- **WHEN** the served `robots.txt` is read for its sitemap reference
- **THEN** the reference is an absolute URL on the origin the pages declare, and the path it names is one this origin serves

### Requirement: The origin publishes a sitemap of its served pages

The deployed origin SHALL answer a request for `/sitemap.xml` with HTTP `200`
and a valid XML body in the Sitemaps protocol's document shape. The document
SHALL carry exactly one `<loc>` per served page that is indexable, and every
`<loc>` MUST be the absolute canonical URL that the page itself declares, on
the origin the pages agree on. A page that declares itself not indexable MUST
NOT appear, and an address the page does not claim as its canonical MUST NOT be
substituted for it.

The sitemap MUST stay in step with the served tree: a page added, removed,
renamed, or re-canonicalized SHALL change the document in the same change, and
the site's own check MUST report a disagreement between the two rather than a
crawler being the party that finds it.

#### Scenario: A crawler reads the inventory

- **WHEN** the served `/sitemap.xml` is parsed as XML
- **THEN** each `<loc>` is an absolute URL on the origin the pages declare, and the set of `<loc>` values is exactly the canonical URLs of the served pages that are indexable

#### Scenario: A page that is not indexable is absent

- **WHEN** a served page declares itself not indexable
- **THEN** it has no `<loc>` in the sitemap, and its absence is not a gap the check reports

#### Scenario: A page joins or moves

- **WHEN** a page is added to the served set, or an existing page changes the canonical address it declares
- **THEN** the site's own check fails until the sitemap lists that address, so the sitemap cannot be delivered out of step with the pages

#### Scenario: The sitemap cannot invent an address

- **WHEN** a `<loc>` in the sitemap is compared with the page it names
- **THEN** the value is that page's own canonical URL rather than a path transcribed beside it, so the sitemap cannot disagree with the page about its own address

### Requirement: The discovery files are served, not merely present

`robots.txt` and `sitemap.xml` SHALL be members of the served set the site's
own check asserts, named in the repository rather than excluded by the ignore
list. Either file MUST NOT be dropped from the deployed origin by an exclusion
that was written for authoring material, and the check MUST report a file that
would ship without being named as an expected member of the served set.

#### Scenario: The discovery files reach a reader

- **WHEN** the site is deployed
- **THEN** `/robots.txt` and `/sitemap.xml` are answered from the site's files, and neither is absent from the served set

#### Scenario: An unnamed file is caught before publication

- **WHEN** either file is added to the site directory without being named in the served set
- **THEN** the site's own check reports it as a file that would ship unexpectedly, and publication does not depend on someone remembering it

### Requirement: Every repeated fact about a page is emitted from one descriptor

Each served page's canonical address, social-preview metadata, theme colour and
favicon SHALL be emitted into its `<head>` from one committed descriptor per
page, and the sitemap SHALL be emitted from that same descriptor, so a page
cannot state an address the inventory does not carry, or the inventory an
address no page declares. The descriptor MUST be the only place those values are
written, and the emitted fragment MUST sit inside the page's own committed
markup rather than being assembled at request time, because the tree has to
render for a reader with no tooling and for a crawler that does not run scripts.

#### Scenario: A page's address has one author

- **WHEN** a page's canonical address, title or description changes
- **THEN** the change is made in the descriptor, and after regeneration the page and the sitemap both carry it

#### Scenario: The metadata is in the response

- **WHEN** a served page's HTML is fetched without running scripts
- **THEN** its canonical, social-preview, theme-colour and favicon metadata are present in the markup

#### Scenario: Drift is reported rather than published

- **WHEN** a committed page or the sitemap disagrees with the descriptor
- **THEN** the site's own check fails and names the file, before any publication

#### Scenario: A page cannot join the site without a descriptor

- **WHEN** a page is added to the served set
- **THEN** it appears in the sitemap only through a descriptor entry, and the checks fail until it has one

### Requirement: The palette and the contrast floor are one checked source

The site's colour tokens SHALL be emitted from the same descriptor as its pages,
and every text and ground pair the site ships MUST meet WCAG AA contrast on its
own ground: 4.5:1 for body text, 3:1 for large text. The check MUST report a
pair below the floor rather than the value reaching a reader.

#### Scenario: A token change is measured, not assumed

- **WHEN** a colour token changes
- **THEN** contrast is recomputed for every text and ground pair in the same check, and a pair below AA fails it

#### Scenario: The theme colour follows the ground

- **WHEN** the paper ground changes
- **THEN** the theme colour and the favicon emitted into every page carry the new ground, because both are emitted from the tokens rather than typed per page
