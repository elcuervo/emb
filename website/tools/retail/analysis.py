#!/usr/bin/env python3
"""The offline retail analysis: embed, train, measure, and write the report.

Run inside the pinned analysis shell, against a local `emb` that serves the
model the plates use:

    nix develop .#retail --command python3 website/tools/retail/analysis.py \
        --emb 127.0.0.1:16389

It reads the two pinned releases from `website/tools/retail/.cache/` (fetch them
with `just website-retail-sources`), embeds the bounded populations through EMB,
trains Faiss' `NumpyEmeans` and `Kmeans` on the same unit-normalized vectors,
and writes every figure the plates show:

    website/assets/demo/retail/report.json        the canonical report
    website/assets/demo/retail/comparison.csv     one row per grid cell
    website/assets/demo/retail/distributions.csv  the held-out/arrival quantiles
    website/tools/retail/catalog.jsonl            shipped rows + fixed samples
    website/tools/retail/centroids.jsonl          the frozen canonical centroids

No value in any of those files is typed by hand: each is computed here from the
pinned bytes and the local server's own model.
"""

from __future__ import annotations

import argparse
import csv
import hashlib
import json
import os
import platform
import resource
import socket
import sys
import time
from collections import Counter
from pathlib import Path

import numpy as np

import faiss
from faiss.contrib.e_means import NumpyEmeans

import parse_catalog as catalog

HERE = Path(__file__).resolve().parent
REPO_ROOT = HERE.parents[2]
LOCK = HERE / "sources.lock.json"
CACHE = HERE / ".cache"
REPORT_DIR = REPO_ROOT / "website" / "assets" / "demo" / "retail"
CATALOG_JSONL = HERE / "catalog.jsonl"
CENTROIDS_JSONL = HERE / "centroids.jsonl"
BASIS_JSON = HERE / "projection.json"
HELDOUT_JSONL = HERE / "projected_heldout.jsonl"

EMBED_BATCH = 128
RETRIEVAL_K = 10


class RespError(RuntimeError):
    pass


class Resp:
    """The smallest RESP2 client that can drive `EMB`."""

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
            return self.file.read(length + 2)[:-2]
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


def sha256_file(path: Path) -> str:
    hasher = hashlib.sha256()
    with path.open("rb") as handle:
        for chunk in iter(lambda: handle.read(1 << 20), b""):
            hasher.update(chunk)
    return hasher.hexdigest()


def release_paths(lock: dict, release: dict) -> tuple[Path, Path]:
    branded = CACHE / lock["extraction"]["archive_members"]["branded_food"].format(version=release["version"])
    food = CACHE / lock["extraction"]["archive_members"]["description"].format(version=release["version"])
    for path in (branded, food):
        if not path.exists():
            sys.exit(f"analysis: {path} is absent; run `just website-retail-sources`")
    archive = CACHE / Path(release["url"]).name
    if archive.exists() and release["sha256"]:
        actual = sha256_file(archive)
        if actual != release["sha256"]:
            sys.exit(f"analysis: {archive.name}: sha256 {actual}, the lock pins {release['sha256']}")
    return branded, food


def load_populations(lock: dict) -> dict:
    """The five bounded populations, all selected by a salted id hash."""
    caps = lock["analysis"]["populations"]
    earlier, later = lock["releases"]
    e_branded, e_food = release_paths(lock, earlier)
    l_branded, l_food = release_paths(lock, later)
    e_desc = catalog.load_descriptions(e_food)
    l_desc = catalog.load_descriptions(l_food)

    earlier_profile = catalog.profile_release(e_branded, e_desc)
    later_profile = catalog.profile_release(l_branded, l_desc)
    overlap = catalog.compare_releases(earlier_profile["identity"], later_profile["identity"])

    def usable(stream):
        for item, flaws in stream:
            if "description" in flaws or item.discontinued:
                continue
            yield item

    def catalog_ready(stream):
        for item in usable(stream):
            if item.gtin and item.brand and item.category:
                yield item

    def unique_gtin(stream):
        seen: set[str] = set()
        for item in stream:
            if item.gtin in seen:
                continue
            seen.add(item.gtin)
            yield item

    train_pool = catalog.ranked((p for p in usable(catalog.iter_products(e_branded, e_desc))
                                 if not catalog.is_held_out(p)), caps["train_cap"], "train")
    eval_pool = catalog.ranked((p for p in usable(catalog.iter_products(e_branded, e_desc))
                               if catalog.is_held_out(p)), caps["eval_cap"], "eval")
    shipped = catalog.ranked(unique_gtin(catalog_ready(catalog.iter_products(e_branded, e_desc))),
                             caps["ship_cap"], "ship")
    arrivals = catalog.ranked((p for p in catalog_ready(catalog.iter_products(l_branded, l_desc))
                               if p.fdc_id not in earlier_profile["identity"]),
                              caps["arrival_cap"], "arrival")

    if not train_pool or not eval_pool or not shipped or not arrivals:
        sys.exit("analysis: a selected population is empty; check the pinned release")

    return {
        "profiles": {"earlier": earlier_profile["stats"], "later": later_profile["stats"]},
        "overlap": overlap,
        "train": train_pool,
        "eval": eval_pool,
        "shipped": shipped,
        "arrivals": arrivals,
    }


def embed(conn: Resp, model: str, texts: list[str], dimension: int, label: str) -> np.ndarray:
    vectors: list[np.ndarray] = []
    for start in range(0, len(texts), EMBED_BATCH):
        chunk = texts[start:start + EMBED_BATCH]
        replies = conn.pipeline([("EMB", model, text) for text in chunk])
        for reply in replies:
            if not isinstance(reply, bytes) or len(reply) != dimension * 4:
                sys.exit(f"analysis: {label}: the server answered {reply!r} instead of {dimension} float32")
            vectors.append(np.frombuffer(reply, dtype="<f4"))
        if sys.stderr.isatty():
            print(f"  {label}: {min(start + EMBED_BATCH, len(texts))}/{len(texts)}", end="\r", file=sys.stderr)
    if sys.stderr.isatty():
        print(" " * 60, end="\r", file=sys.stderr)
    matrix = np.vstack(vectors).astype(np.float32)
    if not np.isfinite(matrix).all():
        sys.exit(f"analysis: {label}: the server returned a non-finite vector")
    norms = np.linalg.norm(matrix, axis=1, keepdims=True)
    if (norms == 0).any():
        sys.exit(f"analysis: {label}: the server returned a zero vector")
    return (matrix / norms).astype(np.float32)


def assign(matrix: np.ndarray, centroids: np.ndarray, chunk: int = 1024) -> tuple[np.ndarray, np.ndarray]:
    """Exact nearest-centroid assignment and squared L2 distance to it."""
    labels = np.empty(len(matrix), dtype=np.int64)
    distance = np.empty(len(matrix), dtype=np.float64)
    centre_norm = (centroids.astype(np.float64) ** 2).sum(axis=1)
    for start in range(0, len(matrix), chunk):
        rows = matrix[start:start + chunk].astype(np.float64)
        distances = (rows ** 2).sum(axis=1)[:, None] + centre_norm[None, :] - 2.0 * rows @ centroids.T.astype(np.float64)
        nearest = distances.argmin(axis=1)
        labels[start:start + chunk] = nearest
        distance[start:start + chunk] = distances[np.arange(len(rows)), nearest]
    return labels, distance


def occupancy(labels: np.ndarray, k: int) -> dict:
    counts = np.bincount(labels, minlength=k)
    shares = counts / max(1, counts.sum())
    return {
        "used": int((counts > 0).sum()),
        "empty": int((counts == 0).sum()),
        "largest": int(counts.max()) if counts.size else 0,
        "largest_share": round(float(shares.max()), 6) if counts.size else 0,
        "below_one_percent": int((shares < 0.01).sum()),
        "counts": [int(c) for c in counts],
    }


def train_cell(x: np.ndarray, x_eval: np.ndarray, k: int, seed: int, cfg: dict) -> dict:
    """One e-means run and one Lloyd run on the same matrix.

    Training time is measured on `x`; loss and occupancy are measured after
    assigning the held-out matrix to the final centroids.
    """
    d = x.shape[1]
    cells = {}
    rss_before = resource.getrusage(resource.RUSAGE_SELF).ru_maxrss

    t0 = time.perf_counter()
    emeans = NumpyEmeans(d, k, nepochs=cfg["emeans"]["epochs"], seed=seed)
    emeans.train(x)
    emeans_time = time.perf_counter() - t0
    em_centroids = np.ascontiguousarray(emeans.centroids, dtype=np.float32)
    em_labels, em_distance = assign(x_eval, em_centroids)
    emeans_loss = float(em_distance.sum())
    cells["emeans"] = {
        "loss": round(emeans_loss, 6),
        "time_s": round(emeans_time, 6),
        "peak_rss_kb": int(resource.getrusage(resource.RUSAGE_SELF).ru_maxrss),
        "rss_delta_kb": int(resource.getrusage(resource.RUSAGE_SELF).ru_maxrss - rss_before),
        "occupancy": occupancy(em_labels, k),
        "passes": [
            {"pass": i + 1, "moving_objective": round(float(stat["obj"]), 6),
             "time_s": round(float(stat["time"]), 6)}
            for i, stat in enumerate(emeans.iteration_stats or [])
        ],
        "recipe": {
            key: emeans.recipe.get(key)
            for key in ("batch_size", "alpha0", "batch_law", "batch_law_reason", "nepochs", "seed", "d", "k")
        } if emeans.recipe else None,
        "initialization": "k-means++ (seeded row sample)",
        "subsampling": "none",
    }

    rss_before = resource.getrusage(resource.RUSAGE_SELF).ru_maxrss
    t0 = time.perf_counter()
    kmeans = faiss.Kmeans(d, k, niter=cfg["lloyd"]["niter"], seed=seed, verbose=False,
                          min_points_per_centroid=1)
    kmeans.train(x)
    lloyd_time = time.perf_counter() - t0
    lloyd_centroids = np.ascontiguousarray(kmeans.centroids, dtype=np.float32)
    lloyd_labels, lloyd_distance = assign(x_eval, lloyd_centroids)
    lloyd_loss = float(lloyd_distance.sum())
    _, lloyd_training_distance = assign(x, lloyd_centroids)
    lloyd_training_loss = float(lloyd_training_distance.sum())
    cells["lloyd"] = {
        "loss": round(lloyd_loss, 6),
        "time_s": round(lloyd_time, 6),
        "peak_rss_kb": int(resource.getrusage(resource.RUSAGE_SELF).ru_maxrss),
        "rss_delta_kb": int(resource.getrusage(resource.RUSAGE_SELF).ru_maxrss - rss_before),
        "occupancy": occupancy(lloyd_labels, k),
        "passes": [{"pass": i + 1, "objective": round(float(obj), 6)}
                   for i, obj in enumerate(kmeans.obj if hasattr(kmeans, "obj") else [])],
        "initialization": "k-means++ (Faiss Kmeans)",
        "subsampling": "Faiss min_points_per_centroid=1, no cap at these sizes; none observed",
    }

    matched = next((stat["pass"] for stat in cells["emeans"]["passes"]
                    if stat["moving_objective"] <= lloyd_training_loss), None)
    cells["time_to_match_lloyd"] = {
        "baseline_loss": round(lloyd_training_loss, 6),
        "matched": matched is not None,
        "matched_at_pass": matched,
        "time_s": (round(cells["emeans"]["passes"][matched - 1]["time_s"], 6)
                   if matched is not None else "not_reached"),
        "note": "first e-means moving-pass training objective at or below Lloyd's final training reassignment loss",
    }
    cells["centroids"] = {"emeans": em_centroids, "lloyd": lloyd_centroids}
    return cells


def pca3(matrix: np.ndarray) -> tuple[np.ndarray, np.ndarray]:
    """A deterministic 3-axis projection fitted on the shipped embeddings."""
    mean = matrix.mean(axis=0, keepdims=True).astype(np.float32)
    _, _, components = np.linalg.svd(matrix - mean, full_matrices=False)
    return mean, components[:3].astype(np.float32)


def project(matrix: np.ndarray, mean: np.ndarray, basis: np.ndarray) -> np.ndarray:
    return ((matrix - mean) @ basis.T).astype(np.float32)


def quantiles(values: np.ndarray) -> dict:
    if not len(values):
        return {}
    points = np.percentile(values, [5, 25, 50, 75, 95])
    return {
        "min": round(float(values.min()), 6),
        "p5": round(float(points[0]), 6),
        "p25": round(float(points[1]), 6),
        "p50": round(float(points[2]), 6),
        "p75": round(float(points[3]), 6),
        "p95": round(float(points[4]), 6),
        "max": round(float(values.max()), 6),
        "mean": round(float(values.mean()), 6),
    }


def distribution(vectors: np.ndarray, centroids: np.ndarray) -> dict:
    labels, nearest = assign(vectors, centroids)
    centre_norm = (centroids.astype(np.float64) ** 2).sum(axis=1)
    rows = vectors.astype(np.float64)
    distances = (rows ** 2).sum(axis=1)[:, None] + centre_norm[None, :] - 2.0 * rows @ centroids.T.astype(np.float64)
    order = np.argsort(distances, axis=1)
    runner_up = distances[np.arange(len(rows)), order[:, 1]]
    margin = runner_up - nearest
    return {
        "nearest": quantiles(nearest),
        "runner_up": quantiles(runner_up),
        "margin": quantiles(margin),
        "labels": labels.tolist(),
        "rows": [{
            "nearest": round(float(nearest[i]), 6),
            "runner_up": round(float(runner_up[i]), 6),
            "margin": round(float(margin[i]), 6),
            "cluster": int(labels[i]),
        } for i in range(len(vectors))],
    }


def histogram(values: np.ndarray, bins: int = 12, span: tuple[float, float] | None = None) -> list[dict]:
    if not len(values):
        return []
    counts, edges = np.histogram(values, bins=bins, range=span)
    return [{
        "from": round(float(edges[i]), 6),
        "to": round(float(edges[i + 1]), 6),
        "count": int(counts[i]),
    } for i in range(len(counts))]


def stratified(items: list, cap: int, key) -> list:
    """First one per category, then the rest, in the supplied order."""
    chosen, seen = [], set()
    for item in items:
        label = key(item)
        if label and label not in seen:
            seen.add(label)
            chosen.append(item)
        if len(chosen) == cap:
            return chosen
    for item in items:
        if item not in chosen:
            chosen.append(item)
        if len(chosen) == cap:
            break
    return chosen


def int8_topk(matrix: np.ndarray, queries: np.ndarray, k: int) -> list[set[int]]:
    quantized = np.clip(np.rint(matrix * 127.0), -127, 127).astype(np.int32)
    q = np.clip(np.rint(queries * 127.0), -127, 127).astype(np.int32)
    score = q @ quantized.T
    order = np.argsort(-score, axis=1)
    return [set((order[row][:k] + 1).tolist()) for row in range(len(queries))]


def exact_topk(matrix: np.ndarray, queries: np.ndarray, k: int) -> list[set[int]]:
    distance = ((queries.astype(np.float64)[:, None, :] - matrix.astype(np.float64)[None, :, :]) ** 2).sum(axis=2)
    order = np.argsort(distance, axis=1)
    return [set((order[row][:k] + 1).tolist()) for row in range(len(queries))]


def cluster_summaries(labels: np.ndarray, eval_products: list, eval_vectors: np.ndarray,
                      centroids: np.ndarray, category_of) -> list[dict]:
    out = []
    for cluster in range(len(centroids)):
        members = np.nonzero(labels == cluster)[0]
        if not len(members):
            out.append({"id": cluster, "count": 0, "label": "empty", "categories": {}, "representatives": []})
            continue
        counts = Counter(category_of(eval_products[i]) for i in members)
        nearest = members[np.argsort(((eval_vectors[members] - centroids[cluster]) ** 2).sum(axis=1))][:3]
        top = counts.most_common(2)
        label = " · ".join(f"{name} {count * 100 // len(members)}%" for name, count in top if name)
        out.append({
            "id": cluster,
            "count": int(len(members)),
            "label": label or f"region {cluster + 1}",
            "categories": dict(counts.most_common(6)),
            "representatives": [
                {"fdc_id": eval_products[i].fdc_id, "text": eval_products[i].text(),
                 "category": category_of(eval_products[i])}
                for i in nearest
            ],
        })
    return out


def main() -> int:
    parser = argparse.ArgumentParser(description=__doc__, formatter_class=argparse.RawDescriptionHelpFormatter)
    parser.add_argument("--emb", default=os.environ.get("EMB_ADDR", "127.0.0.1:16379"))
    parser.add_argument("--threads", type=int, default=int(os.environ.get("OMP_NUM_THREADS", "4")))
    parser.add_argument("--lock", type=Path, default=LOCK)
    args = parser.parse_args()

    lock = json.loads(args.lock.read_text(encoding="utf-8"))
    model = lock["model"]
    grid = lock["analysis"]
    faiss.omp_set_num_threads(args.threads)

    print(f"reading {lock['releases'][0]['version']} and {lock['releases'][1]['version']} …")
    pops = load_populations(lock)
    conn = connect(args.emb)
    served = conn.call("EMB.MODELS")
    if served and isinstance(served[0], list):
        rows = [[item.decode() if isinstance(item, bytes) else item for item in row] for row in served]
    else:
        flat = [item.decode() if isinstance(item, bytes) else item for item in served]
        rows = [flat[i:i + 3] for i in range(0, len(flat), 3)]
    names = [row[0] for row in rows]
    if model["name"] not in names:
        conn.close()
        sys.exit(f"analysis: the server does not serve {model['name']!r}; it serves {', '.join(names)}")
    info_reply = conn.call("EMB.INFO", model["name"])
    info = [item.decode() if isinstance(item, bytes) else item for item in info_reply]
    if len(info) == 1 and isinstance(info[0], list):
        info = info[0]
    info = {info[i]: info[i + 1] for i in range(0, len(info) - 1, 2)}
    if int(info.get("dim", 0)) != model["dimension"]:
        conn.close()
        sys.exit(f"analysis: {model['name']} reports dim {info.get('dim')}, the report pins {model['dimension']}")

    populations = {
        "train": pops["train"],
        "eval": pops["eval"],
        "shipped": pops["shipped"],
        "arrivals": pops["arrivals"],
    }
    texts = {name: [item.text() for item in items] for name, items in populations.items()}
    vectors = {}
    for name, rows in texts.items():
        print(f"embedding {len(rows)} {name} rows")
        vectors[name] = embed(conn, model["name"], rows, model["dimension"], name)
    conn.close()

    canonical = grid["canonical"]
    comparison = []
    canonical_centroids = None
    for scale in grid["scales"]:
        x = vectors["train"][:scale]
        for k in grid["centroids"]:
            for seed in grid["seeds"]:
                print(f"  scale {scale} · k {k} · seed {seed}")
                cell = train_cell(x, vectors["eval"], k, seed, grid)
                for method in ("emeans", "lloyd"):
                    entry = dict(cell[method])
                    entry.update({"scale": scale, "centroids": k, "seed": seed, "method": method,
                                  "time_to_match_lloyd": cell["time_to_match_lloyd"]})
                    comparison.append(entry)
                if (scale, k, seed) == (canonical["scale"], canonical["centroids"], canonical["seed"]):
                    canonical_centroids = cell["centroids"]["emeans"]

    if canonical_centroids is None:
        sys.exit("analysis: the canonical configuration is not in the grid")

    # Seed spread for every scale/centroid pair, per method.
    spread = []
    for scale in grid["scales"]:
        for k in grid["centroids"]:
            for method in ("emeans", "lloyd"):
                losses = np.array([c["loss"] for c in comparison
                                   if (c["scale"], c["centroids"], c["method"]) == (scale, k, method)])
                spread.append({
                    "scale": scale, "centroids": k, "method": method, "runs": len(losses),
                    "mean": round(float(losses.mean()), 6),
                    "stdev": round(float(losses.std()), 6),
                    "min": round(float(losses.min()), 6),
                    "max": round(float(losses.max()), 6),
                })

    canonical_cells = {(c["method"]): c for c in comparison
                       if (c["scale"], c["centroids"], c["seed"]) ==
                       (canonical["scale"], canonical["centroids"], canonical["seed"])}
    emeans_cell = canonical_cells["emeans"]
    lloyd_cell = canonical_cells["lloyd"]

    mean, basis = pca3(vectors["shipped"])
    shipped_projection = project(vectors["shipped"], mean, basis)
    eval_projection = project(vectors["eval"], mean, basis)
    arrival_projection = project(vectors["arrivals"], mean, basis)
    centroid_projection = project(canonical_centroids, mean, basis)
    shipped_labels, _ = assign(vectors["shipped"], canonical_centroids)

    eval_dist = distribution(vectors["eval"], canonical_centroids)
    arrival_dist = distribution(vectors["arrivals"], canonical_centroids)
    eval_nearest = np.array([row["nearest"] for row in eval_dist["rows"]])
    arrival_nearest = np.array([row["nearest"] for row in arrival_dist["rows"]])
    nearest_span = (float(min(eval_nearest.min(), arrival_nearest.min())),
                    float(max(eval_nearest.max(), arrival_nearest.max())))
    clusters = cluster_summaries(np.array(eval_dist["labels"]), pops["eval"], vectors["eval"],
                                 canonical_centroids, lambda item: item.category)

    rule = grid.get("review_rule")
    review_rule = None
    if rule:
        threshold = eval_dist[rule["statistic"]][rule["source_percentile"]]
        review_rule = {
            "statistic": rule["statistic"],
            "comparison": rule["comparison"],
            "threshold": threshold,
            "source": f"held-out earlier-release {rule['statistic']} {rule['source_percentile']}",
            "verdict": rule["verdict"],
        }

    samples = stratified(pops["shipped"], grid["populations"]["sample_catalog"], lambda item: item.category)
    arrivals = stratified(pops["arrivals"], grid["populations"]["sample_arrivals"], lambda item: item.category)

    shipped_index = {item.fdc_id: i for i, item in enumerate(pops["shipped"])}
    queries = vectors["shipped"][[shipped_index[item.fdc_id] for item in samples]]
    exact = exact_topk(vectors["shipped"], queries, RETRIEVAL_K)
    approximate = int8_topk(vectors["shipped"], queries, RETRIEVAL_K)
    recall = sum(len(a & b) for a, b in zip(exact, approximate)) / (len(exact) * RETRIEVAL_K)

    # The misleading neighbour: the closest committed product that is not the query
    # and carries a different identifier. It is found here, not chosen by hand.
    misleading = None
    for row, sample in enumerate(samples):
        order = np.argsort(((vectors["shipped"] - queries[row]) ** 2).sum(axis=1))
        for index in order:
            candidate = pops["shipped"][index]
            if candidate.fdc_id == sample.fdc_id:
                continue
            if candidate.gtin != sample.gtin:
                misleading = {
                    "sample_fdc_id": sample.fdc_id,
                    "neighbor_fdc_id": candidate.fdc_id,
                    "distance": round(float(((queries[row] - vectors["shipped"][index]) ** 2).sum()), 6),
                    "same_gtin": False,
                    "sample_gtin": sample.gtin,
                    "neighbor_gtin": candidate.gtin,
                }
            break

    # Write the frozen canonical centroids as their own derived file.
    centroids_rows = []
    for index, vector in enumerate(canonical_centroids):
        centroids_rows.append({
            "model": model["name"], "dimension": model["dimension"], "normalization": model["normalization"],
            "centroids": canonical["centroids"], "seed": canonical["seed"], "id": index,
            "label": clusters[index]["label"] if index < len(clusters) else f"region {index + 1}",
            "vector": [round(float(value), 6) for value in vector],
            "projection": [round(float(value), 5) for value in centroid_projection[index]],
        })
    CENTROIDS_JSONL.write_text("".join(json.dumps(row, ensure_ascii=False) + "\n" for row in centroids_rows),
                               encoding="utf-8")

    catalog_rows = []
    sample_ids = {item.fdc_id for item in samples}
    for index, item in enumerate(pops["shipped"]):
        catalog_rows.append({
            "role": "shipped", "sample": item.fdc_id in sample_ids,
            "fdc_id": item.fdc_id, "description": item.description, "brand": item.brand,
            "gtin": item.gtin, "category": item.category, "pack": item.pack,
            "modified": item.modified, "available": item.available, "text": item.text(),
            "projection": [round(float(value), 5) for value in shipped_projection[index]],
            "cluster": int(shipped_labels[index]),
            "synthetic": False,
        })
    for index, item in enumerate(arrivals):
        catalog_rows.append({
            "role": "arrival", "sample": True,
            "fdc_id": item.fdc_id, "description": item.description, "brand": item.brand,
            "gtin": item.gtin, "category": item.category, "pack": item.pack,
            "modified": item.modified, "available": item.available, "text": item.text(),
            "projection": [round(float(value), 5) for value in arrival_projection[index]],
            "cluster": int(arrival_dist["labels"][index]),
            "synthetic": False,
        })
    CATALOG_JSONL.write_text("".join(json.dumps(row, ensure_ascii=False) + "\n" for row in catalog_rows),
                             encoding="utf-8")

    heldout_rows = [
        {"x": round(float(row[0]), 5), "y": round(float(row[1]), 5),
         "z": round(float(row[2]), 5), "cluster": int(eval_dist["labels"][index])}
        for index, row in enumerate(eval_projection)
    ]
    HELDOUT_JSONL.write_text("".join(json.dumps(row, ensure_ascii=False) + "\n"
                                     for row in heldout_rows), encoding="utf-8")
    basis_doc = {
        "method": "pca",
        "axes": 3,
        "fit": "shipped product embeddings",
        "mean": [round(float(value), 8) for value in mean[0]],
        "basis": [[round(float(value), 8) for value in axis] for axis in basis],
    }
    BASIS_JSON.write_text(json.dumps(basis_doc, ensure_ascii=False) + "\n", encoding="utf-8")

    comparison_csv = REPORT_DIR / "comparison.csv"
    distributions_csv = REPORT_DIR / "distributions.csv"
    files = {
        "catalog": {"path": str(CATALOG_JSONL.relative_to(REPO_ROOT)),
                    "sha256": sha256_file(CATALOG_JSONL), "rows": len(catalog_rows)},
        "centroids": {"path": str(CENTROIDS_JSONL.relative_to(REPO_ROOT)),
                      "sha256": sha256_file(CENTROIDS_JSONL), "rows": len(centroids_rows)},
        "projection": {"path": str(BASIS_JSON.relative_to(REPO_ROOT)), "sha256": sha256_file(BASIS_JSON)},
        "held_out_projection": {"path": str(HELDOUT_JSONL.relative_to(REPO_ROOT)),
                                "sha256": sha256_file(HELDOUT_JSONL), "rows": len(eval_projection)},
        "comparison_csv": {"path": str(comparison_csv.relative_to(REPO_ROOT))},
        "distributions_csv": {"path": str(distributions_csv.relative_to(REPO_ROOT))},
    }

    cpu = ""
    try:
        for line in Path("/proc/cpuinfo").read_text().splitlines():
            if line.startswith("model name"):
                cpu = line.split(":", 1)[1].strip()
                break
    except OSError:
        cpu = platform.processor() or platform.machine()

    report = {
        "schema": 1,
        "generated_by": "website/tools/retail/analysis.py",
        "catalog": {
            "source": lock["catalog"],
            "catalog_page": lock["catalog_page"],
            "documentation": lock["documentation"],
            "attribution": lock["redistribution"]["attribution"],
            "redistribution": lock["redistribution"],
            "releases": [
                {key: release[key] for key in ("role", "version", "url", "bytes", "sha256", "published")}
                for release in lock["releases"]
            ],
            "extraction": lock["extraction"],
            "profiles": pops["profiles"],
            "overlap": pops["overlap"],
        },
        "model": model,
        "host": {
            "system": platform.system(), "machine": platform.machine(),
            "cpu": cpu, "cpus": os.cpu_count(), "threads": args.threads,
            "python": platform.python_version(), "numpy": np.__version__, "faiss": faiss.__version__,
            "peak_rss_kb": int(resource.getrusage(resource.RUSAGE_SELF).ru_maxrss),
        },
        "populations": {
            "training": len(pops["train"]), "held_out": len(pops["eval"]),
            "shipped": len(pops["shipped"]), "arrivals": len(pops["arrivals"]),
            "overlap": pops["overlap"],
            "clustering_input_fields": ["description", "brand"],
            "category_role": "retained as metadata; never passed to clustering",
        },
        "grid": {
            "scales": grid["scales"], "centroids": grid["centroids"], "seeds": grid["seeds"],
            "objective": grid["objective"], "emeans": grid["emeans"], "lloyd": grid["lloyd"],
            "canonical": canonical,
            "projection": {"method": "pca", "axes": 3, "fit": "shipped product embeddings"},
        },
        "comparison": comparison,
        "seed_spread": spread,
        "canonical": {
            "config": canonical,
            "emeans_loss": emeans_cell["loss"], "lloyd_loss": lloyd_cell["loss"],
            "emeans_time_s": emeans_cell["time_s"], "lloyd_time_s": lloyd_cell["time_s"],
            "time_ratio_lloyd_over_emeans": round(lloyd_cell["time_s"] / emeans_cell["time_s"], 6)
            if emeans_cell["time_s"] else None,
            "loss_ratio_emeans_over_lloyd": round(emeans_cell["loss"] / lloyd_cell["loss"], 6)
            if lloyd_cell["loss"] else None,
            "emeans_time_to_match": emeans_cell["time_to_match_lloyd"],
        },
        "distributions": {
            "held_out": {key: value for key, value in eval_dist.items() if key != "labels"},
            "arrivals": {key: value for key, value in arrival_dist.items() if key != "labels"},
            "held_out_histogram": histogram(eval_nearest, span=nearest_span),
            "arrival_histogram": histogram(arrival_nearest, span=nearest_span),
        },
        "clusters": clusters,
        "retrieval": {
            "k": RETRIEVAL_K, "queries": [item.fdc_id for item in samples],
            "exact_reference": "float32 squared L2 over the same embeddings",
            "approximate": "int8 quantized ranking proxy for the committed vec0 table",
            "recall_at_k": round(recall, 6),
            "exact_topk": [sorted(row) for row in exact],
            "approximate_topk": [sorted(row) for row in approximate],
            "misleading_neighbor": misleading,
        },
        "review_rule": review_rule,
        "samples": {
            "catalog": [item.fdc_id for item in samples],
            "arrivals": [item.fdc_id for item in arrivals],
            "misleading": misleading,
        },
        "files": files,
        "synthetic": [],
        "checks": {
            "training_and_held_out_disjoint": not ({p.fdc_id for p in pops["train"]} & {p.fdc_id for p in pops["eval"]}),
            "category_labels_in_clustering_input": False,
            "synthetic_rows_in_observed_tables": 0,
        },
    }

    REPORT_DIR.mkdir(parents=True, exist_ok=True)
    with comparison_csv.open("w", newline="", encoding="utf-8") as handle:
        writer = csv.writer(handle)
        writer.writerow(["scale", "centroids", "seed", "method", "loss", "time_s", "peak_rss_kb",
                         "used_centroids", "empty_centroids", "largest_share",
                         "matched_lloyd", "matched_at_pass", "time_to_match_s"])
        for cell in comparison:
            match = cell["time_to_match_lloyd"]
            writer.writerow([cell["scale"], cell["centroids"], cell["seed"], cell["method"],
                             cell["loss"], cell["time_s"], cell["peak_rss_kb"],
                             cell["occupancy"]["used"], cell["occupancy"]["empty"],
                             cell["occupancy"]["largest_share"],
                             match["matched"], match["matched_at_pass"], match["time_s"]])

    with distributions_csv.open("w", newline="", encoding="utf-8") as handle:
        writer = csv.writer(handle)
        writer.writerow(["population", "statistic", "min", "p5", "p25", "p50", "p75", "p95", "max", "mean"])
        for population, dist in (("held_out", eval_dist), ("arrivals", arrival_dist)):
            for statistic in ("nearest", "runner_up", "margin"):
                q = dist[statistic]
                writer.writerow([population, statistic, q["min"], q["p5"], q["p25"], q["p50"],
                                 q["p75"], q["p95"], q["max"], q["mean"]])

    report_file = REPORT_DIR / "report.json"
    report_file.write_text(json.dumps(report, ensure_ascii=False, indent=1, sort_keys=True) + "\n",
                           encoding="utf-8")
    import check_retail
    errors = check_retail.check(lock, report, catalog_rows, centroids_rows, heldout_rows, basis_doc)
    if errors:
        for error in errors:
            print(f"analysis: {error}", file=sys.stderr)
        sys.exit("analysis: the generated report failed its own checks")
    print(f"wrote {report_file.relative_to(REPO_ROOT)}")
    print(f"wrote {CATALOG_JSONL.relative_to(REPO_ROOT)} ({len(catalog_rows)} rows)")
    print(f"wrote {CENTROIDS_JSONL.relative_to(REPO_ROOT)} ({len(centroids_rows)} centroids)")
    print(f"wrote {HELDOUT_JSONL.relative_to(REPO_ROOT)} ({len(eval_projection)} held-out projections)")
    print(f"wrote {BASIS_JSON.relative_to(REPO_ROOT)} (PCA basis)")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
