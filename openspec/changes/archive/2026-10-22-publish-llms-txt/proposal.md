# Proposal

## Why

An agent asked to write a client against `emb` lands on `https://emb.is`, which
is the canonical place to ask — and the origin gives it nothing machine-first.
The reference is 1,793 lines of poster HTML, the scripting page another 1,400,
and the markdown that would be cheap to read (`docs/commands.md`,
`docs/scripting.md`, `docs/clients.md`) lives outside `website/`, so it is never
served: the only way to those files is to leave the origin for GitHub.
`/llms.txt` — the address agents already probe at a root — answers with the
site's own not-found page. Nothing tells a fetching agent which pages answer
"how do I call this server", which commands exist, or how to install it.

## What Changes

- **`website/llms.txt` is served.** A hand-written index in the `llms.txt`
  shape — one `# emb` heading, a blockquote summary, then `##` sections of
  `[name](url): note` link lists — pointing at the reference pages on this
  origin (`/docs/`, `/scripting/`, `/gem/`, `/demos/`), the readable markdown
  sources on GitHub (`README.md`, `docs/commands.md`, `configuration.md`,
  `scripting.md`, `operations.md`, `clients.md`, `BENCHMARK.md`), and the
  install paths (`install.sh`, `-model-repo`, Docker, the `emb-server` gem,
  releases).
- **The summary states the two facts a client implementer must know before it
  fetches anything:** replies are raw little-endian float32 bytes by default or
  a `VALUES` envelope, `HELLO 3` selects RESP3, and the reply's shape follows
  the text count — one text is a bulk string, several are an array.
- **It is an index, deliberately.** The command grammar, configuration keys and
  Lua host API stay in the pages it links; the file does not restate them, so it
  cannot drift out of step with the documentation it points at.
- **The file joins the asserted served set.** `llms.txt` is named in `SERVED`
  in `website/tools/published-tree.py`, so publication does not depend on
  someone remembering it, and CI fails on a file that would ship unnamed.
- **Not changed:** the served pages, `sitemap.xml` (the index is not a page and
  declares no canonical), `robots.txt` (`Allow: /` already covers it),
  `_headers` (`.txt` is a known type and served as `text/plain`), the sandbox
  under `website/repl/`, and every server, protocol and gem interface.

## Capabilities

### New Capabilities

None. This is the deployment surface's own discovery contract, which
`site-deployment` already owns.

### Modified Capabilities

- `site-deployment`: the origin additionally answers `/llms.txt` with a
  `text/plain` Markdown index in the `llms.txt` shape, whose links are absolute
  and whose on-origin targets are paths this origin serves; the file is a member
  of the asserted served set.

## Impact

- **New files**: `website/llms.txt` (2,578 bytes).
- **Edited**: `website/tools/published-tree.py` — one entry in `SERVED`.
- **Verification**: `just website-published` exits 0 (`82 served paths, one
  origin at https://emb.is`); the tree served locally answers `GET /llms.txt`
  `200` with `Content-type: text/plain`; and all fourteen unique link targets
  were fetched — the four on-origin pages `200` from the tree, the ten external
  URLs `200` from their origins.
- No **BREAKING** changes, no runtime dependency, no build step, and no change
  to what any existing path returns.
