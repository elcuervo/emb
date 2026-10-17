## ADDED Requirements

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
