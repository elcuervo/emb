# emoji-operations Specification

## Purpose
Turns a query name into a ranked set of emoji by embedding the name as a
sentence and ranking a fixed vocabulary of emoji descriptions against it, so a
caller can ask in words, in emoji, or in a composition of both.

## Requirements

### Requirement: A query name is a sum of terms

A query SHALL be read as a sequence of terms separated by `+` and `-`, evaluated
left to right with no operator precedence. The first term SHALL be added to the
result, each `+` SHALL add the term that follows it, and each `-` SHALL subtract
it. Inside a term, the name's labels SHALL be joined with single spaces so the
term is embedded as a sentence. A name with no terms MUST be refused rather than
answered with a nearest neighbor.

#### Scenario: A sentence term is embedded as a sentence

- **WHEN** the name is `my.mom.is.in.the.hospital`
- **THEN** the term is embedded as the sentence `my mom is in the hospital`

#### Scenario: Terms compose left to right

- **WHEN** the name is `shark-fish+bird`
- **THEN** the result vector is `shark` minus `fish` plus `bird`, in that order

#### Scenario: An empty name is refused

- **WHEN** the name has no term, or a term with no content
- **THEN** the query is refused as unparseable and no emoji is returned

### Requirement: A query of one multi-word term is a sentence

A query SHALL be a sentence when it is exactly one term whose text carries more
than one word — no subtraction, no second term. A single word, a glyph, or any
composition MUST NOT be a sentence, and the distinction SHALL be made on the
term as the query wrote it, before any vocabulary word is spelled out, so that
naming an entry (`shark`) is one word however long its description is. The
surface decides how to phrase an answer; this only says which queries are
sentences.

#### Scenario: A sentence is a sentence

- **WHEN** the name is `my.mom.is.in.the.hospital`
- **THEN** the query is a sentence

#### Scenario: A glyph is not a sentence

- **WHEN** the name is `🦈`
- **THEN** the query is not a sentence

#### Scenario: A composition is not a sentence

- **WHEN** the name is `🦈-🐟+🐦`
- **THEN** the query is not a sentence

#### Scenario: Naming an entry is not a sentence

- **WHEN** the name is `shark`, whose description is several words long
- **THEN** the query is not a sentence, because the query wrote one word

### Requirement: Names are compared case-insensitively

Labels SHALL be lowercased before parsing, so a query answers identically
whatever case its labels arrive in. This holds for ASCII alias terms and for the
glyph terms alongside them.

#### Scenario: Case does not change the answer

- **WHEN** the same name is queried with different ASCII letter casing
- **THEN** both queries return the same ranked result

### Requirement: Emoji glyphs resolve to their vocabulary description

A term that is exactly one emoji glyph present in the vocabulary SHALL be
embedded as that entry's description, not as the glyph itself, so a glyph term
and the entry's ASCII slug are the same query. A glyph absent from the
vocabulary SHALL be treated as ordinary text within its term rather than
refused.

#### Scenario: Glyph and slug are the same query

- **WHEN** `🦈` and `shark` are queried separately
- **THEN** both produce the same ranked result

#### Scenario: An unknown glyph does not fail the query

- **WHEN** a term contains an emoji that is not in the vocabulary
- **THEN** the query is answered from the term's remaining text instead of being refused

### Requirement: The vocabulary bounds answers, never questions

The vocabulary SHALL be a fixed set of entries, each carrying a glyph, a unique
ASCII slug, and a description, and every returned emoji MUST come from it.
Queries SHALL be free text: any term that is not a known glyph is embedded
as written. Entries whose glyphs differ only by skin-tone modifier or gender
presentation MUST be deduplicated so one canonical glyph represents them, and no
two entries may share a slug.

#### Scenario: Only vocabulary entries are returned

- **WHEN** any query is answered
- **THEN** every returned glyph is the canonical glyph of a vocabulary entry

#### Scenario: Free text is accepted

- **WHEN** a term is a word or phrase that is not in the vocabulary
- **THEN** the term is embedded as written and the query is still answered

#### Scenario: Variants do not crowd the result

- **WHEN** a query is close to an emoji that exists in several skin-tone or gender variants
- **THEN** the ranked result names that emoji once, under one canonical glyph

### Requirement: The result is a ranked list ordered by similarity

A query SHALL produce up to `top_k` results ordered by cosine similarity to the
query vector, descending, each computed as a glyph, a vocabulary name, and a
similarity score. Ordering MUST be deterministic: results that score equally
SHALL be ordered identically on every call and after every restart. How much of
each result travels is the surface's decision; the ordering is not.

#### Scenario: A result carries its parts

- **WHEN** a query is answered with at least one result
- **THEN** the result carries a glyph, a vocabulary name, and a similarity score

#### Scenario: Equal scores keep a stable order

- **WHEN** two entries score equally for a query
- **THEN** repeated identical queries return them in the same order

#### Scenario: The list is truncated to top_k

- **WHEN** a query is answered with fewer candidates than `top_k` are requested
- **THEN** every available result is returned and no placeholder or error is produced

### Requirement: A glyph query answers with the entry it names

A query whose only term is a glyph present in the vocabulary SHALL return that
entry first, because the glyph and the entry share one vector.

#### Scenario: An emoji maps to itself first

- **WHEN** a vocabulary glyph is queried alone
- **THEN** its own entry is the first result

### Requirement: Ranking is derived from the loaded vocabulary and model

A served ranking MUST be computed from the vocabulary and model loaded at boot.
Any cache that serves a ranking MUST be invalidated when either the vocabulary
or the model changes, so an edited vocabulary can never be answered from a
result computed under the previous one.

#### Scenario: An edited vocabulary is never served from an old result

- **WHEN** the vocabulary changes and the service is restarted
- **THEN** queries are answered from the new vocabulary, with no result carried over from the previous one

### Requirement: Over-long queries are refused, not truncated

A term or name exceeding the transport's label or name limits SHALL be refused
as unparseable. Implicit truncation that silently answers a different query MUST
NOT happen.

#### Scenario: An over-long name is refused

- **WHEN** a name exceeds the maximum length the transport permits
- **THEN** the query is refused and no emoji is returned

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
