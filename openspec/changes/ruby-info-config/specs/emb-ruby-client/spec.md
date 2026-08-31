## MODIFIED Requirements

### Requirement: Command wrappers

The gem SHALL expose module-level methods for all server commands, delegating to the default client.
Instance clients SHALL expose the same methods.

#### Scenario: List models

- **WHEN** `Emb.models` or `client.models` is called
- **THEN** it SHALL send `EMB.MODELS` and return an array of `{name:, dim:, status:}` hashes

#### Scenario: Model info

- **WHEN** `Emb.info(:minilm)` or `client.info(:minilm)` is called
- **THEN** it SHALL send `EMB.INFO minilm` and return a hash of key-value pairs

#### Scenario: Server stats

- **WHEN** `Emb.stats` or `client.stats` is called
- **THEN** it SHALL send `EMB.STATS` and return a **Hash with Symbol keys** — numeric fields (`uptime_secs`, `total_requests`, `active_requests`, `total_tokens`, `total_errors`, `models_loaded`, `cache_hits`, `cache_misses`, `cache_evictions`) as Integers, `per_model` as a String (was: the raw parsed RESP array)

#### Scenario: Redis-style server info

- **WHEN** `Emb.server_info` or `client.server_info` is called, with optional section names
- **THEN** it SHALL send `INFO [sections...]` and return the parsed nested section Hash

#### Scenario: Hot config read and change

- **WHEN** `Emb.config_get` or `client.config_get` is called, with optional glob patterns
- **THEN** it SHALL send `CONFIG GET` and return a Hash of parameter → String value
- **WHEN** `Emb.config_set(param, value)` or `client.config_set(param, value)` is called
- **THEN** it SHALL send `CONFIG SET param value`, return `true` on success, and raise on server error

#### Scenario: Help text

- **WHEN** `Emb.help` or `client.help` is called
- **THEN** it SHALL send `EMB.HELP` and return the response string

#### Scenario: Ping

- **WHEN** `Emb.ping` or `client.ping` is called
- **THEN** it SHALL send `PING` and return `"PONG"`