#!/usr/bin/env python3
"""Stamp the sandbox preset digests into the site, derived from the preset bytes.

The sandbox calls a preset by the SHA1 of the Lua source the server preloaded,
so the digest the site shows and the digest the server knows must be the same
value. A hand-typed copy drifts the moment a preset byte changes -- and a drift
is not cosmetic: `EMB.EVSHA` would answer "no such script" for a command the
site presents as working.

This tool makes the value derived instead of typed. Every element carrying a
`data-emb-preset-<name>` attribute has that attribute rewritten from the SHA1 of
the preset file named here, the same `sha1(bytes)` the server computes for
`EMB.SCRIPT LOAD` / the preload digest:

    python3 website/tools/stamp-presets.py          # rewrite in place
    python3 website/tools/stamp-presets.py --check  # exit 1 if anything drifted

It also asserts the sandbox's server configuration preloads each preset, so the
site cannot reference a digest the server was never told to load.

`--check` is the drift guard: it is what a CI job or a pre-commit hook runs.
"""

from __future__ import annotations

import argparse
import hashlib
import re
import sys
from pathlib import Path

REPO_ROOT = Path(__file__).resolve().parents[2]

# Preset name -> (model the server loads it for, path relative to the repo).
# The path is the same one `website/repl/sandbox.yaml` lists under `scripts:`,
# which is what makes the stamped digest the loaded digest.
PRESETS = {
    "embed": ("minilm", "website/repl/presets/embed.lua"),
    "classify": ("sst2", "website/repl/presets/classify.lua"),
}

SANDBOX_CONFIG = "website/repl/sandbox.yaml"
TARGETS = ["website/index.html"]

# Leftmost-longest, and narrow: only the marker attribute's own value is
# rewritten (`[^"]*`), never a neighbouring attribute.
marker = re.compile(r'(data-emb-preset-(?P<name>[a-z0-9-]+))="(?P<sha>[^"]*)"')


def preset_digests() -> dict[str, str]:
    digests: dict[str, str] = {}
    for name, (_, relative) in PRESETS.items():
        path = REPO_ROOT / relative
        if not path.exists():
            sys.exit(f"stamp-presets: preset {relative} does not exist")
        digests[name] = hashlib.sha1(path.read_bytes()).hexdigest()
    return digests


def check_preloaded() -> list[str]:
    """Return the presets the sandbox config does NOT list under `scripts:`."""
    path = REPO_ROOT / SANDBOX_CONFIG
    if not path.exists():
        return [f"{SANDBOX_CONFIG} is missing"]
    config = path.read_text(encoding="utf-8")
    missing = []
    for name, (_, relative) in PRESETS.items():
        listed = f"presets/{Path(relative).name}"
        if listed not in config:
            missing.append(f"{SANDBOX_CONFIG} does not preload {listed} (preset {name!r})")
    return missing


def stamp(text: str, digests: dict[str, str]) -> tuple[str, list[str]]:
    replaced: list[str] = []

    def sub(match: re.Match[str]) -> str:
        name = match.group("name")
        if name not in digests:
            return match.group(0)
        old = match.group("sha")
        if old != digests[name]:
            replaced.append(f"{name}: {old or '(empty)'} -> {digests[name]}")
        return f'{match.group(1)}="{digests[name]}"'

    return marker.sub(sub, text), replaced


def main() -> int:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument(
        "--check",
        action="store_true",
        help="do not write; exit 1 if any stamped digest differs from the preset bytes",
    )
    args = parser.parse_args()

    digests = preset_digests()
    failures = check_preloaded()
    touched: list[str] = []
    seen = 0

    for relative in TARGETS:
        path = REPO_ROOT / relative
        if not path.exists():
            failures.append(f"{relative} is missing")
            continue
        source = path.read_text(encoding="utf-8")
        seen += sum(1 for m in marker.finditer(source) if m.group("name") in digests)
        stamped, replaced = stamp(source, digests)
        if stamped == source:
            continue
        if args.check:
            failures.extend(f"{relative}: {line}" for line in replaced)
        else:
            path.write_text(stamped, encoding="utf-8")
            touched.extend(f"{relative}: {line}" for line in replaced)

    if seen == 0:
        failures.append(f"no data-emb-preset-* marker found in {', '.join(TARGETS)}")

    if args.check:
        if failures:
            print("stamp-presets: preset digests are stale", file=sys.stderr)
            for line in failures:
                print(f"  {line}", file=sys.stderr)
            print("  run: python3 website/tools/stamp-presets.py", file=sys.stderr)
            return 1
        print(f"stamp-presets: ok ({seen} marker(s), all {len(digests)} digests current)")
        return 0

    # Write mode rewrites the digests it can, but the configuration failures
    # (missing target, unloaded preset) are not something a write can fix, so
    # they still fail the run rather than printing "nothing to do".
    if failures:
        for line in failures:
            print(f"stamp-presets: {line}", file=sys.stderr)
        return 1

    if touched:
        for line in touched:
            print(f"stamp-presets: {line}")
    else:
        print(f"stamp-presets: nothing to do ({seen} marker(s) already current)")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
