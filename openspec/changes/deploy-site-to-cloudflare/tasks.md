## 1. Declare the boundary while nothing else has moved

- [x] 1.1 Delete `website/assets/img/mountain.jpg`, which `website/README.md` documents as an unused original. Verify `grep -rn 'mountain.jpg' --include='*.md' --include='*.py' --include='*.html' .` returns only the now-stale reference in `website/README.md`, and correct that entry in `website/README.md`'s file tree. Leave `terrain-v2.png` and `terrain-v2.md` in place — a generator reads them.
- [x] 1.2 Write `website/.assetsignore` (gitignore syntax, exact paths and directory entries only, no negation) covering: `/.impeccable/`, `/tools/`, `/PRODUCT.md`, `/README.md`, `/assets/img/terrain-v2.png`, `/assets/img/terrain-v2.md`, and `/.assetsignore`. Verify it contains no glob, and that `grep -c '^/' website/.assetsignore` equals the number of entries.
- [x] 1.3 Write `website/tools/published-tree.py`: walk `website/`, apply the `.assetsignore` patterns, and assert the remaining set equals an expected set declared in the same file. Support only exact paths, directory entries, and simple trailing globs so it agrees with gitignore on this list without reimplementing gitignore. Verify it exits 0 and prints the expected-set count.
- [x] 1.4 Verify the check is red-then-green on both classes of mistake. Add `website/scratch.txt` and confirm it exits 1 naming that file; remove it and confirm a pass. Then remove one entry from the expected set and confirm it exits 1 naming the file it no longer expects; restore it.
- [x] 1.5 Add a `just website-published` recipe invoking the check, placed beside `website-version-check` and with a comment stating that this check is what makes the one-folder layout safe rather than a convenience. Verify `just website-published` passes from a clean shell.
- [x] 1.6 Confirm the exclusion list is complete against the real tree: the expected set must be exactly `index.html`, `docs/index.html`, the three fonts, `styles.css`, `docs.css`, `main.js`, `terrain-matte.png`, `og.png`, `speckle.svg`. Verify `git ls-files website/ | wc -l` equals expected + excluded + the ignore file itself, with no file unaccounted for either side.
- [x] 1.7 Confirm the working folder is unchanged for a developer. Verify `just website` still serves the site at `http://localhost:8080` (not a subpath), that `http://localhost:8080/tools/ink-probe.html` still loads same-origin, and that `just website-ink` reports PASS at all 24 widths.
- [x] 1.8 Confirm the ignored material is still reachable locally, which is the property the one-folder layout is buying. Verify `http://localhost:8080/README.md`, `/PRODUCT.md`, `/.impeccable/surfaces/website-index-html.md`, and `/assets/img/terrain-v2.png` all return 200 from the local server while `.assetsignore` names every one of them.

## 2. Declare the canonical origin

- [x] 2.1 Add to `website/index.html` a `<link rel="canonical" href="https://emb.is/">`, an `og:url` of the same value, and make `og:image` and `twitter:image` absolute (`https://emb.is/assets/img/og.png`). Verify `grep -n 'rel="canonical"\|og:url\|og:image' website/index.html` shows all three literals present and none relative.
- [x] 2.2 Add the Open Graph and Twitter card block the docs surface lacks to `website/docs/index.html` — `og:type`, `og:title`, `og:description`, absolute `og:image`, `og:url` of `https://emb.is/docs/`, `twitter:card` — plus `<link rel="canonical" href="https://emb.is/docs/">`. Verify the page's `<head>` carries the same tag set as the landing's, differing only in title, description, and URL.
- [x] 2.3 Confirm the metadata is inert over `file://` and adds no request. Verify by opening `website/docs/index.html` directly from the filesystem and checking the network panel is empty, and by re-running `just website-ink` for PASS.
- [x] 2.4 Extend `published-tree.py` to assert that every canonical, `og:url`, and social-image literal in the published HTML names exactly one origin, and that the origin matches the one declared in `wrangler.jsonc`. Verify by changing one literal to `https://example.com/` and confirming exit 1 naming the file and value, then reverting.

## 3. The not-found page

- [x] 3.1 Write `website/404.html` from existing atoms only — the page frame, `:root` tokens, the type ladder, the hairline rule, and the annotation form — with no canonical link (it is not a canonical page) and routes back to `/` and `/docs/`. Verify with `just website` that `http://localhost:8080/404.html` renders in the poster's world.
- [x] 3.2 Confirm the page adds no visual primitive and no token. Verify every class it uses already appears in `website/assets/css/styles.css`, and that the file contains no gradient, no `border-radius`, no `box-shadow` beyond the button's existing 5px offset, and no second accent colour, per `product-site`'s composition rule.
- [x] 3.3 Add `website/404.html` to the expected set in `published-tree.py` and verify the check passes, then confirm `just website-ink` still passes (the new page is not measured by the probe, which loads `index.html` — note this rather than assuming coverage).

## 4. Cache headers

- [x] 4.1 Write `website/_headers` with long-lived `immutable` caching for `/assets/fonts/*` and `/assets/img/*` only, and an explicit revalidate policy for `/assets/css/styles.css`, `/assets/js/main.js`, and the HTML pages, so no response depends on an undocumented platform default. Verify the file parses as `_headers` (a path line followed by indented header lines) and add `_headers` to the expected set.
- [x] 4.2 Guard the one way `_headers` can hurt a reader. Add to `published-tree.py` an assertion that no `.html`, `.css`, or `.js` path falls under any `immutable` rule. Verify red-then-green by temporarily adding `/assets/css/*` to the immutable block, confirming exit 1, then reverting.

## 5. The CI gate

- [x] 5.1 Add a `site` job to `.github/workflows/ci.yml` running on pull requests and pushes that sets up Python 3 and runs, in order, `just website-version-check`, `just website-published`, and the origin check. Verify the same three commands pass by hand in that order, and that the job needs no Go, no ONNX, and no browser.
- [x] 5.2 State honestly that the browser-measured ink check is **not** in this job, so a green `site` job is never read as "the ink check passed". Verify a comment naming `just website-ink` as separately run sits above the job's steps.

## 6. The Worker and the deploy workflow

- [x] 6.1 Write `wrangler.jsonc` at the repository root declaring `name: "emb-site"`, a `compatibility_date`, `assets.directory: "./website"`, `assets.html_handling: "auto-trailing-slash"`, `assets.not_found_handling: "404-page"`, `preview_urls: true`, `workers_dev: true`, and the `emb.is` custom-domain route **commented out** for now. Verify `npx wrangler deploy --dry-run` reports the asset directory and lists no file under `tools/`, `.impeccable/`, or the image sources.
- [x] 6.2 Write `.github/workflows/site.yml` with a `deploy` job on `push` to `main` and a `preview` job on `pull_request`, both path-filtered to `website/**`, `VERSION`, `wrangler.jsonc`, and the workflow itself, both gated on the three checks from task 5.1, using `cloudflare/wrangler-action`. The preview job runs `wrangler versions upload --preview-alias pr-<number>` and comments the resulting address on the pull request (`pull-requests: write` on that job only). Verify by inspection that the deploy job's wrangler command is `deploy` and the preview job's is `versions upload` — a preview must never run `deploy`.
- [x] 6.3 Add `concurrency` (cancel superseded runs per ref) and a `workflow_dispatch` trigger, and make the preview job a no-op with an explanatory comment when `github.event.pull_request.head.repo.fork` is true, since fork runs receive no secrets. Verify the workflow parses with `actionlint`, or by reading back the YAML after the edit.

## 7. Deploy, then verify against the real origin

- [ ] 7.1 Deploy once to the `workers.dev` address with the custom domain still commented out. Then audit: fetch every path in `published-tree.py`'s expected set and assert 200.
- [ ] 7.2 On that same address, assert the leaks are closed — `/README.md`, `/PRODUCT.md`, `/.impeccable/surfaces/website-index-html.md`, `/.assetsignore`, `/tools/ink-probe.html`, `/tools/stamp-version.py`, `/tools/published-tree.py`, `/assets/img/terrain-v2.png`, `/assets/img/terrain-v2.md` must each return 404 (or a redirect to the 404 page). Do not attach the domain until 7.1 and 7.2 both pass.
- [ ] 7.3 Attach `emb.is`, then verify the apex serves the landing over `https`, that a plaintext request to the apex redirects rather than serving content, and that `https://emb.is/` and `https://emb.is/docs/` each return their page without a further redirect.
- [ ] 7.4 Verify the not-found behavior against the origin: an unknown path returns a not-found status carrying `404.html`, its `/_headers`-declared caching matches task 4.1, and its two routes resolve.
- [ ] 7.5 Verify the deployed bytes are the audited revision: the version string rendered on the landing equals the repository's `VERSION`, and a browser that had loaded the previous `styles.css` receives a corrected one without clearing cache.

## 8. Prove the delivery contract

- [ ] 8.1 Open a pull request that changes only `website/index.html`, and verify a preview address is produced, is commented on the pull request, and serves the changed page while the apex still serves the default branch's revision.
- [ ] 8.2 Push a second commit to that pull request's branch and verify **the same preview address** now serves the newer revision — the alias is repointed, not replaced (design D4).
- [ ] 8.3 Merge the pull request and verify the apex serves the merged revision within one workflow run.
- [ ] 8.4 Push a commit that touches no file under `website/` to `main` and verify no deploy of the site is produced.
- [ ] 8.5 Drill the version gate end to end: on a scratch branch, set one `data-emb-version` value to a wrong version, confirm both `just website-version-check` and the pull request's `site` job fail, and confirm no preview is published for that revision. Revert.
- [ ] 8.6 Drill the boundary gate end to end: on a scratch branch, add `website/notes.txt`, confirm `just website-published` fails and the pull request's `site` job fails, and confirm no preview is published for that revision. Revert.

## 9. Document the surface

- [x] 9.1 Document the deployment in `website/README.md`: the origin, the two triggers, the preview alias scheme, where the Worker config lives, which files ship and which never do, and how to roll back. Include the reason `just website-published` exists — that `.assetsignore` is the only boundary in a one-folder layout — so a future refactor does not quietly drop it. Verify every command in the new section runs as written.
- [x] 9.2 Note in root `README.md` (or `DESIGN.md` where the site brief lives) that the site is served at `https://emb.is` and that the same tree renders over `file://`. Verify no documented local URL changed: `just website` still serves the site at `/`.

## 10. The documentation surface's inherited controls

- [x] 10.1 Give the docs masthead the disclosure the landing uses, so the breakpoint that hides `.nav` does not leave the reference surface with no navigation at all, and verify it is keyboard-operable and meets the committed target size.
  - Measured before: at 390px the masthead's only visible link was the wordmark — `.nav` is `display:none` below 1000px by design and this surface had none of the replacement the landing supplies (`.mobile-nav` was absent from the page). Root, for contrast: `Menu +` at 68x44.
  - Changed: the landing's own `details.mobile-nav` atom added to `website/docs/index.html` with Home / Install / Configuration / GitHub / Community. No new class, no new rule.
  - Measured after, by focusing the summary and pressing Enter (keyboard path, not a scripted click): `open` true, panel `280x278 at (20,56)` fully inside a 320x640 viewport with nothing clipped, five items at `246x52`, summary `68x44`, `+` rotating to `×`. At 1440px: `.nav` visible, `.mobile-nav` `display:none`, masthead height unchanged at 76px.
- [x] 10.2 Put a pointer-size floor on the brand this page scales down for itself, and verify it clears 24x24 without moving the masthead.
  - The landing's brand is set at a display size and measures 84x36. `docs.css` sets it to `clamp(20px, 1.9vw, 24px)` for a reference masthead, which left the link at **50x22** — under WCAG 2.2 SC 2.5.8's 24x24. The floor is now written as a floor (`min-height: 24px`, `inline-flex`) rather than left to whichever value the clamp resolves to.
  - Verified at 1440px: brand `50x24`, `meets24` true, masthead height still 76px; at 390px: `42x44`.
- [x] 10.3 Verify the two surfaces still share one world, and that the new markup did not break the ink floor at any width.
  - Tokens compared in the browser rather than read from the files: all 13 identical between `/` and `/docs/`, and `.shell` (1720px / 47.36px), `.mono`, `.footer` (rgb(17,17,16)) compute identically. `docs.css` declares 0 custom properties and 0 colour literals; `styles.css` is the only stylesheet either surface loads that declares a token.
  - Contrast walked over every text node with the panel open: **0 violations** page-wide; the five panel links measure 17.28:1 on the paper ground.
  - `just website-ink` PASS at all 24 widths on all three surfaces, including the docs surface at the nine widths where the disclosure is displayed.
