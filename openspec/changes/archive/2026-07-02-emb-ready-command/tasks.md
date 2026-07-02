## 1. Registry readiness

- [x] 1.1 Add `totalModels int` field to `Registry` struct, set in `New` or after all models are registered
- [x] 1.2 Add `ModelsLoaded() (loaded, total int)` method — iterate `List()`, count entries where `loaded` is true
- [x] 1.3 Add `SetModelCount(n int)` method so main can record total after registration

## 2. Server state

- [x] 2.1 Add `serverState` type with `stateLoading`, `stateReady`, `stateDraining` iota constants
- [x] 2.2 Add `state atomic.Int64` field to `Server` struct
- [x] 2.3 Initialize state to `stateLoading` in `New`
- [x] 2.4 Add `SetReady()` method — atomically set state to `stateReady`
- [x] 2.5 Add `SetDraining()` method — atomically set state to `stateDraining`
- [x] 2.6 Add `handleREADY` method — read state, query registry, build response array
- [x] 2.7 Register `handleREADY` in the mux (make it exempt from auth like PING)

## 3. Server transition wiring

- [x] 3.1 After model registration loop in `cmd/emb/main.go`, call `reg.SetModelCount(n)` and `srv.SetReady()`
- [x] 3.2 On SIGTERM in `cmd/emb/main.go`, call `srv.SetDraining()` before initiating shutdown

## 4. Tests

- [x] 4.1 `TestREADYWhenReady` — all preloaded models loaded, EMB.READY returns `+OK`
- [x] 4.2 `TestREADYWhenLoading` — models still loading, EMB.READY returns `-ERR loading`
- [x] 4.3 `TestREADYDraining` — after setting draining, EMB.READY returns `-ERR draining`
- [x] 4.4 `TestREADYNoModels` — no models configured, EMB.READY returns `-ERR no models`

## 5. Help text

- [x] 5.1 Add `EMB.READY - Show server readiness state` to EMB.HELP output

## 6. Ruby gem

- [x] 6.1 Add `def ready` to `Emb::Client` — sends `EMB.READY`, returns the reason string (nil on error)
- [x] 6.2 Add `def ready?` to `Emb::Client` — sends `EMB.READY`, returns `true` on `+OK`, `false` otherwise
- [x] 6.3 Add `.ready` and `.ready?` delegations to `Emb` module in `lib/emb.rb`
- [x] 6.4 Add `describe '.ready?'` test block to `spec/emb_spec.rb` — tests `true` when server is ready
- [x] 6.5 Add `describe '.ready'` test block — tests it returns a string
- [x] 6.6 Add instance `ready` and `ready?` tests to the `'.new'` describe block

## 7. README

- [x] 7.1 Add `EMB.READY` row to the commands table with description of the three states (loading/ready/draining)
- [x] 7.2 Add a brief section or note about using EMB.READY for load balancer health checks
