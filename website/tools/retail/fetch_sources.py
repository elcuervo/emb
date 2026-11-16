#!/usr/bin/env python3
"""Fetch, verify and extract the two pinned USDA releases (opt-in, no model).

    python3 website/tools/retail/fetch_sources.py            # download + extract
    python3 website/tools/retail/fetch_sources.py --verify   # check what is on disk

The archives are large (~430 MB each) and the extracted CSV is larger still, so
nothing here is committed: everything lands in the ignored
`website/tools/retail/.cache/`. The lock's size and SHA-256 are checked before
an archive is read; a digest mismatch stops the build instead of parsing bytes
that are not the release the report names.
"""

from __future__ import annotations

import argparse
import hashlib
import json
import sys
import urllib.request
import zipfile
from pathlib import Path

HERE = Path(__file__).resolve().parent
CACHE = HERE / ".cache"
LOCK = HERE / "sources.lock.json"


def load_lock() -> dict:
    return json.loads(LOCK.read_text(encoding="utf-8"))


def digest(path: Path) -> str:
    hasher = hashlib.sha256()
    with path.open("rb") as handle:
        for chunk in iter(lambda: handle.read(1 << 20), b""):
            hasher.update(chunk)
    return hasher.hexdigest()


def archive_path(release: dict) -> Path:
    return CACHE / Path(release["url"]).name


def member_name(lock: dict, release: dict, kind: str = "branded_food") -> str:
    return lock["extraction"]["archive_members"][kind].format(version=release["version"])


def verify_archive(release: dict, path: Path) -> None:
    expected = release["sha256"]
    if not expected:
        sys.exit(f"fetch_sources: {path.name}: the lock has no pinned sha256; record it before shipping derived rows")
    size = path.stat().st_size
    if size != release["bytes"]:
        sys.exit(f"fetch_sources: {path.name}: {size} bytes, the lock pins {release['bytes']}")
    got = digest(path)
    if got != expected:
        sys.exit(f"fetch_sources: {path.name}: sha256 {got}, the lock pins {expected}")


def download(release: dict, path: Path) -> None:
    CACHE.mkdir(parents=True, exist_ok=True)
    partial = path.with_suffix(path.suffix + ".part")
    with urllib.request.urlopen(release["url"]) as response, partial.open("wb") as out:
        total = int(response.headers.get("content-length") or release["bytes"])
        done = 0
        for chunk in iter(lambda: response.read(1 << 20), b""):
            out.write(chunk)
            done += len(chunk)
            if sys.stderr.isatty():
                print(f"  {path.name}: {done * 100 // total}%", end="\r", file=sys.stderr)
    if sys.stderr.isatty():
        print(" " * 60, end="\r", file=sys.stderr)
    partial.replace(path)


def extract(lock: dict, release: dict, path: Path) -> None:
    wanted = [member_name(lock, release, kind) for kind in ("branded_food", "description")]
    with zipfile.ZipFile(path) as archive:
        present = set(archive.namelist())
        absent = [name for name in wanted if name not in present]
        if absent:
            sys.exit(f"fetch_sources: {path.name}: no {', '.join(absent)}")
        for member in wanted:
            if (CACHE / member).exists():
                continue
            (CACHE / member).parent.mkdir(parents=True, exist_ok=True)
            print(f"  extracting {member}")
            archive.extract(member, CACHE)


def main() -> int:
    parser = argparse.ArgumentParser(description=__doc__, formatter_class=argparse.RawDescriptionHelpFormatter)
    parser.add_argument("--verify", action="store_true", help="check the pinned archives and extracted CSV; download nothing")
    args = parser.parse_args()

    lock = load_lock()
    for release in lock["releases"]:
        path = archive_path(release)
        members = [CACHE / member_name(lock, release, kind) for kind in ("branded_food", "description")]
        if args.verify:
            if path.exists():
                verify_archive(release, path)
                print(f"{release['version']}: archive pinned and verified ({release['bytes']} bytes)")
            else:
                print(f"{release['version']}: archive absent (run without --verify to download)")
            for member in members:
                print(f"{release['version']}: csv {'present' if member.exists() else 'absent'} · {member.relative_to(HERE)}")
            continue
        if not path.exists():
            print(f"fetching {release['url']}")
            download(release, path)
        verify_archive(release, path)
        extract(lock, release, path)
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
