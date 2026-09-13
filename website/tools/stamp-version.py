#!/usr/bin/env python3
"""Stamp the repository VERSION into the site's version-bearing elements.

The site has no build step, so a version string in the markup is a hand-typed
copy of VERSION and drifts silently -- the `emb-top` capture in
`website/index.html` shipped `v0.4.0` against a VERSION of `0.4.0.pre4`.

This tool makes the value derived instead of typed. Any element carrying the
`data-emb-version` attribute has its text content rewritten from `VERSION`:

    emb-top v<span data-emb-version>0.4.0.pre4</span> ·

Run it after bumping VERSION:

    python3 website/tools/stamp-version.py          # rewrite in place
    python3 website/tools/stamp-version.py --check  # exit 1 if anything drifted

`--check` is the drift guard: it is what a CI job or a pre-commit hook runs.
"""

from __future__ import annotations

import argparse
import re
import sys
from pathlib import Path

REPO_ROOT = Path(__file__).resolve().parents[2]
VERSION_FILE = REPO_ROOT / "VERSION"

# Leftmost-longest, and deliberately narrow: the open tag may not contain another
# tag (`[^<>]`), the marker must be a real attribute (`(?=[\s>/])`), and the inner
# text must be a leaf (`[^<>]*`). A looser pattern matches an OUTER tag and treats
# a nested element's marker as its own, which rewrites the wrong text.
MARKED = re.compile(
    r"(<(?P<tag>[a-zA-Z][\w-]*)[^<>]*\sdata-emb-version(?=[\s>/])[^<>]*>)"
    r"(?P<inner>[^<>]*)"
    r"(</(?P=tag)>)"
)

# Files that may carry a stamped version.
TARGETS = [
    "website/index.html",
    "website/docs/index.html",
]


def read_version() -> str:
    if not VERSION_FILE.exists():
        sys.exit(f"stamp-version: no VERSION file at {VERSION_FILE}")
    version = VERSION_FILE.read_text(encoding="utf-8").strip()
    if not version:
        sys.exit("stamp-version: VERSION is empty")
    return version


def stamp(text: str, version: str) -> tuple[str, list[str]]:
    """Return the stamped text plus the versions that were replaced."""
    replaced: list[str] = []

    def sub(match: re.Match[str]) -> str:
        old = match.group("inner").strip()
        if old != version:
            replaced.append(old or "(empty)")
        return f"{match.group(1)}{version}{match.group(4)}"

    return MARKED.sub(sub, text), replaced


def main() -> int:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument(
        "--check",
        action="store_true",
        help="do not write; exit 1 if any stamped value differs from VERSION",
    )
    args = parser.parse_args()

    version = read_version()
    failures: list[str] = []
    touched: list[str] = []
    seen = 0

    for relative in TARGETS:
        path = REPO_ROOT / relative
        if not path.exists():
            continue
        source = path.read_text(encoding="utf-8")
        stamped, replaced = stamp(source, version)
        count = MARKED.search(source)
        seen += len(MARKED.findall(source))
        if stamped == source:
            continue
        if args.check:
            failures.append(f"{relative}: {', '.join(repr(r) for r in replaced)} != {version!r}")
        else:
            path.write_text(stamped, encoding="utf-8")
            touched.append(f"{relative}: {', '.join(repr(r) for r in replaced)} -> {version!r}")
        if count is None:  # pragma: no cover - defensive
            continue

    if args.check:
        if failures:
            print("stamp-version: stamped version(s) are stale", file=sys.stderr)
            for line in failures:
                print(f"  {line}", file=sys.stderr)
            print(f"  run: python3 website/tools/stamp-version.py", file=sys.stderr)
            return 1
        print(f"stamp-version: ok ({seen} stamped element(s), all {version!r})")
        return 0

    if touched:
        for line in touched:
            print(f"stamp-version: {line}")
    else:
        print(f"stamp-version: nothing to do ({seen} stamped element(s) already {version!r})")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
