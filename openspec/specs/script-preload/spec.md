# script-preload

## Purpose

Allows operators to declare Lua script file paths in model config so the server preloads, validates, and caches them at boot time, making `EMB.EVSHA` immediately available without client-side `EMB.SCRIPT LOAD` round-trips.

## Requirements

### Requirement: Script file paths in model config

The server SHALL accept a `scripts` field on each `ModelConfig` entry. The value SHALL be a list of entries, where an entry is either a string naming a Lua script source file or a mapping with a required `path` field and an optional `config` field. Relative `path` values SHALL be resolved against the directory containing the config file. Absolute paths SHALL be used as-is. Paths SHALL be validated at boot: a missing file, unreadable file, or path that is not a regular file SHALL be fatal and prevent server startup. A `config` that is not a mapping, or that contains a non-finite number, or that exceeds the documented size bound, SHALL be fatal at boot with an error naming the script path.

Each entry's `config` SHALL be exposed to that script alone as `emb.script.config`, a Lua table of the same shape as `json.decode` output (mappings to string-keyed tables, sequences to 1-based arrays, scalars to strings/numbers/booleans, null to the `json.null` sentinel). It SHALL be an empty table when the entry declares no config, and SHALL be semantically opaque to the server: the server SHALL NOT interpret, rename, or type any key. The entry's config SHALL participate in the script's reply-cache identity, so that changing a config invalidates previously cached replies for that script rather than serving them under the new config.

#### Scenario: Relative path resolved from config directory

- **WHEN** a model config entry includes `scripts: ["scripts/classify.lua"]` and the config file is at `/etc/emb/config.yaml`
- **THEN** the server reads `/etc/emb/scripts/classify.lua` at boot

#### Scenario: Absolute path used as-is

- **WHEN** a model config entry includes `scripts: ["/etc/emb/classify.lua"]`
- **THEN** the server reads `/etc/emb/classify.lua` at boot without path resolution

#### Scenario: Missing script file prevents startup

- **WHEN** a script path points to a file that does not exist
- **THEN** server startup fails with an error naming the missing path

#### Scenario: Shorthand and mapping entries are equivalent

- **WHEN** one entry is the string `./presets/embed.lua` and another is `{path: ./presets/embed.lua}`
- **THEN** both preload the same script with an empty `emb.script.config`

#### Scenario: A script reads its own config

- **WHEN** a model entry declares `scripts: [{path: ./laya.lua, config: {max_len: 64, temperature: [1.6, 1.25, 1.98]}}]`
- **THEN** inside that script `emb.script.config.max_len` is `64` and `emb.script.config.temperature[2]` is `1.25`, while another preset on the same model sees its own config

#### Scenario: Absent config is an empty table

- **WHEN** a script with no declared config runs
- **THEN** `emb.script.config` is a table with no keys, so `emb.script.config.x or fallback` reads the fallback

#### Scenario: Invalid config prevents startup

- **WHEN** a `scripts` entry's `config` is a string or a list rather than a mapping
- **THEN** server startup fails with an error naming the script path

#### Scenario: Editing a config invalidates cached replies

- **WHEN** a script's reply is cached and the operator changes that script's config and restarts with the same cache snapshot
- **THEN** the request misses the cache and recomputes rather than returning the reply computed under the old config

### Requirement: Boot-time script validation and caching

For each configured script file, the server SHALL read the source, check it against the same size cap as `EMB.SCRIPT LOAD`, compile it with the same Lua validator, and store it in the per-model script cache keyed by SHA1. If validation fails (oversized, invalid Lua syntax), startup SHALL be fatal. The compiled Lua prototype SHALL also be cached so the first `EMB.EVSHA` skips the parse/compile step.

#### Scenario: Valid script is cached at boot

- **WHEN** the server starts with a valid script file configured for model `minilm`
- **THEN** `EMB.SCRIPT EXISTS minilm <sha>` replies `[1]` immediately after the server reports ready

#### Scenario: First EVSHA uses precompiled prototype

- **WHEN** a script was preloaded at boot and a client sends the first `EMB.EVSHA` for that SHA
- **THEN** the request executes without incurring a Lua parse/compile

#### Scenario: Invalid Lua prevents startup

- **WHEN** a configured script file contains invalid Lua (e.g. unclosed string)
- **THEN** server startup fails with a compilation error

#### Scenario: Oversized script prevents startup

- **WHEN** a configured script file exceeds `DefaultMaxScriptBytes`
- **THEN** server startup fails with a size-exceeded error

### Requirement: Preloaded scripts are indistinguishable from dynamically loaded ones

Scripts loaded at boot SHALL reside in the same per-model cache as scripts loaded via `EMB.SCRIPT LOAD`. `EMB.SCRIPT EXISTS`, `EMB.EVSHA`, and `EMB.SCRIPT FLUSH` SHALL treat them identically. After `EMB.SCRIPT FLUSH`, a preloaded script SHALL be gone until the server is restarted.

#### Scenario: Flush drops preloaded scripts

- **WHEN** a preloaded script is present and a client sends `EMB.SCRIPT FLUSH <model>`
- **THEN** `EMB.SCRIPT EXISTS <model> <sha>` subsequently replies `[0]`

#### Scenario: EVSHA works without prior LOAD

- **WHEN** a script was preloaded at boot and a client sends `EMB.EVSHA` with its SHA without ever calling `EMB.SCRIPT LOAD`
- **THEN** the script executes and replies normally
