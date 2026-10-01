# Design

## Problem

`RefTokenizer.Encode` and the offsets path truncate with a plain Go-side slice after the tokenizer's post-processor has already appended special tokens:

```go
// today (internal/tokenizer/reference.go)
ids = ids[:realLen]            // real tokens incl. trailing <eos>
if len(ids) > maxLength {
    ids = ids[:maxLength]      // <-- drops <eos>; keeps maxLength content tokens
}
```

The standard truncation semantics — as implemented by the official library's own pipeline, Ruby's `enable_truncation`, and sentence-transformers — account for the post-processor's special tokens: content is truncated to `maxLength - <special count>` and the specials stay in place. For siglip2 the sequence `<content…> <eos>` at 64 slots becomes `content[:63] + <eos>` under the reference, vs `content[:64]` under emb. Because these models pool `Gather(-1)`, the pooled tokens differ → cosine 0.968–0.972, maxAbsDiff 3.3e-2.

## Options considered

| Option | Verdict |
|---|---|
| **A. Constructor-level truncation** (`tokenizers.FromBytesWithTruncation(json, maxLength, Right)`) | Rejected — the shared tokenizer serves per-call `maxLength` (embedding `cfg.MaxLength`, scripts use e.g. 256 in `siglip2.lua`, offsets/pairs per call); baking one length breaks the other callers, and duplicating tokenizers per length breaks the one-per-model invariant. |
| **B. Generic special-token-preserving truncation in Go** | **Chosen** — behavior-described, faithful for single sequences, no binding/C-library changes, per-call `maxLength` preserved. |
| **C. Extend `daulet/tokenizers` binding + libtokenizers with per-encode truncation** | Out of scope — the C library is pinned via nix; a binding change has no parity test seam here and delays the fix. |

## Chosen approach

Encode with `tokenizers.WithReturnSpecialTokensMask()` alongside the existing attention-mask option, then truncate with one shared helper when `realLen > maxLength`:

```
specialMask[i] = 1  → token i is a special token (CLS/BOS/SEP/EOS…) added by the post-processor
totalSpecials      = count of 1s over the real tokens
budgetContent      = maxLength - totalSpecials     (≥ 0; degenerate maxLength < totalSpecials keeps specials only)

iterate real tokens in order:
  keep every special token (in place)
  keep non-special tokens until budgetContent is exhausted
```

For single sequences the result is the standard reference form: `[CLS] content[:maxLength-S] [SEP]` for BERT-style tokenizers (minilm), `content[:maxLength-1] <eos>` for siglip2/e5. The specials mask arrays from the binding are aligned with the padded ids; the existing `trimByMask` compaction keeps order, so specials are read from the same leading window.

### Application points

1. `reference.go Encode` — both `padOutput` branches call one shared `truncatePreservingSpecialTokens(ids, specialMask, maxLength)`.
2. `pairs.go slicesFromEncoding` (single-sequence script offsets) — same helper on the trimmed ids/offsets, cardinally aligned so `truncatePairPart`'s offset slices stay valid. `EncodePairOffsets` keeps its explicit template budgeting (pair structure already special-aware). `EncodePretokenized` (specials disabled) unchanged.

Non-truncated inputs (the overwhelming majority, and all realistic search traffic) take a byte-identical path: the helper is a no-op when `realLen ≤ maxLength`.

## Verification

- Unit tests (no model needed): boundary inputs — `"7"*127` (siglip2-shaped: expect `content[:63] + <eos>`, length 64, tail id `1`), a BERT-shaped tokenizer fixture (CLS/SEP preserved), pair/pad-output regressions, and the existing `reference_test.go` suite green.
- Corpus parity re-run (the 10,808-entry harness from the parity work): expect tokenizer id parity 10,808/10,808 and the `cos < 0.99` count to drop from 127 to 0; realistic classes unchanged (noise-band only).
- `just test` in `nix develop` (tokenizer, pipeline, server packages).

## Risks / non-goals

- Does not change the int8 kernel/ORT-build drift floor (~4.2e-3 maxAbsDiff) — orthogonal, out of scope.
- Does not address the E5/hyperclusters *measurement* gap (model on credential-gated S3) — the truncation fix applies to that path identically; the corpus run for it remains a separate task (see proposal impact).