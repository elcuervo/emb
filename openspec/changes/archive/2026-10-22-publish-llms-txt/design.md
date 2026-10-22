# Design

## Context

See `proposal.md` — Why. The constraints that shape the approach:

- `website/` is both the working tree and the publish source; there is no build
  step, and `.assetsignore` is the only boundary between an authoring file and a
  public URL.
- The reference material an implementer needs exists twice: as HTML on this
  origin (`/docs/`, `/scripting/`, `/gem/`, `/demos/`) and as Markdown in
  `docs/*.md` and `README.md`, which live **outside** `website/` and are
  therefore never served.
- `published-tree.py` asserts the served set, the cache rules, the internal
  links and the takes' agreement; it is run by both `ci.yml` and `site.yml`, so
  an unnamed file in `website/` fails CI rather than shipping.

## Goals / Non-Goals

**Goals:**

- One fetch tells an agent what emb is, which pages answer which question, and
  how to get the server running.
- The file is data, not a fourth copy of the protocol reference.
- Publication and CI treat it as a first-class member of the tree.

**Non-Goals:**

- Serving the Markdown sources themselves (they are the repository's, versioned
  with it; the index links to them).
- An `llms-full.txt` or a generator that concatenates the Markdown into the
  tree: another artifact to regenerate, and another `--check` to keep honest.
- Changing the pages, the sitemap, `robots.txt`, `_headers`, or any server,
  protocol or gem interface.

## Decisions

**A thin, hand-written index in the `llms.txt` shape.** The alternatives were an
index that inlines the wire contract (~120 lines of grammar restated on the
site) and an index plus a generated `llms-full.txt` built from `docs/*.md`. Both
duplicate material that already has an owner, and the site has no build step to
detect drift. The thin index keeps one owner per fact; its only prose is the
blockquote summary, which states the three facts a client implementer must know
before choosing what to fetch (default reply shape, `HELLO 3`, reply shape by
text count) and nothing more.

**On-origin pages first, Markdown sources second, install third.** The pages are
versioned with the site and are where a reader with a browser should land; the
Markdown is what an agent should actually read, and it is named as such
("tracking the repository's `main` branch"), which is also the honest statement
of its refresh cadence relative to the stamped site version.

**Membership in `SERVED`, and nothing else in the deployment config.** `.txt` is
a known type, so the platform serves it `text/plain` without a `_headers` rule;
`robots.txt` already says `Allow: /`; and the sitemap requirement counts served
pages, so the index is correctly absent from it. Adding rules for any of the
three would declare behaviour the origin already has.

## Risks / Trade-offs

- **Link staleness.** The Markdown links point at `main`, so between a release
  and the next commit they can name documentation slightly ahead of the published
  binary; the on-origin pages remain the release-true copy. Accepted: the
  alternative is pinning every link to a tag, which goes stale in the other
  direction.
- **No automated link check.** The served-set gate proves the file ships, not
  that its targets still resolve. All fourteen targets were fetched when the
  file was written; a future broken external URL would be found by a reader.
  Accepted for a five-link-per-section index, and recorded here rather than
  hidden.
- **Scope creep toward prose.** The file is one edit away from being a second
  reference; the spec says it must not restate the grammar, so the guard is the
  contract rather than a check.
