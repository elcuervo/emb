## 1. Config changes

- [x] 1.1 Add `Password string` field to `Config` struct in `internal/config/config.go`
- [x] 1.2 Add `-password <val>` flag handling to `ParseFlags` in `internal/config/config.go`

## 2. Server auth implementation

- [x] 2.1 Add `password string` field to `Server` struct in `internal/server/server.go`
- [x] 2.2 Create `connState` struct with `authenticated bool`
- [x] 2.3 Initialize `connState` in the accept callback via `conn.SetContext`
- [x] 2.4 Add `handleAUTH` method to `Server`
- [x] 2.5 Add inline auth guard in the dispatch closure with `isExempt` / `isAuthenticated` helpers
- [x] 2.6 Register `handleAUTH` in the mux and wire inline auth guard into the dispatch closure
- [x] 2.7 Update `New` signature to accept `password string`
- [x] 2.8 Update `EMB.HELP` to include AUTH in the help text

## 3. Main wiring

- [x] 3.1 Pass `fc.Password` to `server.New` in `cmd/emb/main.go`

## 4. Tests

- [x] 4.1 `TestAUTHNoPassword` — server without password rejects AUTH with correct error
- [x] 4.2 `TestAUTHWrongPassword` — wrong pass returns invalid password error
- [x] 4.3 `TestAUTHCorrectPassword` — correct pass returns OK
- [x] 4.4 `TestCommandBeforeAuth` — any command returns NOAUTH error
- [x] 4.5 `TestPINGBeforeAuth` — PING works without auth
- [x] 4.6 `TestAUTHDouble` — AUTH twice, both succeed
- [x] 4.7 `TestCommandsWorkAfterAuth` — EMB etc work after successful AUTH

## 5. Config example

- [x] 5.1 Add commented-out `password` field to `config.yaml`
