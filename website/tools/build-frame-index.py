#!/usr/bin/env python3
"""Embed a clip frame by frame, so the plate can search it by a phrase.

This is the video half of the plate: a clip is sampled into a strip of stills,
each still is embedded by the sandbox's own vision model (through the build-only
`clip_embed.lua` preset, against a local `emb`), and the vectors ship with the
frames. The page embeds a typed phrase live and ranks the shipped frames, so the
picture it highlights is the picture that was measured.

    just website-media        # after: just download-model-quantized repo=Xenova/clip-vit-base-patch32 dir=./models/clip

Outputs:

    website/demos/media/frames/<clip>-<i>.jpg   the shipped stills
    website/assets/demo/frames.json             timestamps, stills, 512-d vectors
"""

from __future__ import annotations

import argparse
import base64
import hashlib
import json
import os
import shutil
import socket
import subprocess
import sys
from pathlib import Path

import numpy as np

REPO_ROOT = Path(__file__).resolve().parents[2]
SITE = REPO_ROOT / "website"
FRAMES_DIR = SITE / "demos" / "media" / "frames"
OUT = SITE / "assets" / "demo" / "frames.json"
EMBED_PRESET = REPO_ROOT / "scripts" / "clip_embed.lua"
DEFAULT_EMB = "127.0.0.1:16400"
DEFAULT_CLIP = SITE / "demos" / "media" / "clips" / "mallard.mp4"
COUNT = 16
WIDTH, HEIGHT = 224, 126


class RespError(RuntimeError):
    pass


class Resp:
    """The smallest RESP3 client that can drive a scripted model and read bytes."""

    def __init__(self, host: str, port: int, timeout: float = 300.0):
        self.sock = socket.create_connection((host, port), timeout=timeout)
        self.buf = b""

    def close(self) -> None:
        self.sock.close()

    def send(self, *args) -> None:
        out = [b"*%d\r\n" % len(args)]
        for a in args:
            b = a if isinstance(a, bytes) else str(a).encode()
            out += [b"$%d\r\n" % len(b), b, b"\r\n"]
        self.sock.sendall(b"".join(out))

    def _line(self) -> bytes:
        while b"\r\n" not in self.buf:
            chunk = self.sock.recv(1 << 16)
            if not chunk:
                raise RespError("connection closed")
            self.buf += chunk
        line, self.buf = self.buf.split(b"\r\n", 1)
        return line

    def _take(self, n: int) -> bytes:
        while len(self.buf) < n:
            chunk = self.sock.recv(1 << 16)
            if not chunk:
                raise RespError("connection closed")
            self.buf += chunk
        data, self.buf = self.buf[:n], self.buf[n:]
        return data

    def read(self):
        line = self._line()
        kind, rest = line[:1], line[1:]
        if kind == b"+":
            return rest.decode()
        if kind == b"-":
            raise RespError(rest.decode())
        if kind == b":":
            return int(rest)
        if kind == b"$":
            n = int(rest)
            return None if n == -1 else self._take(n + 2)[:-2]
        if kind == b"*":
            n = int(rest)
            return None if n == -1 else [self.read() for _ in range(n)]
        if kind == b"%":
            return {self.read(): self.read() for _ in range(int(rest))}
        if kind == b"~":
            return [self.read() for _ in range(int(rest))]
        if kind == b",":
            return float(rest)
        if kind == b"_":
            return None
        if kind == b"#":
            return rest == b"t"
        raise RespError(f"unexpected RESP type {kind!r}")

    def call(self, *args):
        self.send(*args)
        return self.read()


def truthy(value) -> bool:
    if isinstance(value, list):
        value = value[0] if value else False
    return value in (1, "1", True, b"1")


def as_hash(reply) -> dict:
    if isinstance(reply, dict):
        return reply
    if isinstance(reply, list) and len(reply) % 2 == 0:
        return {reply[i]: reply[i + 1] for i in range(0, len(reply), 2)}
    raise RespError(f"expected a string-keyed table, got {type(reply).__name__}")


def vector_of(reply) -> np.ndarray:
    table = as_hash(reply)
    raw = table.get(b"bytes") or table.get("bytes")
    if raw is None:
        raise RespError("reply carries no bytes")
    return np.frombuffer(raw, dtype="<f4").astype(np.float64)


def ffmpeg() -> str:
    return os.environ.get("FFMPEG") or shutil.which("ffmpeg") or "ffmpeg"


def ffprobe() -> str:
    named = os.environ.get("FFPROBE")
    if named:
        return named
    return str(Path(ffmpeg()).with_name("ffprobe")) if os.path.sep in ffmpeg() else "ffprobe"


def main() -> int:
    parser = argparse.ArgumentParser()
    parser.add_argument("--emb", default=os.environ.get("EMB_ADDR", DEFAULT_EMB))
    parser.add_argument("--clip", default=str(DEFAULT_CLIP))
    parser.add_argument("--count", type=int, default=COUNT)
    parser.add_argument("--out", default=str(OUT))
    args = parser.parse_args()

    clip = Path(args.clip)
    if not clip.exists():
        sys.exit(f"build-frame-index: no {clip}")
    duration = float(subprocess.run(
        [ffprobe(), "-v", "error", "-show_entries", "format=duration",
         "-of", "csv=p=0", str(clip)], check=True, capture_output=True, text=True).stdout.strip())

    FRAMES_DIR.mkdir(parents=True, exist_ok=True)
    times = [round(duration * (i + 0.5) / args.count, 3) for i in range(args.count)]
    stills = []
    for i, t in enumerate(times):
        path = FRAMES_DIR / f"{clip.stem}-{i:02d}.jpg"
        subprocess.run(
            [ffmpeg(), "-y", "-v", "error", "-ss", f"{t}", "-i", str(clip),
             "-frames:v", "1", "-vf", f"scale={WIDTH}:{HEIGHT}", "-q:v", "4", str(path)], check=True)
        stills.append(path)

    host, _, port = args.emb.partition(":")
    conn = Resp(host or "127.0.0.1", int(port or "6379"))
    try:
        conn.call("HELLO", "3")
        sha = hashlib.sha1(EMBED_PRESET.read_bytes()).hexdigest()
        if not truthy(conn.call("EMB.SCRIPT", "EXISTS", "clip", sha)):
            sys.exit(f"build-frame-index: the {sha[:8]} embedding preset is not loaded on {args.emb}")
        vectors = []
        for path in stills:
            vectors.append(vector_of(conn.call("EMB.EVSHA", "clip", sha, "1", path.read_bytes())))
            print(f"  embedded {path.name}", file=sys.stderr)
    finally:
        conn.close()

    matrix = np.vstack(vectors)
    document = {
        "generated_by": "website/tools/build-frame-index.py",
        "model": "clip",
        "dimension": int(matrix.shape[1]),
        "clip": clip.name,
        "duration": round(duration, 3),
        "frames": [
            {"t": times[i], "picture": f"media/frames/{p.name}",
             "vector": base64.b64encode(matrix[i].astype("<f4").tobytes()).decode()}
            for i, p in enumerate(stills)
        ],
    }
    payload = json.dumps({"clip": document["clip"], "frames": document["frames"]}, sort_keys=True, separators=(",", ":"))
    document["sha256"] = hashlib.sha256(payload.encode()).hexdigest()
    out = Path(args.out)
    out.write_text(json.dumps(document) + "\n", encoding="utf-8")
    print(f"wrote {out} ({len(stills)} frames, {document['dimension']}d)")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
