# Spec Delta

## ADDED Requirements

### Requirement: Retail plates use a pinned, attributable catalog
The catalog-match and arrival-queue plates SHALL use a bounded selection from two consecutive archived releases of a documented public product catalog. The shipped provenance MUST identify the source, release dates or versions, download identity, extraction rule, redistribution check, model, preprocessing, and training/evaluation populations. Card, customer, receipt, and merchant-private data MUST NOT be included.

#### Scenario: Provenance is visible with the plates
- **WHEN** either retail plate is rendered
- **THEN** it identifies the catalog source, releases, model, and applicable redistribution terms
- **AND** it states the training and evaluation populations represented by the shipped evidence

#### Scenario: Private transaction data is absent
- **WHEN** the retail corpus and shipped samples are inspected
- **THEN** they contain no card, customer, receipt, or merchant-private records
- **AND** any synthetic arrival or load example is explicitly labelled as synthetic

### Requirement: Retail figures come from a reproducible offline analysis
Every displayed retail count, distance, cutoff, error, timing, cluster label, or chart value MUST be generated from the pinned inputs and recorded build provenance. A rebuild MUST replace those values from its own output rather than preserve a hand-entered result.

#### Scenario: A rebuild changes measured output
- **WHEN** the retail analysis is rebuilt from its pinned inputs
- **THEN** the shipped figures and labels are produced by that run
- **AND** no page retains a manually selected winner, percentage, distance, timing, or cluster name

#### Scenario: Missing evidence is not invented
- **WHEN** a required measurement or source record is unavailable
- **THEN** the build fails or the plate states that the evidence is unavailable
- **AND** it does not substitute an estimated or curated value

### Requirement: Centroid training reports a fair baseline comparison
Offline centroid training MUST compare e-means with Lloyd k-means on the same embeddings, squared-L2 objective, hardware, dataset sizes, centroid counts, and held-out release. The report MUST include seed spread, initialization or subsampling differences, passes, peak memory, end-to-end time, and whether a stated Lloyd loss was matched.

#### Scenario: Both methods are reported
- **WHEN** the analysis report is rendered
- **THEN** it shows held-out error, occupancy, and timing evidence for both centroid methods
- **AND** it identifies the host, seeds, populations, and any sampling or initialization difference

#### Scenario: A negative comparison remains visible
- **WHEN** the measured curves do not show an e-means loss or time advantage
- **THEN** the report shows that negative result
- **AND** it makes no speed or quality claim for e-means

### Requirement: Centroids explain regions without asserting product identity
Frozen centroids MUST be trained without category labels and assigned using the same model and normalization used to create their training embeddings. A centroid assignment MUST NOT by itself establish that two products are the same item or belong to a labelled category.

#### Scenario: Identity requires structured evidence
- **WHEN** the catalog-match plate presents a semantically near product
- **THEN** it shows the available identifier and pack evidence separately from vector similarity
- **AND** incompatible structured evidence prevents the plate from presenting the products as the same item

#### Scenario: A model mismatch invalidates assignment
- **WHEN** the live query model or normalization differs from the recorded centroid provenance
- **THEN** the plate states that the centroids cannot be used
- **AND** it does not render an assignment or review verdict from them

### Requirement: Catalog search and centroid assignment remain distinct
The catalog-match plate MUST retrieve nearest committed products independently of centroid assignment. Its retrieval quality MUST be checked against an exact reference, and any displayed identifier or pack contradiction MUST remain visible.

#### Scenario: A query returns computed candidates
- **WHEN** a visitor selects a committed catalog example
- **THEN** the plate embeds that example with the named model and searches the committed product index
- **AND** the displayed candidates and distances come from that search rather than a stored ranking

#### Scenario: Retrieval and assignment are evaluated separately
- **WHEN** the analysis reports catalog-match quality
- **THEN** it reports nearest-product retrieval quality separately from nearest-centroid assignment
- **AND** a strong result on one measure is not presented as evidence for the other

### Requirement: Later-release products can require review
The arrival-queue plate MUST compare a fixed product from the later catalog release with centroids and reference distributions built from the earlier release. It MUST show nearest and runner-up distances or margins against held-out earlier products and other arrivals, and it MUST abstain unless a predeclared rule supports a review verdict.

#### Scenario: An unsupported arrival stays in review
- **WHEN** a selected later-release product has no defensible review cutoff or comparison
- **THEN** the plate shows the measured distances and comparison context
- **AND** it displays a review or no-verdict state instead of an automatic classification

#### Scenario: Revised products are not treated as new identities
- **WHEN** a product identifier appears in both catalog releases
- **THEN** the analysis classifies it separately from an identifier introduced in the later release
- **AND** a changed description alone is not presented as proof that the product is new

### Requirement: Retail plates preserve the gallery's read-only boundary
Retail plates MUST offer only committed sample inputs. They MUST NOT add a visitor-authored query, shared corpus write, account, or server-side decision state. On sandbox or evidence failure, they MUST state the failure and leave their explanation and provenance readable without JavaScript.

#### Scenario: A visitor cannot add catalog data
- **WHEN** a visitor uses either retail plate
- **THEN** every selectable input comes from the committed samples
- **AND** no action adds, removes, or modifies a catalog item or shared review state

#### Scenario: Failure leaves no fabricated result
- **WHEN** the sandbox or required shipped evidence is unavailable
- **THEN** the plate states that condition and shows no live result
- **AND** its explanation, provenance, and commands remain readable without JavaScript

### Requirement: Retail evidence is accessible and attributable
Every retail chart MUST have a keyboard-readable table or equivalent textual representation containing the same underlying values. The plate MUST attribute the catalog source, centroid method, and analysis implementation, and it MUST label measured catalog evidence separately from any synthetic arrival or load illustration.

#### Scenario: A chart can be read without the graphic
- **WHEN** a retail chart is rendered
- **THEN** an adjacent table or textual equivalent exposes its displayed values
- **AND** the page remains understandable with the graphic unavailable

#### Scenario: Synthetic evidence cannot be mistaken for observation
- **WHEN** a plate includes a synthetic arrival or load example
- **THEN** that example is visibly labelled synthetic
- **AND** it is not included in a claim about observed transactions or checkout performance
