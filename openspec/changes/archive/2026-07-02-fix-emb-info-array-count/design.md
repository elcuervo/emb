## Context

`handleINFO` in `internal/server/server.go` writes `conn.WriteArray(28)` unconditionally, but the actual number of RESP array elements varies:
- **Without cache**: 22 elements (11 key-value pairs)
- **With cache**: 36 elements (11 non-cache + 7 cache pairs)

The Redis client reads exactly 28 elements as declared, then hangs waiting for 6 phantom elements (when no cache) or 8 extra elements bleed into the next command (when with cache). In CI (no cache), this produces a `ReadTimeoutError` after 1 second.

## Goals / Non-Goals

**Goals:**
- `EMB.INFO` returns the correct array size so the client parses the response without timing out
- Both cached and non-cached server configurations work correctly
- Add a Go-level test that parses the `EMB.INFO` response to prevent regression

**Non-Goals:**
- Refactoring the INFO handler to use a dynamic slice approach
- Adding cache to CI test config

## Decisions

**Compute array count from actual elements written.** The simplest correct fix matches the existing pattern: write the correct count upfront. Two approaches considered:

1. **Conditional branches** — `if s.cache != nil { WriteArray(36); ... } else { WriteArray(22); ... }`
2. **Accumulate in a slice then flush** — Build array of responses, then write size + flush

Approach 1 is simpler (no allocation, no structural change) and matches the existing code style. Selected.

Value counts derive from the Go handler — each pair is one `WriteBulkString` + one `WriteInt`/`WriteBulkString`:
- Without cache: 11 pairs × 2 = 22
- With cache: 18 pairs × 2 = 36

## Test Strategy

The existing `TestServerINFO` only checks `resp[0] != '*'` — it reads whatever is in the TCP
buffer and doesn't parse the full RESP structure. This would pass even with a mismatched array
count, which is why the bug went undetected.

Replace with proper RESP parsing:
1. **`parseRESPArray` helper** — reads a RESP response and returns the declared array count and
   raw elements. Can be reused across tests.
2. **`TestServerINFOArrayCount`** — starts a server without cache, sends `EMB.INFO test`,
   parses the response with `parseRESPArray`, asserts declared count == 22 == actual elements.
3. **`TestCacheInfoArrayCount`** — starts a server with cache (`1GB`), repeats the same
   assertion with expected count 36.

This makes the Go tests the primary validation — no dependency on Ruby gem tests to catch the bug.

## Risks / Trade-offs

- **Hardcoded counts drift** if fields are added/removed. Mitigation: the Go tests directly
  assert the element count and count the actual elements, so any mismatch will fail.
