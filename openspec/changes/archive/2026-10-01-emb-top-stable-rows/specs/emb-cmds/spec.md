# Spec Delta

## ADDED Requirements

### Requirement: EMB.MODELS returns a deterministic order

The server SHALL list models from `EMB.MODELS` in a deterministic order — sorted by model name — so repeated calls return the same sequence and clients can render a stable list. Under RESP3 the reply is a map, so order is not significant there.

#### Scenario: Repeated calls agree

- **GIVEN** a server with one or more models loaded
- **WHEN** a RESP2 client sends `EMB.MODELS` twice
- **THEN** both replies list the models in the same name-sorted order

#### Scenario: Models are listed by name

- **GIVEN** models `minilm`, `bge`, and `e5` are registered
- **WHEN** a RESP2 client sends `EMB.MODELS`
- **THEN** the reply lists them in the order `bge`, `e5`, `minilm`
