#!/usr/bin/env python3
"""Generates the SigLIP2 vision embedding reference fixture.

Run from this directory with Pillow + numpy + onnxruntime installed, and the
fp32 vision export downloaded (`just download-vision-model`):

    python3 generate_siglip2_reference.py

It writes siglip2_vision_pooler.f32, the 768-element float32 LE `pooler_output`
for internal/imageproc/testdata/golden_input.png, preprocessed exactly as the
model's preprocessor_config.json specifies (resize 224x224 bilinear, rescale
1/255, normalize mean/std 0.5). The gated Go test
(TestSigLIP2VisionAutoconfigAndReference) compares the server's EMB.IMG path
against it within a documented tolerance: the Go resize kernel differs from
Pillow's, so this is a parity check, not bit-exactness.
"""
import json
import numpy as np
import onnxruntime as ort
from PIL import Image

MODEL_DIR = "../../../models/siglip2-vision"
INPUT_IMAGE = "../../imageproc/testdata/golden_input.png"
OUTPUT = "siglip2_vision_pooler.f32"

pp = json.load(open(f"{MODEL_DIR}/preprocessor_config.json"))
sess = ort.InferenceSession(f"{MODEL_DIR}/vision_model.onnx", providers=["CPUExecutionProvider"])

img = Image.open(INPUT_IMAGE).convert("RGB")
w, h = pp["size"]["width"], pp["size"]["height"]
resample = {
    0: Image.Resampling.NEAREST,
    1: Image.Resampling.LANCZOS,
    2: Image.Resampling.BILINEAR,
    3: Image.Resampling.BICUBIC,
}[pp["resample"]]
img = img.resize((w, h), resample)

x = np.asarray(img, dtype=np.float32) * pp["rescale_factor"]
x = (x - np.asarray(pp["image_mean"], np.float32)) / np.asarray(pp["image_std"], np.float32)
x = np.transpose(x, (2, 0, 1))[None, ...]  # [1, 3, H, W]

names = [o.name for o in sess.get_outputs()]
outs = sess.run(None, {sess.get_inputs()[0].name: x})
vec = outs[names.index("pooler_output")][0].astype("<f4")
vec.tofile(OUTPUT)
print(f"wrote {OUTPUT}: shape={vec.shape} norm={float(np.linalg.norm(vec)):.6f}")
