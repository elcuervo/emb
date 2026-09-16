## 1. Bridge measurement

- [x] 1.1 Add `ElapsedUs int64 \`json:"elapsed_us,omitempty"\`` to `Envelope` in `website/repl/envelope.go`; verify `go build ./website/repl/` succeeds
- [x] 1.2 In `website/repl/bridge.go` `send()`, time the bracket from `WriteArgv` through `ReadReply` (after `Hello`), return the elapsed value alongside the reply, and thread it into the envelope in `Execute`; verify `go test ./website/repl/ -count=1` passes
- [x] 1.3 Add a bridge test asserting an answered command carries a non-zero `elapsed_us` and a refused command (never reaching `emb`) carries none; verify with `go test ./website/repl/ -run Elapsed -count=1`

## 2. Client rendering

- [x] 2.1 In `website/repl/terminal.js`, make `timing()` format `env.elapsed_us` and push no `k: 'time'` line when it is absent; verify the module still loads (`node --check website/repl/terminal.js`)
- [x] 2.2 Remove `startedAt`, `nowMs()`, and their assignments in `submit`/`retry` so no client clock remains in the timing path; verify no remaining references with `grep -n "startedAt\|nowMs" website/repl/terminal.js` (expect none)

## 3. Docs and end-to-end

- [x] 3.1 Reword the trailer's description in `website/README.md` from the client-measured "wait it cost from submit" to the server-measured answer time; verify the passage no longer claims a client measurement
- [x] 3.2 Run the sandbox locally with `just website-dev` and run an `EMB` command in the console; verify the trailer shows a single-digit-millisecond value that does not change when the page is loaded from a throttled/slow network profile, and that a refused command renders no trailer
- [x] 3.3 Run `openspec validate console-server-timing` and confirm it passes
