## Context

The RESP protocol supports pipelining (multiple commands sent without waiting), but `redcon`'s `ServeMux` processes commands sequentially per connection — the handler must return before the next command is read. This means `EMB model-a text` followed by `EMB model-b text` on the same connection executes strictly sequentially, even when the two models have independent pools and ONNX sessions.

The client workaround is to open multiple connections, but this adds complexity for mixed-model workloads where the caller wants a single response that bundles results from different models.

`EMB.MULTI` solves this at the server level: one command, one response array, internal fan-out.

## Goals / Non-Goals

**Goals:**
- Single-command interface for embedding texts with different models
- Concurrent fan-out: each `(model, text)` pair dispatched to its pool independently
- Order preservation: response array matches input pair order
- MGET-style partial failure: failures return nil per-pair, not command-level error
- `EMB.STATS` counts each pair as one request (N pairs = N requests)
- No changes to existing `EMB` semantics

**Non-Goals:**
- Removing the existing `EMB` command
- Batching same-model texts within `EMB.MULTI` (existing batcher handles this naturally via concurrent arrival)
- Cross-connection aggregation
- Response reordering by completion time

## Decisions

### Pairs-only parsing

`EMB.MULTI model1 text1 model2 text2` — each pair is exactly one model name and one text. No variadic `model text1 text2 text3` grouping. This keeps parsing trivial:

```
if len(args) < 3 || len(args[1:])%2 != 0 {
    // error
}
```

and avoids ambiguity with the existing `EMB` format.

### MGET-style nil for failures

```
EMB.MULTI siglip2 "text" nonexistent "fail" e5 "query"
→ *3
  $3072    ← siglip2 embedding
  $-1      ← nil (nonexistent model)
  $3072    ← e5 embedding
```

Reasons:
- Partial failure doesn't lose successful results
- Client can map results back to input pairs by position
- Consistent with Redis MGET convention

### Fan-out concurrency pattern

```
func (s *Server) handleEMBMULTI(conn redcon.Conn, cmd redcon.Command) {
    pairs := cmd.Args[1:]
    if len(pairs) < 2 || len(pairs)%2 != 0 {
        conn.WriteError(...)
        return
    }
    n := len(pairs) / 2
    results := make([][]byte, n)
    var wg sync.WaitGroup
    for i := range n {
        wg.Add(1)
        go func(i int) {
            defer wg.Done()
            model := string(pairs[i*2])
            text := string(pairs[i*2+1])
            entry, err := s.reg.GetOrInit(model)
            if err != nil {
                return // nil result
            }
            resp, err := entry.Pool.Embed([]string{text})
            if err != nil || resp.Err != nil {
                return // nil result
            }
            results[i] = resp.Embeddings[0]
            s.total.Add(1)
        }(i)
    }
    wg.Wait()
    conn.WriteArray(n)
    for _, r := range results {
        if r == nil {
            conn.WriteNull()
        } else {
            conn.WriteBulk(r)
        }
    }
}
```

Key property: `Pool.Embed()` is goroutine-safe (channel-based for both batcher and worker modes), so no locking needed.

### Request counting

Each successful pair increments `s.total` by 1 (inside the goroutine, after success). This matches the semantics of "N pairs = N requests" without needing to estimate counts upfront.

## Risks / Trade-offs

- [Goroutine per pair] — Each pair spawns a goroutine. For large `EMB.MULTI` calls (32+ pairs), this creates a burst. Acceptable — goroutines are cheap, and the fan-out is bounded by the number of pairs in one command.
- [Response ordering vs latency] — All pairs must complete before the response is written, so one slow pair delays the full response. Acceptable — matches user expectation of ordered results, and concurrency means the slow model doesn't serialize the others.
- [Confusion with EMB format] — Users might expect `EMB.MULTI model1 text1 text2 model2 text3`. Documentation in EMB.HELP resolves this.

### E2E verification approach

The existing `just verify-embeddings` recipe downloads one model, starts the server, and checks embeddings against a Python reference. For EMB.MULTI, we extend this pattern:

1. **Download two models** — `just download-model` for each, stored in separate directories (e.g., `./models/minilm` and `./models/e5`)
2. **Generate an e2e config** — a temp YAML config registering both models with their paths
3. **Start the server** — with the e2e config
4. **Run `cmd/emb-multi-verify`** — a new Go tool that:
   - Sends `EMB.MULTI minilm "text" e5 "other text"`
   - Validates the response is an array of 2 bulk strings with the correct dimensions
   - Sends two sequential `EMB` calls with the same texts and compares byte-equality
   - Tests a same-model MULTI (all pairs target minilm) and checks the batcher merged them

This approach reuses the existing model download infrastructure and only adds a small verification tool. No Python reference generation needed for the cross-model test — the comparison is between `EMB.MULTI` output and sequential `EMB` output, which must match byte-for-byte.

#### Model selection

Two models with different dimensions work best for cross-model testing, since the response format differs (e.g., minilm=384, e5=384). Using the same tokenizer type means both models load without issues.

Candidate pair: `Xenova/all-MiniLM-L6-v2` (384d) and `Xenova/multilingual-e5-small` (384d).
