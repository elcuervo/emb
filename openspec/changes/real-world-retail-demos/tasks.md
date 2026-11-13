# Tasks

## 1. Provenance and fixtures

- [ ] 1.1 Add `website/tools/retail/sources.lock.json`, the redistribution-check record, and parser fixtures; verify the lock names both consecutive USDA releases, digests, extraction rules, population fields, and the terms decision.
- [ ] 1.2 Add the retail source parser and its focused test; verify missing fields, duplicate IDs, revised-ID overlap, new IDs, and category imbalance are counted without treating a revised description as a new product.
- [ ] 1.3 Document the opt-in download, local cache, ignored source archives, and terms gate in `website/README.md`; verify the documented command matches the Just target and no source archive path is served.

## 2. Offline analysis

- [ ] 2.1 Add the pinned analysis environment and script that embeds through local EMB and runs Faiss `NumpyEmeans` and Faiss `Kmeans`; verify one fixture run records host, model, normalization, seeds, initialization, subsampling, passes, memory, and time.
- [ ] 2.2 Generate the canonical comparison report and CSV tables from the analysis output; verify every cell uses final-centroid reassignment, reports `not_reached` when applicable, and includes seed spread across the configured scales and centroid counts.
- [ ] 2.3 Add report-schema and leakage checks; verify category labels are absent from clustering inputs, training/evaluation populations are disjoint, and synthetic examples cannot appear in observed-result tables.

## 3. Shared gallery index

- [ ] 3.1 Add `website/tools/build-retail-index.py` using `open_working(fresh=False)` and `retail_*` tables; verify it fails before the text stage exists and does not drop Poe, projection, region, or media tables.
- [ ] 3.2 Extend `demo_index.build_manifest` with the `retail` section and product model entry; verify finalize retains that section and existing manifest consumers still find Poe and media sections.
- [ ] 3.3 Add the `website-retail` target and update the full-rebuild sequence so retail is rebuilt before finalize; verify a working file that already contains retail metadata fails if the retail stage is omitted.
- [ ] 3.4 Add `website-retail --check` for pins, sample references, report schema, and model/centroid provenance; verify it passes without downloading archives and fails on a deliberate mismatch.

## 4. Gallery plates

- [ ] 4.1 Add retail manifest, product-search, and nearest/runner-up centroid helpers to `website/assets/js/demos.js`; verify model, dimension, and normalization mismatches return a visible error without an assignment.
- [ ] 4.2 Add `website/demos/catalog.html` with fixed sample choices, product search, structured identifier/pack evidence, and the misleading-neighbor case; verify the page has all five teaching sections, exact commands, no free-text input, and readable no-JavaScript content.
- [ ] 4.3 Add `website/demos/queue.html` with later-release sample choices, frozen-centroid comparison, and review/no-verdict behavior; verify an unsupported cutoff cannot produce an automatic classification.
- [ ] 4.4 Render the measured comparison and distribution charts from generated data with equivalent HTML tables; verify displayed values match the report and synthetic load or arrival illustrations are labelled synthetic.
- [ ] 4.5 Register both pages in `website/tools/pages.json`, `website/demos/index.html`, neighbor navigation, and `website/tools/published-tree.py`; verify `just website-pages-check` and `just website-published` pass.

## 5. Integration

- [ ] 5.1 Run the focused parser, report, index-stage, and page checks together; verify the retail stage survives both full and partial rebuild ordering without a second shipped database.
- [ ] 5.2 Exercise both plates in a browser against a local sandbox and an unavailable sandbox; verify live results use committed samples, failures show no fabricated result, and the recorded commands match the issued `EMB` calls.
