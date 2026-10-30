#!/usr/bin/env python3
"""Measure photographs through the sandbox's depth model and apply the plate's gate.

The six photographs shipped with PLATE VII are chosen by this script, not by eye:
each file is bounded exactly as the page bounds it (768 px on the long edge, JPEG
at quality 0.85, stepped down until it fits the bridge's cap), sent through
`EMB.IMG depth BLOB OUTPUTS depth`, and reduced to the numbers `design.md` D27
records — the p5–p95 spread of the min–max normalised map, the share of the frame
in the near and far deciles, the gradient energy, and the reply's own range.

    nix develop .#website            # Pillow and numpy
    just website-dev                 # emb on 6379, bridge on 8081 (another shell)
    python3 website/tools/depth-gate.py website/demos/samples/parts

Exit status is 1 when any file fails the gate, so a re-curation cannot quietly
ship a flat frame. `--json` prints the table as data; `--all` lists failures too.
"""
import argparse
import base64
import io
import json
import sys
import urllib.request
from pathlib import Path

import numpy as np
from PIL import Image

MAX_EDGE = 768          # the page's long edge
MAX_BYTES = 200_000     # the page's own cap, under the bridge's 256 KiB
GATE = {"spread": 0.60, "near": 0.01, "far": 0.01, "gradient": 0.006}


def bound(path):
    """The page's bounding step, byte for byte: 768 px long edge, JPEG q0.85."""
    img = Image.open(path).convert("RGB")
    scale = MAX_EDGE / max(img.width, img.height)
    w, h = max(1, round(img.width * scale)), max(1, round(img.height * scale))
    small = img.resize((w, h), Image.LANCZOS)
    quality = 85
    buf = io.BytesIO()
    small.save(buf, "JPEG", quality=quality, optimize=True)
    while buf.tell() > MAX_BYTES and quality > 35:
        quality -= 10
        buf = io.BytesIO()
        small.save(buf, "JPEG", quality=quality, optimize=True)
    return small.size, buf.getvalue()


def measure(bridge, origin, payload):
    body = json.dumps({
        "args": ["EMB.IMG", "depth", "BLOB", "OUTPUTS", "depth", base64.b64encode(payload).decode()],
        "bin": [5], "proto": 3,
    }).encode()
    req = urllib.request.Request(bridge, data=body,
                                 headers={"Content-Type": "application/json", "Origin": origin})
    reply = json.loads(urllib.request.urlopen(req, timeout=180).read())
    if reply.get("kind") != "map":
        raise SystemExit("the bridge refused the call: " + str(reply.get("text") or reply)[:200])
    raw = base64.b64decode(reply["elems"][1]["b64"])
    values = np.frombuffer(raw, dtype="<f4")
    side = int(round(values.size ** 0.5))
    if side * side != values.size:
        raise SystemExit(f"the reply is {values.size} values, which is not a square map")
    grid = values.reshape(side, side)
    lo, hi = float(grid.min()), float(grid.max())
    norm = (grid - lo) / ((hi - lo) or 1)
    p5, p95 = np.percentile(norm, [5, 95])
    gy, gx = np.gradient(norm)
    return {
        "values": int(values.size),
        "bytes": len(raw),
        "spread": float(p95 - p5),
        "near": float((norm < 0.10).mean()),
        "far": float((norm > 0.90).mean()),
        "gradient": float(np.hypot(gx, gy).mean()),
        "min": lo, "median": float(np.median(grid)), "max": hi,
        "elapsed_us": reply.get("elapsed_us", 0),
    }


def gate_of(row):
    failed = [name for name, limit in GATE.items() if row[name] < limit]
    return failed


def main():
    ap = argparse.ArgumentParser(description=__doc__.splitlines()[0])
    ap.add_argument("paths", nargs="+", type=Path, help="JPEG files or directories of them")
    ap.add_argument("--bridge", default="http://127.0.0.1:8081/api/exec")
    ap.add_argument("--origin", default="http://127.0.0.1:8080")
    ap.add_argument("--json", action="store_true", help="print the table as JSON")
    ap.add_argument("--all", action="store_true", help="list the failures and why")
    args = ap.parse_args()

    files = []
    for path in args.paths:
        files.extend(sorted(path.glob("*.jpg")) if path.is_dir() else [path])
    if not files:
        raise SystemExit("no .jpg files in " + ", ".join(str(p) for p in args.paths))

    rows, refused = [], 0
    for path in files:
        (w, h), payload = bound(path)
        row = {"file": path.name, "width": w, "height": h, "quality_bytes": len(payload)}
        try:
            row.update(measure(args.bridge, args.origin, payload))
        except OSError as err:
            raise SystemExit(f"{path.name}: the bridge did not answer ({err}); is `just website-dev` up?")
        row["gate"] = gate_of(row)
        rows.append(row)
        if row["gate"]:
            refused += 1

    if args.json:
        print(json.dumps(rows, indent=1))
    else:
        print(f'{"file":24s} {"size":>9s} {"spread":>6s} {"near":>5s} {"far":>5s} {"grad":>6s}  '
              f'{"min / median / max":>20s}  gate')
        for r in rows:
            state = "pass" if not r["gate"] else "FAIL " + ",".join(r["gate"])
            print(f'{r["file"]:24s} {r["width"]}x{r["height"]:<4d} {r["spread"]:.3f} {r["near"]:.3f} '
                  f'{r["far"]:.3f} {r["gradient"]:.4f}  '
                  f'{r["min"]:.3f} / {r["median"]:.3f} / {r["max"]:.3f}  {state}')
        print(f'\n{len(rows)} measured, {len(rows) - refused} pass '
              f'(spread ≥ {GATE["spread"]}, near ≥ {GATE["near"]}, far ≥ {GATE["far"]}, '
              f'gradient ≥ {GATE["gradient"]})')
    return 1 if refused else 0


if __name__ == "__main__":
    sys.exit(main())
