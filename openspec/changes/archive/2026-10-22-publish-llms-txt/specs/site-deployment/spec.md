## ADDED Requirements

### Requirement: The origin answers agents at `/llms.txt`

The deployed origin SHALL answer a request for `/llms.txt` with HTTP `200` and a
`text/plain` Markdown body — not the site's not-found page and not an HTML body.
The body SHALL be an index in the `llms.txt` shape: one `# emb` heading, one
blockquote summary, and `##` sections of link lists whose entries are
`[name](url): note`.

#### Scenario: An agent asks for the index

- **WHEN** any client fetches `/llms.txt` from the deployed origin
- **THEN** the response is HTTP `200` with a `text/plain` content type and a Markdown body that opens with a single `# emb` heading and a blockquote summary

### Requirement: The index links the surfaces an implementer needs

The index SHALL name, at absolute URLs, the documentation, the scripting
reference and the client surface on this origin, plus the readable Markdown
sources of the same material. Every link MUST be absolute, and every on-origin
link SHALL name a path this origin serves.

#### Scenario: The index's on-origin links resolve

- **WHEN** the `llms.txt` body is read for its links to this origin
- **THEN** each one is an absolute URL naming a path the origin serves, so following it does not reach the not-found page

#### Scenario: The readable sources are named

- **WHEN** the `llms.txt` body is read for the sources an agent would fetch
- **THEN** it names the Markdown files that carry the same material, including the ones this origin does not serve

### Requirement: The index states the wire facts a reader needs first

The blockquote summary SHALL state the default reply shape, how RESP3 is
selected, and that the reply's shape follows the number of texts sent.

#### Scenario: A reader knows what the reply looks like before fetching

- **WHEN** the blockquote summary is read
- **THEN** it says which reply shape is the default, how RESP3 is selected, and that the reply's shape follows the number of texts sent

### Requirement: The index is a map, not a copy of the reference

Command grammar, configuration keys and the scripting host API SHALL remain in
the pages the index links, and the file MUST NOT restate them.

#### Scenario: The reference changes

- **WHEN** a command, a configuration key or a host-API entry changes
- **THEN** the index needs no edit, because it links the page that owns the change rather than restating it

### Requirement: The index is a member of the asserted served set

`llms.txt` SHALL be named in the served set the site's own check asserts, so a
file added to the site directory without being named is reported as a file that
would ship unexpectedly. The index is a discovery file rather than a page, so it
declares no canonical and MUST NOT appear in the sitemap; the sitemap
requirement — one `<loc>` per indexable served page — is unchanged by its
presence.

#### Scenario: An unnamed file is caught before publication

- **WHEN** `llms.txt` is added to the site directory without being named in the served set
- **THEN** the site's check reports it as a file that would ship unexpectedly, and publication does not depend on someone remembering it

#### Scenario: A discovery file is not a page

- **WHEN** the served sitemap is compared with the served pages
- **THEN** `llms.txt` has no `<loc>` and its absence is not reported as a gap
