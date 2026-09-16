#!/usr/bin/env python3
"""Embed the gallery's corpus, index it, and write the atlas's numbers.

This is the offline half of the gallery: the corpus in `poe.jsonl` goes through
a **local** `emb` server, and the vectors land in a committed SQLite database
with one `sqlite-vec` table per model, plus the build-time 2D projection and
cluster regions the atlas draws. The browser never embeds the corpus; it embeds
one query and searches the shipped index.

    just website-demos                      # minilm + bge-small
    python3 website/tools/build-demo-db.py --models minilm,bge-small

Two files come out, both content-hashed in their names so a rebuild cannot be
served stale and a returning visitor cannot receive the previous index:

    website/assets/demo/poe-<hash8>.db      the index
    website/assets/demo/poe-<hash8>.json    its manifest

Everything a page displays is read from the manifest rather than typed into the
page: dimensions, counts, precision, the model names, the cluster labels, and the
recall probe. Nothing about a vector is transcribed.

The pipeline, in order:

  1. read the corpus and hash it (the hash names the output),
  2. ask the server for `EMB.MODELS` and refuse to continue if a requested model
     is not the model the server serves -- a mismatch is a loud failure, not a
     silently wrong index,
  3. embed every passage against the local server, pipelined over RESP,
  4. project the vectors to 2D with PCA and cluster the projection with k-means,
  5. quantize to int8 and build a `vec0` table per model,
  6. measure the index's `recall@k` against an exact float32 search over the same
     vectors, so the precision is a measurement rather than a promise,
  7. write both files and delete the previous ones.

`int8` halves the corpus's vectors and costs nothing a demo can perceive; the
recall probe in the manifest is what says so.
"""

from __future__ import annotations

import argparse
import hashlib
import json
import os
import random
import re
import socket
import sqlite3
import struct
import sys
from pathlib import Path

import numpy as np

REPO_ROOT = Path(__file__).resolve().parents[2]
CORPUS = REPO_ROOT / "website" / "tools" / "demos" / "poe.jsonl"
OUT_DIR = REPO_ROOT / "website" / "assets" / "demo"

DEFAULT_MODELS = ["minilm", "bge-small"]
DEFAULT_EMB = "127.0.0.1:16379"

EMBED_BATCH = 128
KMEANS_K = 10
KMEANS_SEED = 20260916
RECALL_K = 10
RECALL_QUERIES = 32

# The queries the atlas's caption and the search plate are read against. They
# are embedded too, so the recall probe measures the index against the questions
# the page actually asks.
PROBES = [
    "a guilty conscience that will not stay buried",
    "a ship in a storm",
    "the dead returning",
    "grief for a lost bride",
    "a detective who reads a mind",
]

ATTRIBUTION = "Texts: Edgar Allan Poe (1809–1849), public domain, via Project Gutenberg."

# Cluster regions are hand-named from the computed centroids: the label is what
# turns a cloud of dots into an atlas. Keyed by model then cluster id, and
# curated in task 5.2 after reading the membership of each cluster. An unnamed
# cluster falls back to its index so the atlas is still readable when the corpus
# shifts a centroid.
CLUSTER_LABELS: dict[str, list[str]] = {
    # Read off `--report-regions`, which prints each region's passages closest to
    # its centroid. A region is named only when its members say what it is.
    "minilm": [
        "THE DESCENT",     # Pym's chasm, Pfaall's fall, the vault under Usher
        "THE AIR",         # the balloon, the height, the aerial view
        "THE BRIDE",       # Ligeia, the Spectacles, the Oval Portrait
        "DETECTION",       # the two Dupin cases, the burial that was not one
        "THE LITERATI",    # Thingum Bob, X-ing a Paragrab, the literary quarrel
        "THE METAPHYSIC",  # Mesmeric Revelation, Berenice's deductions, Morella
        "THE DREAM",       # the verse: Tamerlane, The Bells, The Sleeper
        "THE CRIME",       # the mutiny, the deed, the boast
        "THE PIT",         # the pendulum, the doom prepared, Valdemar
        "THE SEA",         # King Pest's sailors, the Maelström, the bottle
    ],
    "bge-small": [
        "THE TREATISE",    # the anatomical account, the cited authority
        "THE AIR",
        "THE DREAM",
        "THE PURSUIT",     # concealment, flight, being hunted
        "THE PLAGUE",      # the Red Death, Valdemar, the malady
        "THE METAPHYSIC",
        "THE SEA",
        "THE TRICK",       # Diddling, the intercepted letter, the con
        "DETECTION",
        "THE WHIRLPOOL",   # the ice, the depths, the whirl
    ],
}


class RespError(RuntimeError):
    pass


class Resp:
    """The smallest RESP2 client that can drive `EMB` and read its replies."""

    def __init__(self, host: str, port: int, timeout: float = 300.0):
        self.sock = socket.create_connection((host, port), timeout=timeout)
        self.file = self.sock.makefile("rb")

    def close(self) -> None:
        self.file.close()
        self.sock.close()

    def send(self, *args: str) -> None:
        out = [b"*%d\r\n" % len(args)]
        for arg in args:
            raw = arg.encode("utf-8") if isinstance(arg, str) else arg
            out.append(b"$%d\r\n" % len(raw))
            out.append(raw + b"\r\n")
        self.sock.sendall(b"".join(out))

    def read(self):
        line = self.file.readline()
        if not line:
            raise RespError("the server closed the connection")
        kind, body = line[:1], line[1:-2]
        if kind == b"+":
            return body.decode("utf-8")
        if kind == b"-":
            raise RespError(body.decode("utf-8"))
        if kind == b":":
            return int(body)
        if kind == b"$":
            length = int(body)
            if length < 0:
                return None
            data = self.file.read(length + 2)
            return data[:-2]
        if kind == b"*":
            count = int(body)
            if count < 0:
                return None
            return [self.read() for _ in range(count)]
        raise RespError(f"unexpected RESP type {kind!r}")

    def call(self, *args: str):
        self.send(*args)
        return self.read()

    def pipeline(self, batches: list[tuple[str, ...]]):
        for batch in batches:
            self.send(*batch)
        return [self.read() for _ in batches]


def connect(spec: str) -> Resp:
    host, _, port = spec.rpartition(":")
    return Resp(host or "127.0.0.1", int(port or 6379))


def server_models(conn: Resp) -> dict[str, int]:
    """`EMB.MODELS` as {name: dimension} -- the server's own model list."""
    reply = conn.call("EMB.MODELS")
    flat = [item.decode("utf-8") if isinstance(item, bytes) else item for item in reply]
    if flat and all(isinstance(item, (str, int)) for item in flat):
        # Flat name/dim/status triples.
        flat = [flat[i:i + 3] for i in range(0, len(flat), 3)]
    models: dict[str, int] = {}
    for row in flat:
        name = row[0].decode("utf-8") if isinstance(row[0], bytes) else row[0]
        models[name] = int(row[1])
    return models


def model_info(conn: Resp, name: str) -> dict[str, str]:
    reply = conn.call("EMB.INFO", name)
    rows = [item.decode("utf-8") if isinstance(item, bytes) else item for item in reply]
    return {rows[i]: rows[i + 1] for i in range(0, len(rows) - 1, 2)}


def embed(conn: Resp, model: str, texts: list[str], label: str) -> np.ndarray:
    """Embed `texts` through the local server and return an (n, dim) float32 array."""
    vectors: list[np.ndarray] = []
    for start in range(0, len(texts), EMBED_BATCH):
        chunk = texts[start:start + EMBED_BATCH]
        replies = conn.pipeline([("EMB", model, text) for text in chunk])
        for reply in replies:
            if isinstance(reply, bytes) and len(reply) % 4 == 0:
                vectors.append(np.frombuffer(reply, dtype="<f4"))
            else:
                sys.exit(f"build-demo-db: {label}: the server answered {reply!r} instead of a vector")
        if sys.stderr.isatty():
            print(f"  {label}: {min(start + EMBED_BATCH, len(texts))}/{len(texts)}", end="\r", file=sys.stderr)
    if sys.stderr.isatty():
        print(" " * 60, end="\r", file=sys.stderr)
    matrix = np.vstack(vectors).astype(np.float32)
    if not np.isfinite(matrix).all():
        sys.exit(f"build-demo-db: {label}: the server returned a non-finite vector")
    return matrix


def pca2(matrix: np.ndarray) -> np.ndarray:
    """Project to 2D with PCA. Build-time only: the page never projects."""
    centred = matrix - matrix.mean(axis=0, keepdims=True)
    _, _, components = np.linalg.svd(centred, full_matrices=False)
    return centred @ components[:2].T


def kmeans(matrix: np.ndarray, k: int, seed: int) -> tuple[np.ndarray, np.ndarray]:
    """k-means++ over the vectors, seeded so a rebuild is a diff.

    Run over the embeddings, not over the 2D projection: a projection that keeps
    two components keeps document length and work identity, and clusters it into
    "which story" rather than "what about". The projection is for drawing; the
    clustering is for meaning, and the centroids are projected afterwards.
    """
    points = matrix.astype(np.float64)
    rng = random.Random(seed)
    centres = [points[rng.randrange(len(points))]]
    for _ in range(1, k):
        distance = np.min([((points - centre) ** 2).sum(axis=1) for centre in centres], axis=0)
        total = distance.sum()
        weights = (distance / total).tolist() if total > 0 else None
        centres.append(points[rng.choices(range(len(points)), weights=weights)[0]])
    centres = np.array(centres, dtype=np.float64)

    norms = (points ** 2).sum(axis=1)[:, None]
    labels = np.zeros(len(points), dtype=np.int64)
    for _ in range(64):
        distance = norms - 2.0 * points @ centres.T + (centres ** 2).sum(axis=1)[None, :]
        labels = distance.argmin(axis=1)
        moved = np.array([
            points[labels == i].mean(axis=0) if (labels == i).any() else centres[i]
            for i in range(k)
        ])
        if np.allclose(moved, centres, atol=1e-9):
            centres = moved
            break
        centres = moved
    return labels, centres


def regions(points: np.ndarray, labels: np.ndarray, works: list[str], model: str) -> list[dict]:
    """Each cluster as a labelled region with the figures the caption carries."""
    names = CLUSTER_LABELS.get(model, [])
    out = []
    for i in range(int(labels.max()) + 1):
        members = points[labels == i]
        if len(members) == 0:
            continue
        centre = members.mean(axis=0)
        radius = float(np.percentile(np.sqrt(((members - centre) ** 2).sum(axis=1)), 95))
        counted = {}
        for name in (works[j] for j in np.nonzero(labels == i)[0]):
            counted[name] = counted.get(name, 0) + 1
        out.append({
            "id": i,
            "label": names[i] if i < len(names) and names[i] else f"REGION {i + 1}",
            "cx": round(float(centre[0]), 4),
            "cy": round(float(centre[1]), 4),
            "radius": round(radius, 4),
            "count": int(len(members)),
            "top_works": [name for name, _ in sorted(counted.items(), key=lambda kv: -kv[1])[:3]],
        })
    return out


def display(path: Path) -> str:
    """A repo-relative path when it has one, so a --out inside the repo reads
    short and a throwaway --out outside it still prints."""
    try:
        return str(path.relative_to(REPO_ROOT))
    except ValueError:
        return str(path)


def quantize(matrix: np.ndarray) -> bytes:
    """int8 vectors: the embeddings are unit-length, so ×127 is the whole scale."""
    clipped = np.clip(np.rint(matrix * 127.0), -128, 127).astype(np.int64)
    return clipped.astype(np.uint8).tobytes()


def exact_topk(matrix: np.ndarray, queries: np.ndarray, k: int) -> list[set[int]]:
    """Exact float32 top-k, as 1-based rowids so it is comparable to `vec0`."""
    distance = ((queries[:, None, :] - matrix[None, :, :]) ** 2).sum(axis=2)
    return [set((np.argsort(row)[:k] + 1).tolist()) for row in distance]


def approximate_topk(db: sqlite3.Connection, table: str, queries: np.ndarray, k: int) -> list[set[int]]:
    out = []
    for query in queries:
        rows = db.execute(
            f"SELECT rowid FROM {table} WHERE embedding MATCH vec_int8(?) AND k = ? ORDER BY distance",
            (sqlite3.Binary(quantize(query.reshape(1, -1))), k),
        ).fetchall()
        out.append({row[0] for row in rows})
    return out


def load_extension(db: sqlite3.Connection, path: str | None) -> None:
    candidates = [path] if path else []
    env = os.environ.get("SQLITE_VEC_EXT")
    if env:
        candidates.append(env)
    lib = os.environ.get("SQLITE_VEC_LIB")
    if lib:
        candidates.extend(str(p) for p in sorted(Path(lib).glob("vec0.*")))
    for candidate in candidates:
        if candidate and Path(candidate).exists():
            db.enable_load_extension(True)
            db.load_extension(candidate)
            db.enable_load_extension(False)
            return
    sys.exit(
        "build-demo-db: sqlite-vec is not on the path.\n"
        "  run inside `nix develop`, which exports SQLITE_VEC_LIB, or pass --sqlite-vec <vec0.dylib>"
    )


def report_regions(models: list[str], passages: list[dict], vectors: dict[str, np.ndarray], n: int) -> None:
    """Print what each computed region actually contains, for the hand-naming."""
    for model in models:
        matrix = vectors[model]
        labels, _ = kmeans(matrix - matrix.mean(axis=0, keepdims=True), KMEANS_K, KMEANS_SEED)
        points = pca2(matrix)
        names = CLUSTER_LABELS.get(model, [])
        print(f"===== {model}")
        for i in range(int(labels.max()) + 1):
            members = np.nonzero(labels == i)[0]
            centre = points[members].mean(axis=0)
            nearest = members[np.argsort(((points[members] - centre) ** 2).sum(axis=1))][:n]
            counted: dict[str, int] = {}
            for j in members:
                counted[passages[j]["work"]] = counted.get(passages[j]["work"], 0) + 1
            top = ", ".join(name for name, _ in sorted(counted.items(), key=lambda kv: -kv[1])[:3])
            current = names[i] if i < len(names) and names[i] else f"REGION {i + 1}"
            print(f"  {i:2d} {current:14s} n={len(members):4d}  {top}")
            for j in nearest:
                passage = passages[j]
                print(f"       [{passage['work']} {passage['year']}] {passage['snippet'][:96]}")
    print("\nName the regions above in CLUSTER_LABELS, then rebuild without --report-regions.")


def main() -> int:
    parser = argparse.ArgumentParser(description=__doc__, formatter_class=argparse.RawDescriptionHelpFormatter)
    parser.add_argument("--models", default=",".join(DEFAULT_MODELS),
                        help=f"comma-separated models the sandbox serves (default: {','.join(DEFAULT_MODELS)})")
    parser.add_argument("--emb", default=os.environ.get("EMB_ADDR", DEFAULT_EMB),
                        help=f"the local emb server (default: {DEFAULT_EMB})")
    parser.add_argument("--corpus", type=Path, default=CORPUS)
    parser.add_argument("--out", type=Path, default=OUT_DIR)
    parser.add_argument("--sqlite-vec", default=None, help="path to the sqlite-vec extension")
    parser.add_argument("--report-regions", type=int, default=0, metavar="N",
                        help="print each computed region's closest passages and exit; this is what the hand-naming reads")
    args = parser.parse_args()

    models = [m.strip() for m in args.models.split(",") if m.strip()]
    if not models:
        sys.exit("build-demo-db: --models is empty")

    corpus_bytes = args.corpus.read_bytes()
    corpus_hash = hashlib.sha256(corpus_bytes).hexdigest()
    passages = [json.loads(line) for line in corpus_bytes.decode("utf-8").splitlines() if line.strip()]
    if not passages:
        sys.exit(f"build-demo-db: {args.corpus} is empty")
    print(f"corpus: {len(passages)} passages · sha256:{corpus_hash[:12]}")

    conn = connect(args.emb)
    served = server_models(conn)
    for model in models:
        if model not in served:
            sys.exit(
                f"build-demo-db: {model!r} is not a model this server serves "
                f"(it serves: {', '.join(sorted(served))}). "
                "An index must name the model that built it."
            )

    texts = [p["text"] for p in passages]
    vectors: dict[str, np.ndarray] = {}
    details: dict[str, dict] = {}
    for model in models:
        info = model_info(conn, model)
        dimension = served[model]
        print(f"{model}: dim {dimension} · max_length {info.get('max_length')} · quantization {info.get('quantization')}")
        if int(info.get("dim", dimension)) != dimension:
            sys.exit(f"build-demo-db: {model}: EMB.MODELS says {dimension} dimensions and EMB.INFO says {info.get('dim')}")
        vectors[model] = embed(conn, model, texts, model)
        if vectors[model].shape[1] != dimension:
            sys.exit(f"build-demo-db: {model}: the server returned {vectors[model].shape[1]}-element vectors, not {dimension}")
        details[model] = {
            "name": model,
            "table": "vec_" + re.sub(r"[^a-z0-9]+", "_", model.lower()).strip("_"),
            "dimension": dimension,
            "max_length": int(info.get("max_length", 0) or 0),
            "quantization": info.get("quantization", "unknown"),
        }

    queries = PROBES + [passages[i]["text"] for i in
                        np.linspace(0, len(passages) - 1, RECALL_QUERIES, dtype=int)]

    if args.report_regions:
        report_regions(models, passages, vectors, args.report_regions)
        return 0

    args.out.mkdir(parents=True, exist_ok=True)
    db_path = args.out / f"poe-{corpus_hash[:8]}.db"
    tmp_path = db_path.with_suffix(".db.part")
    if tmp_path.exists():
        tmp_path.unlink()

    db = sqlite3.connect(tmp_path)
    load_extension(db, args.sqlite_vec)
    db.execute("PRAGMA journal_mode = DELETE")
    db.execute("PRAGMA page_size = 4096")
    db.execute(
        "CREATE TABLE passages (rowid INTEGER PRIMARY KEY, id TEXT UNIQUE NOT NULL, "
        "work TEXT NOT NULL, year INTEGER NOT NULL, idx INTEGER NOT NULL, "
        "snippet TEXT NOT NULL, text TEXT NOT NULL)"
    )
    db.execute("CREATE TABLE projection (model TEXT NOT NULL, rowid INTEGER NOT NULL, x REAL NOT NULL, y REAL NOT NULL, "
               "PRIMARY KEY (model, rowid))")
    db.execute("CREATE TABLE regions (model TEXT NOT NULL, id INTEGER NOT NULL, label TEXT NOT NULL, "
               "cx REAL NOT NULL, cy REAL NOT NULL, radius REAL NOT NULL, count INTEGER NOT NULL, "
               "top_works TEXT NOT NULL, PRIMARY KEY (model, id))")

    for rowid, passage in enumerate(passages, start=1):
        db.execute(
            "INSERT INTO passages (rowid, id, work, year, idx, snippet, text) VALUES (?, ?, ?, ?, ?, ?, ?)",
            (rowid, passage["id"], passage["work"], passage["year"], passage["index"], passage["snippet"], passage["text"]),
        )

    manifest_models = []
    for model in models:
        detail = details[model]
        table = detail["table"]
        matrix = vectors[model]

        db.execute(f"CREATE VIRTUAL TABLE {table} USING vec0(embedding int8[{detail['dimension']}])")
        for rowid, vector in enumerate(matrix, start=1):
            db.execute(f"INSERT INTO {table} (rowid, embedding) VALUES (?, vec_int8(?))",
                       (rowid, sqlite3.Binary(quantize(vector.reshape(1, -1)))))

        points = pca2(matrix)
        # Cluster the embeddings with their shared component removed. Every
        # passage is English prose in one author's voice, so the mean vector is
        # "Poe" and it swamps the axes that carry meaning; subtracting it is
        # what makes the regions themes (the sea, the grave, detection) rather
        # than a single cloud. The projection is drawn from the raw vectors.
        labels, _ = kmeans(matrix - matrix.mean(axis=0, keepdims=True), KMEANS_K, KMEANS_SEED)
        for rowid, (x, y) in enumerate(points, start=1):
            db.execute("INSERT INTO projection (model, rowid, x, y) VALUES (?, ?, ?, ?)",
                       (model, rowid, round(float(x), 5), round(float(y), 5)))
        cluster_rows = regions(points, labels, [p["work"] for p in passages], model)
        for region in cluster_rows:
            db.execute("INSERT INTO regions (model, id, label, cx, cy, radius, count, top_works) "
                       "VALUES (?, ?, ?, ?, ?, ?, ?, ?)",
                       (model, region["id"], region["label"], region["cx"], region["cy"],
                        region["radius"], region["count"], json.dumps(region["top_works"], ensure_ascii=False)))

        query_vectors = embed(conn, model, queries, f"{model} probes")
        expected = exact_topk(matrix, query_vectors, RECALL_K)
        actual = approximate_topk(db, table, query_vectors, RECALL_K)
        recall = sum(len(e & a) for e, a in zip(expected, actual)) / (len(expected) * RECALL_K)

        detail["projection"] = "pca2"
        detail["recall_at_k"] = round(recall, 4)
        detail["recall_k"] = RECALL_K
        detail["regions"] = cluster_rows
        manifest_models.append(detail)
        print(f"{model}: recall@{RECALL_K} {recall:.3f} · {len(cluster_rows)} regions · table {table}")

    db.commit()
    sqlite_vec_version = db.execute("SELECT vec_version()").fetchone()[0]
    db.execute("VACUUM")
    db.close()

    identity = json.dumps({"corpus": corpus_hash, "models": models, "precision": "int8",
                           "k": KMEANS_K, "recall": RECALL_K}, sort_keys=True)
    digest = hashlib.sha256(identity.encode("utf-8")).hexdigest()[:8]
    # A stable name derived from the index's identity, so the file can also keep
    # the identity-derived name the manifest points at.
    final_db = args.out / f"poe-{corpus_hash[:8]}.db"
    tmp_path.replace(final_db)

    try:
        corpus_path = str(args.corpus.relative_to(REPO_ROOT))
    except ValueError:
        corpus_path = str(args.corpus)
    manifest = {
        "asset": {"db": final_db.name, "bytes": final_db.stat().st_size, "hash": digest},
        "attribution": ATTRIBUTION,
        "count": len(passages),
        "corpus": {"path": corpus_path, "sha256": corpus_hash,
                   "works": len({p["work"] for p in passages}), "words": sum(len(p["text"].split()) for p in passages)},
        "generated_by": "website/tools/build-demo-db.py",
        "models": manifest_models,
        "precision": "int8",
        "sqlite_vec": sqlite_vec_version,
        "vector_type": "vec_int8",
    }
    manifest_path = args.out / "manifest.json"
    manifest_path.write_text(json.dumps(manifest, ensure_ascii=False, indent=2, sort_keys=True) + "\n", encoding="utf-8")

    # A stale index is a page that describes a corpus that is no longer there.
    # The manifest keeps its stable name so the page has one entry point; only
    # the index is content-hashed, which is what makes its name unservable-stale.
    for stale in args.out.glob("*"):
        if stale not in (final_db, manifest_path):
            stale.unlink()
            print(f"removed stale {stale.name}")

    print(f"wrote {display(final_db)} ({final_db.stat().st_size} bytes)")
    print(f"wrote {display(manifest_path)}")
    conn.close()
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
