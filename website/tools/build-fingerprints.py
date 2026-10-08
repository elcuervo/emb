#!/usr/bin/env python3
"""Build the music-fingerprint database the plate matches against.

A Shazam-style fingerprint is not an embedding: it is a sparse constellation of
spectrogram peaks, paired into `(f1, f2, dt)` hashes, each stored with the
offset in the track where it occurred. Matching a query is then a hash lookup
and a histogram of the offset differences -- the recording that shares hundreds
of hashes *at one consistent offset* wins, even from a noisy excerpt. That is
why a fingerprint says *this exact recording* while an embedding says *this kind
of thing*; the plate shows both, and this tool builds the first.

    nix develop .#website --command python3 website/tools/build-fingerprints.py
    # ffmpeg on PATH (or FFMPEG=/path/to/ffmpeg)

Outputs:

    website/assets/demo/fingerprints.json   the library's hashes and metadata
    website/demos/media/music/<id>.mp3      the shipped excerpts (input)

The library's own excerpts are prepared once with ffmpeg (trim, mono, mp3) and
committed; this tool reads them, so the shipped audio and the shipped hashes are
the same bytes. The *query* is fingerprinted live in the browser by the plate;
only the library is precomputed, exactly as the gallery ships its text index and
embeds the query live.
"""

from __future__ import annotations

import argparse
import hashlib
import json
import os
import shutil
import subprocess
import sys
from pathlib import Path

import numpy as np

REPO_ROOT = Path(__file__).resolve().parents[2]
MUSIC = REPO_ROOT / "website" / "demos" / "media" / "music"
OUT = REPO_ROOT / "website" / "assets" / "demo" / "fingerprints.json"

SR = 11025
N_FFT = 1024
HOP = 256
PEAK_NEIGHBOURHOOD = (2, 8)      # (time frames, frequency bins)
PEAKS_PER_SECOND = 40
FANOUT = 5                        # pairs per anchor
PAIR_WINDOW = 48                  # frames ahead (~1.1 s at HOP=256)
FREQ_BITS, TIME_BITS = 10, 10

# The shipped library. `file` is committed; `title`/`composer`/`licence` are
# written into the database so the page reads its credits from the data.
LIBRARY = [
    {"id": "brass", "file": "brass.mp3", "title": "Adjutant's Call", "composer": "Ceremonial Brass, United States Air Force Band", "licence": "Public domain"},
    {"id": "flute", "file": "flute.mp3", "title": "DiZi Chinese Flute Sample", "composer": "Gorgoroth6669", "licence": "CC0 1.0"},
    {"id": "steel", "file": "steel.mp3", "title": "Steel guitar playing Hawaiian music", "composer": "Eagledj", "licence": "CC0 1.0"},
    {"id": "guitar", "file": "guitar.mp3", "title": "PipingOfQueens", "composer": "U Can Unlearn Guitar", "licence": "CC0 1.0"},
    {"id": "piano", "file": "piano.mp3", "title": "Prelude No. 14 in E-flat minor, Op. 28", "composer": "Frédéric Chopin, performed by Ivan Ilić", "licence": "CC BY 3.0"},
]

# The shipped queries the plate offers: a 4 s excerpt of one library track, at
# an offset the visitor is not told. `track` is the pinned answer, and the build
# refuses to ship a query the matcher does not actually recover.
QUERIES = [
    {"id": "q1", "file": "queries/q1.mp3", "track": 1},
    {"id": "q2", "file": "queries/q2.mp3", "track": 2},
    {"id": "q3", "file": "queries/q3.mp3", "track": 3},
    {"id": "q4", "file": "queries/q4.mp3", "track": 4},
    {"id": "q5", "file": "queries/q5.mp3", "track": 0},
    {"id": "q6", "file": "queries/q6.mp3", "track": 2, "note": "the same steel-guitar excerpt, degraded with pink noise"},
]


def ffmpeg() -> str:
    return os.environ.get("FFMPEG") or shutil.which("ffmpeg") or "ffmpeg"


def decode(path: Path) -> np.ndarray:
    """Mono float32 at SR via ffmpeg, so the browser's decodeAudioData and this
    tool see the same samples."""
    out = subprocess.run(
        [ffmpeg(), "-v", "error", "-i", str(path), "-f", "f32le", "-ac", "1", "-ar", str(SR), "-"],
        check=True, capture_output=True,
    ).stdout
    return np.frombuffer(out, dtype="<f4").astype(np.float64)


def stft_db(x: np.ndarray) -> np.ndarray:
    win = np.hanning(N_FFT)
    frames = 1 + (len(x) - N_FFT) // HOP
    if frames < 1:
        raise ValueError("audio shorter than one analysis window")
    S = np.empty((N_FFT // 2 + 1, frames))
    for i in range(frames):
        S[:, i] = np.abs(np.fft.rfft(x[i * HOP:i * HOP + N_FFT] * win))
    return 20 * np.log10(S + 1e-6)


def pick_peaks(db: np.ndarray) -> list[tuple[int, int]]:
    """Local maxima in a time-frequency neighbourhood, density-capped."""
    nt, nf = db.shape[1], db.shape[0]
    dt, df = PEAK_NEIGHBOURHOOD
    threshold = np.percentile(db, 78)
    peaks: list[tuple[int, int, float]] = []
    for t in range(nt):
        lo_t, hi_t = max(0, t - dt), min(nt, t + dt + 1)
        for f in range(1, nf - 1):
            v = db[f, t]
            if v < threshold:
                continue
            window = db[max(0, f - df):f + df + 1, lo_t:hi_t]
            if v < window.max():
                continue
            peaks.append((t, f, v))
    cap = int(PEAKS_PER_SECOND * nt * HOP / SR) + 1
    peaks.sort(key=lambda p: -p[2])
    return [(t, f) for t, f, _ in peaks[:cap]]


def fingerprint(x: np.ndarray) -> dict[int, int]:
    """`hash -> anchor frame`, the form the browser also computes."""
    db = stft_db(x)
    peaks = pick_peaks(db)
    by_frame: dict[int, list[int]] = {}
    for t, f in peaks:
        by_frame.setdefault(t, []).append(f)
    frames = sorted(by_frame)
    out: dict[int, int] = {}
    for t in frames:
        for f1 in by_frame[t]:
            targets = [(tt, f2) for tt in frames if t < tt <= t + PAIR_WINDOW for f2 in by_frame[tt]]
            targets.sort(key=lambda p: p[0])
            for tt, f2 in targets[:FANOUT]:
                h = hash_of(f1, f2, tt - t)
                out.setdefault(h, t)
    return out


def hash_of(f1: int, f2: int, dt: int) -> int:
    return ((f1 & ((1 << FREQ_BITS) - 1)) << (FREQ_BITS + TIME_BITS)) \
        | ((f2 & ((1 << FREQ_BITS) - 1)) << TIME_BITS) \
        | (dt & ((1 << TIME_BITS) - 1))


def match(query: dict[int, int], db: dict) -> dict:
    """Hash lookup, then the offset histogram: the winning (track, offset)."""
    votes: dict[tuple[int, int], int] = {}
    for h, qt in query.items():
        for track, lt in db.get(str(h), ()):  # JSON keys are strings
            votes[(track, lt - qt)] = votes.get((track, lt - qt), 0) + 1
    if not votes:
        return {"track": None, "offset": 0, "votes": 0}
    (track, offset), n = max(votes.items(), key=lambda kv: kv[1])
    return {"track": track, "offset": int(offset), "votes": int(n)}


def self_test() -> int:
    """Build a tiny library from four distinct rich signals and recover a noisy
    excerpt of one. Proves the fingerprint, the hash, and the matcher agree."""
    t = np.arange(int(SR * 12)) / SR
    rng = np.random.default_rng(3)

    def instrument(base, seed):
        # A handful of partials with independent tremolo, so the spectrogram has
        # a real constellation of peaks rather than one ridge.
        r = np.random.default_rng(seed)
        x = np.zeros_like(t)
        for k in range(1, 8):
            f = base * k * (1 + 0.01 * r.standard_normal())
            env = 0.5 + 0.5 * np.sin(2 * np.pi * (0.7 + 0.5 * k) * t + r.uniform(0, 6))
            x += (0.9 ** k) * env * np.sin(2 * np.pi * f * t)
        return x + 0.05 * r.standard_normal(len(t))

    signals = [instrument(b, 10 + i) for i, b in enumerate([196, 261, 329, 392])]
    db = {}
    for i, sig in enumerate(signals):
        for h, lt in fingerprint(sig).items():
            db.setdefault(str(h), []).append([i, lt])
    seg = signals[2][int(SR * 4):int(SR * 8)] + 0.35 * rng.standard_normal(int(SR * 4))
    got = match(fingerprint(seg), db)
    ok = got["track"] == 2 and got["votes"] > 10
    print(f"self-test: matched track {got['track']} (want 2) with {got['votes']} votes -> {'ok' if ok else 'FAIL'}")
    return 0 if ok else 1


def main() -> int:
    parser = argparse.ArgumentParser()
    parser.add_argument("--self-test", action="store_true")
    parser.add_argument("--out", default=str(OUT))
    args = parser.parse_args()
    if args.self_test:
        return self_test()

    if not MUSIC.exists():
        sys.exit(f"build-fingerprints: no {MUSIC}")
    db: dict[str, list[list[int]]] = {}
    tracks = []
    for index, track in enumerate(LIBRARY):
        path = MUSIC / track["file"]
        if not path.exists():
            sys.exit(f"build-fingerprints: missing {path}")
        sig = decode(path)
        hashes = fingerprint(sig)
        for h, t in hashes.items():
            db.setdefault(str(h), []).append([index, t])
        tracks.append({**track, "seconds": round(len(sig) / SR, 2), "hashes": len(hashes),
                       "sha256": hashlib.sha256(path.read_bytes()).hexdigest()[:16]})
        print(f"  {track['id']}: {len(hashes)} hashes over {len(sig) / SR:.1f}s")

    # A shipped query whose answer the matcher cannot recover is a broken plate,
    # so the build refuses it rather than shipping a guess.
    queries = []
    for query in QUERIES:
        path = MUSIC / query["file"]
        if not path.exists():
            sys.exit(f"build-fingerprints: missing query {path}")
        got = match(fingerprint(decode(path)), db)
        ok = got["track"] == query["track"] and got["votes"] >= 20
        queries.append({**query, "detected": got["track"], "offset": got["offset"], "votes": got["votes"], "verified": ok})
        want = tracks[query["track"]]["id"] if query["track"] is not None else "?"
        print(f"  {query['id']}: -> {want} with {got['votes']} votes" + (" ok" if ok else " FAIL"))
        if not ok:
            sys.exit(f"build-fingerprints: {query['id']} did not match {want}")

    document = {
        "generated_by": "website/tools/build-fingerprints.py",
        "sample_rate": SR, "hop": HOP, "n_fft": N_FFT,
        "hash_bits": {"freq": FREQ_BITS, "time": TIME_BITS},
        "tracks": tracks,
        "queries": queries,
        "hashes": db,
    }
    payload = json.dumps({"tracks": tracks, "queries": queries, "hashes": db}, sort_keys=True, separators=(",", ":"))
    document["sha256"] = hashlib.sha256(payload.encode()).hexdigest()
    out = Path(args.out)
    out.write_text(json.dumps(document, separators=(",", ":")) + "\n", encoding="utf-8")
    print(f"wrote {out} ({len(db)} distinct hashes, {len(tracks)} tracks, {out.stat().st_size // 1024} KiB)")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
