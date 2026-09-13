## Why

The site is finished but has nowhere to live. It has been built, measured, and
corrected across thirteen passes with the only renderer it has ever had: a
`python3 -m http.server` on `localhost:8080`. That has three consequences the
repository cannot currently see.

**The publish root is the working directory.** `website/` is where the site is
authored, not what ships. Pointed at Cloudflare as-is it would serve ~2.9 MB of
internal material at guessable URLs — `README.md`, `PRODUCT.md`, the
`.impeccable/surfaces/` design notes, the six `tools/` files including the
ink-probe harness, `terrain-v2.png` (the matte's 2.2 MB *source*),
`terrain-v2.md`, and `mountain.jpg` (explicitly unused). The repository has a
rule that every rendered claim is traceable to the reference; it has no rule
about what is *served*.

**The pages have never had a URL.** `og:image` and `twitter:image` are relative
paths, neither surface declares a canonical link or `og:url`, and `docs/` ships
no Open Graph tags at all. This did not matter while the only consumer was a
localhost browser. It matters the moment a human pastes a link into Slack.

**The site is not gated in CI.** `.github/workflows/ci.yml` has no website job,
so `just website-version-check` — the drift guard written *because* an `emb-top`
capture shipped `v0.4.0` against a `0.4.0.pre4` `VERSION` — has never run
automatically. A deploy pipeline without it would publish the exact defect it
was built to prevent.

`emb.is` is now configured on Cloudflare, which makes this the moment.

## What Changes

- **The site deploys to Cloudflare Workers Static Assets**, not Pages. Pages
  stopped receiving new features in April 2025 and every new capability lands in
  Workers Static Assets; static-asset requests are free and unlimited on both,
  so the newer surface costs nothing. The Worker is assets-only — no script.
- **`website/` stays one folder, and `.assetsignore` is the boundary.** The
  authoring material — this README, `PRODUCT.md`, the `.impeccable/` notes, the
  generators, the image sources — stays exactly where it is, beside the pages it
  describes, so the site remains one folder to work in and `just website` keeps
  serving it at `/`. A `.assetsignore` names what the origin must never serve,
  and a check asserts the published set equals the expected set, because
  the ignore list is now the *only* thing between a working file and a public URL.
- **The local workflow and the published tree stop being the same set on
  purpose.** `.assetsignore` is a deploy-time filter, not a server rule, so
  `tools/ink-probe.html` keeps answering at `/tools/ink-probe.html` locally —
  same-origin with `index.html`, which is what lets it measure the page in a
  frame — while contributing nothing to what `emb.is` serves.
- **One origin, stated once.** A single canonical `https://emb.is` origin drives
  the canonical link on both surfaces, absolute `og:image`/`og:url`, and the
  Worker's custom-domain route. `/docs/` gains the Open Graph tags it lacks.
- **`main` deploys; a pull request gets a stable preview URL.** Both are
  path-filtered to the site, gated on the version check, and the PR job comments
  the preview URL back on the pull request.
- **A site job joins `ci.yml`** so the stamp drift guard and a served-page smoke
  check run on every pull request, not only on the path to production.
- **Header policy written for unhashed names.** The site has no build step and
  therefore no content hashes; the HTML, CSS, and JS take the platform's
  revalidate default while the fonts and images — which are genuinely
  content-stable — get long-lived immutable caching.
- **`website/README.md` documents the boundary**, since the folder now holds two
  populations of files that look identical in a file listing: the ones that ship
  and the ones that must not.

## Capabilities

### New Capabilities

- `site-deployment`: what leaves the repository and is served — the published
  tree boundary and its deny list, the canonical origin and the metadata derived
  from it, the continuous delivery contract (`main` deploys, pull requests
  preview), the check gate that must pass before either, and the cache-header
  policy that follows from shipping unhashed asset names.

### Modified Capabilities

- `product-docs`: the docs surface's masthead lost its entire navigation below
  1000px — the shared stylesheet hides the primary nav there and this surface had
  none of the replacement the landing supplies. The requirement that the docs
  surface inherits the poster's world now says so explicitly, and gains a
  scenario for the target-size floor on a control this page scales down for
  itself. No new requirement, and no change to what the surface is.

## Impact

- **New:** `wrangler.jsonc`, `website/_headers`, `website/.assetsignore`,
  `website/tools/published-tree.py`, `.github/workflows/site.yml`.
- **New page:** `website/404.html`, built from the poster's existing atoms.
- **Edited:** `website/index.html` and `website/docs/index.html` (head metadata,
  plus the landing's own mobile disclosure on the docs masthead),
  `website/assets/css/docs.css` (a target-size floor on the scaled-down brand),
  `.github/workflows/ci.yml`, `justfile` (three new website recipes), and
  `website/README.md` (a served-tree section stating what ships).
- **Deleted:** `website/assets/img/mountain.jpg`, documented as unused. Its
  removal is cleanup, not a deployment need; the other sources are kept because
  a generator reads them.
- **Moved:** nothing. No path in the repository changes, so no link, tool
  invocation, or documented command needs repointing; `stamp-version.py` keeps
  its existing repo-root resolution and the generators keep their output paths.
  `CORRECTIONS.md` was already removed in `2397754`.
- **GitHub:** `CLOUDFLARE_API_TOKEN` and `CLOUDFLARE_ACCOUNT_ID` repository
  secrets; `pull-requests: write` on the preview job only.
- **Cloudflare:** one Worker (`emb-site`) with `emb.is` attached as a custom
  domain; `*.workers.dev` stays enabled so preview aliases resolve.
- **No change** to the site's markup language, composition, tokens, console
  behavior, or any server, gem, or Go code.
