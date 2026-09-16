#!/usr/bin/env python3
"""Assert the shape of the tree this folder publishes.

`website/` is both the working tree and the publish source: the pages are edited
where they are served from, and `.assetsignore` is the only thing between an
authoring file and a public URL. Two classes of mistake follow from that, and
neither is visible by reading the ignore file or the markup alone:

1. **The served set.** A missing line in `.assetsignore` publishes a file; a
   stray glob in it can publish a broken page. Checked against an explicit
   served set declared below, with platform configuration kept in its own set
   so a `_headers` that ships as a static file is caught rather than counted.
2. **The canonical origin.** Every page states an absolute origin as a literal,
   because a build step cannot supply it and a relative `og:image` does not
   resolve for a scraper. The value is not pinned: the same tree is served from
   localhost, from preview aliases and from production, so what is asserted is
   that the addresses are absolute and that every page agrees on one origin.
   The value found is reported and, when `wrangler.jsonc` declares a custom
   domain, cross-checked against it with a warning rather than a failure.

    python3 website/tools/published-tree.py

Exits 0 when every check passes, 1 otherwise, naming each offender and why.

The ignore matcher supports only exact paths, directory entries (`/tools/`), and
anchored globs (`/assets/img/*.png`). That is deliberate: it is a small, legible
approximation of the gitignore syntax the asset uploader implements, and the
ignore list is kept to the shapes it handles faithfully rather than growing a
second implementation of gitignore here.
"""

from __future__ import annotations

import json
import posixpath
import re
import sys
from fnmatch import fnmatchcase
from pathlib import Path

REPO_ROOT = Path(__file__).resolve().parents[2]
SITE_DIR = REPO_ROOT / "website"
IGNORE_FILE = SITE_DIR / ".assetsignore"
HEADERS_FILE = SITE_DIR / "_headers"
WRANGLER_CONFIG = REPO_ROOT / "wrangler.jsonc"

# The origin is deliberately NOT pinned here. This one tree is served from
# localhost, from a workers.dev address, from per-branch preview aliases and from
# the production domain, and the check has to be able to run against any of them
# — `just website` is the working loop, not a special case of production. What is
# checked instead is the shape: every address the pages emit is absolute, and
# they all agree on a single origin so a pasted link cannot resolve two ways.
# The origin that was found is reported, and cross-checked against
# `wrangler.jsonc` when that file declares a custom domain.

# What the origin is allowed to serve. Every entry is a site artifact: something
# the pages reference, or configuration the platform reads. Adding a file to the
# site means adding it here; if that feels like friction, that is the point.
SERVED = frozenset(
    {
        "index.html",
        "404.html",
        "docs/index.html",
        "demos/index.html",
        "demos/vector.html",
        "demos/similarity.html",
        "demos/search.html",
        "demos/atlas.html",
        "demos/batch.html",
        "demos/cache.html",
        "demos/graph.html",
        "demos/image.html",
        "demos/lens.html",
        "demos/function.html",
        "demos/samples/raven.jpg",
        "demos/samples/storm.jpg",
        "demos/samples/portrait.jpg",
        "demos/samples/ship.jpg",
        "demos/samples/flowers.jpg",
        "demos/samples/manuscript.jpg",
        "assets/js/demos.js",
        "assets/vendor/sqlite-wasm-vec-0.1.9/sqlite3-bundler-friendly.mjs",
        "assets/vendor/sqlite-wasm-vec-0.1.9/sqlite3.wasm",
        "assets/css/styles.css",
        "assets/css/docs.css",
        "assets/css/asciinema-player-3.17.0.css",
        "assets/js/main.js",
        "assets/js/topviz.js",
        "assets/cast/emb-top-dd60083b.cast",
        "assets/js/asciinema-player-3.17.0.min.js",
        "assets/fonts/archivo-var-latin.woff2",
        "assets/fonts/inter-900-latin.woff2",
        "assets/fonts/jetbrains-mono-var-latin.woff2",
        "assets/img/terrain-matte.png",
        "assets/img/og.png",
        "assets/img/speckle.svg",
    }
)

# In the folder, never served, and never named in `.assetsignore`: the uploader
# reads both as configuration and reports "Ignoring asset" for each on its own.
# Verified with `wrangler deploy --dry-run`: "Ignoring asset: .assetsignore",
# "Ignoring asset: _headers". Modelling them as a third set rather than folding
# them into SERVED keeps the check honest about what a reader can fetch — a
# `_headers` that ships as a static file is a bug, not a success.
PLATFORM_FILES = frozenset({".assetsignore", "_headers"})

# The sandbox service lives under the site directory and is not the site: its
# source, server configuration, preset Lua, image build, and terminal page are a
# program, not a page. Excluding the directory is not enough on its own — the
# exclusion is asserted here, so neither a deleted `.assetsignore` line (which
# would publish service source) nor a re-included file can pass unnoticed.
SERVICE_PREFIX = "repl/"

# Paths whose bytes change under a stable name, because there is no build step
# to hash them. These must never be pinned immutable by `_headers`.
UNHASHED_SUFFIXES = (".html", ".css", ".js", ".json")

# Every page that must declare its own address, and the metadata on it that must
# be an absolute URL on ORIGIN. A relative `og:image` is the specific defect this
# catches: it shipped that way because the site had no hostname until now.
PAGES = (
    "index.html",
    "docs/index.html",
    "demos/index.html",
    "demos/vector.html",
    "demos/similarity.html",
    "demos/search.html",
    "demos/atlas.html",
    "demos/batch.html",
    "demos/cache.html",
    "demos/graph.html",
    "demos/image.html",
    "demos/lens.html",
    "demos/function.html",
)
ABSOLUTE_METADATA = (
    ("canonical", re.compile(r"""<link[^>]*\brel=["']canonical["'][^>]*\bhref=["']([^"']*)["']""", re.I)),
    ("og:url", re.compile(r"""<meta[^>]*\bproperty=["']og:url["'][^>]*\bcontent=["']([^"']*)["']""", re.I)),
    ("og:image", re.compile(r"""<meta[^>]*\bproperty=["']og:image["'][^>]*\bcontent=["']([^"']*)["']""", re.I)),
    ("twitter:image", re.compile(r"""<meta[^>]*\bname=["']twitter:image["'][^>]*\bcontent=["']([^"']*)["']""", re.I)),
)


# The demos index is a generated pair whose file names change when the corpus
# does: the manifest keeps a stable name and the index carries a content hash, so
# the served set cannot be a constant. The manifest is the declaration — it names
# the index it belongs to — and it is asserted to exist and to name a file that
# does, so a rebuild renames the index without editing this check, while a
# manifest pointing at a missing file still fails.
INDEX_DIR = "assets/demo"
INDEX_MANIFEST = f"{INDEX_DIR}/manifest.json"


def index_files() -> tuple[set[str], list[str]]:
    """The served index pair, and the problems that stop it being trusted."""
    path = SITE_DIR / INDEX_MANIFEST
    if not path.exists():
        return {INDEX_MANIFEST}, [f"absent:       website/{INDEX_MANIFEST} (the index's own manifest)"]
    try:
        manifest = json.loads(path.read_text(encoding="utf-8"))
    except json.JSONDecodeError as err:
        return {INDEX_MANIFEST}, [f"unreadable:   website/{INDEX_MANIFEST} ({err})"]
    named = (manifest.get("asset") or {}).get("db")
    if not named:
        return {INDEX_MANIFEST}, [f"unreadable:   website/{INDEX_MANIFEST} names no index"]
    if not (SITE_DIR / INDEX_DIR / named).exists():
        return {INDEX_MANIFEST}, [
            f"absent:       website/{INDEX_DIR}/{named} (the manifest names it and it is not on disk)"
        ]
    return {INDEX_MANIFEST, f"{INDEX_DIR}/{named}"}, []


def read_ignore_patterns(path: Path = IGNORE_FILE) -> list[str]:
    """Return the non-comment, non-blank lines of the ignore file."""
    if not path.exists():
        sys.exit(f"published-tree: no ignore file at {path}")
    patterns = []
    for line in path.read_text(encoding="utf-8").splitlines():
        stripped = line.strip()
        if stripped and not stripped.startswith("#"):
            patterns.append(stripped)
    return patterns


def matches(relative: str, pattern: str) -> bool:
    """Return whether a site-relative posix path is excluded by one pattern.

    Anchoring is unconditional: every pattern is resolved against the site root,
    because a pattern that also matched somewhere else in the repository would
    be a different tool's bug, not this one's. A trailing slash means "this
    directory and everything under it".
    """
    candidate = pattern[1:] if pattern.startswith("/") else pattern
    directory = candidate.endswith("/")
    candidate = candidate.rstrip("/")
    if not candidate:
        return False

    if any(char in candidate for char in "*?["):
        if fnmatchcase(relative, candidate):
            return True
        return directory and fnmatchcase(relative, f"{candidate}/*")

    if directory:
        return relative == candidate or relative.startswith(f"{candidate}/")
    return relative == candidate


def on_disk() -> set[str]:
    """Every file under the site directory, as site-relative posix paths."""
    return {
        path.relative_to(SITE_DIR).as_posix()
        for path in SITE_DIR.rglob("*")
        if path.is_file()
    }


def check_published_set(patterns: list[str]) -> list[str]:
    """Assert on-disk == served + ignored + platform config, with no overlap."""
    files = on_disk()
    generated, index_problems = index_files()
    served_set = SERVED | generated
    ignored = {
        relative
        for relative in files
        if any(matches(relative, pattern) for pattern in patterns)
    }
    served = files - ignored - PLATFORM_FILES

    problems = list(index_problems)
    for relative in sorted(served_set - served):
        if relative not in files:
            problems.append(f"absent:       {relative} (expected, not on disk)")
        elif relative in ignored:
            problems.append(
                f"excluded:     {relative} (expected to ship, named in .assetsignore)"
            )
        else:
            problems.append(f"wrong set:    {relative} (treated as platform config)")
    for relative in sorted(served - served_set):
        problems.append(
            f"unexpected:   {relative} (would ship, not in the served set)"
        )
    for relative in sorted(PLATFORM_FILES):
        if relative not in files:
            problems.append(f"absent:       {relative} (platform config, not on disk)")
        elif relative in ignored:
            problems.append(
                f"listed:       {relative} (platform config, must not be in .assetsignore)"
            )

    for pattern in patterns:
        if not any(matches(relative, pattern) for relative in files):
            print(f"published-tree: warning: {pattern!r} matches nothing", file=sys.stderr)

    service = {relative for relative in files if relative.startswith(SERVICE_PREFIX)}
    if not service:
        problems.append(
            f"absent:       {SERVICE_PREFIX} (the sandbox service directory is missing)"
        )
    for relative in sorted(service):
        if relative not in ignored:
            problems.append(
                f"unexcluded:   {relative} (sandbox service code must never ship)"
            )

    print(
        f"published-tree: {len(served)} served, {len(ignored)} ignored, "
        f"{len(PLATFORM_FILES)} platform config, {len(service)} sandbox service file(s)"
    )
    return problems


def read_header_rules(path: Path = HEADERS_FILE) -> list[tuple[str, str]]:
    """Return (path pattern, cache-control) for each block in `_headers`.

    The format is a path line followed by indented `Name: value` lines, with
    `#` comments. Only `Cache-Control` matters here, because it is the one
    header that can serve a reader a stale page.
    """
    if not path.exists():
        return []
    rules: list[tuple[str, str]] = []
    for line in path.read_text(encoding="utf-8").splitlines():
        if not line.strip() or line.lstrip().startswith("#"):
            continue
        if not line[:1].isspace():
            rules.append((line.strip(), ""))
            continue
        if rules and ":" in line:
            name, _, value = line.strip().partition(":")
            if name.lower() == "cache-control":
                rules[-1] = (rules[-1][0], value.strip())
    return rules


def served_urls() -> list[tuple[str, str]]:
    """Every (request path, file) a reader can ask for, including pretty URLs."""
    generated, _ = index_files()
    urls = [(f"/{relative}", relative) for relative in sorted(SERVED | generated)]
    urls += [("/", "index.html"), ("/docs", "docs/index.html"), ("/docs/", "docs/index.html")]
    urls += [("/demos", "demos/index.html"), ("/demos/", "demos/index.html")]
    urls += [(f"/demos/{Path(relative).stem}", relative)
             for relative in sorted(SERVED) if relative.startswith("demos/") and relative.endswith(".html")]
    # The clean name for both index files, which is what the manifest is read at.
    urls += [("/assets/demo/manifest.json", INDEX_MANIFEST)]
    for relative in sorted(generated):
        if relative.endswith(".db"):
            urls += [(f"/{relative}", relative)]
    return urls


def check_cache_rules() -> list[str]:
    """Assert no unhashed path is pinned immutable by the real `_headers`.

    This reads the shipped file rather than a constant here, because a guard
    that checks its own copy of the policy would pass while the policy it ships
    was wrong. The only way `_headers` can hurt a reader is pinning a file that
    later changes under the same name.
    """
    problems = []
    for pattern, cache_control in read_header_rules():
        if "immutable" not in cache_control.lower():
            continue
        for url, relative in served_urls():
            if relative.endswith(UNHASHED_SUFFIXES) and fnmatchcase(url, pattern):
                problems.append(
                    f"stale risk:   _headers pins {pattern} immutable, which covers "
                    f"{url} ({relative}) — its bytes change under a stable name"
                )
    return problems


# Every internal reference a page can carry. `href` covers links and stylesheets,
# `src` covers scripts, images and the wasm module. A page that links a file the
# origin does not serve is a 404 waiting for a reader, and it is invisible in
# review — which is exactly the class of mistake this tool exists for.
# `(?<![-\w])` keeps `data-src="…"` out of it: a plate's own hooks are not
# references, and a checker that fired on them would be turned off.
REFERENCE = re.compile(r"""(?<![-\w])(?:href|src)\s*=\s*["']([^"']+)["']""", re.I)
EXTERNAL = ("http://", "https://", "//", "data:", "mailto:", "tel:", "javascript:")
# Markup that is not a reference: a script's own strings can look like one.
SCRIPTING = re.compile(r"<(script|style)\b.*?</\1>", re.S | re.I)


def check_internal_links() -> list[str]:
    """Assert every internal reference on a served page resolves to a served file.

    Relative references are what the gallery's own cross-links use, so a plate
    links its neighbours as `similarity.html` and keeps working whether the tree
    is served at the root, from a preview alias, or from `just website`
    locally. Resolving each reference against the page it sits on and then
    against the served set is what turns "these look right" into a check.
    """
    problems: list[str] = []
    served = set(SERVED) | index_files()[0]
    for page in sorted(served):
        if not page.endswith(".html"):
            continue
        source = SCRIPTING.sub("", (SITE_DIR / page).read_text(encoding="utf-8"))
        base = posixpath.dirname(page)
        for reference in REFERENCE.findall(source):
            if reference.startswith("#") or reference.lower().startswith(EXTERNAL):
                continue
            target = reference.split("#", 1)[0].split("?", 1)[0]
            if not target:
                continue
            resolved = posixpath.normpath(
                target if target.startswith("/") else posixpath.join(base, target))
            # A pretty URL is the directory's own index: `/`, `/docs/`, `/demos/`.
            if resolved in ("", "/", "."):
                resolved = "index.html"
            elif resolved.lstrip("/") + "/index.html" in served:
                resolved = resolved.lstrip("/") + "/index.html"
            else:
                resolved = resolved.lstrip("/")
            if resolved not in served:
                problems.append(
                    f"broken link:  website/{page} points at {reference!r}, which resolves to "
                    f"{resolved!r} — not a served path"
                )
                continue
            # Same-directory references stay relative: they are what keeps a
            # link correct under any mount point.
            if base and posixpath.dirname(resolved) == base and target.startswith("/"):
                problems.append(
                    f"absolute:     website/{page} points at {reference!r}; a sibling in "
                    f"{base}/ is reachable relatively"
                )
    return problems


def strip_jsonc_comments(text: str) -> str:
    """Remove // and /* */ comments so the config can be read with json.

    Good enough for a hand-written file this size: it does not understand `//`
    inside a string literal, which is acceptable because no value in
    `wrangler.jsonc` contains one. A route only counts when it is uncommented,
    which is the whole point of reading it this way.
    """
    text = re.sub(r"/\*.*?\*/", "", text, flags=re.S)
    return re.sub(r"(?m)^\s*//.*$", "", text)


def check_url_metadata() -> tuple[list[str], str | None]:
    """Assert every page states an absolute address, and the site states one origin.

    Returns the problems found and the single origin every reference agreed on,
    or None when the references disagree. A relative `og:image` is the defect
    this exists to catch — it is what shipped, because the site had no hostname
    until the deployment work — but the hostname itself is not this tool's
    business.
    """
    problems = []
    origins: dict[str, list[str]] = {}

    for page in PAGES:
        path = SITE_DIR / page
        if not path.exists():
            problems.append(f"absent:       website/{page} (a page that must declare its address)")
            continue
        source = path.read_text(encoding="utf-8")
        for label, pattern in ABSOLUTE_METADATA:
            found = pattern.findall(source)
            if not found:
                problems.append(f"missing:      website/{page} has no {label}")
                continue
            for value in found:
                match = re.match(r"^(https?://[^/]+)(?:/|$)", value)
                if not match:
                    problems.append(
                        f"relative:     website/{page} {label}={value!r} "
                        f"(must be an absolute http(s) URL)"
                    )
                    continue
                origins.setdefault(match.group(1), []).append(f"{page}:{label}")

    if len(origins) > 1:
        split = ", ".join(
            f"{origin} ({len(refs)}: {', '.join(sorted(refs))})"
            for origin, refs in sorted(origins.items())
        )
        problems.append(f"split origin: {split}")
        return problems, None

    return problems, next(iter(origins), None)


def check_wrangler_route(origin: str | None) -> list[str]:
    """Report when the configured custom domain is not the origin in the markup.

    A warning rather than a failure, and not run at all until `wrangler.jsonc`
    declares a route. A mismatch here is worth seeing — the pages would be
    telling scrapers to look somewhere the Worker does not answer — but it is a
    deployment fact, not something to block a commit over.
    """
    if not WRANGLER_CONFIG.exists():
        return [f"absent:       {WRANGLER_CONFIG.name} (declares the served origin)"]

    text = strip_jsonc_comments(WRANGLER_CONFIG.read_text(encoding="utf-8"))
    patterns = re.findall(r"""["']pattern["']\s*:\s*["']([^"']+)["']""", text)
    if not patterns:
        print(
            "published-tree: note: wrangler.jsonc declares no custom domain yet; "
            "the markup's origin is unverified against a route",
            file=sys.stderr,
        )
        return []

    for pattern in patterns:
        if origin and f"https://{pattern}" != origin:
            print(
                f"published-tree: warning: wrangler.jsonc serves {pattern!r} but the "
                f"pages declare {origin!r}",
                file=sys.stderr,
            )
    return []


def main() -> int:
    problems = []
    problems += check_published_set(read_ignore_patterns())
    metadata_problems, origin = check_url_metadata()
    problems += metadata_problems
    problems += check_wrangler_route(origin)
    problems += check_cache_rules()
    problems += check_internal_links()

    if problems:
        print("published-tree: the published tree does not match what we mean", file=sys.stderr)
        for problem in problems:
            print(f"  {problem}", file=sys.stderr)
        return 1

    print(
        f"published-tree: ok ({len(SERVED) + len(index_files()[0])} served paths, one origin at "
        f"{origin or 'no absolute reference'})"
    )
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
