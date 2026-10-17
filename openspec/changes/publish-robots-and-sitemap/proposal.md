# Proposal

## Why

`https://emb.is/robots.txt` answers `404` — and, worse, answers it with the
site's own HTML 404 page (`content-type: text/html`, 5292 bytes), so the
response is a soft 404 rather than an absent file. `https://emb.is/sitemap.xml`
answers the same way. Crawlers and AI agents therefore get neither a directive
nor an inventory: the readiness scan fails `discoverability.robotsTxt`
("robots.txt not found") and `discoverability.sitemap` ("sitemap.xml not
found"), and two further checks — `botAccessControl.robotsTxtAiRules` and
`botAccessControl.contentSignals` — cannot even be evaluated ("Cannot check AI
rules without robots.txt"). Both are named requirements of the scan's next
level, "Basic Web Presence".

Nothing about the site's exposure is in question: every page already declares an
absolute canonical URL, every byte the origin serves is public, and the sandbox
under `website/repl/` is already excluded at the boundary. What is missing is
two root files and the check that keeps them true.

## What Changes

- **`website/robots.txt` is served** — an explicit `User-agent: *` group with
  `Allow: /`, and `Sitemap: https://emb.is/sitemap.xml`. It ships as
  `text/plain` with `200`: the platform infers the type from the extension, and
  `_headers` needs no line for it — `.txt` is not a name whose bytes change
  under it, so the existing `/*` rule (`public, max-age=0, must-revalidate`) is
  already the correct policy for a file that is edited in place.
- **`website/sitemap.xml` is served** — one `<url><loc>` per indexable served
  page: the 19 absolute canonical URLs those pages already declare (`/`,
  `/docs/`, `/scripting/`, `/gem/`, `/demos/`, and the fourteen `demos/*.html`).
  `404.html` is deliberately absent — it carries `noindex` and declares no
  canonical. No `lastmod`, `changefreq`, or `priority`: no build step here could
  state a modification time truthfully, and the protocol makes those elements
  optional.
- **Both files join `published-tree.py`'s `SERVED` set** — the served set is
  exhaustive, so a file on disk that no `.assetsignore` line excludes and no
  `SERVED` entry names fails the gate with "unexpected: … (would ship, not in
  the served set)". That is the mechanism that lets them ship, and it is why
  neither file is added to `.assetsignore`.
- **The site's own check keeps the two files honest.** `published-tree.py`
  grows three assertions over the served tree: the sitemap's `<loc>` set is
  exactly the canonical set of the served pages (each `loc` is a canonical of a
  served page, every indexable served page has one, a `noindex` page has none),
  every `loc` sits on the single origin the pages already agree on, and
  robots.txt's `Sitemap:` URL names that same origin and a path this tree
  serves. The set is derived from the served tree, **not** from the check's own
  `PAGES` tuple — that tuple holds 17 entries and omits `scripting/index.html`
  and `gem/index.html`, which are served and do declare canonicals, so deriving
  from it would publish a sitemap that silently omits two pages.
- **The served set is documented** — `website/README.md`'s "What ships" section
  names the two new files and the assertions that cover them, beside the
  existing sets table.
- **Not included, deliberately: per-bot AI rules, a `Content-Signal` line, and
  Cloudflare's dashboard-managed `robots.txt`.** `emb` is MIT, the repository
  itself is crawlable, and every byte this origin serves is public, so the
  honest policy is a wildcard allow — which is also the shape the scan passes
  `botAccessControl.robotsTxtAiRules` on ("No AI-specific bot rules; wildcard
  rules apply to all crawlers including AI bots"). Cloudflare's managed file
  (AI Crawl Control / Bot Preference Sync) prepends to an origin `robots.txt`
  rather than replacing it, so it can be turned on later without editing this
  file; authoring AI groups here *and* enabling that would give one crawler two
  directives from two sources.
- No **BREAKING** changes. No page is edited, no `.assetsignore` line, no
  `_headers` rule, no `wrangler.jsonc` setting, and no build step is introduced.

## Capabilities

### New Capabilities

None. These are two more files the existing deployment surface serves, not a new
system behavior; the requirements they must satisfy belong to the capability
that already owns what leaves this repository and what a reader can fetch.

### Modified Capabilities

- `site-deployment`: the origin additionally serves `robots.txt` (as
  `text/plain`, `200`, carrying an explicit `User-agent` group with allow
  rules and naming the sitemap) and `sitemap.xml` (naming the canonical URL of
  every indexable served page). Both are members of the asserted served set, and
  the site's own check — which today asserts the served set, the canonical
  origin, and the cache rules — also asserts that the sitemap lists exactly the
  served pages' canonicals and that robots.txt points at it, on the one origin
  the pages agree on.

## Impact

- **Files**: `website/robots.txt` and `website/sitemap.xml` (new);
  `website/tools/published-tree.py` (`SERVED` plus the three assertions);
  `website/README.md`'s "What ships" section.
- **Gates**: `just website-published` locally; `ci.yml`'s `site` job and
  `site.yml`'s `checks` job run the same script, so a missing entry, a stale
  `loc`, or a robots.txt pointing at a path that does not exist fails before
  anything is published. No Go, gem, or benchmark surface is touched.
- **Delivery**: static files only. They are live once `site.yml` publishes a
  push to `main` that touches `website/**`; the Worker needs no configuration
  change, and `/robots.txt` and `/sitemap.xml` are not HTML paths, so
  `html_handling` does not apply to them.
- **Verification**: locally, `just website` (a plain static server over
  `website/`) already answers `/robots.txt` as `text/plain` and
  `/sitemap.xml` as XML, and `just website-published` must exit 0. After a
  deploy, `curl -sI https://emb.is/robots.txt` must return `200` with
  `text/plain`, `curl -sI https://emb.is/sitemap.xml` must return `200`, and a
  rescan must report `checks.discoverability.robotsTxt.status == "pass"` and
  `checks.discoverability.sitemap.status == "pass"`.
- **Risk**: the check reads the tree, not the response. Anything Cloudflare
  injects ahead of a served file — the managed `robots.txt` above all — is
  invisible to it, which is why the rescan is part of this change's verification
  rather than part of its gate.
