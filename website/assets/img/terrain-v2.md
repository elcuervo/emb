# Terrain asset provenance

File: `terrain-v2.png`, 2172 × 724, RGB PNG.
Created 12 September 2026 with the built-in image generation tool, using the
user-provided `drop-20260912-180449.png` poster as a visual reference.
The image has a light background, not an alpha channel. The website blends
it onto its cream paper using CSS. The original Unsplash asset is unused.

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
