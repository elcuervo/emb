## MODIFIED Requirements

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
