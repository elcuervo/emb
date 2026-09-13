#!/usr/bin/env python3
"""Generates the golden image-preprocessing fixtures for imageproc tests.

Run with a Python that has Pillow + numpy installed, from this directory:

    python3 generate_golden.py

It writes:
  golden_input.png    deterministic RGB gradient source image
  golden_clip.f32     Pillow/numpy reference tensor for the CLIP-style plan
  golden_siglip.f32   Pillow/numpy reference tensor for the SigLIP-style plan

The Go test (golden_test.go) reproduces the same plans with the Go pipeline and
asserts agreement within a documented tolerance. Go resize kernels differ from
Pillow's, so this is a parity check, not bit-exactness: do not mix pipelines for
one index (see README).
"""
import numpy as np
from PIL import Image

W, H = 40, 30
size = 16

clip_mean = [0.48145466, 0.4578275, 0.40821073]
clip_std = [0.26862954, 0.26130258, 0.27577711]
siglip_mean = [0.5, 0.5, 0.5]
siglip_std = [0.5, 0.5, 0.5]


def make_input():
    arr = np.zeros((H, W, 3), dtype=np.uint8)
    for y in range(H):
        for x in range(W):
            arr[y, x] = ((x * 7 + y * 3) % 256, (x * 13) % 256, (y * 11 + 40) % 256)
    return Image.fromarray(arr, "RGB")


def preprocess(img, crop, resample, mean, std):
    im = img.convert("RGB")
    w, h = im.size
    if crop == "center":
        if w <= h:
            nw, nh = size, int(size * h / w)
        else:
            nh, nw = size, int(size * w / h)
        im = im.resize((nw, nh), resample)
        left = (nw - size) // 2
        top = (nh - size) // 2
        im = im.crop((left, top, left + size, top + size))
    else:
        im = im.resize((size, size), resample)
    x = np.asarray(im, dtype=np.float32) / 255.0
    x = (x - np.array(mean, dtype=np.float32)) / np.array(std, dtype=np.float32)
    x = np.transpose(x, (2, 0, 1))[np.newaxis, ...]  # [1, 3, size, size]
    return x.astype("<f4")


def main():
    img = make_input()
    img.save("golden_input.png")

    clip = preprocess(img, "center", Image.Resampling.BICUBIC, clip_mean, clip_std)
    siglip = preprocess(img, "none", Image.Resampling.BILINEAR, siglip_mean, siglip_std)
    clip.tofile("golden_clip.f32")
    siglip.tofile("golden_siglip.f32")
    print("clip", clip.shape, "siglip", siglip.shape)


if __name__ == "__main__":
    main()
