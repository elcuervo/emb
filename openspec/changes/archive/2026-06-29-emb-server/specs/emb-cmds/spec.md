## ADDED Requirements

### Requirement: EMB command generates embeddings

The server SHALL accept `EMB <model> <text> [text...]` commands and return raw float32 embeddings.

#### Scenario: Single text embedding

- **WHEN** client sends `EMB siglip2 "hello world"` to a server with the `siglip2` model loaded
- **THEN** server responds with `$3072\r\n<768×float32 bytes>` (RESP bulk string containing raw embedding)

#### Scenario: Batch text embedding

- **WHEN** client sends `EMB siglip2 "hello" "world" "foo"`
- **THEN** server responds with a RESP array of bulk strings, one per input text, each containing `dim*4` raw float32 bytes

#### Scenario: Unknown model error

- **WHEN** client sends `EMB nonexistent "test"`
- **THEN** server responds with `-ERR model 'nonexistent' not found\r\n`

#### Scenario: Empty input error

- **WHEN** client sends `EMB siglip2` with no text arguments
- **THEN** server responds with `-ERR wrong number of arguments for 'EMB' command\r\n`

### Requirement: EMB.MODELS lists available models

The server SHALL respond to `EMB.MODELS` with a RESP array of model descriptors.

#### Scenario: List models

- **WHEN** client sends `EMB.MODELS`
- **THEN** server responds with a RESP array where each element is a RESP array `[name, dim, status]`

#### Scenario: No models loaded

- **WHEN** no models are configured and client sends `EMB.MODELS`
- **THEN** server responds with an empty RESP array `*0\r\n`

### Requirement: EMB.INFO shows model details

The server SHALL respond to `EMB.INFO <model>` with model metadata and usage statistics.

#### Scenario: Model info

- **WHEN** client sends `EMB.INFO siglip2`
- **THEN** server responds with a RESP array of key-value pairs: `[dim, 768, requests, 1423, avg_latency_us, 234]`

#### Scenario: Unknown model

- **WHEN** client sends `EMB.INFO nonexistent`
- **THEN** server responds with `-ERR model 'nonexistent' not found\r\n`

### Requirement: EMB.STATS shows server statistics

The server SHALL respond to `EMB.STATS` with server-wide statistics.

#### Scenario: Server stats

- **WHEN** client sends `EMB.STATS`
- **THEN** server responds with a RESP array of key-value pairs: `[uptime_secs, 3600, total_requests, 5432, models_loaded, 2]`

### Requirement: EMB.HELP lists commands

The server SHALL respond to `EMB.HELP` with a human-readable list of available EMB commands.

#### Scenario: Help output

- **WHEN** client sends `EMB.HELP`
- **THEN** server responds with a RESP bulk string containing usage text for each EMB command
