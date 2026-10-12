-- Smoke-gate fixture preset: fixed inputs, no ARGV, deterministic output.
--
-- Lives beside the vendored Laya export on purpose. The presets under
-- `scripts/` and `website/repl/presets/` are stamped into the site by SHA1
-- (website/tools/stamp-presets.py) and are called by clients; this one exists
-- only to drive the autotune contract end to end from `internal/server` tests,
-- so it is deliberately not one of them.
--
-- The graph is the Laya decision export, whose inputs are
--   input_ids      i64 [b, seq]
--   attention_mask i64 [b, seq]
--   marker_pos     i64 [b, markers]
--   marker_mask    b1  [b, markers]
--   qtype          i64 [b]
-- and whose `logits` output is [b, markers]. The inputs are constants, so the
-- same request always produces the same reply: the identity assertion compares
-- these bytes across arms.

local SEQ = 32
local MARKERS = 4

local ids, mask, mpos, mmask = {}, {}, {}, {}
for j = 1, SEQ do
  ids[#ids + 1] = (j % 7) + 2
  mask[#mask + 1] = j <= 16 and 1 or 0
end
for j = 1, MARKERS do
  mpos[#mpos + 1] = 1 + j
  mmask[#mmask + 1] = 1
end

local out = emb.run({
  input_ids = { shape = { 1, SEQ }, data = ids },
  attention_mask = { shape = { 1, SEQ }, data = mask },
  marker_pos = { shape = { 1, MARKERS }, data = mpos },
  marker_mask = { shape = { 1, MARKERS }, data = mmask, dtype = "b1" },
  qtype = { shape = { 1 }, data = { 1 } },
}, { bytes = true, outputs = { "last_hidden_state" } })

-- One deterministic scalar out of the pooled hidden state: the first component
-- of the masked mean over the sequence. (The Laya `logits` output is
-- `[batch, markers]`, which the pooling helper does not accept.)
local pooled = emb.math.mean_pool(out.last_hidden_state.bytes, out.last_hidden_state.shape, mask)
return pooled[1][1]
