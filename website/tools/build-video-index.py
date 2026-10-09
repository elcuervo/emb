#!/usr/bin/env python3
"""Embed the video library with X-CLIP into the gallery's shared index.

The video half of the plate, the real way: each committed clip is sampled into
**eight** frames (the export's fixed contract), resized and centre-cropped to
224x224 RGB in [0,1] — X-CLIP's own mean/std are inside the graph — and packed
`[1,8,3,224,224]` float32 into the X-CLIP **video tower** through the build-time
preset. The query is a phrase embedded by the X-CLIP **text tower** live in the
sandbox, and the two share a 512-d space. The video vectors are appended as
`vec_xclip(embedding float[512])` in the gallery's one index.

    just website-media-models        # needs a local emb with xclip-video/xclip-text

A pinned evaluation runs at the end: every phrase must retrieve its clip, and the
build fails if one does not.
"""

from __future__ import annotations

import argparse
import hashlib
import os
import shutil
import sqlite3
import subprocess
import sys
import tempfile
from pathlib import Path

import numpy as np
from PIL import Image

import demo_index

REPO_ROOT = Path(__file__).resolve().parents[2]
VIDEO = REPO_ROOT / "website" / "demos" / "media" / "video"
VIDEO_PRESET = REPO_ROOT / "scripts" / "xclip_video.lua"
TEXT_PRESET = REPO_ROOT / "scripts" / "xclip_text.lua"
DEFAULT_EMB = "127.0.0.1:16401"
FRAMES, SIDE = 8, 224

LIBRARY = [
    {"id": "volcano", "file": "volcano.mp4", "title": "Sarychev Peak eruption from the ISS", "source": "NASA", "licence": "Public domain"},
    {"id": "moon", "file": "moon.mp4", "title": "Moon transit of the sun (STEREO-B)", "source": "NASA", "licence": "Public domain"},
    {"id": "arecibo", "file": "arecibo.mp4", "title": "Collapse of the Arecibo Radio Telescope", "source": "NSF", "licence": "Public domain"},
    {"id": "turtle", "file": "turtle.mp4", "title": "An olive ridley turtle close up", "source": "USFWS", "licence": "Public domain"},
    {"id": "launch", "file": "launch.mp4", "title": "STS-134 launch", "source": "NASA", "licence": "Public domain"},
    {"id": "storm", "file": "storm.mp4", "title": "A February 2011 storm crossing the U.S.", "source": "NOAA/NASA", "licence": "Public domain"},
]

QUERIES = [
    {"text": "a rocket launching into the sky", "clip": 4},
    {"text": "a sea turtle swimming underwater", "clip": 3},
    {"text": "a volcanic eruption", "clip": 0},
    {"text": "a storm system over land", "clip": 5},
    {"text": "the moon passing in front of the sun", "clip": 1},
    {"text": "a giant telescope falling apart", "clip": 2},
]


def ffmpeg() -> str:
    return os.environ.get("FFMPEG") or shutil.which("ffmpeg") or "ffmpeg"


def ffprobe() -> str:
    return os.environ.get("FFPROBE") or str(Path(ffmpeg()).with_name("ffprobe"))


def duration(path: Path) -> float:
    return float(subprocess.run([ffprobe(), "-v", "error", "-show_entries", "format=duration", "-of", "csv=p=0", str(path)],
                                check=True, capture_output=True, text=True).stdout.strip())


def frames_of(path: Path) -> np.ndarray:
    """`[8, 3, 224, 224]` float32 in [0,1]: uniform sampling, resize + centre crop."""
    dur = duration(path)
    with tempfile.TemporaryDirectory() as tmp:
        subprocess.run([ffmpeg(), "-y", "-v", "error", "-i", str(path), "-vf",
                        f"fps={FRAMES}/{dur},scale={SIDE}:{SIDE}:force_original_aspect_ratio=increase,crop={SIDE}:{SIDE}",
                        "-frames:v", str(FRAMES), os.path.join(tmp, "%02d.png")], check=True)
        pngs = sorted(Path(tmp).glob("*.png"))
        if len(pngs) != FRAMES:
            raise SystemExit(f"{path.name}: expected {FRAMES} frames, got {len(pngs)}")
        out = np.stack([np.asarray(Image.open(p).convert("RGB"), dtype=np.float32) / 255.0 for p in pngs])
    return np.transpose(out, (0, 3, 1, 2))  # [T, C, H, W]


def load_resp():
    import importlib.util
    spec = importlib.util.spec_from_file_location("bfi", REPO_ROOT / "website" / "tools" / "build-frame-index.py")
    mod = importlib.util.module_from_spec(spec)
    spec.loader.exec_module(mod)
    return mod


def main() -> int:
    parser = argparse.ArgumentParser()
    parser.add_argument("--emb", default=os.environ.get("EMB_ADDR", DEFAULT_EMB))
    args = parser.parse_args()

    m = load_resp()
    host, _, port = args.emb.partition(":")
    conn = m.Resp(host or "127.0.0.1", int(port or "6379"))
    v_sha = hashlib.sha1(VIDEO_PRESET.read_bytes()).hexdigest()
    t_sha = hashlib.sha1(TEXT_PRESET.read_bytes()).hexdigest()
    try:
        conn.call("HELLO", "3")
        for model, sha in (("xclip-video", v_sha), ("xclip-text", t_sha)):
            if not m.truthy(conn.call("EMB.SCRIPT", "EXISTS", model, sha)):
                sys.exit(f"build-video-index: {model} is not loaded on {args.emb}")

        clips, vectors = [], []
        for clip in LIBRARY:
            path = VIDEO / clip["file"]
            if not path.exists():
                sys.exit(f"build-video-index: missing {path}")
            packed = frames_of(path).astype("<f4").tobytes()
            vec = m.vector_of(conn.call("EMB.EVSHA", "xclip-video", v_sha, "1", packed))
            vectors.append(vec)
            clips.append({**clip, "frames": FRAMES, "sha256": hashlib.sha256(path.read_bytes()).hexdigest()[:16]})
            print(f"  embedded {clip['id']}", file=sys.stderr)

        matrix = np.vstack(vectors)
        matrix = matrix / np.linalg.norm(matrix, axis=1, keepdims=True)
        queries = []
        for q in QUERIES:
            text = m.vector_of(conn.call("EMB.EVSHA", "xclip-text", t_sha, "1", q["text"]))
            scores = matrix @ (text / np.linalg.norm(text))
            got = int(np.argmax(scores))
            ok = got == q["clip"]
            queries.append({**q, "detected": got, "score": round(float(scores[got]), 4), "verified": ok})
            want = LIBRARY[q["clip"]]["id"]
            print(f"  {q['text']!r} -> {LIBRARY[got]['id']} (want {want}) {scores[got]:.3f}" + (" ok" if ok else " FAIL"))
            if not ok:
                sys.exit(f"build-video-index: {q['text']!r} did not retrieve {want}")
    finally:
        conn.close()

    document = {
        "model": "xclip",
        "dimension": int(matrix.shape[1]),
        "frames": FRAMES,
        "clips": clips,
        "queries": queries,
    }
    db = demo_index.open_working()
    demo_index.load_extension(db)
    db.execute("DROP TABLE IF EXISTS vec_xclip")
    db.execute("DROP TABLE IF EXISTS video_clips")
    db.execute("CREATE VIRTUAL TABLE vec_xclip USING vec0(embedding float[512])")
    db.execute("CREATE TABLE video_clips (rowid INTEGER PRIMARY KEY, id TEXT, title TEXT, source TEXT, "
               "licence TEXT, file TEXT, frames INTEGER, sha256 TEXT)")
    for i, (clip, vector) in enumerate(zip(clips, matrix), start=1):
        db.execute("INSERT INTO video_clips (rowid, id, title, source, licence, file, frames, sha256) "
                   "VALUES (?, ?, ?, ?, ?, ?, ?, ?)",
                   (i, clip["id"], clip["title"], clip["source"], clip["licence"],
                    clip["file"], clip["frames"], clip["sha256"]))
        db.execute("INSERT INTO vec_xclip (rowid, embedding) VALUES (?, vec_f32(?))",
                   (i, sqlite3.Binary(vector.astype("<f4").tobytes())))
    demo_index.set_meta(db, "media.video", {
        "model": document["model"],
        "table": "vec_xclip",
        "element_type": "float32",
        "vector_type": "vec_f32",
        "dimension": document["dimension"],
        "frames": FRAMES,
        "clips": [{**clip, "rowid": i + 1} for i, clip in enumerate(clips)],
        "queries": queries,
        "generated_by": "website/tools/build-video-index.py",
    })
    db.commit()
    db.close()
    print(f"wrote {len(clips)} clips into {demo_index.WORK_DB.relative_to(REPO_ROOT)}")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
