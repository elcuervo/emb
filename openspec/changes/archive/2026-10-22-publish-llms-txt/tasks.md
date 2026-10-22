# Tasks

All work is implemented and verified; boxes are checked with the evidence that
closed them, in the style the archived changes use.

## 1. The index

- [x] 1.1 Write `website/llms.txt`: one `# emb` heading, a blockquote summary
  (raw float32 bytes by default or `VALUES`, `HELLO 3` for RESP3, one text →
  bulk string and several → array), then `## Reference`, `## Markdown sources`,
  `## Install and run` and `## Optional`. Verified: fetched through a served
  tree, it answers `200` with `Content-type: text/plain` and the body as
  written; the grammar itself stays in the linked pages, per the spec.
- [x] 1.2 Verify every link. The four on-origin targets (`/docs/`, `/scripting/`,
  `/gem/`, `/demos/`) answer `200` from the served tree; the ten external
  targets — seven raw-Markdown files, `install.sh`, releases, the `emb-server`
  gem — answer `200` from their own origins. Verified by fetching all fourteen.

## 2. The gate

- [x] 2.1 Add `"llms.txt"` to `SERVED` in `website/tools/published-tree.py`, so
  an unnamed file in the site directory fails rather than shipping silently.
  Verified: `just website-published` reports `82 served paths`, `sitemap lists
  19 of 19 indexable page(s)`, and exits 0 — and the same check reports
  `unexpected: llms.txt (would ship, not in the served set)` with the entry
  removed.
- [x] 2.2 Confirm the index is a discovery file rather than a page: it has no
  `<loc>` in `sitemap.xml` and its absence is not reported as a gap, because the
  sitemap check derives its expected set from served `.html` pages only.
- [x] 2.3 Leave the deployment configuration alone, and record why: `.txt` is a
  known type so no `_headers` rule is needed, `robots.txt` already carries
  `Allow: /`, and no sitemap entry is owed.
