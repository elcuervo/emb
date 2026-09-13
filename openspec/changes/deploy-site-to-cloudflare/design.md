## Context

See `proposal.md — Why`. The relevant current state:

- `website/` holds 23 files, ~2.9 MB of which is authoring material against
  ~1.2 MB of site. There is no build step, no dependency, and no fingerprinting;
  the site is a directory that `python3 -m http.server` renders as well as a CDN
  would. `CORRECTIONS.md` (1,762 lines) was removed in `2397754`, so the
  authoring material is now the brief, the README, the `.impeccable/` notes, six
  `tools/` files, and three image sources.
- `website/index.html` and `website/docs/index.html` are the two surfaces;
  `product-site` and `product-docs` already fix their paths, so **the directory
  `website/` must keep its name and both files must stay where they are.**
- `website/tools/stamp-version.py` rewrites every `data-emb-version` element
  from the repo `VERSION` and has a `--check` drift mode. It resolves the repo
  root as `Path(__file__).resolve().parents[2]`.
- `website/.impeccable/surfaces/*.md` are the design-notes pair for the two
  surfaces. They sit inside the publish root and are a dot-directory, which is
  the shape most likely to survive review unnoticed.
- `website/tools/ink-probe.html` loads `index.html` in a **same-origin iframe**
  at 24 widths. Same-origin is not incidental — the probe reads geometry out of
  the framed document. It is served by `just website`, which serves `website/`
  at `/`.
- The authoring material and the shipped material are the same directory today,
  and `website/README.md`'s file tree presents them as one list. Nothing
  distinguishes them except the reader's judgement.
- `.github/workflows/ci.yml` runs lint, tests, and a Rails suite. No job touches
  the site, so `--check` has never run automatically.
- `emb.is` is configured on Cloudflare. It is the apex of a single zone.

## Goals / Non-Goals

**Goals:**

- One folder that holds the whole site — pages, generators, brief, critique log,
  and image sources — so working on it stays a matter of editing files in place.
- A declared exclusion between that folder and the origin, checked rather than
  remembered, so shipping stays a property of one small reviewed file.
- A production origin that cannot lag `main`, and a preview origin a reviewer
  can open before merging.
- The version-stamp drift guard running on every change, not only on deploys.
- Head metadata that is correct on the open web, without adding a build step.
- A caching policy that is safe *because* the site has no content hashes.

**Non-Goals:**

- Splitting the site into a working tree and a publish source, or introducing any
  staging, copy, or build step. `website/` is edited and `website/` is published;
  the only transformation between them is the ignore list.
- Any change to the poster's composition, tokens, type, console, copy, or
  claims. This is plumbing plus five `<head>` tags plus one small page.
- Asset fingerprinting or a bundler. The absence of a build step is a spec-level
  property of the docs surface (`product-docs`) and is preserved deliberately.
- Hosting anything dynamic. The console stays a transcript client; no RESP
  endpoint is introduced, per `product-site`.
- Migrating state. There is none.

## Decisions

### D1 — Cloudflare Workers Static Assets, not Pages

Pages stopped taking new features in April 2025; Workers Static Assets is where
the routing, header, and ignore capabilities this change needs are maintained.
**Static-asset requests are free and unlimited on Workers** — they do not consume
the 100k/day Worker-request allowance — so the expected cost objection does not
apply; there is none. The Worker is assets-only and carries no script.

*Alternatives:* **Pages** — zero-config with dashboard Git integration and the
same free tier. Rejected because it is frozen and its ignore/header story is the
older one. **Cloudflare Workers Builds** (CF-side CI) — rejected under D5.
**GitHub Pages** — rejected: no `.assetsignore`, no per-branch preview aliases,
and it would put the docs under `github.io` while the apex domain is already here.

### D2 — The configuration lives in the repository, not the dashboard

Everything — worker name, asset directory, trailing-slash handling, not-found
handling, custom domain, preview URLs — is declared in a checked-in
`wrangler.jsonc` and applied by `wrangler deploy`. A dashboard-configured
project drifts from the repository invisibly, and this repository's whole
review culture is "the claim and the artifact travel together".

`not_found_handling` is set to serve `website/404.html`, and `html_handling`
is left at `auto-trailing-slash` so `/docs` and `/docs/` both resolve to the
documentation surface, which is what the landing's `docs/index.html#…` links
and a human's typed URL both need.

### D3 — One `website/` folder, with `.assetsignore` as the only boundary

Two shapes were available:

| | one folder + ignore list | split working tree from publish source |
|---|---|---|
| Folders to work in | one | two |
| Build step | none — the tree **is** the output | needs a copy or build step |
| Local render | `just website` unchanged, site at `/` | server root moves, site at `/website/` |
| Ink probe | stays same-origin, no change | forced to follow, or the root moves |
| Tool paths | unchanged | every invocation and docstring repointed |
| Leak prevention | one reviewed list, plus a check | files are not there to leak |

Working in one folder is worth more here than a structural boundary, and the
site's most valuable property — **the served bytes are the authored bytes**, a
git diff away from review — survives intact. So the authoring material stays put
and a checked-in `website/.assetsignore` (gitignore syntax, exact paths and
directory entries, no negation) names what the origin must never serve:

```
# Authoring material — present in the folder, never served
/.impeccable/
/tools/
/PRODUCT.md
/README.md
/assets/img/terrain-v2.png
/assets/img/terrain-v2.md

# The ignore file itself
/.assetsignore
```

Note what is deliberately **absent** from that list: `website/404.html` and
`website/_headers` are configuration that *does* ship, and `website/assets/img/`
also contains `terrain-matte.png`, `og.png`, and `speckle.svg`, which do. A
reader skimming for "things that are not pages" would guess wrong on all five,
which is why the list is written as an explicit allow/deny pair in
`published-tree.py` rather than as a deny glob — `/assets/img/*` looks tidier and
would silently drop the matte, turning a leak into a broken page. The excluded
set also includes `.impeccable/`, a dot-directory, which is exactly the kind of
path that survives a visual scan of a folder listing.

**Why the probe staying inside `website/` is now a feature, not an exception.**
`.assetsignore` is applied by the deploy tooling, not by the local server. So
`just website` still serves `website/` at `/`, and `tools/ink-probe.html` still
answers at `/tools/ink-probe.html` — same-origin with `index.html`, which is
what lets it read the framed page's geometry — while contributing nothing to the
published tree. Measuring the site and publishing the site therefore consume the
same folder at different levels of strictness, which is exactly the property the
split design had to buy with a second directory and a second server root.

**The cost, stated plainly: the ignore list is now the only boundary.** In the
split design a forgotten `CORRECTIONS.md` would have been in the wrong directory
and therefore not published. Here it publishes unless the list says otherwise.
That is why `published-tree.py` exists as a first-class check rather than a
convenience — it walks `website/`, applies the ignore list, and asserts the
result equals an expected set declared in the same file, so an added file and a
forgotten ignore entry fail the same check. The matcher is deliberately small
(exact paths, directory entries, simple globs; no nested negation) so it agrees
with gitignore semantics on this list without reimplementing gitignore; the list
is kept to the shapes that matcher handles faithfully.

*Alternatives:* a sibling `website-dev/` workbench (rejected — two folders to
work in for no behavioral gain); a build step copying into `dist/` (rejected —
against `product-docs`, and it makes the served bytes unreviewable as a diff);
relying on discipline (rejected — that is the current state, and it would publish
~2.9 MB of authoring material).

### D4 — `main` deploys; a pull request gets an aliased preview

```
push to main        →  wrangler deploy
                       → https://emb.is

pull_request        →  wrangler versions upload --preview-alias pr-<number>
                       → https://pr-<number>-emb-site.<subdomain>.workers.dev
                       → posted as a comment on the PR
```

`versions upload` creates a version with preview URLs without touching the
production deployment, so a preview cannot disturb `emb.is`. The **alias** is
what satisfies "revisiting a branch finds the same address": a versioned URL is
unique per upload, whereas an alias is repointed by each new upload, so
`pr-123-…` is stable across commits to that branch while always serving the
newest one.

Both jobs filter on `paths: [website/**, VERSION, .github/workflows/site.yml,
wrangler.jsonc]` so unrelated work — the Go server, the gems — deploys nothing.
`concurrency` cancels superseded runs per ref. A `workflow_dispatch` trigger is
added so a deploy can be re-run by hand without an empty commit.

*Alternatives:* preview on every branch push (noisy, and branches are not what
reviewers read). Versioned URL only (moves on every commit, so a reviewer's
link dies). A separate `preview` environment with its own worker (doubles the
resource to configure, and sharing one worker is what makes the alias cheap).

### D5 — GitHub Actions with `cloudflare/wrangler-action`, not CF-side CI

CF's own Git integration would build and deploy without any secret in GitHub,
but it offers **no place to run the gates**: the version-stamp check is a
Python invocation, and the ink probe needs a served site plus a browser. Those
are the checks this repository has already been burned for skipping. Running the
deploy from Actions puts the gate and the publication in one job, in the same
place as every other gate.

The cost is two repository secrets (`CLOUDFLARE_API_TOKEN`, `CLOUDFLARE_ACCOUNT_ID`)
and that **previews are unavailable for pull requests from forks**, since fork
runs do not receive secrets. That is acceptable for a repository whose
contributors push branches; the alternative, `pull_request_target` with a
checkout of unreviewed code, is a privilege-escalation footgun and is not used.

### D6 — The canonical origin is a literal, and a check keeps the literals honest

Each page carries a literal absolute canonical link, `og:url`, and `og:image`, in
the markup. A variable, a computed value, or a template-injected placeholder
would all need code to run before the address exists — which stacks against
`product-docs`' "renders completely over `file://`" and against the no-build-step
rule. Metadata values are inert: an absolute `og:image` is a string a scraper
reads, not a resource the page fetches, so the `file://` scenario still holds.

**The value is not pinned, and the checks do not pin it.** One tree is served
from `localhost:8080`, from the Worker's `workers.dev` address, from per-branch
preview aliases and from the production origin. A check that demanded
`https://emb.is` in the markup would fail on every one of those except the last,
which turns the working loop into a special case of production and invites
someone to disable the check to get their afternoon back. What is asserted
instead is the shape that actually matters: every address is absolute — a
relative `og:image` is the real defect, and it is what shipped — and every page
agrees on a single origin, so a pasted link cannot resolve two ways. The origin
the pages declare is reported, and compared against `wrangler.jsonc`'s route
when one is configured. A disagreement there is printed as a warning, because it
is a deployment fact worth seeing, not a reason to block a commit.

Because literals can drift, the check still fails on a page that loses its
canonical, on a relative social image, and on two pages disagreeing about the
origin. The `emb.is` value lives in the markup and in `wrangler.jsonc` behind a
comment; nothing else restates it.

### D7 — Caching: revalidate the unhashed, pin the immutable

Workers static assets default every response to
`Cache-Control: public, max-age=0, must-revalidate`. That default is **correct**
for the HTML, the stylesheet, and the script, whose filenames never change and
whose bytes do. `website/_headers` overrides it only where the platform default
is leaving value on the table:

```
/assets/fonts/*            Cache-Control: public, max-age=31536000, immutable
/assets/img/*              Cache-Control: public, max-age=31536000, immutable
/assets/css/styles.css     (platform default — revalidate)
/assets/js/main.js         (platform default — revalidate)
```

The two cached globs cover three WOFF2 subsets, the terrain matte, the social
card, and the speckle; the fonts are versioned subsets and the images are
generated artifacts whose names are bumped when they change. Nothing in the
cached set is expected to change under a stable name — and if one does, the
guest is a stale image, not a stale stylesheet. Writing the policy explicitly
also means the site does not inherit a change in platform defaults silently.

*Alternative:* add content hashes at deploy time. Rejected — it reintroduces the
build step and rewrites references in HTML and in the script's preload list to
save roughly 200 KB on a returning visit.

### D8 — The 404 is the poster's page, built from existing atoms

`not_found_handling: "404-page"` serves `website/404.html`. It is composed from
tokens, the type ladder, the hairline rule, and the annotation form that already
exist in `assets/css/styles.css` — no new primitive, per `product-site`'s
composition rule — and it links back to the landing and to `/docs`.

*Alternative:* accept the platform's default not-found response. Rejected: an
unstyled default is exactly the "broken page" state the site's own specs refuse
elsewhere, and it would be the only page on the origin outside the site's world.

## Risks / Trade-offs

- **The ignore list is now the only boundary, so a forgotten entry publishes.**
  In a split layout a misplaced file is in the wrong directory and therefore
  not served; here it ships unless the list says otherwise. →
  `published-tree.py` asserts the published set equals a declared expected set,
  so adding a file and forgetting the list fail the same check, and the check
  runs in `ci.yml` on every pull request and again before each deploy
  (tasks 2.2, 6.1, 7.2).
- **The first deploy is the only one that can leak.** The Worker is created and
  `emb.is` attached in one `deploy`; if the ignore list were wrong, the leak is
  public for the seconds before it is corrected. → Deploy once to the
  `workers.dev` address with the custom domain still commented out, audit the
  served origin, and attach the domain only after that passes (tasks 8.1, 8.2).
- **A glob in the ignore list would drop a shipped asset.** `/assets/img/*`
  looks like a tidy way to exclude the two image sources and would silently
  remove `terrain-matte.png`, `og.png`, and `speckle.svg` — a broken page, not a
  leak, and harder to notice. → The list uses exact paths only, and the expected
  set in `published-tree.py` names all five images individually.
- **Fork pull requests get no preview.** → Stated in the design and in the
  workflow; reviewers of fork PRs check out locally, which is already the
  documented workflow (`just website`).
- **`_headers` is a real rule set that can be wrong.** Marking a file immutable
  that later changes under the same name is the one way a user sees stale
  content. → The immutable globs are fonts and generated images only; task 5.2
  asserts no HTML, CSS, or JS path falls under them.
- **Preview URLs are guessable within the account's `workers.dev` subdomain.**
  → Preview content is the site's own public content; nothing private is served
  from any revision. No mitigation needed, but worth knowing before anyone
  assumes a preview is secret.
- **The one-folder choice concentrates risk in one file.** → Accepted knowingly,
  and bought back with a check that fails on both classes of mistake — the
  unexpected file and the missing expectation. If that check is ever deleted
  during a refactor, the boundary silently degrades to no boundary; it is
  therefore named in `website/README.md` as the reason the file can be trusted,
  not as a convenience script.

## Migration Plan

Rollout, in order, each step verifiable before the next:

1. **Declare the boundary** — `.assetsignore` and `published-tree.py` — while
   nothing else has changed, so the check can be verified red-then-green against
   the current 22-file folder before any deploy exists. Verify
   `just website-version-check`, `just website`, and `just website-ink` all still
   pass, since nothing moved.
2. **Add the metadata and the 404** to both surfaces and `website/404.html`, and
   extend the check to assert the origin and the expected asset set.
3. **Add the header policy** (`_headers`) and its immutable-path guard.
4. **Add the CI gate** (`ci.yml`) — this is a pure win and can land first if the
   rest slips.
5. **Add the Worker** and deploy it to its `workers.dev` address only; audit the
   served tree by fetching every path in the expected set and asserting nothing
   else responds, including a probe of each ignored path family.
6. **Attach `emb.is`** as a custom domain, then verify the apex, the `http` →
   `https` redirect, and that both surfaces' canonical URLs resolve without a
   further redirect.
7. **Enable the PR preview job** and confirm one PR gets an alias that survives a
   second commit to the same branch.

**Rollback:** Workers retains previous versions, so `wrangler rollback` restores
the apex in seconds; because every deploy is one commit, redeploying the previous
commit with the same workflow is equivalent and needs no dashboard access. If
the pipeline itself breaks, `wrangler deploy` run by hand from a checkout is the
fallback, which is the reason D5 keeps the deploy a single CLI command in a
checked-in config rather than dashboard state. The site is static and has no
data, so no rollback can lose anything but a minute.

## Open Questions

- **Does `www.emb.is` need to exist?** Only `emb.is` is configured today.
  Adding a `www` that redirects to the apex is an additive `routes` entry and a
  redirect rule; it changes no requirement here and can be added later without
  touching the specs. Not decided now because it is a DNS choice, not a design one.
- **Should a tagged release also deploy the site?** `main` deploying means the
  apex always shows the newest page, which is what a marketing site wants. If the
  site should instead track released versions, that is a change to the *trigger*,
  not to the requirements, and can be decided once a release actually moves the
  site.
- **Wrangler's major version to pin.** Pin whatever the first successful deploy
  uses and let Dependabot-style bumps be explicit; the config surface used here
  (`assets`, `routes`, `preview_urls`) is stable across recent majors.
