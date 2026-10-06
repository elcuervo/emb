# Spec Delta

## ADDED Requirements

### Requirement: A query name may be a conjunction of terms

A query SHALL accept `*` beside `+` and `-`. Terms separated by `*` SHALL be
read as a conjunction: `a*b*c` asks for the entries closest to all of its terms
at once, and has no precedence to apply. Inside a term, the name's labels SHALL
still join with single spaces so the term is embedded as a sentence, and a
glyph or a vocabulary word SHALL still resolve to its entry's description.

#### Scenario: A conjunction of two terms

- **WHEN** the name is `🍕*🦅`
- **THEN** the query is a conjunction of the pizza entry and the eagle entry

#### Scenario: A conjunction of more than two terms

- **WHEN** the name is `🍕*🦅*🍔`
- **THEN** every listed entry is a term of the conjunction and the answer is ranked against all of them

#### Scenario: A word term in a conjunction still resolves

- **WHEN** the name is `pizza*eagle`
- **THEN** it asks the same conjunction as `🍕*🦅`

### Requirement: Operators are not mixed, and a query needs a term

An operator's terms SHALL all be of the same kind: a name that mixes `*` with
`+` or `-` MUST be refused as unparseable rather than read under a precedence
rule. A name made only of operators MUST also be refused, so a lone `*` label
names no term and is never interpreted as a literal asterisk.

#### Scenario: A mixed name is refused

- **WHEN** the name is `🍕*🦅+🍔`
- **THEN** the query is refused as unparseable and no emoji is returned

#### Scenario: A lone operator is refused

- **WHEN** the name is `*`
- **THEN** the query is refused for having no term

### Requirement: A conjunction answers with an entry that is not one of its terms

A conjunction SHALL rank every vocabulary entry by the product of its similarity
to each of the query's terms, descending, and SHALL return the best-ranked entry
that is not itself one of those terms. Its answer MUST be allowed to be an entry
that no term names, so a conjunction answers what its terms have in common
instead of averaging them. Equally ranked entries SHALL keep vocabulary order.

#### Scenario: The answer is a third entry

- **WHEN** a conjunction's best joint entry is not one of its terms
- **THEN** that entry is the first result

#### Scenario: A term is not its own answer

- **WHEN** a conjunction's terms are the best joint entries for themselves
- **THEN** they are still excluded and the best remaining entry is returned

#### Scenario: Equal joint scores keep a stable order

- **WHEN** two entries have the same joint similarity to a conjunction's terms
- **THEN** repeated identical queries return them in the same order

### Requirement: A conjunction reports how much its terms actually agree

Every result of a conjunction SHALL carry the joint similarity it was ranked by
and, for each of the query's terms, that result's similarity to that term, so a
surface can show what the answer is made of. A conjunction MUST NOT be presented
with the confidence of a single-term query, and it MUST still be answered when
no entry is close to all of its terms.

#### Scenario: The score travels with the result

- **WHEN** a conjunction is answered
- **THEN** each result carries the joint similarity it was ranked by

#### Scenario: The working travels with the result

- **WHEN** a conjunction of two terms is answered
- **THEN** each result carries its similarity to each of those terms

#### Scenario: A weak conjunction is still answered

- **WHEN** no vocabulary entry is close to all of a conjunction's terms
- **THEN** the best available entry is returned with its low score rather than a refusal

### Requirement: The vocabulary includes the source's flag entries

The vocabulary SHALL include the flag entries the annotation source publishes
beside its emoji annotations, so a query can answer with a flag and a country
answer is reachable at all. The shipped vocabulary asset SHALL record every
source it was built from, so a rebuild is reproducible from those sources, and
the number of entries the service reports MUST reflect the entries actually
served.

#### Scenario: A flag is a vocabulary entry

- **WHEN** a flag is queried by its glyph
- **THEN** it resolves to its own entry and is answered from the vocabulary

#### Scenario: Both sources are recorded

- **WHEN** the shipped vocabulary asset is inspected
- **THEN** it names every source it was built from, with a digest for each

#### Scenario: The reported count follows the asset

- **WHEN** the vocabulary is rebuilt with flag entries added and the service restarted
- **THEN** the count the service reports includes them
