# Tasks

## 1. The directives file

- [x] 1.1 Write `website/robots.txt`: one explicit `User-agent: *` group with
  `Allow: /`, a comment stating why no `Disallow` is published (the origin serves
  five page surfaces and their assets; the sandbox is already excluded at the
  boundary), and `Sitemap: https://emb.is/sitemap.xml`. Verify with
  `just website` in one shell and `curl -sI http://localhost:8080/robots.txt`:
  `200` with a `text/plain` content type, and the body carrying the group and the
  sitemap reference.

## 2. The sitemap

- [x] 2.1 Write `website/sitemap.xml` in the Sitemaps document shape
  (`<urlset>` with the protocol's namespace), one `<url><loc>` per indexable
  served page, each value **copied from that page's own `rel="canonical"`**
  rather than retyped — `/`, `/docs/`, `/scripting/`, `/gem/`, `/demos/`, and the
  fourteen `demos/*.html` — with `404.html` absent because it carries `noindex`.
  Verify: parse it as XML (`python3 -c "import xml.dom.minidom as m;
  m.parse('website/sitemap.xml')"`) and confirm the file holds 19 `<loc>`
  elements.
- [x] 2.2 Confirm the document names addresses and only addresses the pages
  claim: extract every served page's canonical into a sorted list, extract the
  sitemap's `<loc>` values into another, and `diff` them — the two must be
  identical, which also proves no `404.html` entry and no `.html`-less demos
  path crept in.

## 3. The served set and the site's check

- [x] 3.1 Add `robots.txt` and `sitemap.xml` to `SERVED` in
  `website/tools/published-tree.py`. Verify with `just website-published`: exit
  0, the two files counted in the served total, and neither reported as
  `unexpected:` — before the change the same command reports them as files that
  would ship unnamed.
- [x] 3.2 Assert in `published-tree.py` that the sitemap's `<loc>` set equals the
  canonical set of the served pages, computed from the served tree (every served
  `.html` minus the ones that declare `noindex`) and **not** from the `PAGES`
  tuple, which holds 17 entries and omits `scripting/index.html` and
  `gem/index.html`. Verify by planting two regressions and confirming a
  named failure each time, then reverting both: delete one `<loc>` (the check
  names the page missing from the sitemap), and point one `<loc>` at a path no
  page claims.
- [x] 3.3 Assert in `published-tree.py` that every `<loc>` is absolute on the one
  origin the pages agree on, and that `robots.txt`'s `Sitemap:` URL names that
  origin and a path this tree serves. Verify by planting two regressions and
  confirming a named failure each time, then reverting both: change the
  `Sitemap:` line to another host, and change one `<loc>` to a `http://` value.

## 4. Documentation

- [x] 4.1 Update `website/README.md`'s "What ships" section: name the two files
  in the served set and add the new assertions to the prose that describes the
  check. Verify by reading the section against `published-tree.py` — every check
  it claims is one the script performs — and re-running `just website-published`
  to confirm exit 0.

## 5. Local end-to-end

- [x] 5.1 Exercise the served tree as a reader would: with `just website`
  running, fetch `/robots.txt` (200, `text/plain`, the group present) and
  `/sitemap.xml` (200, XML parse clean, 19 `<loc>`), and fetch a path that does
  not exist to confirm the not-found page still answers it — the two new files
  must not have taken over unknown paths.

## 6. Publication and confirmation against the origin

- [ ] 6.1 Publish: merge to `main` so `site.yml` runs its `checks` job (which
  includes `published-tree.py`) and then deploys. Verify the workflow run is
  green and the deploy step reports a published version.
- [ ] 6.2 Confirm the live origin rather than the upload:
  `curl -sI https://emb.is/robots.txt` returns `200` with `text/plain`,
  `curl -sI https://emb.is/sitemap.xml` returns `200` with an XML content type,
  and a rescan (`POST https://isitagentready.com/api/scan` with
  `{"url": "https://emb.is"}`) reports
  `checks.discoverability.robotsTxt.status == "pass"` and
  `checks.discoverability.sitemap.status == "pass"`.

## Workflow follow-up

- After task 6.2 confirms the deployed origin, archive the change
  (`/openspec-archive-change`), which syncs the `site-deployment` delta into
  `openspec/specs/site-deployment/spec.md` and moves the change under
  `openspec/changes/archive/`.
- The scan's remaining failures — `Link` headers, `llms.txt`, markdown for
  agents, A2A/MCP discovery — are capabilities of their own and are deliberately
  not part of this change.
