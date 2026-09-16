# Terrain asset provenance

File: `terrain-v2.png`, 2172 × 724, RGB PNG.
Created 12 September 2026 with the built-in image generation tool, using the
user-provided `drop-20260912-180449.png` poster as a visual reference.
The image has a light background, not an alpha channel. The original Unsplash
asset is unused.

## The shipped cut-out

The page does not load this file. It loads `terrain-matte.png`, a 2172 × 724
grey+alpha cut-out derived from it by `tools/gen-terrain-matte.py`:

```
alpha = 1 - L / 235        rgb = 0 (black)
```

235 is the ground's own level, taken as the 1st percentile of the sky sample
(the top strip plus the upper third of both side columns). The file only
ever carried a flat ground rather than real transparency, so keying it by
luminance is exactly what `mix-blend-mode: multiply` was already doing at
composite time — now baked in, where no compositor state can undo it.
Compositing the matte over `#F3F0E8` reproduces the old pipeline to a mean
error of 0.33/255 and a peak of 1.6/255; the ground lands on alpha 0 (37 of
797,399 sky pixels exceed alpha 8). The matte is 0.80 MB against this file's
2.29 MB.

## Generation prompt

Create a website asset from the supplied reference poster: ONLY the rocky
terrain formation at the very bottom of the poster, with all typography,
orange lines, background paper and other elements removed. Output a real
transparent-background PNG cutout, landscape aspect 3:1. Reproduce the same
rugged, dry, craggy, weathered rock massif in monochrome engraved photographic
/ high contrast black-and-cream grain texture. NOT snowy mountains, NO
snowfields, NO glaciers. Match its silhouette: ground starts at bottom-left
corner, rises gradually through low rocky foothills, highest twin crag peak
around 67% of the image width, drops steeply to a saddle around 86%, and a low
rocky outcrop rises slightly at the right edge. The rock formation fills the
bottom edge completely. The tallest peak should be near the top edge. Keep
intricate natural crags and strongly directional lighting, black shadowed
left faces and warm off-white detailed highlights. Transparent sky everywhere
above the rock silhouette. No orange route, no text, no watermark, no border,
no shadow behind the cutout. This asset will render around 850 pixels wide
and 255 pixels tall on cream paper.

## Final edit prompt

The initial result contained a checkerboard in its RGB pixels. A second
built-in image edit used this prompt:

Edit only the background. Preserve the rock formation exactly, every crag,
silhouette, lighting, texture, composition, size and position. Remove the
entire gray checkerboard pattern and replace it with perfectly flat uniform
warm off-white #F3F0E8. No background texture whatsoever, no checkerboard, no
shadows or haze in the background. The background must be a single uniform
solid color up to the sharp edge of the rocks. All rock pixels should remain
unchanged. Keep the wide 3:1 aspect.

## The reading breaks

Thirteen `terrain-<name>.png` bands are the same keyed matte, each framing a
different stretch of the ridge at its own scale, with four of them mirrored
(`range`, `descent`, `outcrop`, `bluff`) so no two dividers show the same
picture. They are bottom-anchored 8:1 crops of the source — `slope` is the
left slope, `crag` the twin crag, `saddle` the saddle, `west` the whole
range — quantised with `pngquant --quality=65-85`. Alpha is derived the same
way it is for the full cut-out (`gen-terrain-matte.py`'s keying, re-run over
the crop): the pixels are `terrain-v2.png`'s own and nothing is redrawn.

They exist so the reference and the gallery can carry a band of rock between
text blocks without every page paying for the full 2172 × 722 cut-out. They
run from 55 KB to 166 KB against the matte's 0.80 MB, which the closing band
still reads on its own, and every page loads exactly one. All of them are
declared in `tools/published-tree.py`'s `SERVED` set and inherit
`/assets/img/*`'s cache rule, so a change to one is a rename and a new URL
rather than a stale one.
