#!/usr/bin/env python3
"""Generate the two pieces of isometric artwork in index.html.

  * the four-plate pipeline SVG  (dimetric projection, half-width 197,
    top-face ratio .3046, pitch 130, thickness 22)
  * the terrain SVG: the photograph clipped by the skyline traced from it,
    so the frame is knocked straight out of the paper

Run `python3 tools/gen-isometric.py` and paste the two fragments into
index.html (they are marked in the markup).
"""
import json, math

K = 0.326               # dimetric ratio (poster: half-height 59.3 / half-width 182)
HW = 182                # plate half-width (plan square side == HW)
TY0 = 6                 # first plate top-vertex y
PITCH = 130
THICK = 28
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
    out.append('      <g class="slab" data-slab="%s">' % name)
    out.append('        <path d="%s" fill="%s"/>' % (face_r, "#B6B2A7"))
    out.append('        <path d="%s" fill="%s"/>' % (face_l, "#9E9A90"))
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
        px = 16 + rnd() * (HW - 32)
        py = 16 + rnd() * (HW - 32)
        s = 0.36 + rnd() * 0.46
        c = rnd()
        kind = "w" if c < .46 else "g" if c < .72 else "d" if c < .92 else "o"
        sx = ox + px - py
        sy = oy + (px + py) * K
        items.append((px + py, sx, sy, s, kind))
    items.sort()
    return ["<use href=\"#cube\" class=\"cb cb--%s\" transform=\"translate(%.1f %.1f) scale(%.2f)\"/>"
            % (k, x, y, s) for _, x, y, s, k in items]

# ── pipeline ──────────────────────────────────────────────────────────
H = TY0 + 3 * PITCH + 2 * HW * K + THICK + 10
parts = []
parts.append('<svg class="pipeline__svg" viewBox="0 0 %d %d" aria-hidden="true" focusable="false">' % (2 * HW, round(H)))
KS = "%g" % K
DEFS = """  <defs>
    <pattern id="p-dots" width="34" height="34" patternUnits="userSpaceOnUse" patternTransform="matrix(1 {k} -1 {k} 0 0)">
      <circle cx="5" cy="8" r="1.9"/><circle cx="21" cy="4" r="1.3"/>
      <circle cx="12" cy="19" r="2.1"/><circle cx="28" cy="23" r="1.4"/>
      <circle cx="3" cy="28" r="1.2"/><circle cx="22" cy="31" r="1.7"/>
      <circle cx="14" cy="10" r="1"/><circle cx="8" cy="15" r=".9"/>
    </pattern>
    <pattern id="p-grid" width="20" height="20" patternUnits="userSpaceOnUse" patternTransform="matrix(1 {k} -1 {k} 0 0)">
      <path d="M0 0H20M0 0V20" fill="none" stroke="#0B0B0B" stroke-width="1.1"/>
    </pattern>
    <pattern id="p-fine" width="9" height="9" patternUnits="userSpaceOnUse" patternTransform="matrix(1 {k} -1 {k} 0 0)">
      <path d="M0 0H9M0 0V9" fill="none" stroke="#F3F0E8" stroke-width=".55"/>
    </pattern>
    <linearGradient id="ramp" x1="0" y1=".62" x2="1" y2=".06">
      <stop offset="0" stop-color="#000"/><stop offset=".52" stop-color="#000"/>
      <stop offset=".93" stop-color="#fff"/><stop offset="1" stop-color="#fff"/>
    </linearGradient>
    <mask id="ramp-mask"><rect x="0" y="0" width="{vw}" height="{h}" fill="url(#ramp)"/></mask>
    <g id="cube">
      <path d="M-13 -6.125 L0 -1.25 L0 12.75 L-13 7.875Z" fill="var(--cl)"/>
      <path d="M13 -6.125 L0 -1.25 L0 12.75 L13 7.875Z" fill="var(--cr)"/>
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
        parts.append('        <path d="%s" fill="url(#p-dots)"/>' % face_t)
    if name == "inference":
        parts.append('        <path d="%s" fill="url(#p-grid)" opacity=".5"/>' % face_t)
        parts.append('        <g mask="url(#ramp-mask)">')
        parts.append('          <path d="%s" fill="#0B0B0B"/>' % face_t)
        parts.append('          <path d="%s" fill="url(#p-fine)" opacity=".45"/>' % face_t)
        parts.append('        </g>')
    if name == "embedding":
        parts.append('        <g class="cube-field">')
        for c in cubes(ox, oy, 74, 20260911):
            parts.append("          " + c)
        parts.append('        </g>')
    if name == "serve":
        parts.append('        <path d="%s" fill="url(#p-fine)" opacity=".45"/>' % face_t)
    sw = "1" if name != "serve" else "1.2"
    stroke = "#0B0B0B" if name != "serve" else "#F3F0E8"
    parts.append('        <path d="%s" fill="none" stroke="%s" stroke-width="%s"/>' % (face_t, stroke, sw))
    parts.append('        <text class="plate-label%s" x="26" y="%.1f" transform="matrix(1 %s -1 %s %d %d)">%s</text>'
                 % (" plate-label--light" if name == "serve" else "", 152 * K / .3046, KS, KS, ox, oy, label))
    parts.append('      </g>')

parts.append('')
parts.append('      <!-- the signal rides ON TOP of the stack -->')
parts.append('      <line class="sig" x1="%d" y1="-520" x2="%d" y2="%d" pathLength="1"/>' % (HW, HW, round(H)))
parts.append('</svg>')
open('/tmp/pipeline.svg', 'w').write("\n".join(parts) + "\n")
print("pipeline height", round(H))

# ── the terrain's skyline (used for the inline SVG clip-path) ────────
# skyline of assets/img/mountain.jpg, sampled every 16px by scanning down each
# column for the first non-sky pixel (bright + low-variance flood fill from the
# top edge, then keeping only the component that touches the bottom)
SKYLINE = [[0,898],[16,906],[32,912],[48,914],[64,912],[80,916],[96,906],[112,898],[128,904],[144,912],[160,898],[176,904],[192,904],[208,968],[224,946],[240,932],[256,904],[272,902],[288,918],[304,922],[320,914],[336,902],[352,902],[368,908],[384,898],[400,904],[416,894],[432,876],[448,870],[464,854],[480,856],[496,858],[512,854],[528,850],[544,848],[560,856],[576,862],[592,860],[608,868],[624,870],[640,862],[656,864],[672,884],[688,878],[704,868],[720,866],[736,850],[752,846],[768,844],[784,844],[800,838],[816,834],[832,840],[848,838],[864,842],[880,852],[896,856],[912,864],[928,840],[944,824],[960,806],[976,792],[992,772],[1008,762],[1024,756],[1040,746],[1056,740],[1072,738],[1088,724],[1104,722],[1120,704],[1136,698],[1152,666],[1168,650],[1184,644],[1200,668],[1216,660],[1232,682],[1248,740],[1264,754],[1280,752],[1296,762],[1312,858],[1328,868],[1344,840],[1360,818],[1376,824],[1392,838],[1408,850],[1424,870],[1440,900],[1456,904],[1472,906],[1488,916],[1504,930],[1520,938],[1536,940],[1552,942],[1568,936],[1584,934],[1600,928],[1616,938],[1632,914],[1648,910],[1664,916],[1680,928],[1696,932],[1712,932],[1728,934],[1744,932],[1760,950],[1776,942],[1792,930],[1808,944],[1824,948],[1840,912],[1856,920],[1872,918],[1888,928],[1904,916],[1920,916],[1936,914],[1952,912],[1968,912],[1984,910],[2000,912],[2016,920],[2032,916],[2048,910],[2064,902],[2080,900],[2096,884],[2112,874],[2128,860],[2144,844],[2160,832],[2176,834],[2192,836],[2208,836],[2224,850],[2240,868],[2256,886],[2272,910],[2288,928],[2304,926],[2320,928],[2336,932],[2352,946],[2368,874],[2384,864],[2400,848],[2416,850],[2432,862],[2448,878],[2464,892],[2480,906],[2496,920],[2512,912],[2528,914],[2544,916],[2560,910],[2576,902],[2592,898]]
sky = SKYLINE
CX, CY, CW, CH = 140, 590, 1900, 790   # frame of mountain.jpg used on the page
# (the poster's massif sits under the signal spine, ~118px right of the
#  photo's default 500-wide origin crop)

def simplify(points, tol):
    """Douglas-Peucker, keeps the peak"""
    if len(points) < 3:
        return points
    (x1, y1), (x2, y2) = points[0], points[-1]
    dmax, idx = 0, 0
    for i in range(1, len(points) - 1):
        x0, y0 = points[i]
        num = abs((y2 - y1) * x0 - (x2 - x1) * y0 + x2 * y1 - y2 * x1)
        den = math.hypot(y2 - y1, x2 - x1) or 1
        if num / den > dmax:
            dmax, idx = num / den, i
    if dmax > tol:
        return simplify(points[:idx + 1], tol)[:-1] + simplify(points[idx:], tol)
    return [points[0], points[-1]]

inside = [(x - CX, max(0, min(CH, y - CY))) for x, y in sky if CX <= x <= CX + CW]
if inside[0][0] > 0:
    inside.insert(0, (0, inside[0][1]))
if inside[-1][0] < CW:
    inside.append((CW, inside[-1][1]))
inside = simplify(inside, 5.0)
# cut the left/right edges on a diagonal so the frame reads as terrain
# falling out of the picture rather than a photograph's rectangle
FEATHER = 150
inside = [p for p in inside if FEATHER <= p[0] <= CW - FEATHER] + [(CW, CH)]
d = " ".join(["M0 %d" % CH] + ["L%d %d" % (round(x), round(y)) for x, y in inside] + ["Z"])

terrain = f'''<svg class="landscape__photo" viewBox="0 0 {CW} {CH}" preserveAspectRatio="none" aria-hidden="true" focusable="false">
              <defs>
                <clipPath id="skyline" clipPathUnits="userSpaceOnUse">
                  <path d="{d}"/>
                </clipPath>
                <pattern id="print-dots" width="9" height="9" patternUnits="userSpaceOnUse">
                  <circle cx="2.2" cy="2.2" r="1.1" fill="#0B0B0B"/>
                  <circle cx="6.8" cy="6.8" r=".8" fill="#0B0B0B"/>
                </pattern>
              </defs>
              <image href="assets/img/mountain.jpg" x="-{CX}" y="-{CY}" width="2600" height="1722"
                     preserveAspectRatio="none" clip-path="url(#skyline)"/>
              <!-- print screen: clipped to the traced skyline so it never
                   prints a hard rectangle over the paper -->
              <rect x="0" y="0" width="{CW}" height="{CH}" fill="url(#print-dots)"
                    clip-path="url(#skyline)" opacity=".14" style="mix-blend-mode:multiply"/>
            </svg>'''
open('/tmp/terrain.svg', 'w').write(terrain + "\n")
print("pipeline -> /tmp/pipeline.svg   terrain -> /tmp/terrain.svg")
print("clip-path points:", len(inside))
