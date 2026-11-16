#!/usr/bin/env python3
"""Validate the retail report, its shipped samples and its frozen centroids.

    python3 website/tools/retail/check_retail.py --check

The checks are the report's own acceptance surface: they prove every grid cell
carries both methods and final-centroid occupancy, that the samples are real
rows of the shipped catalog, that category labels never enter the clustering
input, that held-out and training populations are disjoint, and that no
synthetic row appears in an observed table. They read committed and generated
files only; nothing is downloaded and no model or database is needed.
"""

from __future__ import annotations

import argparse
import hashlib
import json
import math
import sys
from pathlib import Path

HERE = Path(__file__).resolve().parent
REPO_ROOT = HERE.parents[2]
LOCK = HERE / "sources.lock.json"
CATALOG = HERE / "catalog.jsonl"
CENTROIDS = HERE / "centroids.jsonl"
BASIS = HERE / "projection.json"
HELDOUT = HERE / "projected_heldout.jsonl"
REPORT_DIR = REPO_ROOT / "website" / "assets" / "demo" / "retail"

METHODS = ("emeans", "lloyd")
ROLES = ("shipped", "arrival")
REQUIRED_TOP = (
    "schema", "generated_by", "catalog", "model", "host", "populations", "grid",
    "comparison", "seed_spread", "canonical", "distributions", "clusters",
    "retrieval", "review_rule", "samples", "synthetic", "checks", "files",
)


def load_jsonl(path: Path) -> list[dict]:
    return [json.loads(line) for line in path.read_text(encoding="utf-8").splitlines() if line.strip()]


def check(lock: dict, report: dict, catalog: list[dict], centroids: list[dict],
          held_out: list[dict] | None = None, basis: dict | None = None) -> list[str]:
    held_out = held_out or []
    basis = basis or {}
    errors: list[str] = []

    def fail(message: str) -> None:
        errors.append(message)

    if report.get("schema") != 1:
        fail("report: schema is not 1")
    for key in REQUIRED_TOP:
        if key not in report:
            fail(f"report: missing section {key!r}")

    # ── provenance ────────────────────────────────────────────────────
    releases = report.get("catalog", {}).get("releases", [])
    roles = [release.get("role") for release in releases]
    if roles != ["training", "evaluation"]:
        fail(f"catalog: releases are {roles}, expected training then evaluation")
    locked = {release["role"]: release for release in lock["releases"]}
    for release in releases:
        pinned = locked.get(release.get("role"))
        if not pinned:
            fail(f"catalog: {release.get('role')} is not in the lock")
            continue
        for field in ("version", "url", "bytes", "sha256"):
            if release.get(field) != pinned.get(field):
                fail(f"catalog: {release['role']}.{field} differs from the lock")
    if releases and releases[0].get("version", "") >= releases[1].get("version", ""):
        fail("catalog: the training release is not earlier than the evaluation release")
    redistribution = report.get("catalog", {}).get("redistribution", {})
    if redistribution.get("decision") != "permitted" or not redistribution.get("basis"):
        fail("catalog: the redistribution decision is not recorded")
    for profile in ("earlier", "later"):
        stats = report.get("catalog", {}).get("profiles", {}).get(profile, {})
        if not stats.get("rows"):
            fail(f"catalog: {profile} profile has no row count")
    overlap = report.get("catalog", {}).get("overlap", {})
    if overlap.get("revised", 0) > overlap.get("common", 0):
        fail("catalog: revised overlap exceeds the common ids")

    model = report.get("model", {})
    for field in ("name", "dimension", "normalization"):
        if model.get(field) != lock["model"][field]:
            fail(f"model: {field} differs from the lock")
    if model.get("normalization") != "l2":
        fail("model: clustering input is not L2-normalized")

    # ── populations and grid ──────────────────────────────────────────
    populations = report.get("populations", {})
    if populations.get("clustering_input_fields") != ["description", "brand"]:
        fail("populations: clustering inputs are not description and brand only")
    if populations.get("training", 0) < max(report.get("grid", {}).get("scales", [0]) or [0]):
        fail("populations: the training population is smaller than the largest scale")
    if not populations.get("held_out", 0) or not populations.get("arrivals", 0):
        fail("populations: the held-out or arrival population is empty")
    if not populations.get("overlap", {}).get("new"):
        fail("populations: the later release introduces no new ids")

    grid = report.get("grid", {})
    if grid.get("scales") != lock["analysis"]["scales"]:
        fail("grid: scales differ from the lock")
    if grid.get("centroids") != lock["analysis"]["centroids"]:
        fail("grid: centroid counts differ from the lock")
    if grid.get("seeds") != lock["analysis"]["seeds"]:
        fail("grid: seeds differ from the lock")

    comparison = report.get("comparison", [])
    cells = {}
    for cell in comparison:
        key = (cell.get("scale"), cell.get("centroids"), cell.get("seed"), cell.get("method"))
        if cell.get("method") not in METHODS:
            fail(f"comparison: unknown method {cell.get('method')!r}")
            continue
        if key in cells:
            fail(f"comparison: duplicate cell {key}")
        cells[key] = cell
        if not isinstance(cell.get("loss"), (int, float)) or cell["loss"] < 0:
            fail(f"comparison: {key} has no final-centroid loss")
        if not isinstance(cell.get("time_s"), (int, float)) or cell["time_s"] <= 0:
            fail(f"comparison: {key} has no measured time")
        counts = cell.get("occupancy", {}).get("counts")
        if not isinstance(counts, list) or len(counts) != cell.get("centroids"):
            fail(f"comparison: {key} occupancy is not one count per centroid")
        elif sum(counts) != report.get("populations", {}).get("held_out"):
            fail(f"comparison: {key} occupancy does not cover the held-out population")
        if cell["method"] == "emeans":
            passes = cell.get("passes") or []
            if len(passes) != lock["analysis"]["emeans"]["epochs"]:
                fail(f"comparison: {key} does not carry every e-means pass")
            for step in passes:
                if not isinstance(step.get("time_s"), (int, float)) or step["time_s"] < 0:
                    fail(f"comparison: {key} has a pass without time")
            match = cell.get("time_to_match_lloyd", {})
            if match.get("matched") is True and not isinstance(match.get("matched_at_pass"), int):
                fail(f"comparison: {key} matched without a pass")
            if match.get("matched") is False and match.get("time_s") != "not_reached":
                fail(f"comparison: {key} did not match but does not say not_reached")

    for scale in grid.get("scales", []):
        for k in grid.get("centroids", []):
            for method in METHODS:
                present = [seed for seed in grid.get("seeds", [])
                           if (scale, k, seed, method) in cells]
                if present != grid.get("seeds"):
                    fail(f"comparison: scale {scale} k {k} {method} has seeds {present}")

    for row in report.get("seed_spread", []):
        if row.get("runs") != len(grid.get("seeds", [])):
            fail(f"seed_spread: {row.get('scale')}/{row.get('centroids')}/{row.get('method')} "
                 f"covers {row.get('runs')} seeds")

    canonical = report.get("canonical", {})
    config = canonical.get("config", {})
    canonical_cell = cells.get((config.get("scale"), config.get("centroids"),
                                config.get("seed"), "emeans"))
    if not canonical_cell:
        fail("canonical: the canonical configuration has no e-means cell")
    elif canonical.get("emeans_loss") != canonical_cell.get("loss"):
        fail("canonical: the e-means loss is not the canonical cell's final reassignment loss")

    # ── distributions and review rule ─────────────────────────────────
    distributions = report.get("distributions", {})
    for population in ("held_out", "arrivals"):
        section = distributions.get(population, {})
        for statistic in ("nearest", "runner_up", "margin"):
            q = section.get(statistic, {})
            ordered = [q.get(key) for key in ("min", "p5", "p25", "p50", "p75", "p95", "max")]
            if any(value is None for value in ordered) or ordered != sorted(ordered):
                fail(f"distributions: {population}.{statistic} quantiles are not ordered")
        if not distributions.get(f"{'held_out' if population == 'held_out' else 'arrival'}_histogram"):
            fail(f"distributions: {population} has no histogram")
    rule = report.get("review_rule")
    if rule is not None:
        if rule.get("statistic") not in ("nearest", "runner_up", "margin"):
            fail("review_rule: unknown statistic")
        expected = distributions.get("held_out", {}).get(rule.get("statistic"), {}).get("p95")
        if rule.get("threshold") != expected:
            fail("review_rule: the threshold was not generated from the held-out p95")
        if rule.get("verdict") not in ("review", "no_verdict"):
            fail("review_rule: unknown verdict")

    # ── clusters and retrieval ────────────────────────────────────────
    clusters = report.get("clusters", [])
    if len(clusters) != config.get("centroids"):
        fail("clusters: the summary count is not the canonical centroid count")
    if sum(cluster.get("count", 0) for cluster in clusters) != populations.get("held_out"):
        fail("clusters: the cluster sizes do not cover the held-out population")

    retrieval = report.get("retrieval", {})
    if retrieval.get("queries") != report.get("samples", {}).get("catalog"):
        fail("retrieval: the queries are not the committed catalog samples")
    if not isinstance(retrieval.get("recall_at_k"), (int, float)) or not 0 <= retrieval["recall_at_k"] <= 1:
        fail("retrieval: recall is not a fraction")
    if len(retrieval.get("exact_topk", [])) != len(retrieval.get("queries", [])):
        fail("retrieval: the exact reference does not cover every query")

    misleading = retrieval.get("misleading_neighbor")
    if misleading is not None:
        if misleading.get("sample_fdc_id") not in report.get("samples", {}).get("catalog", []):
            fail("retrieval: the misleading neighbour does not belong to a committed sample")
        if misleading.get("same_gtin"):
            fail("retrieval: the misleading neighbour shares the sample's identifier")
    elif report.get("samples", {}).get("catalog"):
        fail("retrieval: no misleading neighbour was measured for any sample")

    if report.get("synthetic") != []:
        fail("synthetic: the report carries synthetic rows in an observed table")
    checks = report.get("checks", {})
    if checks.get("training_and_held_out_disjoint") is not True:
        fail("checks: training and held-out ids are not disjoint")
    if checks.get("category_labels_in_clustering_input") is not False:
        fail("checks: a category label entered the clustering input")
    if checks.get("synthetic_rows_in_observed_tables") != 0:
        fail("checks: synthetic rows appear in observed tables")

    # ── the shipped derived files ─────────────────────────────────────
    files = report.get("files", {})
    for name, path in (("catalog", CATALOG), ("centroids", CENTROIDS),
                       ("projection", BASIS), ("held_out_projection", HELDOUT)):
        recorded = files.get(name, {})
        if not path.exists():
            fail(f"files: {name} is absent")
            continue
        actual = hashlib.sha256(path.read_bytes()).hexdigest()
        if recorded.get("sha256") != actual:
            fail(f"files: {name} sha256 differs from the report")
        rows_by_name = {"catalog": catalog, "centroids": centroids, "held_out_projection": held_out}
        if name in rows_by_name and recorded.get("rows") != len(rows_by_name[name]):
            fail(f"files: {name} row count differs from the report")

    if not catalog:
        fail("catalog: the selected product file is empty")
    roles_seen = {row.get("role") for row in catalog}
    if roles_seen - set(ROLES):
        fail(f"catalog: unknown roles {sorted(roles_seen - set(ROLES))}")
    shipped = [row for row in catalog if row.get("role") == "shipped"]
    samples = [row for row in shipped if row.get("sample")]
    arrivals = [row for row in catalog if row.get("role") == "arrival"]
    if len(shipped) != populations.get("shipped"):
        fail("catalog: the shipped row count differs from the report")
    if len(arrivals) != len(report.get("samples", {}).get("arrivals", [])):
        fail("catalog: the committed arrivals are not the fixed arrival samples")
    if {row["fdc_id"] for row in samples} != set(report.get("samples", {}).get("catalog", [])):
        fail("catalog: the fixed catalog samples are not the committed sample rows")
    if {row["fdc_id"] for row in arrivals} != set(report.get("samples", {}).get("arrivals", [])):
        fail("catalog: the fixed arrival samples are not the committed arrival rows")
    shipped_ids = {row["fdc_id"] for row in shipped}
    if {row["fdc_id"] for row in arrivals} & shipped_ids:
        fail("catalog: an arrival is also a shipped catalog row")
    gtins = [row["gtin"] for row in shipped]
    if len(set(gtins)) != len(gtins):
        fail("catalog: the shipped rows do not carry distinct identifiers")
    for row in catalog:
        if row.get("synthetic"):
            fail(f"catalog: {row['fdc_id']} is synthetic but sits in an observed role")
        expected = f"{row['description']} — {row['brand']}" if row["brand"] else row["description"]
        if row["text"] != expected:
            fail(f"catalog: {row['fdc_id']} text is not description + brand")
        if row["role"] == "shipped" and not row.get("gtin"):
            fail(f"catalog: {row['fdc_id']} ships without an identifier")
        projection = row.get("projection")
        if not isinstance(projection, list) or len(projection) != 3 or not all(
                isinstance(value, (int, float)) and math.isfinite(value) for value in projection):
            fail(f"catalog: {row['fdc_id']} has no finite 3-axis projection")

    if len(centroids) != config.get("centroids"):
        fail("centroids: the file does not hold the canonical centroid count")
    for row in centroids:
        if (row.get("model"), row.get("dimension"), row.get("normalization"), row.get("seed")) != (
                model.get("name"), model.get("dimension"), model.get("normalization"), config.get("seed")):
            fail(f"centroids: {row.get('id')} provenance differs from the report")
            break
        vector = row.get("vector") or []
        if len(vector) != model.get("dimension") or not all(math.isfinite(value) for value in vector):
            fail(f"centroids: {row.get('id')} is not a finite {model.get('dimension')}-element vector")
            break
        projection = row.get("projection") or []
        if len(projection) != 3 or not all(math.isfinite(value) for value in projection):
            fail(f"centroids: {row.get('id')} has no finite 3-axis projection")
            break

    if report.get("grid", {}).get("projection", {}).get("axes") != 3:
        fail("grid: the projection is not three axes")
    if len(held_out) != populations.get("held_out"):
        fail("projection: the held-out projection does not cover the held-out population")
    for row in held_out:
        if not all(isinstance(row.get(key), (int, float)) and math.isfinite(row[key]) for key in ("x", "y", "z")):
            fail("projection: a held-out point is not finite")
            break
    mean = basis.get("mean") or []
    axes = basis.get("basis") or []
    if len(mean) != model.get("dimension") or len(axes) != 3 or any(
            len(axis) != model.get("dimension") for axis in axes):
        fail("projection: the basis is not three axes over the model dimension")
    elif not all(math.isfinite(value) for axis in axes for value in axis):
        fail("projection: the basis is not finite")

    return errors


def main() -> int:
    parser = argparse.ArgumentParser(description=__doc__, formatter_class=argparse.RawDescriptionHelpFormatter)
    parser.add_argument("--check", action="store_true", help="validate and exit non-zero on drift")
    parser.add_argument("--report", type=Path, default=REPORT_DIR / "report.json")
    parser.add_argument("--lock", type=Path, default=LOCK)
    parser.add_argument("--catalog", type=Path, default=CATALOG)
    parser.add_argument("--centroids", type=Path, default=CENTROIDS)
    args = parser.parse_args()

    missing = [str(path) for path in (args.lock, args.report, args.catalog, args.centroids, BASIS, HELDOUT)
               if not path.exists()]
    if missing:
        sys.exit("check_retail: absent: " + ", ".join(missing) + "\n  run the analysis first")
    lock = json.loads(args.lock.read_text(encoding="utf-8"))
    report = json.loads(args.report.read_text(encoding="utf-8"))
    held_out = load_jsonl(HELDOUT) if HELDOUT.exists() else []
    basis = json.loads(BASIS.read_text(encoding="utf-8")) if BASIS.exists() else {}
    errors = check(lock, report, load_jsonl(args.catalog), load_jsonl(args.centroids), held_out, basis)
    if errors:
        print("check_retail: FAILED", file=sys.stderr)
        for error in errors:
            print(f"  {error}", file=sys.stderr)
        return 1
    print(f"check_retail: {len(report['comparison'])} grid cells, "
          f"{len(report['clusters'])} frozen regions, "
          f"{len(report['samples']['catalog'])} + {len(report['samples']['arrivals'])} samples · all checks pass")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
