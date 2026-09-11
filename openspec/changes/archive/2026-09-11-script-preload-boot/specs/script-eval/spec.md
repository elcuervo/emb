## MODIFIED Requirements

### Requirement: Script identity and per-model caching

Scripts SHALL be cached per model keyed by SHA1 of the script source. `EMB.SCRIPT LOAD <model> <script>` SHALL compile and cache the script, replying with its SHA1. `EMB.EVSHA <model> <sha> <numtexts> <text...> <arg...>` SHALL execute the cached script by SHA1. `EMB.SCRIPT EXISTS <model> <sha...>` SHALL reply with an array of 1/0 flags, and `EMB.SCRIPT FLUSH [<model>]` SHALL clear cached scripts (all models when the model name is omitted). The script cache MAY be pre-populated at boot from files declared in model config; preloaded scripts SHALL behave identically to dynamically loaded ones.

#### Scenario: Load then evaluate by SHA

- **WHEN** a client loads a script with `EMB.SCRIPT LOAD` and then sends `EMB.EVSHA` with the returned SHA1
- **THEN** the cached script executes with the same KEYS/ARGV semantics as `EMB.EVAL`

#### Scenario: Unknown SHA

- **WHEN** `EMB.EVSHA` names a SHA1 that is not cached for the model
- **THEN** the server replies with a no-such-script error

#### Scenario: Exists reflects the per-model cache

- **WHEN** a script is loaded for model A but not for model B
- **THEN** `EMB.SCRIPT EXISTS A <sha>` replies `[1]` and `EMB.SCRIPT EXISTS B <sha>` replies `[0]`

#### Scenario: Load rejects an invalid script

- **WHEN** `EMB.SCRIPT LOAD` receives a script that fails to compile
- **THEN** the server replies with an error and caches nothing

#### Scenario: Boot-preloaded script is present without LOAD

- **WHEN** a script was preloaded at boot from a config file path
- **THEN** `EMB.SCRIPT EXISTS` reports it and `EMB.EVSHA` executes it without any client-side `EMB.SCRIPT LOAD`
