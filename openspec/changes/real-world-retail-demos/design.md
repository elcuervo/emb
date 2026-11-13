# Design

## Context

See `proposal.md` for the motivation and `specs/embedding-demos/spec.md` for the required behavior.

The gallery already has one staged SQLite build. `website/tools/build-demo-db.py` opens the working file with `fresh=True`, writes the Poe tables and `meta["index"]`, and later stages append their own tables and metadata. `website/tools/demo_index.py finalize` hashes the result, copies known metadata into one manifest, and removes stale database files. `just website-demos` starts the local server, runs the text stage, and finalizes immediately, so a retail stage must be an explicit step in that sequence rather than an unrelated database.

The browser-side gallery client in `website/assets/js/demos.js` embeds through `EMB`, searches manifest-declared `vec0` tables, and records the exact command trace. New pages must also be added to `website/tools/pages.json`, stamped into page metadata, and listed in `website/tools/published-tree.py`.

## Goals / Non-Goals

**Goals:**

- Add one opt-in retail stage that can run after the text stage and before finalize without dropping Poe or media tables.
- Keep all centroid training and report generation offline, with pinned inputs and generated outputs.
- Ship only the small product sample, frozen centroids, provenance, and aggregate evidence needed by two plates.

**Non-Goals:**

- No change to `website/repl/`, the sandbox allowlist, or the EMB command surface.
- No runtime Faiss/PyTorch dependency in the site or Go server.
- No attempt to make the existing Poe k-means path use e-means.

## Decisions

### 1. Use a separate, opt-in index stage

Add `website/tools/build-retail-index.py` and a `website-retail` Just target. It uses `demo_index.open_working(fresh=False)`, creates only `retail_*` tables, and writes `meta["retail"]`. It refuses to run unless the working file already contains the text stage, and it never drops `passages`, `projection`, `regions`, or media tables.

The existing `website-demos` target currently finalizes immediately. Change that orchestration so the normal text-only command remains available, while the documented full retail rebuild runs text stage, retail stage, then one finalize. Media-only rebuilds remain unchanged.

Alternative considered: extend `build-demo-db.py` directly. That would couple a large optional download and extra Python environment to every Poe rebuild and make partial rebuilds harder to reason about.

### 2. Keep source data local and generated evidence small

`website/tools/retail/sources.lock.json` pins both USDA Branded Foods archive URLs, expected sizes, SHA-256 values, release dates, and the redistribution-check record. Downloaded archives and full embedding matrices stay in ignored build directories. The committed derived inputs are the selected product JSONL, the frozen sample/centroid vectors, and the generated aggregate report—not the multi-gigabyte source archives.

The stage records source item counts, exclusions, duplicate handling, ID overlap between releases, and the training/evaluation split. Category labels are retained as metadata but are not passed to clustering.

Alternative considered: vendor both complete releases. That makes the site repository unnecessarily large and publishes far more product data than the plates need.

### 3. Compare pinned upstream implementations offline

The analysis script runs in a pinned Python environment recorded beside the report. It calls the local EMB server for embeddings, then uses Faiss `NumpyEmeans` and Faiss `Kmeans` on the same normalized float32 matrix. The comparison grid is fixed in the source lock or analysis config: dataset scales, centroid counts, seeds, and epoch budgets. Each cell records final reassignment loss, time, peak memory, occupancy, and seed identity.

The script writes a canonical JSON report and CSV tables. Pages and charts read those generated artifacts; they contain no literal measured values. If e-means does not reach the reference Lloyd loss within the declared budget, the cell records `not_reached`.

Alternative considered: port e-means to Go or NumPy. That creates a second implementation whose negative or positive result would not be evidence about upstream e-means.

### 4. Extend the shared manifest without overloading Poe metadata

`demo_index.build_manifest` copies `meta["retail"]` to a top-level `retail` object, alongside the existing `media` and `fingerprints` sections. The retail model also gets a manifest model entry only for the product `vec0` table the browser searches. Centroid vectors live in a normal SQLite table so assignment can expose nearest and runner-up results rather than only the opaque top-k result from product search.

The shipped product table contains the selected catalog rows and their vectors. The report contains population counts, comparison cells, cluster summaries, held-out distributions, and the chosen sample IDs. A plate reads both; it does not recompute the offline report in the browser.

### 5. Use two fixed-choice plates over one shared gallery client

Add `website/demos/catalog.html` and `website/demos/queue.html`, then link them as the next gallery entries and previous/next neighbors. Each page follows the existing five-section plate structure and uses one mechanism strip.

The client gains narrowly named retail helpers: load the retail manifest section, run `EMB` for a committed sample, search the product table, and compute exact nearest/runner-up centroid assignment from the shipped centroid table. Sample buttons select committed IDs; there is no free-text input. The browser checks model name, dimension, and normalization against the retail provenance before assignment.

Charts are inline SVG or HTML rendered from the report, with an HTML table containing the same values. The visual treatment uses the existing paper, dark instrument, mono captions, and one accent signal.

Alternative considered: send every visitor query to the sandbox. That conflicts with the read-only sample boundary and widens exposure through the sandbox cache.

### 6. Gate publication on provenance and generated output

Before committing derived product text, the source-lock task must record the redistribution decision and stop if terms cannot be verified. The retail build has a `--check` mode that validates pins, report schema, sample references, and model provenance without downloading. The full build remains opt-in because it needs the archives, model, and local server.

`published-tree.py` gains the two pages and any shipped sample/report assets. `stamp-pages.py` remains the generator for titles, descriptions, canonical URLs, and sitemap entries.

## Risks / Trade-offs

- [USDA terms or release layout differ from the documented pages] → Pin exact archives and fail until the redistribution check and parser fixtures pass; do not vendor unverified text.
- [Full embedding and clustering are expensive] → Keep the full run opt-in and cache matrices locally; ship only selected rows and aggregates.
- [Faiss Lloyd and e-means differ in initialization or subsampling] → Record every such difference in the report and require final reassignment before comparison.
- [A later text-stage rebuild omits the retail stage] → Detect a working file that already has `meta["retail"]` and fail with a rerun instruction instead of silently finalizing without it.
- [The existing site copy overclaims visitor-text ephemerality] → Do not edit that copy here; fixed samples prevent this change from widening the exposure, and record the discrepancy as unresolved.

## Migration Plan

1. Add the ignored-source pipeline, fixtures, and provenance check.
2. Add the opt-in index stage and manifest section while preserving current tables.
3. Generate the report and ship the bounded retail artifacts.
4. Add the two pages, client helpers, gallery links, and publication inventory.
5. Roll back by removing the retail pages, manifest section, and retail tables; existing Poe and media stages remain independently rebuildable.

## Open Questions

None. The centroid-count and scale grid may be adjusted after the first measured run only by changing the pinned analysis config and regenerating the report; it does not change the required comparison or plate behavior.
