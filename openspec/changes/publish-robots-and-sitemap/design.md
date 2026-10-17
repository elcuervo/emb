# Design

## Context

- `website/` is both the working tree and the publish source: `wrangler.jsonc`
  deploys it as an assets-only Worker at `emb.is`, and `.assetsignore` is the
  only boundary between an authoring file and a public URL.
- `website/tools/published-tree.py` already owns the three facts this change
  needs: the served set (asserted exhaustively, both directions), the canonical
  origin (parsed from the pages' own metadata, never pinned to production), and
  the cache rules. `ci.yml`'s `site` job and `site.yml`'s `checks` job both run
  it before anything is published.
- Both paths answer `404` today, with `404.html` — `text/html`, and `noindex`.
- Every served page but `404.html` declares an absolute canonical on
  `https://emb.is`: 19 values, one origin, no duplicates.
- There is no build step, and the platform infers a content type from the
  extension when it knows one. `_headers` documents the single case where it
  does not (`.cast`, served as `text/plain` by declaration).

## Goals / Non-Goals

**Goals:**

- A crawler or agent gets a stated policy and a complete inventory, both true of
  the tree that ships.
- Drift between either file and the pages fails `just website-published` — and
  therefore CI — rather than reaching a reader.
- No new tool, no new dependency, no build step, no change to what the origin
  already serves, and nothing outside `website/`.

**Non-Goals:**

- The scan's other failing checks — `Link` headers, `llms.txt`, markdown for
  agents, A2A/MCP discovery. Each is a capability of its own; this change
  deliberately closes two.
- Cloudflare's dashboard-managed `robots.txt` (AI Crawl Control / Bot
  Preference Sync) and any `Content-Signal` line (see D1).
- `lastmod`, `changefreq`, and `priority` in the sitemap (see D2).
- Automating a post-deploy origin confirmation in `site.yml` (see Risks).

## Decisions

**D1 — The policy is an explicit wildcard allow, and nothing else.**
`website/robots.txt` carries one `User-agent: *` group with `Allow: /`, the
`Sitemap` reference, and a comment saying why no `Disallow` is published: the
origin serves five page surfaces and their assets, the sandbox is already
excluded at the boundary, and there is no path here worth withholding. This
satisfies the requirement that a group be explicit rather than defaulted, and it
is the shape the readiness scan passes its AI-rules check on — measured against
a real site: "No AI-specific bot rules; wildcard rules apply to all crawlers
including AI bots", `ruleSource: wildcard`.

Alternatives considered: `Disallow: /assets/` to spare crawlers the ~850 KiB
cast and the media plates — rejected, it costs image and media indexing for no
policy gain; per-bot AI groups and/or `Content-Signal: search=yes,
ai-train=no` — rejected for this change, because `emb` is MIT, the repository is
crawlable, and the marketing bytes are public, so a training block here asserts
a stance the project does not otherwise take, while handing a crawler the same
directive from two sources if Cloudflare's managed file is later enabled. That
stance is a spec-level change of its own, not a line to slip in here.

**D2 — The sitemap is hand-written, minimal, and derived from the pages.** 19
`<url><loc>` values, each the absolute canonical the page itself already
declares; `404.html` absent because it is `noindex` and declares none. No
`lastmod`: the tree has no truthful modification time (no build step; file
mtimes are not in git) and the protocol makes the element optional, so inventing
one would be the kind of unbacked claim this repository's site rules exist to
prevent. No `changefreq`/`priority`: search engines ignore both.

Alternative considered: generating the file at deploy time — rejected. There is
no build step to hook, and a generated file whose truth lives in a generator is
a file the served-set check cannot reason about.

**D3 — The assertions belong in `published-tree.py`, and their URL source is the
served tree, not `PAGES`.** Three additions:

1. the sitemap's `<loc>` set equals the canonical set of the served pages,
   computed by reading every served `.html` and excluding the ones that declare
   `noindex` — so `404.html` is excluded by its own markup rather than by a list;
2. every `<loc>` sits on the single origin the pages agree on (the same origin
   the file already extracts and reports);
3. `robots.txt`'s `Sitemap:` URL names that origin and a path this tree serves,
   and the file states a complete policy — a `User-agent` group carrying a rule
   — whose `Disallow` rules name paths this origin actually has.

Reading the URLs from the served tree is the point: `PAGES` holds 17 entries and
omits `scripting/index.html` and `gem/index.html`, both served and both carrying
canonicals, so any check that derived the sitemap from `PAGES` would bless a
document missing two pages. Deriving from the served tree makes that class of
omission impossible: a page cannot be served without a `<loc>`, and a `<loc>`
cannot name a page the check does not see.

Alternatives considered: a `website/tools/sitemap.py` that writes the document
with a `--check` counterpart (the `stamp-*.py` idiom) — rejected, it would be a
second tool with a second idea of what ships, and two more CI commands in two
workflows for a 19-line file that rarely changes; asserting only that each
`<loc>` resolves to a served file — rejected, that cannot catch a missing page,
which is the drift worth catching.

**D4 — Neither file gets a `_headers` rule.** `.txt` and `.xml` are extensions
the platform knows, so the types are inferred the same way `assets/css/*` is not
declared either; and `/*` already applies `public, max-age=0, must-revalidate`,
which is the correct policy for a file edited in place. `UNHASHED_SUFFIXES`
stays as it is, because neither file is pinned immutable anywhere — extending it
would widen what the cache check must guard for no reason.

Alternative considered: declaring `Content-Type` for both, following the `.cast`
precedent — rejected, that precedent exists only because the platform does not
know the extension, and a declaration here would be a rule that must stay true
for no observable gain.

**D5 — `website/README.md`'s "What ships" gains the two files and the new
assertions.** The section is the description of the boundary and the check, and
it already names the checks that exist; leaving the two discovery files out of
it would make the documentation less true than the check.

## Risks / Trade-offs

| Risk | Mitigation |
|---|---|
| The check reads the tree, so anything Cloudflare injects ahead of a served file is invisible to it — a managed `robots.txt` would prepend to ours | Bot Preference Sync stays off; the post-deploy probe and rescan are part of this change's tasks, and the blindness is written down in the proposal |
| A hand-written sitemap is a second place that names addresses | The equality assertion (D3.1) makes a page's canonical the only place an address can be introduced; the sitemap follows or the check fails |
| The tree cannot state its own content type, so `text/plain` is a platform behaviour rather than a checked fact | Confirmed live after deploy (`curl -sI`); a future `_headers` or platform change that broke it would be seen there, not in CI |
| Two files that must stay in step with 19 pages | Same assertion; adding a page already requires a `SERVED` entry, which is exactly where the omission would be noticed |
| The readiness scan's verdict is a third party's reading of RFC 9309 | The requirements stand on the RFC and the protocol, not on the scan; the scan is used as a post-deploy observation, not as the specification |

## Migration Plan

1. Land in one commit: `website/robots.txt`, `website/sitemap.xml`, the
   `SERVED` entries and the three assertions in
   `website/tools/published-tree.py`, and the `website/README.md` update. There
   is nothing to sequence — the check and the files it asserts are the same
   commit by construction.
2. Verify locally: `just website-published` exits 0, and `just website` answers
   `/robots.txt` as `text/plain` and `/sitemap.xml` as XML from a plain static
   server over the folder.
3. Merge to `main`. `site.yml` runs the checks and publishes; the two paths
   become live in that deployment, with no configuration change and no cache
   hazard (both files are served `must-revalidate`).
4. Confirm against the origin: `curl -sI https://emb.is/robots.txt` (200,
   `text/plain`), the sitemap parsed as XML from the live URL, and a rescan
   reporting `discoverability.robotsTxt` and `discoverability.sitemap` at
   `pass`.
5. Rollback: revert the commit. The previous revision answers `404` for both
   paths, which is their state today; nothing else unwinds, and no other surface
   depends on either file.

## Open Questions

- Whether `site.yml`'s deploy job should confirm the two paths against the
  origin automatically. It does not confirm any path today, and this change
  verifies by hand; automating it is a change of its own and would cover more
  than these two files.
- Whether the sitemap should eventually carry `lastmod` from git. Nothing needs
  it yet, and it would bring a stamp tool and its `--check` with it.
