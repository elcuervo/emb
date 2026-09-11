## 1. Config and Path Resolution

- [x] 1.1 Add `Scripts []string` to `internal/config.ModelConfig` and verify `go build ./internal/config/` succeeds
- [x] 1.2 Resolve relative script paths against the config file directory in `config.Load()`: store `configDir` from `filepath.Dir(path)`, prepend it to non-absolute entries. Verify with `go test ./internal/config/ -run TestLoad` or add a test case
- [x] 1.3 Add `scripts` entries to `test-two-models.yaml` (pointing at test fixture files) and verify `config.Load` resolves them correctly

## 2. Compiler Precompile

- [x] 2.1 Add `Precompile(model, source string) error` to `internal/script/compiler.go` that calls the existing `compile` method and discards the proto (warming the cache). Verify with `go test ./internal/script/`
- [x] 2.2 Ensure `Precompile` respects the same size cap and returns the same errors as `Eval`. Verify with a unit test: oversized source returns `ErrScriptTooLarge`, invalid Lua returns a syntax error

## 3. Server PreloadScript and Refactor

- [x] 3.1 Add `PreloadScript(model, src string) (string, error)` to `internal/server/server.go`: validate model exists, check size, call `script.Compile`, load into `scriptCache`, call `compiler.Precompile`, return SHA1. Verify with `go build ./internal/server/`
- [x] 3.2 Refactor `handleScriptLoad` in `internal/server/script.go` to delegate to `PreloadScript` instead of inlining validation and cache insertion. Verify `go test ./internal/server/ -run TestScriptLoadExistsFlush` still passes
- [x] 3.3 Ensure `PreloadScript` rejects unknown models with the same error as `handleScriptLoad`. Verify with a unit test

## 4. Boot-Time Preload in main.go

- [x] 4.1 In `cmd/emb/main.go`, after `server.New` and before `SetReady`, iterate `fc.Models`, read each script file with `os.ReadFile`, call `srv.PreloadScript`, and fail fatally on error. Verify `go build ./cmd/emb/` succeeds
- [x] 4.2 Add a valid script fixture file under `testdata/` or `scripts/` and reference it from a test config; verify the server starts and `EMB.SCRIPT EXISTS` reports the SHA

## 5. Tests

- [x] 5.1 Add `TestScriptPreloadExists` in `internal/server/script_test.go`: boot a server with a preloaded script (via test helper or file fixture), dial, and assert `EMB.SCRIPT EXISTS` returns `[1]`
- [x] 5.2 Add `TestScriptPreloadEvshaWithoutLoad` in `internal/server/script_test.go`: send `EMB.EVSHA` for a preloaded script without any `EMB.SCRIPT LOAD`, verify the reply matches an equivalent `EMB.EVAL`
- [x] 5.3 Add `TestScriptPreloadInvalidLuaFatal` in `internal/server/script_test.go` (or a config test): create a server with an invalid Lua script file and assert that `New`/`ListenAndServe` fails with a compilation error (covered by `TestPrecompileRejectsInvalidLua` in compiler_test.go)
- [x] 5.4 Add `TestScriptPreloadFlushDrops` in `internal/server/script_test.go`: boot with a preloaded script, `EMB.SCRIPT FLUSH`, then assert `EMB.SCRIPT EXISTS` returns `[0]`
- [x] 5.5 Add `TestScriptPreloadCompilerWarmed` in `internal/server/script_test.go`: boot with a preloaded script, note `compiler.Compiles` before the first `EMB.EVSHA`, send `EMB.EVSHA`, and assert `Compiles` did not increment (prototype was precompiled at boot)
- [x] 5.6 Add `TestScriptPreloadMissingFileFatal` in `internal/server/script_test.go` or config test: configure a nonexistent script path and assert startup fails naming the missing file (covered by `TestLoadScriptsMissingFile` in config_test.go)
- [x] 5.7 Run `just test` and ensure all tests pass
- [x] 5.8 Run `just lint` and ensure no lint errors

## 6. Integration and Documentation

- [x] 6.1 Add a preloaded script example to `test-two-models.yaml` (commented or active with a test fixture) so the integration suite exercises the feature
- [x] 6.2 Update `EMB.HELP` text in `internal/server/server.go` to mention that scripts can be preloaded from config (one-line addition)
- [x] 6.3 Run `just all` end-to-end and verify the server starts, preloads scripts, and Ruby client tests pass
