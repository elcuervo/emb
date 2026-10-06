# Spec Delta

## RENAMED Requirements

- FROM: `### Requirement: The demos run against the models the sandbox reports`
- TO: `### Requirement: The demos run against the models the surface serving them reports`

## MODIFIED Requirements

### Requirement: The demos run against the models the surface serving them reports

Every model a demo names SHALL be resident on the surface that answers its
queries and reported by that surface's metadata. Where a demo compares models it
MUST show that vectors from two different models are not interchangeable,
because each model defines its own space. A demo MUST NOT name a model the
surface it calls does not serve, and a plate served by a surface other than the
sandbox MUST state which surface answered it.

#### Scenario: Every named model is loaded

- **WHEN** a demo names a model
- **THEN** the surface the demo calls reports that model as loaded, and the page's model list is derived from the server rather than typed

#### Scenario: The comparison teaches incomparability

- **WHEN** a demo presents two models over one corpus
- **THEN** it states that the two models' vectors are not comparable, and demonstrates that the same text occupies different neighbourhoods under each

#### Scenario: A plate on another surface names it

- **WHEN** a plate's queries are answered by a surface other than the sandbox
- **THEN** the plate states which surface answered, and reports that surface's own model and index metadata rather than the sandbox's

## ADDED Requirements

### Requirement: A plate may query the zone without a resolver

The gallery SHALL include a plate that runs the DNS zone's queries over the
zone's own read-only HTTP surface, so a reader needs no resolver and no
terminal, and SHALL show the `dig` invocation that would issue the same query —
the same name, the same record type — beside it. The plate MUST refuse a query
the DNS transport would refuse, so it never demonstrates a name the zone would
not answer, and it MUST state that the two routes run the same query against the
same service.

#### Scenario: The browser and the resolver ask the same question

- **WHEN** a reader runs a query on the plate
- **THEN** the answer is the service's reply over HTTP, and the plate shows the `dig` command carrying the same name and record type

#### Scenario: A refused name is refused on the plate

- **WHEN** a reader enters a name the zone refuses — unparseable, empty, or over the transport's limit
- **THEN** the plate states the refusal and shows no result, exactly as the resolver route would

#### Scenario: The plate names the surface that answered

- **WHEN** the plate renders a result
- **THEN** it names the service that answered and the model that embedded the query

### Requirement: The plate shows the working the record omits

Because each record the zone answers with carries only its glyph, the plate SHALL
show every ranked result's vocabulary name and its similarity score, in the order
the records were ranked.

#### Scenario: Names and scores are shown in rank order

- **WHEN** the plate renders a result
- **THEN** each ranked result shows its vocabulary name and its score, in the order the DNS records carry them

#### Scenario: The plate shows the working the record omits

- **WHEN** the plate renders a result
- **THEN** each ranked result shows its vocabulary name and its score, in the order the DNS records carry them

### Requirement: The plate shows the answer as the wire carries it

Because every resolver escapes non-ASCII `TXT` data, the plate SHALL show both
the escaped record data the DNS transport produces and the glyph it stands for,
side by side, and SHALL state why the two differ. The escaped form SHALL be
derived from the reply the service returned rather than transcribed into the
page.

#### Scenario: The escaped bytes and the glyph are shown together

- **WHEN** a query returns an emoji
- **THEN** the plate shows the escaped `TXT` data beside the glyph, and both come from the same reply

#### Scenario: The reason is stated

- **WHEN** the escaped form is shown
- **THEN** the plate states that the escaping happens in the resolver and in the reader's tool, not in the zone

### Requirement: The plate's examples are a fixed, verified set

The plate SHALL ship a fixed set of example queries covering each mode the zone
answers — a sentence, an emoji, and a composition — each reachable from one
control and each named by the URL fragment. Every shipped example MUST be
verified against the zone before deployment, and the plate SHALL state which
mode each example demonstrates.

#### Scenario: One control per mode

- **WHEN** the plate is rendered
- **THEN** a reader can run a sentence example, an emoji example, and a composition example from one control each

#### Scenario: An example is named by the URL

- **WHEN** a reader selects an example
- **THEN** the fragment names it, and opening that fragment selects it again

#### Scenario: A shipped example is verified, not asserted

- **WHEN** the examples are checked before deployment
- **THEN** each one returns the result the plate's own copy claims for it, and a drifting example fails the check rather than shipping
