#!/usr/bin/env python3
"""Embed the music library with CLAP and write the shipped audio index.

The audio half of the plate, the real way: each committed excerpt is decoded to
48 kHz mono, turned into the CLAP log-mel by `clap_mel.py` (validated against
Transformers' own feature extractor), and embedded by the CLAP **audio tower**
through the build-time preset. The query is a phrase embedded by the CLAP **text
tower** live in the sandbox. Both land in one 512-d space.

    just website-media-models        # needs a local emb with clap-audio/clap-text

    EMB.EVSHA clap-audio <sha1> 1 <packed f32 mel>   # per excerpt, here
    EMB.EVSHA clap-text  <sha1> 1 "a solo piano"     # per query, on the server

A pinned evaluation runs at the end: every query must retrieve its expected
track, and the build fails if one does not, so a changed mel or model is a
decision rather than a silent regression.
"""

from __future__ import annotations

import argparse
import base64
import hashlib
import json
import os
import shutil
import subprocess
import sys
from pathlib import Path

import numpy as np

sys.path.insert(0, str(Path(__file__).resolve().parent))
import clap_mel  # noqa: E402

REPO_ROOT = Path(__file__).resolve().parents[2]
MUSIC = REPO_ROOT / "website" / "demos" / "media" / "music"
OUT = REPO_ROOT / "website" / "assets" / "demo" / "audio.json"
AUDIO_PRESET = REPO_ROOT / "scripts" / "clap_audio.lua"
TEXT_PRESET = REPO_ROOT / "scripts" / "clap_text.lua"
DEFAULT_EMB = "127.0.0.1:16401"

LIBRARY = [
    {"id": "brass", "file": "brass.mp3", "title": "Adjutant's Call", "artist": "Ceremonial Brass, U.S. Air Force Band", "licence": "Public domain"},
    {"id": "flute", "file": "flute.mp3", "title": "DiZi Chinese Flute Sample", "artist": "Gorgoroth6669", "licence": "CC0 1.0"},
    {"id": "steel", "file": "steel.mp3", "title": "Steel guitar playing Hawaiian music", "artist": "Eagledj", "licence": "CC0 1.0"},
    {"id": "guitar", "file": "guitar.mp3", "title": "PipingOfQueens", "artist": "U Can Unlearn Guitar", "licence": "CC0 1.0"},
    {"id": "piano", "file": "piano.mp3", "title": "Prelude No. 14 in E-flat minor, Op. 28", "artist": "Chopin, performed by Ivan Ilić", "licence": "CC BY 3.0"},
]

# The pinned evaluation: a phrase and the track it must retrieve.
QUERIES = [
    {"text": "a brass band playing a fanfare", "track": 0},
    {"text": "a solo bamboo flute", "track": 1},
    {"text": "a lap steel guitar", "track": 2},
    {"text": "a fast dance tune", "track": 3},
    {"text": "a solo piano", "track": 4},
]


def ffmpeg() -> str:
    return os.environ.get("FFMPEG") or shutil.which("ffmpeg") or "ffmpeg"


def decode48(path: Path) -> np.ndarray:
    out = subprocess.run(
        [ffmpeg(), "-v", "error", "-i", str(path), "-f", "f32le", "-ac", "1", "-ar", "48000", "-"],
        check=True, capture_output=True).stdout
    return np.frombuffer(out, dtype="<f4")


def load_resp():
    import importlib.util
    spec = importlib.util.spec_from_file_location("bfi", REPO_ROOT / "website" / "tools" / "build-frame-index.py")
    mod = importlib.util.module_from_spec(spec)
    spec.loader.exec_module(mod)
    return mod


def main() -> int:
    parser = argparse.ArgumentParser()
    parser.add_argument("--emb", default=os.environ.get("EMB_ADDR", DEFAULT_EMB))
    parser.add_argument("--out", default=str(OUT))
    args = parser.parse_args()

    m = load_resp()
    host, _, port = args.emb.partition(":")
    conn = m.Resp(host or "127.0.0.1", int(port or "6379"))
    a_sha = hashlib.sha1(AUDIO_PRESET.read_bytes()).hexdigest()
    t_sha = hashlib.sha1(TEXT_PRESET.read_bytes()).hexdigest()
    try:
        conn.call("HELLO", "3")
        for model, preset in (("clap-audio", AUDIO_PRESET), ("clap-text", TEXT_PRESET)):
            if not m.truthy(conn.call("EMB.SCRIPT", "EXISTS", model, hashlib.sha1(preset.read_bytes()).hexdigest())):
                sys.exit(f"build-audio-index: {model} is not loaded on {args.emb}")

        tracks, vectors = [], []
        for track in LIBRARY:
            path = MUSIC / track["file"]
            if not path.exists():
                sys.exit(f"build-audio-index: missing {path}")
            mel = clap_mel.packed(decode48(path))
            vec = m.vector_of(conn.call("EMB.EVSHA", "clap-audio", a_sha, "1", mel))
            vectors.append(vec)
            tracks.append({**track, "seconds": round(seconds_of(path), 2),
                           "sha256": hashlib.sha256(path.read_bytes()).hexdigest()[:16]})
            print(f"  embedded {track['id']}", file=sys.stderr)

        matrix = np.vstack(vectors)
        matrix = matrix / np.linalg.norm(matrix, axis=1, keepdims=True)
        queries = []
        for q in QUERIES:
            text = m.vector_of(conn.call("EMB.EVSHA", "clap-text", t_sha, "1", q["text"]))
            scores = matrix @ (text / np.linalg.norm(text))
            got = int(np.argmax(scores))
            ok = got == q["track"]
            queries.append({**q, "detected": got, "score": round(float(scores[got]), 4), "verified": ok})
            want = LIBRARY[q["track"]]["id"]
            print(f"  {q['text']!r} -> {LIBRARY[got]['id']} (want {want}) {scores[got]:.3f}" + (" ok" if ok else " FAIL"))
            if not ok:
                sys.exit(f"build-audio-index: {q['text']!r} did not retrieve {want}")
    finally:
        conn.close()

    document = {
        "generated_by": "website/tools/build-audio-index.py",
        "model": "clap",
        "dimension": int(matrix.shape[1]),
        "tracks": tracks,
        "queries": queries,
        "vectors": {LIBRARY[i]["id"]: base64.b64encode(matrix[i].astype("<f4").tobytes()).decode()
                    for i in range(len(LIBRARY))},
    }
    payload = json.dumps({"tracks": tracks, "queries": queries, "vectors": document["vectors"]},
                         sort_keys=True, separators=(",", ":"))
    document["sha256"] = hashlib.sha256(payload.encode()).hexdigest()
    out = Path(args.out)
    out.write_text(json.dumps(document) + "\n", encoding="utf-8")
    print(f"wrote {out} ({len(tracks)} tracks, {len(queries)} pinned queries)")
    return 0


def seconds_of(path: Path) -> float:
    probe = os.environ.get("FFPROBE") or str(Path(ffmpeg()).with_name("ffprobe"))
    return float(subprocess.run([probe, "-v", "error", "-show_entries", "format=duration",
                                 "-of", "csv=p=0", str(path)], check=True, capture_output=True, text=True).stdout.strip())


if __name__ == "__main__":
    raise SystemExit(main())
