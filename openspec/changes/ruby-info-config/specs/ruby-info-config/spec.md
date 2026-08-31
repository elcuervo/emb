## Purpose

Ruby client wrappers for the server's Redis-style observability surface: a hash-shaped `EMB.STATS`, parsed sectioned `INFO`, and hot `CONFIG GET` / `CONFIG SET`.

## ADDED Requirements

### Requirement: Server stats hash

`Emb.stats` and `Client#stats` SHALL send `EMB.STATS` and return a Hash with Symbol keys — `:uptime_secs`, `:total_requests`, `:active_requests`, `:total_tokens`, `:total_errors`, `:models_loaded`, `:per_model`, `:cache_hits`, `:cache_misses`, `:cache_evictions` — with values exactly as the RESP client decodes them (Integer where the server sends RESP integers, String otherwise). No client-side value transformation SHALL be applied.

#### Scenario: Stats is a plain hash

- **WHEN** `Emb.stats` is called against a server that has served requests
- **THEN** the result SHALL be a Hash, not an Array
- **THEN** `result[:models_loaded]` SHALL be an Integer equal to the loaded model count (the server sends it as an integer)
- **THEN** `result[:per_model]` SHALL be the String the server sent
- **THEN** `result[:active_requests]` SHALL be whatever the server sent (a numeric String `"0"` today — passed through unchanged)

### Requirement: Parsed sectioned INFO

`Emb.server_info(*sections)` and `Client#server_info(*sections)` SHALL send `INFO [sections...]` (no sections = all) and return a nested Hash of the Redis section format: each `# Section` header becomes a top-level Symbol key, each `key:value` line a sub-key. Values SHALL pass through as the RESP client decoded them; no client-side value transformation SHALL be applied.

#### Scenario: Full INFO parses to nested hash

- **WHEN** `Emb.server_info` is called with no arguments
- **THEN** the result SHALL include `:Server` with `:redis_version` and `:emb_version` Strings, `:Cache` with `:cache_hit_rate` a String, and `:Stats`, `:Keyspace`, `:Clients` sections

#### Scenario: Section filtering

- **WHEN** `Emb.server_info(:server, :cache)` is called
- **THEN** the request SHALL be `INFO server cache`
- **THEN** the result SHALL contain only the `Server` and `Cache` sections

#### Scenario: Keyspace lines parse per model

- **WHEN** `Emb.server_info(:keyspace)` is called with two loaded models
- **THEN** the result SHALL contain a `:Keyspace` value with one entry per model whose values are the server's decoded values (`keys`/`hits`/`misses` Integers, `hit_rate` String)

### Requirement: Hot config read

`Emb.config_get(*patterns)` and `Client#config_get` SHALL send `CONFIG GET` (with the patterns when given) and return a Hash of String parameter → String value (e.g. `{"cache" => "auto", "listen" => ":6379"}`). No pattern SHALL fetch all parameters; an unmatched pattern SHALL return an empty Hash.

#### Scenario: All parameters as string values

- **WHEN** `Emb.config_get` is called
- **THEN** the result SHALL include `cache`, `password`, `listen`, `cache_file`, `cache_save`, `models`, `tls_cert`, `tls_key`
- **THEN** every value SHALL be a String

#### Scenario: Glob pattern filters

- **WHEN** `Emb.config_get("cache*")` is called
- **THEN** the result SHALL contain only keys starting with `cache`

### Requirement: Hot config change

`Emb.config_set(param, value)` and `Client#config_set(param, value)` SHALL send `CONFIG SET <param> <value>` and return `true` on success. Failures (unknown or read-only parameter, invalid value, or `NOAUTH` when a password is configured) SHALL raise an exception carrying the server's error message.

#### Scenario: Set success returns true

- **WHEN** `Emb.config_set(:cache_file, "/tmp/emb.rdb")` is called
- **THEN** the command SHALL be `CONFIG SET cache_file /tmp/emb.rdb`
- **THEN** the return value SHALL be `true`

#### Scenario: Read-only parameter raises

- **WHEN** `Emb.config_set(:listen, ":9999")` is called
- **THEN** an exception SHALL be raised whose message names the read-only parameter

#### Scenario: Auth required surfaces as exception

- **WHEN** `Emb.config_get` or `Emb.config_set` is called against a password-protected server without authenticating
- **THEN** an exception SHALL be raised with the server's `NOAUTH` message

### Requirement: Module-level delegation

The gem SHALL expose `Emb.stats` (pre-existing method, new return shape), `Emb.server_info`, `Emb.config_get`, and `Emb.config_set` delegating to the default client, mirroring the existing wrapper style.

#### Scenario: Module methods delegate

- **WHEN** `Emb.server_info`, `Emb.config_get`, and `Emb.config_set` are called
- **THEN** each SHALL delegate to the default client's same-named method (same as `Emb.stats` delegates)