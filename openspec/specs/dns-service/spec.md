# dns-service Specification

## Purpose
Serves the emoji query zone as an authoritative DNS service over UDP and TCP,
with an HTTP surface for the parts `dig` cannot render and for operational
state, so a caller can reach real semantic retrieval through an ordinary
resolver.

## Requirements

### Requirement: One authoritative zone with a reserved apex

The service SHALL be authoritative for a single zone whose apex carries the
`SOA` and `NS` records and a usage `TXT`. Every name strictly beneath the apex
SHALL be answered synthetically as a query, with no zone file entries and no
wildcard record. A name outside the zone, including the apex's own `NS` target
names, MUST be refused rather than answered, so the service is never an open
resolver.

#### Scenario: A name beneath the apex is a query

- **WHEN** `i.lost.my.job` under the apex is queried
- **THEN** it is answered as an emoji query and never as a missing record

#### Scenario: The apex explains itself

- **WHEN** the apex is queried for `TXT`
- **THEN** the usage of the zone is returned

#### Scenario: Out-of-zone names are refused

- **WHEN** a name outside the zone, or a recursion-desired query for one, arrives
- **THEN** the reply is a refusal and the service performs no lookup on the caller's behalf

### Requirement: A query is answered with a ranked list, one glyph per record

A query SHALL be answered with one `TXT` record per result, up to the configured
`top_k`, in rank order and after any emoji sentence the query's shape calls for,
and each record's data SHALL be that result's glyph and nothing else — no score,
no name. Every answer record MUST be owned by the queried name; no result name
and no other owner name may be published.

#### Scenario: The first ranked record is the answer

- **WHEN** a query that is not a sentence returns more than one result
- **THEN** the first answer record carries the top-ranked emoji

#### Scenario: A record carries only its glyph

- **WHEN** an answer record is inspected
- **THEN** its data is one glyph, with no score and no name beside it

#### Scenario: No synthetic names are published

- **WHEN** any answer is returned
- **THEN** every record's owner is the queried name, so no result name enters a resolver's cache

### Requirement: A sentence is answered with an emoji sentence

When a query is a sentence, the reply SHALL carry one further `TXT` record whose
data is every ranked glyph joined in rank order with no separator, and that
record SHALL be the first in the answer. A query that is not a sentence MUST NOT
carry a joined record at all, so the joke belongs to the shape of the question
rather than to a setting.

#### Scenario: A sentence answers with a sentence

- **WHEN** a sentence is queried with `top_k` of 3 and three results come back
- **THEN** the first record's data is the three glyphs joined in rank order, and the three ranked records follow it

#### Scenario: A single word answers with one emoji

- **WHEN** a single word, a glyph, or a composition is queried
- **THEN** no joined record is carried, and the first record is the top-ranked glyph

#### Scenario: One result is a sentence of one

- **WHEN** a sentence is queried and `top_k` is 1
- **THEN** the joined record carries that one glyph

### Requirement: Only TXT questions are answered with data

A `TXT` question SHALL be answered with the ranked list. Any other question type
SHALL be answered as NODATA with the zone's `SOA` in the authority section, so
the absence is cacheable and no data is smuggled into a record type the caller
did not ask for. `ANY` SHALL be answered as the ranked list.

#### Scenario: An A question is empty, not wrong

- **WHEN** a query name is asked for `A`
- **THEN** the reply carries no answer record and carries the `SOA` in authority

#### Scenario: ANY is answered

- **WHEN** a query name is asked for `ANY`
- **THEN** the reply carries the ranked `TXT` records

### Requirement: Zone transfers and unserved types are refused

`AXFR` and `IXFR` MUST be refused. The service SHALL NOT claim `DNSSEC`
support, and MUST NOT return `SIG`/`RRSIG` records.

#### Scenario: A transfer is refused

- **WHEN** `AXFR` is requested for the zone
- **THEN** the reply is a refusal and no records are transferred

### Requirement: TTLs cache answers without freezing failures

Answer records SHALL carry the configured positive TTL, which MUST be short
enough that a vocabulary or model change is observable within minutes, and
negative answers SHALL use a separate, shorter negative TTL so a typo or a
parse failure is not cached as permanent. Because a given query name answers
identically across restarts for a given vocabulary, caching an answer for its
TTL MUST be safe.

#### Scenario: A positive answer carries the positive TTL

- **WHEN** a query is answered with results
- **THEN** each answer record carries the configured positive TTL

#### Scenario: A refusal or an empty answer expires quickly

- **WHEN** a name is unparseable or has no answer of the asked type
- **THEN** the negative TTL bounds how long the failure is cached

### Requirement: UDP and TCP are both served, from the address the query arrived on

The service SHALL answer UDP queries on port 53 of a public dedicated IPv4
address and TCP queries on port 53, and a UDP reply MUST be sent from the same
address and port that received the query. A UDP reply SHALL honor the payload
size the caller advertised through EDNS0, setting the truncated bit rather than
exceeding it, in which case the whole answer MUST be available over TCP. The
`AAAA` address MUST NOT be advertised as a UDP endpoint.

#### Scenario: A UDP reply comes back from the queried address

- **WHEN** a UDP query is sent to the public address on port 53
- **THEN** the reply arrives from that address on that port

#### Scenario: EDNS0 is answered in kind

- **WHEN** a query carries an EDNS0 OPT record
- **THEN** the reply carries an OPT record with the service's payload size

#### Scenario: An oversized answer falls back to TCP

- **WHEN** an answer would exceed the advertised UDP payload size
- **THEN** the UDP reply sets the truncated bit and the full answer is served over TCP

### Requirement: An HTTP surface carries what DNS cannot

The same service SHALL expose HTTP on its app: a plain-text route for a query
name, returning the ranked results as raw UTF-8, and a JSON route for the same
query. Both routes SHALL carry what the DNS record omits — each result's
vocabulary name and its similarity score — in rank order. It MUST also expose a
health route and a statistics route. HTTP routes MUST NOT accept any
state-changing request.

#### Scenario: The readable form shows real emoji

- **WHEN** a query is fetched over the plain-text HTTP route
- **THEN** the glyphs are returned as UTF-8 bytes, not as escapes

#### Scenario: The score the record omits is readable here

- **WHEN** a query is fetched over either read route
- **THEN** each result carries its vocabulary name and its score, in the same order the DNS records were ranked

#### Scenario: The same query is available as JSON

- **WHEN** a query is fetched over the JSON route
- **THEN** the ranked results, names, and scores are returned as structured data

### Requirement: No emoji is served before the vocabulary is ready

The service MUST NOT answer a query until the vocabulary index is loaded and
embeddable through the upstream model. Before that point a query SHALL be
answered with a server failure, never with a nearest neighbor computed from a
partial index and never as a negative answer, and the health route SHALL report
the service as not ready.

#### Scenario: Queries during startup fail loudly

- **WHEN** a query arrives before the index is loaded
- **THEN** the reply is a server failure and the health route reports not ready

#### Scenario: Readiness precedes traffic

- **WHEN** the health route reports ready
- **THEN** every subsequent query is answered from the complete vocabulary

### Requirement: Load is bounded per source

The service SHALL rate-limit queries per source address with a configurable
limit, so a flood cannot consume the upstream model or the machine. A
rate-limited query MUST be refused rather than answered with a lower-ranked or
partial result, and rate limiting MUST NOT prevent the health route from
answering.

#### Scenario: A flood is refused, not degraded

- **WHEN** one source exceeds the configured rate
- **THEN** its excess queries are refused and no result is invented for them

#### Scenario: Health survives a flood

- **WHEN** a source is being rate-limited
- **THEN** the health route still answers

### Requirement: Query text is not logged by default

The service MUST NOT log query names or their text by default; it SHALL record
counters and latency instead, and verbose query logging MUST be an explicit
operator opt-in. A logged name MUST be logged only in the verbose mode.

#### Scenario: Default logging omits the query

- **WHEN** a query is served with default configuration
- **THEN** its name and text do not appear in the service's logs

#### Scenario: Verbose mode is opt-in

- **WHEN** verbose query logging is enabled
- **THEN** query names appear in the logs and the configuration that enabled it is reported

### Requirement: The model is not reachable from outside the machine

The service SHALL run its upstream model on loopback in the same machine and
MUST NOT expose the upstream protocol's port publicly, and no credential for the
upstream may be given to a caller. The public surfaces are the DNS port and the
HTTP port.

#### Scenario: The upstream port is not public

- **WHEN** the upstream model's protocol port is contacted from outside the machine
- **THEN** it is not answered by the upstream server

#### Scenario: A caller receives no credential

- **WHEN** any query or HTTP response is inspected
- **THEN** it carries no upstream address, password, or token

### Requirement: The service reports the metadata a client surface needs

A client surface SHALL be able to read, without a resolver, the name of the
model that embeds queries, its dimension, the number of vocabulary entries, the
default result count, and whether the vocabulary is ready. These values MUST
come from the running service rather than from a page, so a rebuilt vocabulary
or a changed model changes what a surface reports without a page edit.

#### Scenario: A surface reads the served values

- **WHEN** the metadata route is fetched
- **THEN** it names the model, its dimension, the vocabulary size, the default result count, and readiness

#### Scenario: A rebuilt vocabulary changes the reading

- **WHEN** the vocabulary is rebuilt with a different number of entries and the service restarted
- **THEN** the metadata route reports the new count without any change to a client surface

### Requirement: The HTTP surface answers the site's own origins

The service SHALL answer cross-origin requests to its read-only HTTP routes from
the site's origins, and MUST NOT answer a cross-origin request from any other
origin. It MUST NOT allow a cross-origin request to change anything, and a
preflight for a non-read method MUST be refused.

#### Scenario: The gallery can query the zone from the browser

- **WHEN** a page served from the site's own origin fetches a query over the HTTP surface
- **THEN** the reply is readable by that page

#### Scenario: Another origin is refused

- **WHEN** a page from an unrelated origin requests the same route
- **THEN** the reply is not readable by that page

### Requirement: Statistics report the served surface

The statistics route SHALL report at least the number of queries served, the
number refused as unparseable, the number refused for rate limiting, the number
of upstream failures, and the upstream call latency, alongside whether the
vocabulary is ready.

#### Scenario: Refusals are distinguishable from failures

- **WHEN** the statistics are read after unparseable queries and upstream errors have occurred
- **THEN** the two counts are reported separately and neither is folded into the served count

### Requirement: The zone recovers a lost upstream connection

A lost connection to the upstream MUST NOT end the zone's service. When a call
fails because the connection itself is gone, the zone SHALL dial again and retry
that call once, and it MUST load the composition preset on the new connection,
because the server that answers it may be a restarted one. A failure the server
replied with MUST NOT be treated as a lost connection.

#### Scenario: An idle close is recovered

- **WHEN** the upstream closes a connection that was idle for its own timeout
- **THEN** the next query is answered, after the zone dials again

#### Scenario: The preset is reloaded on a redial

- **WHEN** the zone redials and the server has no composition preset loaded
- **THEN** the preset is loaded before the query is retried, and the query is answered

#### Scenario: A reply is not retried

- **WHEN** the server answers a call with an error reply
- **THEN** that error is returned and no redial is attempted

### Requirement: One upstream connection serves one caller at a time

A call on the upstream is a write, a flush, and a read on one socket, so the zone
SHALL serialize its upstream calls: two queries in flight at once MUST NOT
interleave their exchanges on the connection.

#### Scenario: Concurrent queries share the connection safely

- **WHEN** several queries are served concurrently
- **THEN** each is answered, and they use the one connection the zone holds

#### Scenario: A redial is not interleaved

- **WHEN** a call is redialling after a lost connection
- **THEN** no other call uses the connection until the redial and its retry are done
