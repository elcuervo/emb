#!/usr/bin/env python3
"""Generates the DINOv3 reference fixtures, one row per dialect.

Run with Pillow + numpy + onnxruntime installed and the export downloaded
(`just download-dinov3-model <repo> <dir>`):

    python3 generate_dinov3_reference.py <model.onnx> <dialect> <prefix>

`dialect` is `community` (onnx-community: dynamic axes, `last_hidden_state` +
`pooler_output`, preprocessor_config.json) or `kornia` (static batch 1,
`tokens` + `descriptor`, no metadata). It writes two files next to this script:

    <prefix>_pooled.f32   the pooled vector for golden_input.png
    <prefix>_patch.f32    the first patch row of that image's grid

Both are L2-normalized little-endian float32, matching the server's
`normalize: true` path. Preprocessing is Pillow's, so the Go test compares by
cosine (>= 0.999), not bit equality: emb resizes in uint8 before rescaling.
"""
import json
import os
import sys

import numpy as np
import onnxruntime as ort
from PIL import Image

HERE = os.path.dirname(os.path.abspath(__file__))
INPUT_IMAGE = os.path.join(HERE, "..", "..", "imageproc", "testdata", "golden_input.png")

IMAGENET_MEAN = [0.485, 0.456, 0.406]
IMAGENET_STD = [0.229, 0.224, 0.225]

RESAMPLE = {
    0: Image.Resampling.NEAREST,
    1: Image.Resampling.LANCZOS,
    2: Image.Resampling.BILINEAR,
    3: Image.Resampling.BICUBIC,
}


def preprocessing(model_path, dialect):
    """The plan the server resolves for this dialect, mirrored here."""
    if dialect == "community":
        with open(os.path.join(os.path.dirname(model_path), "preprocessor_config.json")) as fh:
            pp = json.load(fh)
        size = pp["size"]["height"]
        return size, RESAMPLE[pp["resample"]], pp["rescale_factor"], pp["image_mean"], pp["image_std"]
    if dialect == "kornia":
        # No preprocessor_config.json: the preset's constants at the graph's size.
        return 336, Image.Resampling.BILINEAR, 1.0 / 255.0, IMAGENET_MEAN, IMAGENET_STD
    sys.exit(f"unknown dialect {dialect}")


def layout(model_path, dialect):
    """(pooled tensor, token tensor, first patch row) for this dialect."""
    if dialect == "community":
        md = json.load(open(os.path.join(os.path.dirname(model_path), "config.json")))
        return "pooler_output", "last_hidden_state", 1 + md["num_register_tokens"]
    return "descriptor", "tokens", 5


def normalize(vec):
    norm = np.linalg.norm(vec)
    return vec if norm == 0 else vec / norm


def main():
    if len(sys.argv) != 4:
        sys.exit("usage: generate_dinov3_reference.py <model.onnx> <community|kornia> <prefix>")
    model_path, dialect, prefix = sys.argv[1], sys.argv[2], sys.argv[3]

    size, resample, rescale, mean, std = preprocessing(model_path, dialect)
    pooled_name, token_name, start = layout(model_path, dialect)

    img = Image.open(INPUT_IMAGE).convert("RGB").resize((size, size), resample)
    x = np.asarray(img, dtype=np.float32) * rescale
    x = (x - np.asarray(mean, np.float32)) / np.asarray(std, np.float32)
    x = np.transpose(x, (2, 0, 1))[None, ...]

    sess = ort.InferenceSession(model_path, providers=["CPUExecutionProvider"])
    names = [o.name for o in sess.get_outputs()]
    outs = sess.run(None, {sess.get_inputs()[0].name: x})

    pooled = normalize(outs[names.index(pooled_name)][0].astype("<f4"))
    tokens = outs[names.index(token_name)][0]
    patch = normalize(tokens[start].astype("<f4"))

    for suffix, arr in (("pooled", pooled), ("patch", patch)):
        path = os.path.join(HERE, f"{prefix}_{suffix}.f32")
        arr.tofile(path)
        print(f"wrote {path}: shape={arr.shape} norm={float(np.linalg.norm(arr)):.6f}")


if __name__ == "__main__":
    main()
