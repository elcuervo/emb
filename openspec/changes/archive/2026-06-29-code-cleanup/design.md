## Context

The codebase evolved through 5 changes (emb-server → fix-tensor-shapes → quality-tooling → model-autoconfig → model-lifecycle), each adding functionality without revisiting earlier code. Dead struct fields, unused methods, and C-style loops accumulated. Auto-detection of config values made some logging misleading (dim=0 in preload log). The Go version advanced to 1.25 but the code wasn't updated to use new idioms.

## Goals / Non-Goals

**Goals:**
- Remove all dead code identified by static analysis
- Convert all eligible C-style loops to Go 1.22+ `range over int`
- Use built-in `max()`/`min()` where applicable
- Use `clear()` for map cleanup
- Extract duplicate mask initialization in tokenizer
- Fix untracked `conn.Read` errors
- Fix go.mod direct/indirect metadata
- Remove triple-logging of model loading
- Fix the dim=0 log message bug for auto-detected models
- No spec-level behavior changes

**Non-Goals:**
- Architectural refactoring
- Moving files between packages
- Adding new public APIs
- Performance optimization
- Functional changes to embedding pipeline

## Decisions

### Approach: struct fields first, then idioms, then extraction

Order matters to avoid merge conflicts:
1. Dead code removal (struct fields, methods, branches)
2. Go 1.22+ idiom conversion (range over int, max/min, clear)
3. Helper extraction (mask init, float32 decode)
4. Error handling (conn.Read fixes)
5. go.mod metadata fix

### Generics assessment

Reviewed across all files:
- `L2Normalize` currently operates on `[]float32`. The only other float type used is `float64` in the verify tool (which has its own `cosineSimilarity`). No code path needs both `float32` and `float64` in the same function → generics would add complexity without eliminating duplication.
- Pooling, batch padding, tensor operations all use fixed types per ORT's API → no generic benefit.
- Conclusion: **no generics changes** — existing type usage is appropriate.

### No new specs needed

This change only modifies implementation internals. No RESP protocol changes, no config format changes, no requirement-level behavior changes. The specs artifact is skipped.

## Risks / Trade-offs

- [range over int changes are mechanical but widespread] → Risk of off-by-one errors. Each change is a direct `s/for i := 0; i < N; i++/for i := range N/` with identical semantics.
- [Removing `Registry.Get` may affect external consumers] → The method is unused across the entire codebase. `GetOrInit` replaces it.
- [go.mod metadata change] → `go mod tidy` may update other dependencies. Run it and verify no functional change.
