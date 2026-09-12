#!/usr/bin/env python3
"""Generate the four-plate SVG. Use --write to update index.html in place.

The terrain is a separate generated image, assets/img/terrain-v2.png.
"""
import argparse
from pathlib import Path
import re

K = 0.326               # dimetric ratio (poster: half-height 59.3 / half-width 182)
HW = 182                # plate half-width (plan square side == HW)
TY0 = 6                 # first plate top-vertex y
PITCH = 119
THICK = 24
N = 4

def plate(i):
    """screen coords of the four top-face vertices + the two side faces"""
    ox, oy = HW, TY0 + i * PITCH
    top = (ox, oy)
    right = (ox + HW, oy + HW * K)
    bottom = (ox, oy + 2 * HW * K)
    left = (ox - HW, oy + HW * K)
    return ox, oy, top, right, bottom, left

def n(v):
    v = round(v)
    return str(int(v)) if abs(v - round(v)) < 1e-9 else str(round(v, 1))

def d(*pts):
    return "M" + " L".join("%s %s" % (n(x), n(y)) for x, y in pts) + "Z"

def slab(i, name, label):
    ox, oy, top, right, bottom, left = plate(i)
    face_l = d(left, bottom, (bottom[0], bottom[1] + THICK), (left[0], left[1] + THICK))
    face_r = d(right, bottom, (bottom[0], bottom[1] + THICK), (right[0], right[1] + THICK))
    face_t = d(top, right, bottom, left)
    out = []
    out.append('')
    out.append('      <!-- ── %02d %s -->' % (i + 1, name.upper()))
    out.append('      <g class="slab" data-slab="%s" filter="url(#plate-grain)">' % name)
    out.append('        <path d="%s" fill="%s"/>' % (face_r, "#292823" if name == "serve" else "#C9C6BD"))
    out.append('        <path d="%s" fill="%s"/>' % (face_l, "#171714" if name == "serve" else "#BDBAB1"))
    for face in (face_l, face_r):
        out.append('        <path d="%s" fill="url(#edge-lines)" opacity=".3"/>' % face)
    out.append('        <path d="%s" fill="%s"/>' % (face_t, label["plate"]))
    return "\n".join(out), face_t, ox, oy, bottom

def cubes(ox, oy, count, seed):
    """deterministic scatter of isometric blocks in plan space, painter-sorted"""
    st = seed
    def rnd():
        nonlocal st
        st = (st * 1103515245 + 12345) & 0x7FFFFFFF
        return st / 0x7FFFFFFF
    items = []
    for i in range(count):
        px = 28 + (i % 8) * 18 + rnd() * 6
        py = 18 + (i // 8) * 23 + rnd() * 6
        s = 0.23 + rnd() * 0.20
        c = rnd()
        kind = "w" if c < .46 else "g" if c < .72 else "d" if c < .92 else "o"
        sx = ox + px - py
        sy = oy + (px + py) * K
        items.append((px + py, sx, sy, s, kind))
    items.sort()
    return ["<use href=\"#cube\" class=\"cb cb--%s\" transform=\"translate(%.1f %.1f) scale(%.2f)\"/>"
            % (k, x, y, s) for _, x, y, s, k in items]

# ── pipeline ──────────────────────────────────────────────────────────
H = 553  # Preserve the landscape handoff while tightening the slab spacing.
parts = []
parts.append('<svg class="pipeline__svg" viewBox="0 0 %d %d" aria-hidden="true" focusable="false">' % (2 * HW, round(H)))
KS = "%g" % K
DEFS = """  <defs>
    <filter id="plate-grain" x="-5%" y="-10%" width="110%" height="120%">
      <feTurbulence type="fractalNoise" baseFrequency=".65" numOctaves="3" seed="12"/>
      <feColorMatrix type="saturate" values="0"/>
      <feComponentTransfer><feFuncA type="linear" slope=".23"/></feComponentTransfer>
      <feComposite in2="SourceGraphic" operator="in"/>
      <feBlend in2="SourceGraphic" mode="multiply"/>
    </filter>
    <pattern id="edge-lines" width="23" height="24" patternUnits="userSpaceOnUse">
      <path d="M0 0V24M2 0V24M0 7H23" fill="none" stroke="#F3F0E8" stroke-width=".6"/>
    </pattern>
    <pattern id="p-grid" width="20" height="20" patternUnits="userSpaceOnUse" patternTransform="matrix(1 {k} -1 {k} 0 0)">
      <path d="M0 0H20M0 0V20" fill="none" stroke="#0B0B0B" stroke-width="1.1"/>
    </pattern>
    <pattern id="p-fine" width="24" height="24" patternUnits="userSpaceOnUse" patternTransform="matrix(1 {k} -1 {k} 0 0)">
      <path d="M0 0H24M0 0V24" fill="none" stroke="#F3F0E8" stroke-width=".65"/>
    </pattern>
    <linearGradient id="ramp" x1="0" y1=".62" x2="1" y2=".06">
      <stop offset="0" stop-color="#000"/><stop offset=".28" stop-color="#000"/>
      <stop offset=".64" stop-color="#fff"/><stop offset="1" stop-color="#fff"/>
    </linearGradient>
    <mask id="ramp-mask"><rect x="0" y="0" width="{vw}" height="{h}" fill="url(#ramp)"/></mask>
    <mask id="signal-depth" maskUnits="userSpaceOnUse" x="0" y="-500" width="{vw}" height="1100">
      <rect x="0" y="-500" width="{vw}" height="1100" fill="white"/>
      <path d="M182 42V149M182 200V268M182 289V387M182 460V506" stroke="black" stroke-width="10"/>
    </mask>
    <g id="cube">
      <ellipse cx="3" cy="20" rx="17" ry="5" fill="#0B0B0B" opacity=".16"/>
      <path d="M-13 -6.125 L0 -1.25 L0 21 L-13 16.125Z" fill="var(--cl)"/>
      <path d="M13 -6.125 L0 -1.25 L0 21 L13 16.125Z" fill="var(--cr)"/>
      <path d="M0 -11 L13 -6.125 L0 -1.25 L-13 -6.125Z" fill="var(--ct)"/>
    </g>
  </defs>"""
parts.append(DEFS.format(k=KS, vw=2 * HW, h=round(H)))

labels = [
    ("input",     "TEXT",    {"plate": "#EFECE3"}),
    ("inference", "MODEL",   {"plate": "#E9E6DC"}),
    ("embedding", "VECTORS", {"plate": "#E9E6DC"}),
    ("serve",     "REDIS",   {"plate": "#111110"}),
]
for i, (name, label, cfg) in enumerate(labels):
    body, face_t, ox, oy, bottom = slab(i, name, cfg)
    parts.append(body)
    if name == "input":
        parts.append('        <path d="%s" fill="url(#p-grid)" opacity=".065"/>' % face_t)
        for j in range(46):
            px = 22 + ((j * 47) % 140)
            py = 16 + ((j * 31) % 116)
            parts.append('        <ellipse cx="%.1f" cy="%.1f" rx="1.8" ry=".8" fill="#34332F"/>' % (ox + px - py, oy + (px + py) * K))
    if name == "inference":
        parts.append('        <path d="%s" fill="url(#p-grid)" opacity=".5"/>' % face_t)
        parts.append('        <g mask="url(#ramp-mask)">')
        parts.append('          <path d="%s" fill="#0B0B0B"/>' % face_t)
        parts.append('          <path d="%s" fill="url(#p-fine)" opacity=".45"/>' % face_t)
        parts.append('        </g>')
    if name == "embedding":
        parts.append('        <path d="%s" fill="url(#p-grid)" opacity=".09"/>' % face_t)
        parts.append('        <g class="cube-field">')
        for c in cubes(ox, oy, 48, 20260911):
            parts.append("          " + c)
        parts.append('        </g>')
    if name == "serve":
        parts.append('        <path d="%s" fill="url(#p-fine)" opacity=".45"/>' % face_t)
    if name in ("inference", "serve"):
        # A second, lifted lattice gives the diagram the reference's wire volume.
        for z in (10, 22):
            for u in range(48, 163, 23):
                x1, y1 = ox + u - 24, oy + (u + 24) * K
                x2, y2 = ox + u - 160, oy + (u + 160) * K
                parts.append('        <path d="M%.1f %.1fL%.1f %.1fL%.1f %.1fM%.1f %.1fV%.1f" fill="none" stroke="#B8B5AC" stroke-width=".55" opacity=".35"/>' % (x1, y1, x1, y1-z, x2, y2-z, x2, y2-z, y2))
                x1, y1 = ox + 48 - u, oy + (48 + u) * K
                x2, y2 = ox + 163 - u, oy + (163 + u) * K
                parts.append('        <path d="M%.1f %.1fL%.1f %.1f" fill="none" stroke="#B8B5AC" stroke-width=".55" opacity=".35"/>' % (x1, y1-z, x2, y2-z))
    sw = "1" if name != "serve" else "1.2"
    stroke = "#F3F0E8"
    parts.append('        <path d="%s" fill="none" stroke="%s" stroke-width="%s"/>' % (face_t, stroke, sw))
    parts.append('        <text class="plate-label%s" transform="matrix(1 -%s 1 %s 48 %d)">%s</text>'
                 % (" plate-label--light" if name == "serve" else "", KS, KS, oy + 68, label))
    parts.append('      </g>')

parts.append('')
parts.append('      <!-- The signal enters each surface and passes behind its front edge. -->')
parts.append('      <line class="sig" x1="%d" y1="-490" x2="%d" y2="%d" pathLength="1" mask="url(#signal-depth)"/>' % (HW, HW, round(H)))
parts.append('</svg>')

svg = "\n".join(parts) + "\n"
parser = argparse.ArgumentParser(description=__doc__)
parser.add_argument("--write", action="store_true", help="replace the pipeline SVG in index.html")
args = parser.parse_args()
if args.write:
    page = Path(__file__).resolve().parents[1] / "index.html"
    content, count = re.subn(r'<svg class="pipeline__svg".*?</svg>', svg.rstrip(), page.read_text(), count=1, flags=re.S)
    if count != 1:
        raise SystemExit("Expected one pipeline SVG in index.html")
    page.write_text(content)
    print("Updated", page)
else:
    print(svg, end="")
