## Why

emb instances behind a load balancer (NLB, envoy, haproxy) need a way to know when an instance is actually ready to serve traffic. Currently `PING` returns `PONG` as soon as the port opens, before models finish loading. In a cluster, this means traffic hits instances that respond "model not found." A dedicated `EMB.READY` command lets operators drain instances gracefully and only route traffic to fully warm backends.

## What Changes

- Add an `EMB.READY` command handler returning structured server state
- Three readiness states: `loading`, `ready`, `draining`
- The `ready` state is reached when all preloaded models are loaded
- The `draining` state is set on SIGTERM so the LB can mark the instance unhealthy before shutdown completes
- `PING` continues to work (layer 4 health checks), but `EMB.READY` provides the richer signal layer 7-aware LBs need
- Update `EMB.HELP` to document the new command

## Capabilities

### New Capabilities
- `emb-ready`: Redis RESP command returning server readiness state with model loading progress

### Modified Capabilities
- (none)

## Impact

- `internal/server/server.go` — new `handleREADY` method, register in mux, track server state transitions, add `draining` signal
- `internal/server/server_test.go` — new tests for all readiness scenarios
- `internal/registry/registry.go` — expose a method to check whether all preloaded models are loaded
- `cmd/emb/main.go` — on SIGTERM, set draining state before initiating shutdown
- `gems/emb/lib/emb/client.rb` — add `ready` (returns status string) and `ready?` (returns boolean) methods wrapping `EMB.READY`
- `gems/emb/lib/emb.rb` — add `.ready` and `.ready?` delegations on the module
- `gems/emb/spec/emb_spec.rb` — add tests for `ready` on client and module
- `README.md` — add `EMB.READY` to the commands table and document the three states
