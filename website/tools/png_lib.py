"""Minimal, dependency-free PNG reader/writer for the terrain toolchain.

Only the variants the project actually ships are supported; anything else
fails loudly rather than silently producing a wrong asset:

  read  8-bit RGB (colour type 2), RGBA (6), grey (0), grey+alpha (4)
  write 8-bit grey+alpha (4) and RGB (2), non-interlaced, filter type 0

The encoder picks the per-row filter that minimises the sum of absolute
differences. That is not the most sophisticated heuristic in the world, but
it is deterministic and it gets a two-tone matte down to a sane size without
pulling a codec dependency into a static marketing site.
"""

import struct
import zlib

PNG_SIG = b"\x89PNG\r\n\x1a\n"


def _paeth(a, b, c):
    p = a + b - c
    pa, pb, pc = abs(p - a), abs(p - b), abs(p - c)
    if pa <= pb and pa <= pc:
        return a
    if pb <= pc:
        return b
    return c


def _unfilter(raw, width, height, bpp, stride):
    out = bytearray(height * stride)
    pos = 0
    prev = bytearray(stride)
    for y in range(height):
        ft = raw[pos]
        pos += 1
        line = bytearray(raw[pos:pos + stride])
        pos += stride
        if ft == 1:
            for i in range(bpp, stride):
                line[i] = (line[i] + line[i - bpp]) & 0xFF
        elif ft == 2:
            for i in range(stride):
                line[i] = (line[i] + prev[i]) & 0xFF
        elif ft == 3:
            for i in range(stride):
                a = line[i - bpp] if i >= bpp else 0
                line[i] = (line[i] + ((a + prev[i]) >> 1)) & 0xFF
        elif ft == 4:
            for i in range(stride):
                a = line[i - bpp] if i >= bpp else 0
                c = prev[i - bpp] if i >= bpp else 0
                line[i] = (line[i] + _paeth(a, prev[i], c)) & 0xFF
        elif ft != 0:
            raise ValueError("unsupported PNG filter type %d on row %d" % (ft, y))
        out[y * stride:(y + 1) * stride] = line
        prev = line
    return out


class Plane(object):
    """A decoded raster: `mode` plus normalised per-pixel accessors."""

    def __init__(self, width, height, mode, data):
        self.width = width
        self.height = height
        self.mode = mode  # one of rgb, rgba, grey, greya
        self.data = data  # row-major bytes

    @property
    def channels(self):
        return {"grey": 1, "greya": 2, "rgb": 3, "rgba": 4}[self.mode]

    def rgb(self, x, y):
        i = (y * self.width + x) * self.channels
        d = self.data
        if self.mode == "rgb":
            return d[i], d[i + 1], d[i + 2]
        if self.mode == "rgba":
            return d[i], d[i + 1], d[i + 2]
        g = d[i]
        return g, g, g


def read(path):
    blob = open(path, "rb").read()
    if blob[:8] != PNG_SIG:
        raise ValueError("%s is not a PNG" % path)
    pos = 8
    idat = bytearray()
    width = height = depth = ctype = interlace = None
    while pos < len(blob):
        (length,) = struct.unpack(">I", blob[pos:pos + 4])
        ctag = blob[pos + 4:pos + 8]
        body = blob[pos + 8:pos + 8 + length]
        if ctag == b"IHDR":
            width, height, depth, ctype, _comp, _filt, interlace = struct.unpack(">IIBBBBB", body)
        elif ctag == b"IDAT":
            idat += body
        elif ctag == b"IEND":
            break
        pos += 12 + length
    if depth != 8:
        raise ValueError("%s: only 8-bit channels are supported (got %s)" % (path, depth))
    if interlace != 0:
        raise ValueError("%s: interlaced PNG is not supported" % path)
    mode = {0: "grey", 2: "rgb", 4: "greya", 6: "rgba"}.get(ctype)
    if mode is None:
        raise ValueError("%s: unsupported colour type %s" % (path, ctype))
    bpp = {"grey": 1, "greya": 2, "rgb": 3, "rgba": 4}[mode]
    stride = width * bpp
    raw = zlib.decompress(bytes(idat))
    expect = height * (stride + 1)
    if len(raw) != expect:
        raise ValueError("%s: expected %d inflated bytes, got %d" % (path, expect, len(raw)))
    return Plane(width, height, mode, _unfilter(raw, width, height, bpp, stride))


def _chunk(tag, payload):
    return (struct.pack(">I", len(payload)) + tag + payload
            + struct.pack(">I", zlib.crc32(tag + payload) & 0xFFFFFFFF))


def _filter_rows(data, width, channels, height):
    """Encode rows with the best of none/sub/up/paeth per row."""
    stride = width * channels
    out = bytearray()
    prev = bytearray(stride)
    for y in range(height):
        line = data[y * stride:(y + 1) * stride]
        up = bytearray(stride)
        paeth = bytearray(stride)
        for i in range(stride):
            a = line[i - channels] if i >= channels else 0
            b = prev[i]
            c = prev[i - channels] if i >= channels else 0
            up[i] = (line[i] - b) & 0xFF
            paeth[i] = (line[i] - _paeth(a, b, c)) & 0xFF
        sub = bytearray(stride)
        for i in range(stride):
            a = line[i - channels] if i >= channels else 0
            sub[i] = (line[i] - a) & 0xFF
        cands = [(0, line), (1, sub), (2, up), (4, paeth)]
        cost = lambda buf: sum(v if v < 128 else 256 - v for v in buf)
        best = min(cands, key=lambda c: cost(c[1]))
        out.append(best[0])
        out += best[1]
        prev = line
    return bytes(out)


def write(path, width, height, mode, data, level=9):
    ctype = {"grey": 0, "rgb": 2, "greya": 4}.get(mode)
    if ctype is None:
        raise ValueError("cannot write mode %r" % mode)
    channels = {"grey": 1, "rgb": 3, "greya": 2}[mode]
    if len(data) != width * height * channels:
        raise ValueError("buffer is %d bytes, expected %d" % (len(data), width * height * channels))
    ihdr = struct.pack(">IIBBBBB", width, height, 8, ctype, 0, 0, 0)
    body = zlib.compress(_filter_rows(bytes(data), width, channels, height), level)
    blob = PNG_SIG + _chunk(b"IHDR", ihdr) + _chunk(b"IDAT", body) + _chunk(b"IEND", b"")
    with open(path, "wb") as fh:
        fh.write(blob)
    return len(blob)
