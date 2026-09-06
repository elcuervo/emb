-- qa.lua: extractive QA example (Xenova/distilbert-base-cased-distilled-squad:
-- input_ids + attention_mask -> start_logits, end_logits [-1, -1]).
-- Demonstrates: pair encoding with per-part offsets + sep position, two
-- named outputs, constrained argmax, answer slicing without a decode block.
--
--   EMB.EVSHA qa <sha> 1 "when was the Mac launched" "Apple launched the Mac in 1976."
--   -> hash {answer = "1976", start = 21, stop = 40, score = ...}

local enc = emb.tokenize.encode_pair(KEYS[1], KEYS[2], 384)

local out = emb.run({
  input_ids      = { shape = {1, #enc.ids}, data = enc.ids },
  attention_mask = { shape = {1, #enc.ids}, data = enc.mask },
})

-- Both outputs come back from the single run.
local s, e = out.start_logits.data, out.end_logits.data

-- Model logic: the answer lives in the second part, so the span crosses the
-- inter-part [SEP] and stays within the sequence (max span width 30).
local bs, be, best = enc.sep, enc.sep, -1e9
for i = enc.sep, #s do
  local stop_i = math.min(#s, i + 30)
  for j = i, stop_i do
    local sc = s[i] + e[j]
    if sc > best then
      bs, be, best = i, j, sc
    end
  end
end

-- Blocks: offsets slice the ORIGINAL context; end offsets are 0-based byte
-- spans and Lua string.sub is 1-based, so sub(start+1, stop).
local a, b = enc.offsets[bs][1], enc.offsets[be][2]
local answer = ""
if b > 0 then
  answer = string.sub(KEYS[2], a + 1, b):gsub("^%s+", ""):gsub("%s+$", "")
end

return { answer = answer, start = a, stop = b, score = best }