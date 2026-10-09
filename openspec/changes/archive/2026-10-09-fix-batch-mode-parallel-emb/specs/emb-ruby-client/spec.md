## ADDED Requirements

### Requirement: Script calls honor the lazy mode

`eval` and `evalsha` SHALL honor the client's configured `lazy` mode the same way the proxy embed API does: eager under `false`, deferred under `:multi` and `:batch` with behavior defined by the `ruby-batch-loading` "Deferred script evaluation" requirement. The explicit `multi` block API is unaffected and stays eager.

#### Scenario: Deferred eval does not send at call time

- **WHEN** `client = Emb.new(lazy: :batch)` calls `client.eval(:minilm, script, ["a"])`
- **THEN** no command SHALL be sent until the returned value is used
