#!/usr/bin/env python3
"""Derive an alpha matte from the generated terrain artwork.

`assets/img/terrain-v2.png` is a two-tone illustration: near-black rock on a
flat warm off-white ground, with no alpha channel. The page used to fake
transparency with `mix-blend-mode: multiply` + `filter: grayscale() brightness()`
+ a polygon `clip-path`. That is a backdrop-dependent composite: whenever the
image lands in its own stacking context the backdrop is empty, multiply has
nothing to darken, and the raw off-white ground paints as an opaque rectangle
until something forces a repaint. It also puts a full-width filtered, blended
layer on the compositor — the expensive kind on phones.

This tool bakes the same grade into the pixels and writes a real cut-out:

    alpha = 1 - L / white        (clamped)
    rgb   = 0  (black)

Compositing black at that alpha over the paper reproduces `multiply` exactly:
`paper * (1 - a) == paper * L/white`. With `white` set to the artwork's own
ground level the background lands on alpha 0, so it cannot show as a rectangle
no matter what the compositor does.

    python3 tools/gen-terrain-matte.py            # report only
    python3 tools/gen-terrain-matte.py --write    # rewrite the matte asset
"""

import argparse
from pathlib import Path
import sys

sys.path.insert(0, str(Path(__file__).resolve().parent))
import png_lib  # noqa: E402

HERE = Path(__file__).resolve().parent
SOURCE = HERE.parent / "assets" / "img" / "terrain-v2.png"
TARGET = HERE.parent / "assets" / "img" / "terrain-matte.png"

# The artwork is 2172 x 724; the page gives it a 2172 x 770.2 box, the extra
# 46.2 units at the top being sky that lets the route start level with the
# band's top edge (see the comment in index.html).
BOX_RATIO = 770.2 / 2172.0


def luminance(r, g, b):
    """sRGB-weighted grey, the same quantity CSS `grayscale(1)` produces."""
    return (r * 54 + g * 183 + b * 19) >> 8


def sky_samples(plane):
    """Luminance of pixels that are certainly ground, not rock.

    The rock touches every edge of the frame, so the sample is the top strip
    plus the upper third of the two side columns.
    """
    w, h = plane.width, plane.height
    out = []
    for y in range(0, 8):
        for x in range(w):
            out.append(luminance(*plane.rgb(x, y)))
    for x in list(range(0, 8)) + list(range(w - 8, w)):
        for y in range(0, int(h * 0.35)):
            out.append(luminance(*plane.rgb(x, y)))
    return out


def main():
    ap = argparse.ArgumentParser()
    ap.add_argument("--write", action="store_true", help="write %s" % TARGET.name)
    ap.add_argument("--percentile", type=float, default=1.0,
                    help="sky percentile used as the white point (default 1)")
    args = ap.parse_args()

    plane = png_lib.read(SOURCE)
    w, h = plane.width, plane.height
    samples = sorted(sky_samples(plane))
    white = samples[max(0, int(len(samples) * args.percentile / 100.0) - 1)]

    print("source      %s  %d x %d  %s" % (SOURCE.name, w, h, plane.mode))
    print("sky sample  %d px  min %d  p1 %d  median %d  max %d"
          % (len(samples), samples[0], white, samples[len(samples) // 2], samples[-1]))
    print("white point %d  (alpha = 1 - L/%d)" % (white, white))

    # grey+alpha, grey pinned to 0: the matte carries the whole picture
    out = bytearray(w * h * 2)
    lut = [max(0, min(255, 255 - round(v * 255.0 / white))) for v in range(256)]
    for y in range(h):
        row = y * w
        for x in range(w):
            r, g, b = plane.rgb(x, y)
            i = (row + x) * 2
            out[i] = 0
            out[i + 1] = lut[luminance(r, g, b)]

    # ---- verification -------------------------------------------------
    # 1. the ground must vanish. The rock touches every edge, so "sky" is
    #    measured against the silhouette: every pixel above the topmost
    #    clearly-inked one in its column.
    worst = 0
    sky = 0
    for x in range(w):
        crest = None
        for y in range(h):
            if out[((y * w + x) * 2) + 1] > 24:
                crest = y
                break
        if crest is None:
            continue
        for y in range(0, max(0, crest - 6)):
            sky += 1
            a = out[((y * w + x) * 2) + 1]
            if a > worst:
                worst = a
    loud = 0
    for x in range(w):
        crest = None
        for y in range(h):
            if out[((y * w + x) * 2) + 1] > 24:
                crest = y
                break
        if crest is None:
            continue
        for y in range(0, max(0, crest - 6)):
            if out[((y * w + x) * 2) + 1] > 8:
                loud += 1
    print("sky residual        max %d/255, %d px above 8 over %d sky px (%.4f%%)"
          % (worst, loud, sky, 100.0 * loud / max(1, sky)))
    # Isolated single-pixel grain left in the ground is far below the paper
    # texture and cannot read as a rectangle; a connected blob is a bug.
    if loud > max(64, sky // 10000):
        raise SystemExit("ground is not transparent: %d sky px above alpha 8" % loud)

    # 2. the cut-out must carry the same picture as the old multiply chain
    #    paper x clamp(L * 1.08) / 255   vs   paper x (1 - alpha/255)
    paper = (0xF3, 0xF0, 0xE8)
    total = 0
    peak = 0
    for y in range(0, h, 4):
        for x in range(0, w, 4):
            L = luminance(*plane.rgb(x, y))
            old = min(255, L * 1.08) / 255.0
            new = 1.0 - out[((y * w + x) * 2) + 1] / 255.0
            d = abs(old - new) * 255.0
            total += d
            if d > peak:
                peak = d
    n = (h // 4 + 1) * (w // 4 + 1)
    print("vs old multiply     mean %.2f/255  peak %.1f/255" % (total / n, peak))

    opaque = sum(1 for i in range(1, len(out), 2) if out[i] > 127)
    print("rock coverage       %.1f%% of the frame" % (100.0 * opaque / (w * h)))
    print("box                 aspect %.5f (%.1f x %.1f units)" % (BOX_RATIO, w, w * BOX_RATIO))

    if args.write:
        size = png_lib.write(TARGET, w, h, "greya", out)
        before = SOURCE.stat().st_size
        print("wrote       %s  %.2f MB  (%.0f%% of %s)"
              % (TARGET.name, size / 1048576.0, 100.0 * size / before, SOURCE.name))
        preview = Path("/tmp/terrain-matte-preview.png")
        flat = bytearray(w * h * 3)
        for i in range(w * h):
            a = out[i * 2 + 1]
            for c in range(3):
                flat[i * 3 + c] = (paper[c] * (255 - a)) // 255
        png_lib.write(preview, w, h, "rgb", flat)
        print("preview     %s  (matte composited over the page paper)" % preview)


if __name__ == "__main__":
    main()
