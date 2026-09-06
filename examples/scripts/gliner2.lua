-- gliner2.lua: GLiNER2 span extraction (example: cuerbot/gliner2-multi-v1 ONNX
-- export). This is a *demonstration* of the script building blocks, not a
-- maintained server component -- reuse emb.tokenize.words / pretokenized /
-- emb.run in your own scripts for other models.
--
--   EMB.EVSHA gliner2 <sha> 1 <text> PERSON ORG PRODUCT
--   -> hash {PERSON = {"Tim Cook"}, ORG = {"Apple"}, PRODUCT = {"iPhone 15"}}
--
-- KEYS[1] = text, ARGV[1..] = labels. Schema baked into the sequence:
--   ( [P] prompt ( [E] LAB1 [E] LAB2 ) ) [SEP_TEXT] word1 word2 ...

local PREFIX = "[E]"            -- label marker (span extraction)
local SEP_TEXT = "[SEP_TEXT]"
local PROMPT = ""               -- single-word instruction; "" = default
local MAX_LEN = 512             -- export's max_seq_len
local THRESHOLD = 0.5           -- sigmoid cutoff

-- Building block 1: generic word split (BertPreTokenizer rules, byte offsets).
local function split_words(norm)
  local res = emb.tokenize.words(norm)
  -- this model lowercases words before tokenization (decoder.split_words)
  local words = {}
  for i = 1, #res.words do
    words[i] = string.lower(res.words[i])
  end
  return words, res.starts, res.ends
end

local function normalize(text)
  local s = text:gsub("^%s+", ""):gsub("%s+$", "")
  if s == "" then return "." end
  local last = s:sub(-1)
  if last == "." or last == "!" or last == "?" then return s end
  return s .. "."
end

local function sigmoid(x) return 1 / (1 + math.exp(-x)) end

local text = KEYS[1]
local labels = {}
for i = 1, #ARGV do labels[i] = ARGV[i] end
local nlab = #labels
if nlab == 0 then return {} end

local norm = normalize(text)
local words, starts, ends = split_words(norm)

-- Schema (model-specific): 4 header words, [E] + label per label, closing
-- parens, then [SEP_TEXT] and the text words.
local schema = { "(", "[P]", PROMPT, "(" }
for i = 1, nlab do
  schema[#schema + 1] = PREFIX
  schema[#schema + 1] = labels[i]
end
schema[#schema + 1] = ")"
schema[#schema + 1] = ")"

local combined = {}
for i = 1, #schema do combined[#combined + 1] = schema[i] end
combined[#combined + 1] = SEP_TEXT
for i = 1, #words do combined[#combined + 1] = words[i] end

-- Building block 2: word-aligned encoding with per-subword word indices.
local enc = emb.tokenize.pretokenized(combined, MAX_LEN)
local ids, wids = enc.ids, enc.word_ids
local seq = #ids

-- pos2word: first subword of each text word -> local 1-based word index.
-- text_start = 0-based combined index of the first text word.
local text_start = #schema + 1
local pos2word, seen, text_len = {}, {}, 0
for idx = 1, seq do
  local wid = wids[idx]
  if wid >= text_start and not seen[wid] then
    seen[wid] = true
    local w = wid - text_start + 1
    pos2word[idx] = w
    if w > text_len then text_len = w end
  end
end

-- label_positions: token position of each [E] marker (combined idx 4+2(i-1)).
local label_positions = {}
for i = 1, nlab do
  local combined_idx = 4 + (i - 1) * 2
  local found = nil
  for idx = 1, seq do
    if wids[idx] == combined_idx then found = idx break end
  end
  if found == nil then error("could not locate label marker for " .. labels[i]) end
  label_positions[i] = found
end

local words_mask, attn, label_mask = {}, {}, {}
for idx = 1, seq do
  words_mask[idx] = pos2word[idx] and 1 or 0
  attn[idx] = 1
end
for i = 1, nlab do label_mask[i] = 1 end

-- Building block 3: named-tensor inference over the 7 GLiNER inputs.
local out = emb.run({
  input_ids       = { shape = {1, seq}, data = ids },
  attention_mask  = { shape = {1, seq}, data = attn },
  words_mask      = { shape = {1, seq}, data = words_mask },
  text_lengths    = { shape = {1},     data = {text_len} },
  task_type       = { shape = {1},     data = {0} },
  label_positions = { shape = {1, nlab}, data = label_positions },
  label_mask      = { shape = {1, nlab}, data = label_mask },
})

local logits = out.logits.data
local log_shape = out.logits.shape -- {1, seq, max_width, num_labels}
local max_width, num_labels = log_shape[3], log_shape[4]

-- Flat row-major access helper for decoded tensors (kept user-space: trivial).
local function at(data, shape, ...)
  local offset, args = 0, { ... }
  for i = 1, #args do offset = offset * shape[i] + (args[i] - 1) end
  return data[offset + 1]
end

-- Span scan (model-specific decode): word-start positions x width, sigmoid
-- vs THRESHOLD, overlap suppression.
local function find_spans(li)
  local spans = {}
  for pos = 1, seq do
    local sw = pos2word[pos]
    if sw then
      for w = 0, max_width - 1 do
        local end_word = sw - 1 + w
        if end_word >= text_len then break end
        local score = sigmoid(at(logits, log_shape, 1, pos, w + 1, li))
        if score >= THRESHOLD then
          local cs, ce = starts[sw], ends[sw + w]
          local span_text = string.sub(norm, cs, ce - 1):gsub("^%s+", ""):gsub("%s+$", "")
          if span_text ~= "" then
            spans[#spans + 1] = { text = span_text, score = score, s = cs, e = ce }
          end
        end
      end
    end
  end
  return spans
end

local function format_spans(spans)
  if #spans == 0 then return {} end
  table.sort(spans, function(a, b) return a.score > b.score end)
  local selected = {}
  for _, sp in ipairs(spans) do
    local overlaps = false
    for _, sel in ipairs(selected) do
      if not (sp.e <= sel.s or sp.s >= sel.e) then overlaps = true break end
    end
    if not overlaps then selected[#selected + 1] = sp end
  end
  local out = {}
  for i, sp in ipairs(selected) do out[i] = sp.text end
  return out
end

local entities = {}
for li = 1, num_labels do
  entities[labels[li]] = format_spans(find_spans(li))
end
return entities