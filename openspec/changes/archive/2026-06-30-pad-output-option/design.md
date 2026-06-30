## Context

The Ruby `Siglip2Text.encode` produces embeddings with padding noise because:
1. Tokenizer truncates+pads to 64 tokens
2. Only `input_ids` is passed to ONNX (no `attention_mask`)
3. ONNX Runtime defaults mask to all-1s → padded zeros are attended to

emb (with the official-tokenizer change) strips padding and passes correct attention mask. The embeddings are correct but incompatible with Ruby's.

## Goals / Non-Goals

**Goals:**
- Add `pad_output` config option so per-model behavior can match Ruby
- Default `pad_output: false` (correct behavior) for new models
- Maintain exact Ruby compatibility when `pad_output: true`

**Non-Goals:**
- Fixing the Ruby implementation (separate concern)
- Changing how `PadEncodings` works (it's correct for both paths)
- Adding runtime detection of "is this a pre-pooled model"

## Decisions

### Approach: control at the tokenizer level

The `pad_output` flag lives on `RefTokenizer` and affects `Encode()` output:

```
normal → Encode: strip padding → [CLS] hello [SEP] (4 tokens)
                                 → PadEncodings → mask 4×1, 60×0 → correct

pad_output=true → Encode: keep padding, pad to maxLength
                         → [CLS] hello [SEP] [PAD]×60 (64 tokens)
                         → PadEncodings → mask 64×1 → matches Ruby
```

This is a 5-line change in `Encode` — either strip or don't, then either pad to maxLength or don't.

### Why not change `PadEncodings`?

`PadEncodings` already does the right thing for both cases:
- With real-length sequences: pads to batch max, sets correct mask
- With fixed-length sequences (pad_output=true): all already same length, mask=all-1s

Changing `PadEncodings` would affect all models, not just the ones that need compatibility.

### Bool, not int

Using `bool` not `int` (`pad_to: 64`) because:
- The value is always `maxLength` — re-stating the number is redundant
- `pad_output: true` pads to the model's `max_length`
- Simpler config surface

## Risks / Trade-offs

- [pad_output=true embedings include padding noise] → Impact is tiny (cos-sim > 0.9999 with correct output). Acceptable for a compatibility bridge.
- [New models default to correct behavior] → `pad_output` defaults to `false`. Only explicitly set for backwards compatibility.
- [Migration path] → When Ruby is fixed, flip `pad_output: false` and re-index.
