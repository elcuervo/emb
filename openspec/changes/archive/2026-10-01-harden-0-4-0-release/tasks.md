## 1. Server: concurrency-safe runtime caps

- [x] 1.1 Change `max_texts`, `max_pairs`, `max_images`, `max_image_bytes`, `max_image_pixels`, `max_command_bytes` to `atomic.Int64` in `internal/server/server.go`; update `New` defaults, the `With*` options, and every read/write; verify `go build ./...` succeeds.
- [x] 1.2 Rework `intCapSetter`/`int64CapSetter`/`setConfigMaxCommandBytes` in `internal/server/config.go` to store atomically; verify `CONFIG GET` still returns each cap and `CONFIG SET` applies it.
- [x] 1.3 Add `TestConfigCapsRace` (`internal/server/hardening_test.go`) issuing concurrent `CONFIG SET` (all six caps) alongside `EMB`, `EMB.MULTI`, and `EMB.IMGMULTI`; run `go test -race -run TestConfigCapsRace ./internal/server/` and confirm zero data races.
- [x] 1.4 Add `TestImageCapsRace` (`serveImage`, `EMB.IMG`/`EMB.IMGMULTI`) so the `max_image_bytes`/`max_image_pixels` readers in `embedImages` are race-covered too.

## 2. Server: CONFIG GET reports live values

- [x] 2.1 Make `cacheConfig` an `atomic.Value`; have `setConfigCache` store the operator's input and the `cache` getter load it; verify in `TestConfigGetCacheEchoesSet` (`internal/server/hardening_test.go`) that `CONFIG SET cache 128mb` then `CONFIG GET cache` returns `128mb`.

## 3. Server: arity-scoped, duplicate-safe script reply cache

- [x] 3.1 Add the text count to `script.CacheKey` (and its private `cacheKey`) in `internal/script/cache.go`; update `internal/server/script.go` lookups/writes; update `internal/script/cache_test.go`, add `TestCacheKeyDistinctByTextCount`, and regenerate the `internal/script/compat_test.go` digest.
- [x] 3.2 Add `TestScriptReplyCacheScopedByArity` (`internal/server/hardening_test.go`) evaluating a `#KEYS`-dependent script with one text, then two, then one, asserting the third equals a cold single-text run.
- [x] 3.3 Bypass the reply cache for an evaluation whose multi-text KEYS repeat a text (`hasDuplicateTexts` in `internal/server/script.go`), so one per-text key can never carry two element replies; add the spec scenario and `TestScriptReplyCacheBypassesDuplicateKeys`.
- [x] 3.4 Correct the reply-cache contract in `examples/scripts/README.md` and `docs/scripting.md` (key includes the KEYS count; per-text independence; repeated KEYS bypass the cache).

## 4. Server: cache stores owned copies

- [x] 4.1 Copy the value in `Cache.Set` (`internal/server/cache.go`); add `TestCacheSetStoresOwnedCopy` and `TestCacheSetRowDoesNotAliasBatchBuffer` in `internal/server/hardening_test.go`.

## 5. Ruby gem: readiness, protocol, null slots, packaging

- [x] 5.1 Make `Emb::Client#ready` total (returns the server's `OK`/error/connection text) and `#ready?` return `ready == 'OK'`; add specs for ready/not-ready/unreachable (`gems/emb/spec/emb/client_ready_spec.rb`).
- [x] 5.2 Validate `protocol` in `Configuration#protocol=` and `Client#merged_redis_options` (allow only `2`); add specs that `Emb.configure { |c| c.protocol = 3 }` and `Emb.new(protocol: 3)` raise `ArgumentError`.
- [x] 5.3 Make the eager binary embed path in `Emb::Proxy#[]` nil-tolerant via `unpack_set`; add `gems/emb/spec/emb/proxy_spec.rb`.
- [x] 5.4 Copy the root `LICENSE` to `gems/emb/LICENSE`; the gemspec lists it.
- [x] 5.5 Document RESP2-only protocol, the connection-error behavior of `ready`/`ready?`, and the cache's per-text-independence assumption where they are user-visible (`gems/emb/README.md`, `docs/scripting.md`, `examples/scripts/README.md`).

## 6. Specs, examples, and release gates

- [x] 6.1 Repair the clobbered main specs `openspec/specs/script-eval/spec.md` and `openspec/specs/lru-cache/spec.md` (delta headers had overwritten them) and replace the `lru-cache` `TBD` Purpose.
- [x] 6.2 Add the `gem-redis-client-config` delta so the option-forwarding requirement no longer promises to forward `protocol: 3`.
- [x] 6.3 Fix `examples/scripts/snippets/qa.lua` to read the context from `ARGV[1]`, matching its documented one-text invocation, and update `internal/script/examples_test.go`.
- [x] 6.4 Add the pre-existing `Bridge.Execute` test seam to `deadcode-allow.txt` so `just deadcode` (a release gate) passes.

## 7. Validation

- [x] 7.1 Run `go build ./... && go vet ./... && golangci-lint run ./...` and confirm clean.
- [x] 7.2 Run `go test ./...`, `go test -race -count=1 ./internal/server/ ./internal/pipeline/ ./internal/script/ ./internal/registry/`, and `just deadcode`; confirm clean.
- [x] 7.3 Run the Ruby suite (`cd gems/emb && bundle exec rake`) against the two-model server and confirm all examples pass.
- [x] 7.4 Run `openspec validate harden-0-4-0-release --strict` and confirm valid.
- [x] 7.5 Run the two-pass advisor council review and apply its findings (spec contradiction, duplicate-KEYS guard, `ready` totality, QA example, race-test widening, doc placement).
