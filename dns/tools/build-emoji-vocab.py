#!/usr/bin/env python3
"""Build `dns/emoji-vocab.json`: the vocabulary the DNS zone answers from.

The source is the CLDR English annotation set, which is where an emoji's name
and its search keywords live:

    <annotation cp="😭">bawling | cry | crying | face | loudly | sad | sob | tear | tears | unhappy</annotation>
    <annotation cp="😭" type="tts">loudly crying face</annotation>

Flags are not in that file: CLDR keeps them with the derived annotations, whose
other entries are the skin-tone and gender variants the vocabulary deliberately
folds. So the derived file is read for its flag entries alone, which is what
lets a country joke have a country to answer with.

Each entry carries the glyph, a slug, and a description. The description is what
the model embeds — the name plus the keywords the name does not already say — so
a query is matched against the words people actually search emoji by rather than
against a title alone.

Determinism: the CLDR revision is pinned, the entries are sorted by slug, and the
JSON is written with a fixed indent and a trailing newline, so `--check` catches
drift from a changed source, a changed rule, or an edited asset.

    python3 dns/tools/build-emoji-vocab.py           # write the asset
    python3 dns/tools/build-emoji-vocab.py --check   # fail on drift

Run it through `just emoji-vocab` / `just emoji-vocab-check`, which is what CI
and the DNS service's own check call.
"""

from __future__ import annotations

import argparse
import hashlib
import json
import sys
import tempfile
import unicodedata
import urllib.request
import xml.etree.ElementTree as ET
from collections.abc import Callable
from pathlib import Path

CLDR_REPO = "https://raw.githubusercontent.com/unicode-org/cldr"
# Pinned: a moving `main` would make the committed asset drift on its own.
CLDR_REF = "release-48"
CLDR_PATH = "common/annotations/en.xml"
CLDR_URL = f"{CLDR_REPO}/{CLDR_REF}/{CLDR_PATH}"
# The flags live here, not in the annotations file above: CLDR keeps derived
# annotations (flags, and the skin-tone and gender variants of other entries)
# apart. Only the flags are taken, because the variants are exactly what the
# vocabulary folds to one canonical glyph.
CLDR_DERIVED_PATH = "common/annotationsDerived/en.xml"
CLDR_DERIVED_URL = f"{CLDR_REPO}/{CLDR_REF}/{CLDR_DERIVED_PATH}"
CLDR_LICENSE = "Unicode-3.0"

ROOT = Path(__file__).resolve().parents[1]
ASSET = ROOT / "emoji-vocab.json"

# U+1F3FB..U+1F3FF are the skin-tone modifiers: a toned emoji is a variant of the
# emoji without them, not a second emoji, and keeping both spends the whole top
# three of a result list on one glyph in three tones.
SKIN_TONE = {chr(c) for c in range(0x1F3FB, 0x1F400)}
# U+FE0F asks for emoji presentation. A glyph with it and a glyph without it are
# one emoji in two spellings, so only the fuller spelling is kept.
PRESENTATION = "\ufe0f"


def slugify(name: str) -> str:
    """`loudly crying face` -> `loudly_crying_face`.

    The separator is `_`, never `-`: the query grammar spends `-` on
    subtraction and `+` on term composition, so a slug carrying either would be
    indistinguishable from an operator.
    """
    kept = []
    for ch in unicodedata.normalize("NFKD", name.lower()):
        kept.append(ch if (ch.isascii() and ch.isalnum()) else " ")
    return "_".join("".join(kept).split())


def description(name: str, keywords: list[str]) -> str:
    """The name plus the keywords it does not already say.

    A keyword is dropped when a word of the name starts with it, which is what
    folds `cry` into `crying face` and `face` into the name outright.
    """
    words = name.lower().split()
    kept = [k for k in keywords if not any(w.startswith(k) for w in words)]
    return " ".join([name, *kept])


def parse(source: bytes, keep: Callable[[str], bool] | None = None) -> list[dict[str, str]]:
    try:
        tree = ET.fromstring(source)
    except ET.ParseError as exc:  # pragma: no cover - the pinned source parses
        raise SystemExit(f"emoji-vocab: the CLDR annotations did not parse: {exc}")

    names: dict[str, str] = {}
    keywords: dict[str, str] = {}
    for node in tree.iter("annotation"):
        cp = node.get("cp")
        text = (node.text or "").strip()
        if not cp or not text:
            continue
        if node.get("type") == "tts":
            names[cp] = text
        else:
            keywords.setdefault(cp, text)

    entries: dict[str, dict[str, str]] = {}
    for glyph, name in names.items():
        # The five bare skin-tone modifiers name a modifier, not an emoji: there
        # is no query that should answer with a tone swatch.
        if glyph in SKIN_TONE:
            continue
        if keep is not None and not keep(name):
            continue
        slug = slugify(name)
        if not slug:
            continue
        words = [k.strip() for k in keywords.get(glyph, "").split("|") if k.strip()]
        # One entry per slug. The source is deduplicated already, and two slugs
        # sharing a glyph would be refused by the loader at boot rather than
        # answered ambiguously, so nothing here has to fold variants.
        entries.setdefault(
            slug,
            {
                "glyph": glyph,
                "slug": slug,
                "name": name,
                "description": description(name, words),
            },
        )

    return sorted(entries.values(), key=lambda e: e["slug"])


def is_flag(name: str) -> bool:
    """CLDR names every flag `flag: <place>`, and nothing else starts that way."""
    return name.lower().startswith("flag")


def render(entries: list[dict[str, str]], sources: list[tuple[str, bytes]]) -> bytes:
    document = {
        "source": f"{CLDR_REPO}/{CLDR_REF}/{CLDR_PATH}",
        "license": CLDR_LICENSE,
        "source_sha256": hashlib.sha256(sources[0][1]).hexdigest(),
        # Every source the asset was built from, so a rebuild is reproducible
        # from the recorded digests rather than from whatever the ref serves.
        "sources": [
            {
                "url": f"{CLDR_REPO}/{CLDR_REF}/{path}",
                "sha256": hashlib.sha256(source).hexdigest(),
            }
            for path, source in sources
        ],
        "generated_by": "dns/tools/build-emoji-vocab.py",
        "entries": entries,
    }
    body = json.dumps(document, ensure_ascii=False, indent=2)
    return (body + "\n").encode("utf-8")


def fetch(url: str) -> bytes:
    try:
        with urllib.request.urlopen(url, timeout=60) as response:
            return response.read()
    except OSError as exc:
        raise SystemExit(f"emoji-vocab: could not fetch {url}: {exc}")


def build() -> tuple[bytes, list[dict[str, str]]]:
    annotations = fetch(CLDR_URL)
    derived = fetch(CLDR_DERIVED_URL)
    # The annotations file comes first, so an entry it already carries wins the
    # slug it shares with a derived one.
    merged: dict[str, dict[str, str]] = {entry["slug"]: entry for entry in parse(annotations)}
    for entry in parse(derived, keep=is_flag):
        merged.setdefault(entry["slug"], entry)
    entries = sorted(merged.values(), key=lambda e: e["slug"])
    if not entries:
        raise SystemExit("emoji-vocab: the source produced no entries")
    return render(entries, [(CLDR_PATH, annotations), (CLDR_DERIVED_PATH, derived)]), entries


def main() -> int:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument(
        "--check",
        action="store_true",
        help="compare the committed asset with a fresh build and fail on drift",
    )
    args = parser.parse_args()

    rendered, entries = build()

    if args.check:
        committed = ASSET.read_bytes() if ASSET.exists() else b""
        if committed == rendered:
            print(f"emoji-vocab-check: {len(entries)} entries, byte for byte")
            return 0
        try:
            have = len(json.loads(committed)["entries"])
        except Exception:
            have = 0
        print(
            f"emoji-vocab-check: {ASSET} is stale ({have} entries committed, "
            f"{len(entries)} built) - regenerate with `just emoji-vocab`",
            file=sys.stderr,
        )
        return 1

    ASSET.write_bytes(rendered)
    print(f"emoji-vocab: wrote {ASSET} ({len(entries)} entries)")
    return 0


if __name__ == "__main__":
    sys.exit(main())
