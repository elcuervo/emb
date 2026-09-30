-- laya.lua — Laya decision preset (ruby-laya 0.3.7 parity).
--
-- Serves a Laya checkpoint mounted like any scripted model:
--
--   EMB.EVAL <model> <this-script> 1 <state> <questions-json> [<config-json>]
--   EMB.EVSHA <model> <sha> 1 <state> <questions-json> [<config-json>]
--
--   KEYS[1]   the state, serialized the way the Ruby gem feeds it: a String
--             passes through unchanged; anything structured is Python-style
--             JSON ({"a": 1, "b": 2} — spaces after every comma and colon,
--             non-ASCII kept as is), because the checkpoints were trained on
--             exactly those strings.
--   ARGV[1]   questions as a JSON object of id → definition; each definition
--             has type ("choice" | "score" | "noul"), instructions, and
--             criteria following the gem's schema. Criteria label ORDER is
--             significant (marker order maps to label order), so criteria
--             objects must be sent as JSON objects and are read back in
--             document order via json.decode_ordered.
--   ARGV[2]   optional per-call config JSON that overrides the checkpoint
--             envelope declared on the model entry (emb.script.config). The
--             envelope itself — max_len, head_max_len, min_seq, min_markers,
--             temperature, temperature_by_options — normally lives on the
--             model's scripts config, so a client sends only state + questions.
--             Buckets: k <= 2 "2", <= 5 "3-5", <= 10 "6-10", else "11+".
--             Values outside [0.5, 5.0] clamp; non-numeric values answer with
--             1.0 — exactly the gem's rules.
--
-- One forward pass answers every question. The reply is
--
--   json.encode({answers = {<id> = <answer>...},
--                usage = {input_tokens = <n>, output_tokens = 0}})
--
-- with the answer payload shapes of the gem: choice {type, choice,
-- probabilities, confidence, action}, score {type, score, legend,
-- probabilities, confidence, action}, noul {type, noul, confidence, action},
-- values rounded to four places.

local QTYPES = { choice = 0, score = 1, noul = 2 }
local MAX_OPTION_TOKENS = 48
local DEFAULT_MIN_MARKERS = 2
local DEFAULT_MIN_SEQ = 8
local TEMP_MIN = 0.5
local TEMP_MAX = 5.0
local DEFAULT_MAX_LEN = 512
local DEFAULT_HEAD_MAX_LEN = 192

local SQRT_PATTERN_MAGIC = "()[]^$*.%+-?"

-- Replace every literal occurrence of find in s with repl (gsub treats its
-- pattern argument as a Lua pattern; escape it so a mask token or state text
-- cannot be read as one).
local function replace_all(s, find, repl)
  if find == nil or find == "" then
    return s
  end
  local escaped = find:gsub("([%^$()%%.[]*+%-?])", "%%%1")
  return (s:gsub(escaped, repl))
end

-- round4 mirrors Ruby Float#round(4) (half away from zero; values here are
-- non-negative, so floor(x*1e4 + 0.5)/1e4 is exact).
local function round4(x)
  return math.floor(x * 10000 + 0.5) / 10000
end

local function clamp01(x)
  if x < 0 then return 0 end
  if x > 1 then return 1 end
  return x
end

-- Python-style JSON for render_criterion / non-string instructions: a Space
-- after every comma and colon, like json.dumps(obj, ensure_ascii=False). An
-- ordered object (json.decode_ordered) is a table of {k, v} pairs carrying
-- .obj = true; a plain integer-keyed table is an array. Strings escape the
-- control characters Python escapes; everything else passes through.
local function pyjson(v, seen)
  local t = type(v)
  if v == json.null then
    return "null"
  end
  if t == "string" then
    local out = v:gsub("\\", "\\\\"):gsub('"', '\\"'):gsub("\n", "\\n")
      :gsub("\r", "\\r"):gsub("\t", "\\t"):gsub("\b", "\\b"):gsub("\f", "\\f")
    return '"' .. out .. '"'
  end
  if t == "number" then
    if v == math.floor(v) and math.abs(v) < 1e15 then
      return string.format("%d", v)
    end
    return tostring(v)
  end
  if t == "boolean" then
    return v and "true" or "false"
  end
  if t == "table" then
    if seen and seen[v] then
      error("pyjson: cyclic structure")
    end
    seen = seen or {}
    seen[v] = true
    if v.obj then
      local parts = {}
      for i = 1, v.n do
        local pair = v[i]
        parts[#parts + 1] = pyjson(pair.k, seen) .. ": " .. pyjson(pair.v, seen)
      end
      seen[v] = nil
      return "{" .. table.concat(parts, ", ") .. "}"
    end
    local parts = {}
    for i = 1, #v do
      parts[#parts + 1] = pyjson(v[i], seen)
    end
    seen[v] = nil
    return "[" .. table.concat(parts, ", ") .. "]"
  end
  return v and tostring(v) or "null"
end

local function blank(v)
  return v == nil or v == "" or v == json.null
end

-- render_criterion: strings pass through unquoted; anything structured becomes
-- Python-style JSON (the model was trained on those exact strings).
local function criterion_text(v)
  if type(v) == "string" then
    return v
  end
  return pyjson(v)
end

-- The option texts in label order (the strings the model was trained on).
local function render_options(q)
  local typ = q.type
  local criteria = q.criteria
  if typ == "choice" then
    local out = {}
    if criteria.obj then
      for i = 1, criteria.n do
        local label, desc = criteria[i].k, criteria[i].v
        if blank(desc) then
          out[#out + 1] = tostring(label)
        else
          out[#out + 1] = tostring(label) .. ": " .. criterion_text(desc)
        end
      end
    else
      for i = 1, #criteria do
        out[#out + 1] = tostring(criteria[i])
      end
    end
    return out
  end
  if typ == "score" then
    local out = {}
    for i = 1, #criteria do
      out[#out + 1] = "level " .. (i - 1) .. ": " .. criterion_text(criteria[i])
    end
    return out
  end
  -- noul: always [false, true]; optional true/false descriptions.
  local false_text, true_text
  if criteria and criteria.obj then
    for i = 1, criteria.n do
      local key = tostring(criteria[i].k):lower()
      local value = criteria[i].v
      if key == "false" then
        false_text = value
      elseif key == "true" then
        true_text = value
      end
    end
  end
  return {
    "false: " .. (blank(false_text) and "no, the statement does not hold"
      or criterion_text(false_text)),
    "true: " .. (blank(true_text) and "yes, the statement holds"
      or criterion_text(true_text)),
  }
end

-- Too many options for the budget: every one is cut to an equal share.
local function trim_options(rendered, head_max_len)
  local n = #rendered
  local per = math.max(4, math.floor((head_max_len - 16) / math.max(1, n)))
  local out = {}
  for i = 1, n do
    local piece = {}
    for j = 1, math.min(per, #rendered[i]) do
      piece[#piece + 1] = rendered[i][j]
    end
    out[i] = piece
  end
  return out
end

local function slice(a, from, to)
  local out = {}
  local n = 0
  for i = from, math.min(to, #a) do
    n = n + 1
    out[n] = a[i]
  end
  return out
end

local function concat_into(dst, src)
  for i = 1, #src do
    dst[#dst + 1] = src[i]
  end
  return dst
end

local function sum_len(rows)
  local s = 0
  for i = 1, #rows do
    s = s + #rows[i]
  end
  return s
end

-- Build one question's token sequence:
--   [CLS] <type> question: <instructions> [SEP] [MASK] opt0 [MASK] opt1 …
--   [SEP] <state> [SEP]
-- Returns ids (capped at max_len), marker positions, and the pre-pad length
-- (the input_tokens accounting the gem reports).
local function build_sequence(specials, state, q, cfg)
  local mask_id, cls_id, sep_id = specials.mask, specials.cls, specials.sep
  local mask_token = specials.mask_token

  local function enc(text)
    return emb.tokenize.encode_plain(text, 0).ids
  end

  local options = render_options(q)
  local instructions = replace_all(tostring(q.instructions or ""), mask_token, " ")

  local head = enc(q.type .. " question: " .. instructions)
  local rendered = {}
  for i = 1, #options do
    local piece = { mask_id }
    concat_into(piece, slice(enc(" " .. replace_all(options[i], mask_token, " ")),
      1, MAX_OPTION_TOKENS))
    rendered[i] = piece
  end
  if cfg.head_max_len - sum_len(rendered) < 16 then
    rendered = trim_options(rendered, cfg.head_max_len)
  end
  local head_len = math.max(8, cfg.head_max_len - sum_len(rendered))
  if #head > head_len then
    head = slice(head, 1, head_len)
  end

  local ids = { cls_id }
  concat_into(ids, head)
  ids[#ids + 1] = sep_id
  local markers = {}
  for i = 1, #rendered do
    markers[#markers + 1] = #ids
    concat_into(ids, rendered[i])
  end
  ids[#ids + 1] = sep_id

  local room = math.max(0, cfg.max_len - #ids - 1)
  local state_ids = enc(replace_all(state, mask_token, " "))
  concat_into(ids, slice(state_ids, 1, room))
  ids[#ids + 1] = sep_id

  local cap = math.min(#ids, cfg.max_len)
  ids = slice(ids, 1, cap)
  local live_markers = {}
  for i = 1, #markers do
    if markers[i] < cfg.max_len then
      live_markers[#live_markers + 1] = markers[i]
    end
  end
  return ids, live_markers
end

local function softmax(logits, temperature)
  local scaled = {}
  for i = 1, #logits do
    scaled[i] = logits[i] / temperature
  end
  local m = scaled[1]
  for i = 2, #scaled do
    if scaled[i] > m then
      m = scaled[i]
    end
  end
  local total = 0
  for i = 1, #scaled do
    scaled[i] = math.exp(scaled[i] - m)
    total = total + scaled[i]
  end
  for i = 1, #scaled do
    scaled[i] = scaled[i] / total
  end
  return scaled
end

-- Confidence as normalized Shannon entropy: 1 - H(p) / log(k); 1.0 when k < 2.
local function confidence_from_probs(probs, k)
  if k < 2 then
    return 1.0
  end
  local entropy = 0
  for i = 1, k do
    local p = probs[i]
    if p < 1e-12 then
      p = 1e-12
    end
    entropy = entropy - p * math.log(p)
  end
  return clamp01(1.0 - entropy / math.log(k))
end

-- The calibration bucket for a question type and option count.
local function temp_bucket(typ, k)
  local size
  if k <= 2 then
    size = "2"
  elseif k <= 5 then
    size = "3-5"
  elseif k <= 10 then
    size = "6-10"
  else
    size = "11+"
  end
  return typ .. ":" .. size
end

-- The usable temperature: clamped to [0.5, 5.0]; anything non-numeric is 1.0.
local function clamp_temperature(value)
  if type(value) ~= "number" then
    return 1.0
  end
  if value ~= value or value == math.huge or value == -math.huge then
    return 1.0
  end
  if value < TEMP_MIN then
    return TEMP_MIN
  end
  if value > TEMP_MAX then
    return TEMP_MAX
  end
  return value
end

-- First-index argmax: Ruby's each_with_index.max_by { |p, i| [p, -i] }.
local function argmax_first(probs)
  local best, bestp = 1, probs[1]
  for i = 2, #probs do
    if probs[i] > bestp then
      best, bestp = i, probs[i]
    end
  end
  return best
end

local function validate(questions)
  for id, def in pairs(questions) do
    if type(def) ~= "table" then
      error("question '" .. tostring(id) .. "': definition must be a JSON object", 0)
    end
    local typ = def.type
    if typ ~= "choice" and typ ~= "score" and typ ~= "noul" then
      error("question '" .. tostring(id) .. "': unknown type "
        .. tostring(typ) .. "; use choice, score, or noul", 0)
    end
    if def.instructions == nil then
      error("question '" .. tostring(id) .. "': no 'instructions'", 0)
    end
    local criteria = def.criteria
    if typ == "choice" then
      local ok = criteria ~= nil and (criteria.obj and criteria.n > 0 or (#criteria > 0))
      if not ok then
        error("question '" .. tostring(id) .. "': a choice question needs at least one criterion", 0)
      end
    elseif typ == "score" then
      if criteria == nil or #criteria == 0 then
        error("question '" .. tostring(id) .. "': a score question needs at least one level", 0)
      end
    else
      if criteria ~= nil and not (criteria.obj) then
        error("question '" .. tostring(id) .. "': a noul question takes optional true/false descriptions", 0)
      end
    end
  end
end

local function choice_answer(q, probs, confidence, act_probability)
  local labels = {}
  if q.criteria.obj then
    for i = 1, q.criteria.n do
      labels[i] = q.criteria[i].k
    end
  else
    for i = 1, #q.criteria do
      labels[i] = q.criteria[i]
    end
  end
  local best = argmax_first(probs)
  local probabilities = {}
  for i = 1, #labels do
    probabilities[tostring(labels[i])] = round4(probs[i])
  end
  return {
    type = "choice",
    choice = tostring(labels[best]),
    probabilities = probabilities,
    confidence = confidence,
    action = { act_probability = act_probability },
  }
end

local function score_answer(q, probs, confidence, act_probability)
  local total, legend, probabilities = 0, {}, {}
  for i = 1, #q.criteria do
    total = total + (i - 1) * probs[i]
    legend[tostring(i - 1)] = q.criteria[i]
    probabilities[tostring(i - 1)] = round4(probs[i])
  end
  return {
    type = "score",
    score = round4(total),
    legend = legend,
    probabilities = probabilities,
    confidence = confidence,
    action = { act_probability = act_probability },
  }
end

local function noul_answer(q, probs, action_probability)
  local p = probs[2]
  return {
    type = "noul",
    noul = round4(p),
    confidence = round4(math.max(p, 1 - p)),
    action = { act_probability = action_probability },
  }
end

-- Build the whole batch: one row per question, padded to the batch width
-- (min_seq), markers padded to the batch marker count (min_markers).
local function collate(rows, cfg, pad_id)
  local width = DEFAULT_MIN_SEQ
  local marker_count = DEFAULT_MIN_MARKERS
  for i = 1, #rows do
    if #rows[i].ids > width then
      width = #rows[i].ids
    end
    if #rows[i].markers > marker_count then
      marker_count = #rows[i].markers
    end
  end
  if cfg.min_seq > width then
    width = cfg.min_seq
  end
  if cfg.min_markers > marker_count then
    marker_count = cfg.min_markers
  end

  local input_ids, attention_mask = {}, {}
  local marker_pos, marker_mask, qtype = {}, {}, {}
  for i = 1, #rows do
    local row = rows[i]
    local ids = row.ids
    for j = 1, width do
      input_ids[#input_ids + 1] = j <= #ids and ids[j] or pad_id
      attention_mask[#attention_mask + 1] = j <= #ids and 1 or 0
    end
    for j = 1, marker_count do
      marker_pos[#marker_pos + 1] = j <= #row.markers and row.markers[j] or 0
      marker_mask[#marker_mask + 1] = j <= #row.markers and 1 or 0
    end
    qtype[#qtype + 1] = row.qtype
  end

  return {
    input_ids = { shape = { #rows, width }, data = input_ids },
    attention_mask = { shape = { #rows, width }, data = attention_mask },
    marker_pos = { shape = { #rows, marker_count }, data = marker_pos },
    marker_mask = { shape = { #rows, marker_count }, data = marker_mask, dtype = "b1" },
    qtype = { shape = { #rows }, data = qtype },
  }
end

-- entry point
if #ARGV < 1 then
  return json.encode({ err = "laya: questions JSON missing (ARGV[1])" })
end

local questions_ordered = json.decode_ordered(ARGV[1])
if type(questions_ordered) ~= "table" or not questions_ordered.obj then
  return json.encode({ err = "laya: questions must be a JSON object" })
end
local questions = {}
for i = 1, questions_ordered.n do
  local def = questions_ordered[i].v
  if type(def) == "table" and def.obj then
    local flat = {}
    for j = 1, def.n do
      flat[def[j].k] = def[j].v
    end
    questions[questions_ordered[i].k] = flat
  else
    questions[questions_ordered[i].k] = def
  end
end
if next(questions) == nil then
  return json.encode({
    answers = {},
    usage = { input_tokens = 0, output_tokens = 0 },
  })
end

local cfg = {
  max_len = DEFAULT_MAX_LEN,
  head_max_len = DEFAULT_HEAD_MAX_LEN,
  min_seq = DEFAULT_MIN_SEQ,
  min_markers = DEFAULT_MIN_MARKERS,
  temperature = { 1.0, 1.0, 1.0 },
  temperature_by_options = {},
}
-- apply_config layers a decoded envelope over the defaults. It is called first
-- with the model entry's own config (emb.script.config, the checkpoint's facts)
-- and then with ARGV[2] if present, so a per-call value overrides the model's.
local function apply_config(c, target)
  if type(c) ~= "table" then
    return
  end
  if type(c.max_len) == "number" and c.max_len >= 1 then target.max_len = c.max_len end
  if type(c.head_max_len) == "number" and c.head_max_len >= 1 then target.head_max_len = c.head_max_len end
  if type(c.min_seq) == "number" and c.min_seq >= 1 then target.min_seq = c.min_seq end
  if type(c.min_markers) == "number" and c.min_markers >= 1 then target.min_markers = c.min_markers end
  if type(c.temperature) == "table" then
    for i = 1, math.min(3, #c.temperature) do
      target.temperature[i] = clamp_temperature(c.temperature[i])
    end
  end
  if type(c.temperature_by_options) == "table" then
    for k, v in pairs(c.temperature_by_options) do
      target.temperature_by_options[k] = clamp_temperature(v)
    end
  end
end
apply_config(emb.script and emb.script.config or nil, cfg)
if ARGV[2] then
  apply_config(json.decode(ARGV[2]), cfg)
end

validate(questions)

local answer_ids = {}
for id in pairs(questions) do
  answer_ids[#answer_ids + 1] = id
end
if #answer_ids == 0 then
  return json.encode({
    answers = {},
    usage = { input_tokens = 0, output_tokens = 0 },
  })
end

local specials = emb.tokenize.special_ids()
local state = KEYS[1] and tostring(KEYS[1]) or ""

-- Build every sequence up front so validation and tokenization finish before
-- any inference (there is no inference before this point).
local rows = {}
local by_id = {}
for i = 1, #answer_ids do
  local id = answer_ids[i]
  local def = questions[id]
  local typ = def.type
  local q = { type = typ, instructions = def.instructions, criteria = def.criteria }
  if type(q.instructions) ~= "string" then
    q.instructions = pyjson(q.instructions)
  end
  local ids, markers = build_sequence(specials, state, q, cfg)
  if #markers ~= #render_options(q) then
    error("question '" .. tostring(id) .. "' has options that do not fit "
      .. "head_max_len=" .. cfg.head_max_len .. "; shortlist them or raise the budget", 0)
  end
  local row = {
    ids = ids,
    markers = markers,
    qtype = QTYPES[typ],
    id = id,
    q = q,
  }
  rows[#rows + 1] = row
  by_id[id] = row
end

local batch = collate(rows, cfg, specials.pad)
local tokens = 0
for i = 1, #rows do
  tokens = tokens + #rows[i].ids
end

local out = emb.run(batch, { outputs = { "logits", "act_logits" } })
local logits, act_logits = out.logits.data, out.act_logits.data

local answers = {}
local marker_count = batch.marker_mask.shape[2]
for i = 1, #rows do
  local row = rows[i]
  local k = #row.markers
  local scale = cfg.temperature_by_options[temp_bucket(row.q.type, k)]
    or cfg.temperature[row.qtype + 1] or 1.0
  -- softmax over only the live options, exactly as the gem's
  -- `logits[row].first(options)` does; padded markers are never scored.
  local live = {}
  for j = 1, k do
    live[j] = logits[(i - 1) * marker_count + j]
  end
  local probs = softmax(live, scale)
  local confidence = round4(confidence_from_probs(probs, k))
  local act = softmax({ act_logits[(i - 1) * 2 + 1], act_logits[(i - 1) * 2 + 2] }, 1.0)
  local act_probability = round4(act[1])
  if row.q.type == "choice" then
    answers[row.id] = choice_answer(row.q, probs, confidence, act_probability)
  elseif row.q.type == "score" then
    answers[row.id] = score_answer(row.q, probs, confidence, act_probability)
  else
    answers[row.id] = noul_answer(row.q, probs, act_probability)
  end
end

return json.encode({
  answers = answers,
  usage = { input_tokens = tokens, output_tokens = 0 },
})