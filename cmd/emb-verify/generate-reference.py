#!/usr/bin/env python3
"""Generate reference embeddings for validation using sentence-transformers."""

import json, hashlib, os, sys

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

OUTPUT = "reference-embeddings.json"


def main():
    if os.path.exists(OUTPUT):
        print(f"✓ {OUTPUT} already exists, skipping generation")
        return

    print(f"Loading {MODEL}...")
    from sentence_transformers import SentenceTransformer

    model = SentenceTransformer(MODEL)

    print(f"Generating embeddings for {len(TEST_SENTENCES)} sentences...")
    embeddings = model.encode(TEST_SENTENCES, normalize_embeddings=True)

    data = {
        "model": MODEL,
        "dim": embeddings.shape[1],
        "sentences": TEST_SENTENCES,
        "embeddings": [e.tolist() for e in embeddings],
    }
    with open(OUTPUT, "w") as f:
        json.dump(data, f, indent=2)
    print(f"✓ {OUTPUT} saved ({len(embeddings)} × {embeddings.shape[1]} dim)")


if __name__ == "__main__":
    main()
