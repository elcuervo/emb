#!/usr/bin/env python3
"""The gallery's one index writer.

Every retrieval the demos perform — the text corpus's vectors, the media
vectors, and the fingerprint library — lives in one content-hashed SQLite file
described by one manifest. The file is assembled in stages, because each stage
needs a different model server (or none): the text stage embeds the corpus, the
media stages append CLAP/X-CLIP/CLIP vectors, and the fingerprint stage appends
the precomputed hash library.

Each stage opens the same working file, creates its own tables, and records a
JSON description of what it added under a key in the `meta` table. The finalize
step then hashes the finished bytes, renames the file to `gallery-<hash8>.db`,
and writes the manifest from those descriptions:

    python3 website/tools/demo_index.py finalize

The working file is seeded from the currently published index when there is one,
so a media-only rebuild carries the text corpus forward instead of dropping it.
"""

from __future__ import annotations

import hashlib
import json
import os
import shutil
import sqlite3
import struct
import sys
from pathlib import Path

REPO_ROOT = Path(__file__).resolve().parents[2]
OUT_DIR = REPO_ROOT / "website" / "assets" / "demo"
WORK_DB = OUT_DIR / "index.build.db"
MANIFEST_PATH = OUT_DIR / "manifest.json"
ARTEFACT_PREFIX = "gallery-"

# The media sections a stage may contribute, in the order the manifest lists
# them. Each stage writes `meta["media.<kind>"]`.
MEDIA_KINDS = ("audio", "video", "frames")


def published_db() -> Path | None:
    """The index the manifest currently names, or None when there is none."""
    if not MANIFEST_PATH.exists():
        return None
    try:
        doc = json.loads(MANIFEST_PATH.read_text(encoding="utf-8"))
    except json.JSONDecodeError:
        return None
    name = (doc.get("asset") or {}).get("db")
    if not name:
        return None
    path = OUT_DIR / name
    return path if path.exists() else None


def open_working(fresh: bool = False) -> sqlite3.Connection:
    """Open the shared working file, seeded from the published index if absent.

    `fresh=True` is for the text stage, which starts a full build: it discards a
    leftover working file so an aborted run cannot leave half a corpus behind.
    The media stages reuse whatever the text stage (or a previous build) left.
    """
    OUT_DIR.mkdir(parents=True, exist_ok=True)
    if fresh and WORK_DB.exists():
        WORK_DB.unlink()
    if not WORK_DB.exists():
        seed = published_db()
        if seed is not None:
            shutil.copyfile(seed, WORK_DB)
    db = sqlite3.connect(WORK_DB)
    db.execute("PRAGMA journal_mode = DELETE")
    db.execute("PRAGMA page_size = 4096")
    db.execute("CREATE TABLE IF NOT EXISTS meta (key TEXT PRIMARY KEY, value TEXT NOT NULL)")
    return db


def set_meta(db: sqlite3.Connection, key: str, value) -> None:
    """Record one stage's JSON description, replacing any previous value."""
    db.execute(
        "INSERT INTO meta (key, value) VALUES (?, ?) "
        "ON CONFLICT(key) DO UPDATE SET value = excluded.value",
        (key, json.dumps(value, ensure_ascii=False, sort_keys=True)),
    )


def read_meta(db: sqlite3.Connection) -> dict:
    return {key: json.loads(value) for key, value in db.execute("SELECT key, value FROM meta")}


def load_extension(db: sqlite3.Connection, path: str | None = None) -> None:
    """Load sqlite-vec from an explicit path, `$SQLITE_VEC_EXT`, or `$SQLITE_VEC_LIB`."""
    candidates = [path] if path else []
    env = os.environ.get("SQLITE_VEC_EXT")
    if env:
        candidates.append(env)
    lib = os.environ.get("SQLITE_VEC_LIB")
    if lib:
        candidates.extend(str(p) for p in sorted(Path(lib).glob("vec0.*")))
    for candidate in candidates:
        if candidate and Path(candidate).exists():
            db.enable_load_extension(True)
            db.load_extension(candidate)
            db.enable_load_extension(False)
            return
    sys.exit(
        "demo_index: sqlite-vec is not on the path.\n"
        "  run inside `nix develop`, which exports SQLITE_VEC_LIB, or pass --sqlite-vec <vec0.dylib>"
    )


def build_manifest(final: Path, data: bytes, meta: dict) -> dict:
    """Assemble the one manifest: the text stage's base plus the media sections.

    The media models join `models[]` with their declared element type and
    dimension, so the client can find every table — text or media — through the
    same lookup.
    """
    base = dict(meta.get("index", {}))
    models = list(base.get("models", []))
    media = {}
    for kind in MEDIA_KINDS:
        section = meta.get("media." + kind)
        if not section:
            continue
        media[kind] = section
        models.append({
            "name": section["model"],
            "table": section["table"],
            "dimension": section["dimension"],
            "element_type": section["element_type"],
            "vector_type": section["vector_type"],
            "quantization": section["element_type"],
        })
    base["models"] = models
    if media:
        base["media"] = media
    if "fingerprints" in meta:
        base["fingerprints"] = meta["fingerprints"]
    base["asset"] = {
        "db": final.name,
        "hash": hashlib.sha256(data).hexdigest()[:8],
        "bytes": len(data),
    }
    base["generated_by"] = "website/tools/demo_index.py"
    return base


def canonicalise(data: bytes) -> bytes:
    """Zero SQLite's volatile header bookkeeping so the same content hashes the
    same.

    Three fields in the 100-byte header change on every write without changing
    a byte of readable content: the file change counter (24), the schema cookie
    (40) and the version-valid-for number (92). A build that starts from the
    previously published file — which is the point of the staged assembly —
    inherits whatever values that file carried, so two builds of identical
    content would otherwise name themselves differently. SQLite rewrites all
    three on its next write; nothing reads them from a deserialized copy.
    """
    buf = bytearray(data)
    for offset in (24, 40, 92):
        struct.pack_into(">I", buf, offset, 1)
    return bytes(buf)


def finalize() -> int:
    """Hash the working file, name it, and write the manifest."""
    if not WORK_DB.exists():
        sys.exit("demo_index: no working index to finalize (run a builder first)")
    db = sqlite3.connect(WORK_DB)
    db.execute("PRAGMA journal_mode = DELETE")
    # VACUUM rewrites every table, so the vec0 module must be loaded even though
    # finalize reads no vector: without it SQLite cannot reconstruct the virtual
    # tables it is compacting.
    load_extension(db)
    db.commit()
    db.execute("VACUUM")
    meta = read_meta(db)
    db.close()

    data = canonicalise(WORK_DB.read_bytes())
    WORK_DB.write_bytes(data)
    hash8 = hashlib.sha256(data).hexdigest()[:8]
    final = OUT_DIR / f"{ARTEFACT_PREFIX}{hash8}.db"
    WORK_DB.replace(final)

    manifest = build_manifest(final, data, meta)
    MANIFEST_PATH.write_text(
        json.dumps(manifest, ensure_ascii=False, indent=2, sort_keys=True) + "\n",
        encoding="utf-8",
    )
    # One index ships: a rebuild renames it, so the previous artefact (and any
    # other db that found its way here) must go.
    for stale in OUT_DIR.glob("*.db"):
        if stale != final:
            stale.unlink()
            print(f"removed stale {stale.name}")
    print(f"wrote {final.name} ({len(data)} bytes)")
    print(f"wrote {MANIFEST_PATH.name}")
    return 0


def main() -> int:
    if len(sys.argv) > 1 and sys.argv[1] == "finalize":
        return finalize()
    print(__doc__)
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
