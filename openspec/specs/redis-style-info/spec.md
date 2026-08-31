## Purpose

Redis-style sectioned `INFO` for emb: version, global statistics, and cache-hit ratios (overall and per model) in the format Redis clients and dashboards already parse.

## Requirements

### Requirement: Sectioned INFO command

The server SHALL respond to `INFO` (and `INFO <section>...`) with a RESP2 bulk string in Redis section format: lines grouped under `# SectionName` headers, keys as `key:value` with `\r\n` line endings.

#### Scenario: No arguments returns all sections

- **WHEN** a Redis client sends `INFO` with no arguments
- **THEN** the reply SHALL be a bulk string containing at least `# Server`, `# Cache`, `# Keyspace`, `# Stats`, and `# Clients` sections in that order

#### Scenario: Section argument filters

- **WHEN** a Redis client sends `INFO server`
- **THEN** the reply SHALL contain only the `# Server` section

#### Scenario: Multiple section arguments

- **WHEN** a Redis client sends `INFO cache stats`
- **THEN** the reply SHALL contain only the `# Cache` and `# Stats` sections

#### Scenario: Unknown section name

- **WHEN** a Redis client sends `INFO nonexistent`
- **THEN** the reply SHALL be a bulk string with no sections (empty body)

### Requirement: Server section carries the build version

The `# Server` section SHALL expose `redis_version:` equal to the injected build version (`dev` when unset), plus `emb_version:` with the same value, `uptime_secs`, and the real `process_id`. Only measurable values SHALL be reported; no invented Redis fields (e.g., fake `connected_clients` or `used_memory`) SHALL appear.

#### Scenario: Version lines carry the injected build

- **WHEN** a client sends `INFO server` against a server built with an injected version (e.g. `0.2.4`)
- **THEN** the reply SHALL contain `redis_version:0.2.4` and `emb_version:0.2.4`
- **WHEN** the same server had no version injected
- **THEN** both lines SHALL read `dev`

### Requirement: Cache hit ratio is a headline section

The `# Cache` section SHALL report `cache_hits`, `cache_misses`, `cache_hit_rate`, `cache_evictions`, `cache_entries`, `cache_max_bytes`, and `cache_memory_bytes`. `cache_hit_rate` SHALL be the ratio of cache hits to (hits + misses), formatted as a percentage (e.g., `90.0%`), matching the value `EMB.INFO <model>` reports.

#### Scenario: Hit ratio reflects the counters

- **WHEN** the cache has 90 hits and 10 misses recorded
- **THEN** `INFO cache` SHALL contain `cache_hits:90`, `cache_misses:10`, and `cache_hit_rate:90.0%`

#### Scenario: No requests yet

- **WHEN** no cache activity has occurred
- **THEN** `cache_hit_rate` SHALL be `0.0%` (no division-by-zero)

### Requirement: Per-model hit ratios in Keyspace

The `# Keyspace` section SHALL contain one line per loaded model in the form `db0:model=<name>,keys=<entries>,hits=<hits>,misses=<misses>,hit_rate=<rate>`.

#### Scenario: Two models with different rates

- **WHEN** models `minilm` and `bge` are loaded with distinct cache counters
- **THEN** `INFO` SHALL contain two `db0:` lines, each with that model's own entries, hits, misses, and hit_rate

### Requirement: Unauthenticated INFO probe

When a password is configured, `INFO` SHALL answer without prior `AUTH`, like `PING` and `EMB.READY`.

#### Scenario: Pre-auth on password-protected server

- **WHEN** a password is configured and a client sends `INFO` without authenticating
- **THEN** the server SHALL reply with the sectioned info (not an auth error)

### Requirement: INFO documented in EMB.HELP

`EMB.HELP` SHALL document `INFO` with its section syntax.

#### Scenario: Help lists the command

- **WHEN** a client sends `EMB.HELP`
- **THEN** the response SHALL include a line for `INFO` describing `<section...>` usage