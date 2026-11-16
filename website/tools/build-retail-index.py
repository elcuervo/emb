#!/usr/bin/env python3
"""Append the retail stage to the gallery's one working index.

This stage runs after the text stage and before `demo_index.py finalize`. It
reads the offline report, the committed catalog selection and the frozen
centroids, embeds the shipped rows through the same local EMB the text stage
used, and writes only `retail_*` tables plus `meta["retail"]`. Poe, projection,
region and media tables are never dropped.

    just website-retail                      # text stage, this stage, finalize
    python3 website/tools/build-retail-index.py --emb 127.0.0.1:16389
    python3 website/tools/build-retail-index.py --check   # no archive, no server

`--check` validates the pins, the report, the committed samples and the
model/centroid provenance, and — when a working or published index exists — the
retail tables that index carries.
"""

from __future__ import annotations

import argparse
import hashlib
import json
import os
import re
import socket
import sqlite3
import sys
from pathlib import Path

import numpy as np

HERE = Path(__file__).resolve().parent
REPO_ROOT = HERE.parents[1]
sys.path.insert(0, str(HERE))
sys.path.insert(0, str(HERE / "retail"))

import check_retail  # noqa: E402
import demo_index  # noqa: E402

LOCK = HERE / "retail" / "sources.lock.json"
REPORT = REPO_ROOT / "website" / "assets" / "demo" / "retail" / "report.json"
CATALOG = HERE / "retail" / "catalog.jsonl"
CENTROIDS = HERE / "retail" / "centroids.jsonl"
BASIS = HERE / "retail" / "projection.json"
HELDOUT = HERE / "retail" / "projected_heldout.jsonl"
MANIFEST = demo_index.MANIFEST_PATH
EMBED_BATCH = 128
RETRIEVAL_K = 10


class Resp:
    def __init__(self, host: str, port: int, timeout: float = 600.0):
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
            raise RuntimeError("the server closed the connection")
        kind, body = line[:1], line[1:-2]
        if kind == b"+":
            return body.decode("utf-8")
        if kind == b"-":
            raise RuntimeError(body.decode("utf-8"))
        if kind == b"$":
            length = int(body)
            return None if length < 0 else self.file.read(length + 2)[:-2]
        if kind == b":":
            return int(body)
        if kind == b"*":
            count = int(body)
            return None if count < 0 else [self.read() for _ in range(count)]
        raise RuntimeError(f"unexpected RESP type {kind!r}")

    def call(self, *args: str):
        self.send(*args)
        return self.read()

    def pipeline(self, batches):
        for batch in batches:
            self.send(*batch)
        return [self.read() for _ in batches]


def connect(spec: str) -> Resp:
    host, _, port = spec.rpartition(":")
    return Resp(host or "127.0.0.1", int(port or 6379))


def load_jsonl(path: Path) -> list[dict]:
    return [json.loads(line) for line in path.read_text(encoding="utf-8").splitlines() if line.strip()]


def sha256(path: Path) -> str:
    return hashlib.sha256(path.read_bytes()).hexdigest()


def load_plan() -> tuple[dict, dict, list[dict], list[dict], dict, list[dict]]:
    missing = [str(path) for path in (LOCK, REPORT, CATALOG, CENTROIDS, BASIS, HELDOUT) if not path.exists()]
    if missing:
        sys.exit("build-retail-index: absent: " + ", ".join(missing) + "\n  run `just website-retail-analysis` first")
    lock = json.loads(LOCK.read_text(encoding="utf-8"))
    report = json.loads(REPORT.read_text(encoding="utf-8"))
    catalog = load_jsonl(CATALOG)
    centroids = load_jsonl(CENTROIDS)
    basis = json.loads(BASIS.read_text(encoding="utf-8"))
    held_out = load_jsonl(HELDOUT)
    errors = check_retail.check(lock, report, catalog, centroids, held_out, basis)
    if errors:
        for error in errors:
            print(f"build-retail-index: {error}", file=sys.stderr)
        sys.exit("build-retail-index: the retail plan failed its checks")
    return lock, report, catalog, centroids, basis, held_out


def embed(conn: Resp, model: str, texts: list[str], dimension: int) -> np.ndarray:
    vectors = []
    for start in range(0, len(texts), EMBED_BATCH):
        replies = conn.pipeline([("EMB", model, text) for text in texts[start:start + EMBED_BATCH]])
        for reply in replies:
            if not isinstance(reply, bytes) or len(reply) != dimension * 4:
                sys.exit(f"build-retail-index: the server answered {reply!r} instead of {dimension} float32")
            vectors.append(np.frombuffer(reply, dtype="<f4"))
    matrix = np.vstack(vectors).astype(np.float32)
    norms = np.linalg.norm(matrix, axis=1, keepdims=True)
    if (norms == 0).any() or not np.isfinite(matrix).all():
        sys.exit("build-retail-index: the server returned a non-finite or zero vector")
    return (matrix / norms).astype(np.float32)


def quantize(matrix: np.ndarray) -> list[bytes]:
    clipped = np.clip(np.rint(matrix * 127.0), -127, 127).astype(np.int8)
    return [clipped[row].tobytes() for row in range(len(clipped))]


def exact_topk(matrix: np.ndarray, queries: np.ndarray, k: int) -> list[set[int]]:
    distance = ((queries.astype(np.float64)[:, None, :] - matrix.astype(np.float64)[None, :, :]) ** 2).sum(axis=2)
    return [set((np.argsort(row)[:k] + 1).tolist()) for row in distance]


def table_name(model: str) -> str:
    return "vec_retail_" + re.sub(r"[^a-z0-9]+", "_", model.lower()).strip("_")


def insert_rows(db: sqlite3.Connection, table: str, rows: list[dict], sample_flag: bool) -> None:
    db.executemany(
        f"INSERT INTO {table} (fdc_id, description, brand, gtin, category, pack, text, sample, "
        "x, y, z, cluster) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)",
        [(row["fdc_id"], row["description"], row["brand"], row["gtin"], row["category"],
          row["pack"], row["text"], 1 if sample_flag or row.get("sample") else 0,
          row["projection"][0], row["projection"][1], row["projection"][2],
          row.get("cluster")) for row in rows],
    )


def retail_summary(report: dict, catalog: list[dict], centroid_rows: list[dict], table: str) -> dict:
    model = report["model"]
    config = report["canonical"]["config"]
    samples = [row["fdc_id"] for row in catalog if row.get("sample")]
    arrivals = [row["fdc_id"] for row in catalog if row["role"] == "arrival"]
    return {
        "model": model["name"],
        "model_entry": f"retail-{model['name']}",
        "dimension": model["dimension"],
        "normalization": model["normalization"],
        "table": table,
        "centroid_table": "retail_centroids",
        "centroid_count": config["centroids"],
        "seed": config["seed"],
        "source": {
            "catalog": report["catalog"]["source"],
            "catalog_page": report["catalog"]["catalog_page"],
            "documentation": report["catalog"]["documentation"],
            "attribution": report["catalog"]["attribution"],
            "releases": report["catalog"]["releases"],
            "terms": report["catalog"]["redistribution"],
            "overlap": report["catalog"]["overlap"],
        },
        "populations": report["populations"],
        "samples": {"catalog": report["samples"]["catalog"], "arrivals": arrivals},
        "report": "retail/report.json",
        "report_sha256": sha256(REPORT),
        "catalog_sha256": sha256(CATALOG),
        "centroids_sha256": sha256(CENTROIDS),
        "review_rule": report["review_rule"],
        "retrieval": {"k": RETRIEVAL_K, "recall_at_k": report["retrieval"]["recall_at_k"]},
        "projection": {"method": "pca", "axes": 3, "fit": "shipped product embeddings"},
        "clusters": [{"id": cluster["id"], "label": cluster["label"], "count": cluster["count"]}
                     for cluster in report["clusters"]],
        "sample_count": len(samples),
    }


def build(args) -> int:
    lock, report, catalog, centroids, basis, held_out = load_plan()
    db = demo_index.open_working(fresh=False)
    meta = demo_index.read_meta(db)
    if "index" not in meta:
        db.close()
        sys.exit("build-retail-index: the working file has no text stage; run `just website-demos` first "
                 "(retail appends to the text index, it does not create one)")
    tables = {row[0] for row in db.execute("SELECT name FROM sqlite_master WHERE type IN ('table', 'view')")}
    if "passages" not in tables:
        db.close()
        sys.exit("build-retail-index: the working file has no passages table; the text stage did not finish")
    demo_index.load_extension(db, args.sqlite_vec)

    shipped = [row for row in catalog if row["role"] == "shipped"]
    arrivals = [row for row in catalog if row["role"] == "arrival"]
    samples = [row for row in shipped if row.get("sample")]
    if not shipped or not samples or not arrivals:
        db.close()
        sys.exit("build-retail-index: the catalog selection is missing shipped, sample or arrival rows")

    model = report["model"]
    table = table_name(model["name"])
    for name in ("retail_products", "retail_arrivals", "retail_centroids", "retail_clusters",
                 "retail_heldout", "retail_projection", table):
        db.execute(f"DROP TABLE IF EXISTS {name}")
    db.execute(
        "CREATE TABLE retail_products (rowid INTEGER PRIMARY KEY, fdc_id INTEGER UNIQUE NOT NULL, "
        "description TEXT NOT NULL, brand TEXT NOT NULL, gtin TEXT NOT NULL, category TEXT NOT NULL, "
        "pack TEXT NOT NULL, text TEXT NOT NULL, sample INTEGER NOT NULL, embedding BLOB, "
        "x REAL NOT NULL, y REAL NOT NULL, z REAL NOT NULL, cluster INTEGER)"
    )
    db.execute(
        "CREATE TABLE retail_arrivals (rowid INTEGER PRIMARY KEY, fdc_id INTEGER UNIQUE NOT NULL, "
        "description TEXT NOT NULL, brand TEXT NOT NULL, gtin TEXT NOT NULL, category TEXT NOT NULL, "
        "pack TEXT NOT NULL, text TEXT NOT NULL, sample INTEGER NOT NULL, embedding BLOB, "
        "x REAL NOT NULL, y REAL NOT NULL, z REAL NOT NULL, cluster INTEGER)"
    )
    db.execute(
        "CREATE TABLE retail_centroids (model TEXT NOT NULL, centroid_count INTEGER NOT NULL, "
        "seed INTEGER NOT NULL, id INTEGER NOT NULL, label TEXT NOT NULL, vector BLOB NOT NULL, "
        "x REAL NOT NULL, y REAL NOT NULL, z REAL NOT NULL, PRIMARY KEY (model, id))"
    )
    db.execute(
        "CREATE TABLE retail_clusters (id INTEGER PRIMARY KEY, label TEXT NOT NULL, count INTEGER NOT NULL, "
        "categories TEXT NOT NULL, representatives TEXT NOT NULL)"
    )
    db.execute(
        "CREATE TABLE retail_heldout (rowid INTEGER PRIMARY KEY, x REAL NOT NULL, y REAL NOT NULL, "
        "z REAL NOT NULL, cluster INTEGER NOT NULL)"
    )
    db.execute(
        "CREATE TABLE retail_projection (id INTEGER PRIMARY KEY, mean BLOB NOT NULL, basis BLOB NOT NULL)"
    )

    insert_rows(db, "retail_products", shipped, False)
    insert_rows(db, "retail_arrivals", arrivals, True)

    conn = connect(args.emb)
    reply = conn.call("EMB.MODELS")
    if reply and isinstance(reply[0], list):
        rows = [[item.decode() if isinstance(item, bytes) else item for item in row] for row in reply]
    else:
        flat = [item.decode() if isinstance(item, bytes) else item for item in reply]
        rows = [flat[i:i + 3] for i in range(0, len(flat), 3)]
    served = [row[0] for row in rows]
    if model["name"] not in served:
        db.close()
        conn.close()
        sys.exit(f"build-retail-index: the server does not serve {model['name']!r}; it serves {', '.join(served)}")
    print(f"embedding {len(shipped)} shipped products through {model['name']}")
    matrix = embed(conn, model["name"], [row["text"] for row in shipped], model["dimension"])
    conn.close()

    db.execute(f"CREATE VIRTUAL TABLE {table} USING vec0(embedding int8[{model['dimension']}])")
    for rowid, (row, blob, vector) in enumerate(zip(shipped, quantize(matrix), matrix), start=1):
        db.execute(f"INSERT INTO {table} (rowid, embedding) VALUES (?, vec_int8(?))",
                   (rowid, sqlite3.Binary(blob)))
        # The float32 vector beside the quantized row: the page prints a
        # squared L2 distance comparable to the report's, while the vec0 table
        # still supplies the ranking.
        db.execute("UPDATE retail_products SET embedding = ? WHERE rowid = ?",
                   (sqlite3.Binary(np.asarray(vector, dtype="<f4").tobytes()), rowid))

    for row in centroids:
        db.execute(
            "INSERT INTO retail_centroids (model, centroid_count, seed, id, label, vector, x, y, z) "
            "VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)",
            (row["model"], row["centroids"], row["seed"], row["id"], row["label"],
             sqlite3.Binary(np.asarray(row["vector"], dtype="<f4").tobytes()),
             row["projection"][0], row["projection"][1], row["projection"][2]),
        )
    for cluster in report["clusters"]:
        db.execute("INSERT INTO retail_clusters (id, label, count, categories, representatives) "
                   "VALUES (?, ?, ?, ?, ?)",
                   (cluster["id"], cluster["label"], cluster["count"],
                    json.dumps(cluster["categories"], ensure_ascii=False),
                    json.dumps(cluster["representatives"], ensure_ascii=False)))

    db.executemany("INSERT INTO retail_heldout (x, y, z, cluster) VALUES (?, ?, ?, ?)",
                   [(row["x"], row["y"], row["z"], row["cluster"]) for row in held_out])
    db.execute("INSERT INTO retail_projection (mean, basis) VALUES (?, ?)",
               (sqlite3.Binary(np.asarray(basis["mean"], dtype="<f4").tobytes()),
                sqlite3.Binary(np.asarray(basis["basis"], dtype="<f4").tobytes())))

    # Measure the shipped table against an exact float32 reference over the
    # same vectors. This is the retrieval number the plate reads; it is the
    # committed `vec0` table answering, not a numpy proxy.
    sample_index = {row["fdc_id"]: i for i, row in enumerate(shipped)}
    queries = matrix[[sample_index[row["fdc_id"]] for row in samples]]
    exact = exact_topk(matrix, queries, RETRIEVAL_K)
    actual = []
    for vector in queries:
        hits = db.execute(
            f"SELECT rowid FROM {table} WHERE embedding MATCH vec_int8(?) AND k = ? ORDER BY distance",
            (sqlite3.Binary(np.clip(np.rint(vector * 127.0), -127, 127).astype(np.int8).tobytes()), RETRIEVAL_K),
        ).fetchall()
        actual.append({row[0] for row in hits})
    recall = sum(len(row & hit) for row, hit in zip(exact, actual)) / (len(exact) * RETRIEVAL_K)

    summary = retail_summary(report, catalog, centroids, table)
    summary["retrieval"] = {"k": RETRIEVAL_K, "recall_at_k": round(recall, 6)}
    demo_index.set_meta(db, "retail", summary)

    db.commit()
    db.execute("VACUUM")
    db.close()
    print(f"retail: {len(shipped)} products · {len(arrivals)} arrivals · {len(centroids)} centroids · "
          f"recall@{RETRIEVAL_K} {recall:.3f}")
    print("run `python3 website/tools/demo_index.py finalize` to name the shipped index")
    return 0


def check(args) -> int:
    lock, report, catalog, centroids, basis, held_out = load_plan()
    manifest = None
    if MANIFEST.exists():
        manifest = json.loads(MANIFEST.read_text(encoding="utf-8"))
    working = demo_index.WORK_DB
    published = demo_index.published_db()
    summary = None
    if working.exists():
        db = sqlite3.connect(working)
        summary = demo_index.read_meta(db).get("retail")
        tables = {row[0] for row in db.execute("SELECT name FROM sqlite_master WHERE type IN ('table', 'view')")}
        db.close()
        if summary is None and manifest and manifest.get("retail"):
            sys.exit("build-retail-index: the working file dropped the published retail stage; "
                     "run `just website-retail` (text + retail + finalize) or remove `meta['retail']` "
                     "from the published manifest")
        if summary is not None:
            expected = {"retail_products", "retail_arrivals", "retail_centroids", "retail_clusters",
                        "retail_heldout", "retail_projection", summary["table"]}
            absent = expected - tables
            if absent:
                sys.exit(f"build-retail-index: the working file lacks {', '.join(sorted(absent))}")
            if summary["report_sha256"] != sha256(REPORT):
                sys.exit("build-retail-index: the working retail section names a different report")
            if summary["model"] != report["model"]["name"] or summary["dimension"] != report["model"]["dimension"]:
                sys.exit("build-retail-index: the working retail section names a different model")
    if published and manifest and manifest.get("retail"):
        summary = manifest["retail"]
        db = sqlite3.connect(published)
        tables = {row[0] for row in db.execute("SELECT name FROM sqlite_master WHERE type IN ('table', 'view')")}
        counts = {
            "retail_products": db.execute("SELECT COUNT(*) FROM retail_products").fetchone()[0],
            "retail_centroids": db.execute("SELECT COUNT(*) FROM retail_centroids").fetchone()[0],
        }
        db.close()
        for name in ("retail_products", "retail_arrivals", "retail_centroids", "retail_clusters",
                     "retail_heldout", "retail_projection", summary["table"]):
            if name not in tables:
                sys.exit(f"build-retail-index: the published index lacks {name}")
        if summary["report_sha256"] != sha256(REPORT):
            sys.exit("build-retail-index: the published retail section names a different report")
        if counts["retail_centroids"] != summary["centroid_count"]:
            sys.exit("build-retail-index: the published centroid table disagrees with the manifest")
    print(f"build-retail-index: pins, report, samples, centroids and manifest agree "
          f"({report['populations']['training']} training · {report['canonical']['config']['centroids']} centroids)")
    return 0


def main() -> int:
    parser = argparse.ArgumentParser(description=__doc__, formatter_class=argparse.RawDescriptionHelpFormatter)
    parser.add_argument("--emb", default=os.environ.get("EMB_ADDR", "127.0.0.1:16389"))
    parser.add_argument("--sqlite-vec", default=None)
    parser.add_argument("--check", action="store_true", help="validate pins, report, samples and provenance; download nothing")
    args = parser.parse_args()
    return check(args) if args.check else build(args)


if __name__ == "__main__":
    raise SystemExit(main())
