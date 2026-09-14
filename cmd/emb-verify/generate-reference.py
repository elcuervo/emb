#!/usr/bin/env python3
"""Generate the reference embeddings for `just verify-embeddings`.

The artifact records the model, the sentence set, the generator version, the
installed dependency versions, and a checksum over the embeddings, so a stale
or foreign file cannot be silently compared against (the Go loader validates
the checksum and the requested model/dim).

The checksum is the sha256 of the embeddings as little-endian float32 bytes in
row-major order — the same bytes `internal/embverify` hashes, so the value is
reproducible across the language boundary.
"""

import argparse
import hashlib
import json
import os
import platform
import sys
from datetime import datetime, timezone

GENERATOR = "generate-reference.py"
GENERATOR_VERSION = "2"

MODEL = "sentence-transformers/all-MiniLM-L6-v2"

TEST_SENTENCES = [
    "hello world",
    "this is a test sentence",
    "the quick brown fox jumps over the lazy dog",
    "embeddings are useful for semantic search",
    "how are you doing today",
    "machine learning is transforming technology",
    "I love programming in Go",
    "the weather is nice this morning",
    "natural language processing is fascinating",
    "once upon a time in a far away land",
    "please generate an embedding for this text",
    "similarity between sentences can be measured",
    "the cat sat on the mat",
    "artificial intelligence is evolving rapidly",
    "have a great day",
    "Redis is an in-memory data structure store",
    "vector databases enable similarity search",
    "the capital of France is Paris",
    "deep learning models require large datasets",
    "goodbye and see you later",
]

DEFAULT_OUTPUT = "reference-embeddings.json"


def checksum(embeddings) -> str:
    import numpy as np

    arr = np.asarray(embeddings, dtype="<f4")  # little-endian float32, row-major
    return hashlib.sha256(arr.tobytes()).hexdigest()


def installed_versions() -> dict:
    from importlib import metadata

    out = {"python": platform.python_version()}
    for name in ("sentence-transformers", "torch"):
        try:
            out[name] = metadata.version(name)
        except metadata.PackageNotFoundError:
            out[name] = "unknown"
    return out


def main() -> int:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--output", default=DEFAULT_OUTPUT, help="artifact path")
    parser.add_argument("--model", default=MODEL, help="sentence-transformers model")
    parser.add_argument(
        "--refresh",
        action="store_true",
        help="overwrite an existing artifact instead of leaving it in place",
    )
    args = parser.parse_args()

    if os.path.exists(args.output) and not args.refresh:
        print(f"✓ {args.output} already exists (use --refresh to regenerate)")
        return 0

    print(f"Loading {args.model}...")
    from sentence_transformers import SentenceTransformer

    model = SentenceTransformer(args.model)
    print(f"Generating embeddings for {len(TEST_SENTENCES)} sentences...")
    embeddings = model.encode(TEST_SENTENCES, normalize_embeddings=True)

    rows = [row.tolist() for row in embeddings]
    data = {
        "model": args.model,
        "dim": int(embeddings.shape[1]),
        "sentences": TEST_SENTENCES,
        "embeddings": rows,
        "generator": GENERATOR,
        "generator_version": GENERATOR_VERSION,
        "created": datetime.now(timezone.utc).isoformat(timespec="seconds"),
        "requirements": installed_versions(),
        "checksum": checksum(embeddings),
    }
    with open(args.output, "w", encoding="utf-8") as f:
        json.dump(data, f, indent=2)
    print(f"✓ {args.output} saved ({len(rows)} × {data['dim']} dim, sha256 {data['checksum'][:12]}…)")
    return 0


if __name__ == "__main__":
    sys.exit(main())
