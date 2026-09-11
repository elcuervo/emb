# emb-ruby-client Specification (delta)

## MODIFIED Requirements

### Requirement: Embed queries accept a reply format

The gem's embed surface SHALL accept a `format:` option with values `:binary` (default) and `:values`. `:binary` SHALL keep sending the plain `EMB <model> <text...>` command and unpack the float32 bulk reply exactly as today. `:values` SHALL send `EMB <model> VALUES <text...>` and parse the VALUES envelope reply into Ruby floats. The option SHALL exist on the per-instance embed path and the proxy/batch loaders.

#### Scenario: Default stays binary

- **GIVEN** an `Emb::Client` instance
- **WHEN** `client` embeds `"hello world"` on model `minilm` with no `format:` argument
- **THEN** the command sent SHALL be `EMB minilm "hello world"` (no keyword)
- **AND** the returned array SHALL be the float32-unpacked values

#### Scenario: VALUES embeds with the keyword

- **WHEN** the caller passes `format: :values`
- **THEN** the command sent SHALL be `EMB minilm VALUES "hello world"`
- **AND** the returned values SHALL match the binary-path values when compared as floats

### Requirement: VALUES envelopes decode to floats

The gem SHALL parse a VALUES envelope reply (flat alternating key/value array, since the gem speaks RESP2) into a Hash, and convert each `values` entry from its decimal bulk string into a Ruby Float. Row-major ordering SHALL be preserved per text.

#### Scenario: Single-text envelope decodes

- **GIVEN** a model with `dim` 3
- **WHEN** the reply is the envelope `["dtype","FLOAT","shape",[1,3],"values",["0.1","0.2","0.3"]]`
- **THEN** the parsed result SHALL be a Hash with `dtype: "FLOAT"`, `shape: [1, 3]`
- **AND** `values` SHALL be `[0.1, 0.2, 0.3]` as Floats

#### Scenario: Multi-text envelope preserves order

- **WHEN** the reply envelope carries `shape [2, 2]` and four `values`
- **THEN** the gem SHALL return the values grouped per text (rows of the shape), preserving request order

#### Scenario: Invalid value text errors

- **WHEN** a `values` entry is not a parseable decimal
- **THEN** the gem SHALL raise an error naming the offending entry