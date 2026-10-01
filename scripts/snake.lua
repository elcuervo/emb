-- snake.lua — a decision loop run where the model runs.
--
-- One `EMB.EVSHA` returns a whole episode: the preset owns the board, the
-- safety planner, the three typed questions and the per-tick forward passes,
-- so the browser never pays a round trip per frame. The classic client loop
-- (one call per tick) cannot look realtime behind a rate-limited, serialized
-- bridge; this is the same loop with locality.
--
--   EMB.EVSHA laya <snake.lua sha1> 1 '<request-json>'
--
--   KEYS[1] = {
--     "ticks": 96,                 -- episode length, 1..200 (bounded)
--     "board": {...} | nil,        -- continue this board, or nil for a new game
--     "width": 20, "height": 14, "seed": 7, "initial_length": 6
--   }
--
--   -> { "frames": [ {board, probs, proposed, executed, intervened, risk,
--                     food, confidence, input_tokens}, ... ],
--        "board": {...},           -- the board the next episode resumes from
--        "usage": { "input_tokens": N, "output_tokens": 0 } }
--
-- The checkpoint envelope (max_len, head_max_len, temperature, ...) comes from
-- the model entry's scripts config as emb.script.config; a per-call override is
-- accepted as ARGV[1]. Nothing here is Snake-specific in the host: this is a
-- task preset, and the rules below are a port of the reference implementation's
-- own game loop (laya_coreml/snake/{game,policy}.py).

local DIRS = { "UP", "DOWN", "LEFT", "RIGHT" }
local VEC = { UP = { 0, -1 }, DOWN = { 0, 1 }, LEFT = { -1, 0 }, RIGHT = { 1, 0 } }

local MAX_OPTION_TOKENS = 48
local MAX_TICKS = 200
local DEFAULT_WIDTH = 20
local DEFAULT_HEIGHT = 14
local DEFAULT_SEED = 7
local DEFAULT_LENGTH = 6

-- The board is a client-supplied spec (new game and resumed board alike), so
-- both dimensions are capped before cycle() allocates width x height cells: a
-- 10,000 x 10,000 request would otherwise build 100M Lua tables.
local MAX_SIDE = 64

-- ── the checkpoint envelope ─────────────────────────────────────────────────
local cfg = {
  max_len = 64,
  head_max_len = 32,
  min_seq = 8,
  min_markers = 2,
  temperature = { 1.0, 1.0, 1.0 },
  temperature_by_options = {},
}
local TEMP_MIN, TEMP_MAX = 0.5, 5.0

local function clamp_temperature(v)
  if type(v) ~= "number" or v ~= v or v == math.huge or v == -math.huge then
    return 1.0
  end
  if v < TEMP_MIN then return TEMP_MIN end
  if v > TEMP_MAX then return TEMP_MAX end
  return v
end

local function apply_config(c, target)
  if type(c) ~= "table" then return end
  if type(c.max_len) == "number" and c.max_len >= 1 then target.max_len = c.max_len end
  if type(c.head_max_len) == "number" and c.head_max_len >= 1 then target.head_max_len = c.head_max_len end
  if type(c.min_seq) == "number" and c.min_seq >= 1 then target.min_seq = c.min_seq end
  if type(c.min_markers) == "number" and c.min_markers >= 1 then target.min_markers = c.min_markers end
  if type(c.temperature) == "table" then
    for i = 1, math.min(3, #c.temperature) do target.temperature[i] = clamp_temperature(c.temperature[i]) end
  end
  if type(c.temperature_by_options) == "table" then
    for k, v in pairs(c.temperature_by_options) do target.temperature_by_options[k] = clamp_temperature(v) end
  end
end
apply_config(emb.script and emb.script.config or nil, cfg)
if ARGV[1] then apply_config(json.decode(ARGV[1]), cfg) end

-- ── the board ───────────────────────────────────────────────────────────────
-- Hamiltonian safety cycle, exactly as the reference builds it: a single path
-- visits every cell once, and a move is "safe" when it advances along it
-- without crossing the tail or skipping the food.
local function cycle(width, height)
  if type(width) ~= "number" or type(height) ~= "number"
    or width % 1 ~= 0 or height % 1 ~= 0
    or width > MAX_SIDE or height > MAX_SIDE then
    error("board dimensions must be whole numbers no larger than " .. MAX_SIDE, 0)
  end
  if math.min(width, height) < 4 or (width % 2 == 1 and height % 2 == 1) then
    error("board dimensions must be >= 4 with one even side", 0)
  end
  if height % 2 == 1 then
    local swapped = cycle(height, width)
    local out = {}
    for i, cell in ipairs(swapped) do out[i] = { cell[2], cell[1] } end
    return out
  end
  local path = { { 0, 0 } }
  for y = 0, height - 1 do
    if y % 2 == 0 then
      for x = 1, width - 1 do path[#path + 1] = { x, y } end
    else
      for x = width - 1, 1, -1 do path[#path + 1] = { x, y } end
    end
  end
  for y = height - 1, 1, -1 do path[#path + 1] = { 0, y } end
  return path
end

local key = function(cell) return cell[1] .. "," .. cell[2] end

-- A Lehmer LCG (m = 2^31 - 1). Plain arithmetic only: no bit library, and the
-- product stays exact in a float64. The cursor travels in the board, so a
-- chained episode resumes the same food sequence a single long game would have.
local LCG_MOD, LCG_MUL = 2147483647, 48271

-- Forward declaration: new_game seeds the first food, and spawn_food is
-- defined with the rest of the board rules further down.
local spawn_food

local function new_game(width, height, seed, length)
  local cells = cycle(width, height)
  local index = {}
  for i, cell in ipairs(cells) do index[key(cell)] = i end
  local capacity = width * height
  local game = {
    width = width, height = height, seed = seed,
    cycle = cells, index = index, capacity = capacity,
    body = {}, score = 0, ticks = 0, alive = true, won = false,
    cursor = (seed % LCG_MOD) + 1,
  }
  local start = index[key({ math.floor(width / 2), math.floor(height / 2) })]
  for i = 0, length - 1 do
    game.body[#game.body + 1] = cells[((start - i - 1) % capacity) + 1]
  end
  game.food = nil
  game.food = spawn_food(game)
  return game
end

local function load_game(spec)
  if spec.board and spec.board.body then
    local b = spec.board
    local game = {
      width = b.width, height = b.height, seed = b.seed or DEFAULT_SEED,
      body = {}, score = b.score or 0, ticks = b.ticks or 0,
      alive = b.alive ~= false, won = b.won or false,
      cursor = b.cursor or 1,
    }
    for _, cell in ipairs(b.body) do game.body[#game.body + 1] = { cell[1], cell[2] } end
    game.food = b.food and { b.food[1], b.food[2] } or nil
    game.cycle = cycle(game.width, game.height)
    game.index = {}
    for i, cell in ipairs(game.cycle) do game.index[key(cell)] = i end
    game.capacity = game.width * game.height
    return game
  end
  return new_game(spec.width or DEFAULT_WIDTH, spec.height or DEFAULT_HEIGHT,
    spec.seed or DEFAULT_SEED, spec.initial_length or DEFAULT_LENGTH)
end

local function target(game, direction)
  local v = VEC[direction]
  return { game.body[1][1] + v[1], game.body[1][2] + v[2] }
end

local function legal_reason(game, direction)
  local cell = target(game, direction)
  if cell[1] < 0 or cell[1] >= game.width or cell[2] < 0 or cell[2] >= game.height then
    return "wall"
  end
  if key(cell) == key(game.body[2]) then return "reverse" end
  local occupied = {}
  for _, b in ipairs(game.body) do occupied[key(b)] = true end
  if key(cell) ~= key(game.food) then occupied[key(game.body[#game.body])] = nil end
  if occupied[key(cell)] then return "body" end
  return "legal"
end

local function moves(game)
  local head_index = game.index[key(game.body[1])]
  local tail_distance = (game.index[key(game.body[#game.body])] - head_index + game.capacity) % game.capacity
  local food_distance = (game.index[key(game.food)] - head_index + game.capacity) % game.capacity
  local out = {}
  for _, dir in ipairs(DIRS) do
    local reason = legal_reason(game, dir)
    local legal = reason == "legal"
    local cell = target(game, dir)
    local advance = (((game.index[key(cell)] or head_index) - head_index) + game.capacity) % game.capacity
    local eats = key(cell) == key(game.food)
    local safe = legal
    if safe and (advance > tail_distance or (advance == tail_distance and eats)) then safe = false end
    if safe and (advance == 0 or advance > food_distance) then safe = false end
    out[#out + 1] = { direction = dir, legal = legal, safe = safe, advance = advance, eats = eats }
  end
  return out
end

local function food_reachable(game)
  local blocked = {}
  for i = 2, #game.body do blocked[key(game.body[i])] = true end
  local seen = { [key(game.body[1])] = true }
  local queue, head, tail = { game.body[1] }, 1, 1
  while head <= tail do
    local cell = queue[head]
    head = head + 1
    for _, v in pairs(VEC) do
      local n = { cell[1] + v[1], cell[2] + v[2] }
      if n[1] >= 0 and n[1] < game.width and n[2] >= 0 and n[2] < game.height
        and not blocked[key(n)] and not seen[key(n)] then
        seen[key(n)] = true
        tail = tail + 1
        queue[tail] = n
      end
    end
  end
  return seen[key(game.food)] == true
end

local function spawn_food_impl(game)
  local occupied = {}
  for _, b in ipairs(game.body) do occupied[key(b)] = true end
  local empty = {}
  for _, cell in ipairs(game.cycle) do
    if not occupied[key(cell)] then empty[#empty + 1] = cell end
  end
  if #empty == 0 then return nil end
  game.cursor = (game.cursor * LCG_MUL) % LCG_MOD
  return empty[(game.cursor % #empty) + 1]
end
spawn_food = spawn_food_impl

local function step(game, direction)
  if not game.alive or game.won then return end
  game.ticks = game.ticks + 1
  if legal_reason(game, direction) ~= "legal" then
    game.alive = false
    return
  end
  local cell = target(game, direction)
  table.insert(game.body, 1, cell)
  if key(cell) == key(game.food) then
    game.score = game.score + 1
    if #game.body == game.capacity then
      game.won = true
      game.food = nil
    else
      game.food = spawn_food(game)
    end
    return
  end
  table.remove(game.body)
end

-- ── the typed questions ─────────────────────────────────────────────────────
local function replace_plain(s, find, repl)
  if not find or find == "" then return s end
  local out, i = {}, 1
  while true do
    local a, b = string.find(s, find, i, true)
    if not a then out[#out + 1] = string.sub(s, i) break end
    out[#out + 1] = string.sub(s, i, a - 1)
    out[#out + 1] = repl
    i = b + 1
  end
  return table.concat(out)
end

local function render_options(q)
  if q.type == "choice" then
    local out = {}
    for label, desc in pairs(q.criteria) do out[#out + 1] = { label, desc } end
    table.sort(out, function(a, b) return q.order[a[1]] < q.order[b[1]] end)
    local rendered = {}
    for i, pair in ipairs(out) do rendered[i] = pair[1] .. ": " .. pair[2] end
    return rendered
  end
  if q.type == "noul" then
    return { "false: no, the statement does not hold", "true: yes, the statement holds" }
  end
  local rendered = {}
  for i, level in ipairs(q.criteria) do rendered[i] = "level " .. (i - 1) .. ": " .. level end
  return rendered
end

local function slice(a, from, to)
  local out, n = {}, 0
  for i = from, math.min(to, #a) do n = n + 1; out[n] = a[i] end
  return out
end

local function build_row(specials, state, q)
  local mask_id, cls_id, sep_id = specials.mask, specials.cls, specials.sep
  local mask_token = specials.mask_token
  local function enc(text) return emb.tokenize.encode_plain(text, 0).ids end

  local options = render_options(q)
  local instructions = replace_plain(q.instructions, mask_token, " ")
  local head = enc(q.type .. " question: " .. instructions)

  local rendered = {}
  for i, option in ipairs(options) do
    local piece = { mask_id }
    local ids = slice(enc(" " .. replace_plain(option, mask_token, " ")), 1, MAX_OPTION_TOKENS)
    for _, id in ipairs(ids) do piece[#piece + 1] = id end
    rendered[i] = piece
  end
  local function total_len(rows)
    local n = 0
    for _, r in ipairs(rows) do n = n + #r end
    return n
  end
  if cfg.head_max_len - total_len(rendered) < 16 then
    local per = math.max(4, math.floor((cfg.head_max_len - 16) / math.max(1, #rendered)))
    for i, r in ipairs(rendered) do rendered[i] = slice(r, 1, per) end
  end
  local head_len = math.max(8, cfg.head_max_len - total_len(rendered))
  head = slice(head, 1, head_len)

  local ids = { cls_id }
  for _, id in ipairs(head) do ids[#ids + 1] = id end
  ids[#ids + 1] = sep_id
  local markers = {}
  for _, piece in ipairs(rendered) do
    markers[#markers + 1] = #ids
    for _, id in ipairs(piece) do ids[#ids + 1] = id end
  end
  ids[#ids + 1] = sep_id
  local room = math.max(0, cfg.max_len - #ids - 1)
  for _, id in ipairs(slice(enc(replace_plain(state, mask_token, " ")), 1, room)) do ids[#ids + 1] = id end
  ids[#ids + 1] = sep_id
  return slice(ids, 1, cfg.max_len), markers
end

local QTYPES = { choice = 0, noul = 2 }

local function collate(rows, specials)
  local width, marker_count = cfg.min_seq, cfg.min_markers
  for _, row in ipairs(rows) do
    if #row.ids > width then width = #row.ids end
    if #row.markers > marker_count then marker_count = #row.markers end
  end
  local input_ids, attention, marker_pos, marker_mask, qtype = {}, {}, {}, {}, {}
  for _, row in ipairs(rows) do
    for j = 1, width do
      input_ids[#input_ids + 1] = j <= #row.ids and row.ids[j] or specials.pad
      attention[#attention + 1] = j <= #row.ids and 1 or 0
    end
    for j = 1, marker_count do
      marker_pos[#marker_pos + 1] = (j <= #row.markers) and row.markers[j] or 0
      marker_mask[#marker_mask + 1] = (j <= #row.markers) and 1 or 0
    end
    qtype[#qtype + 1] = row.qtype
  end
  return {
    input_ids = { shape = { #rows, width }, data = input_ids },
    attention_mask = { shape = { #rows, width }, data = attention },
    marker_pos = { shape = { #rows, marker_count }, data = marker_pos },
    marker_mask = { shape = { #rows, marker_count }, data = marker_mask, dtype = "b1" },
    qtype = { shape = { #rows }, data = qtype },
  }, marker_count
end

local function temp_bucket(typ, k)
  local size = "11+"
  if k <= 2 then size = "2" elseif k <= 5 then size = "3-5" elseif k <= 10 then size = "6-10" end
  return typ .. ":" .. size
end

local function confidence(probs)
  local k = #probs
  if k < 2 then return 1.0 end
  local entropy = 0
  for _, p in ipairs(probs) do
    if p < 1e-12 then p = 1e-12 end
    entropy = entropy - p * math.log(p)
  end
  local c = 1.0 - entropy / math.log(k)
  if c < 0 then return 0 end
  if c > 1 then return 1 end
  return c
end

-- ── one tick ────────────────────────────────────────────────────────────────
-- Builds the three typed questions for the current board, runs one forward
-- pass, applies the shield, and advances the game. Returns the frame.
local function decide(specials, game)
  local ms = moves(game)
  local safe, preferred, preferred_advance = {}, nil, -1
  for _, m in ipairs(ms) do
    if m.safe then
      safe[#safe + 1] = m.direction
      if m.advance > preferred_advance then
        preferred, preferred_advance = m.direction, m.advance
      end
    end
  end
  if not preferred then preferred = "NONE" end
  local reachable = food_reachable(game)
  local state = "Safe route: " .. (#safe > 0 and "yes" or "no")
    .. ". Food reachable through empty cells: " .. (reachable and "yes" or "no") .. "."

  local criteria, order = {}, {}
  for i, m in ipairs(ms) do
    local desc
    if not m.legal then desc = "Blocked. Collision."
    elseif not m.safe then desc = "Unsafe. Traps the snake."
    elseif m.eats then desc = "Safe. Eat food now. Best."
    elseif m.direction == preferred then desc = "Safe. Best route to food."
    else desc = "Safe. Slower route." end
    criteria[m.direction] = desc
    order[m.direction] = i
  end

  local questions = {
    { id = "move", type = "choice", instructions = "Choose the best safe move toward food.", criteria = criteria, order = order },
    { id = "risk", type = "noul", instructions = "Is a safe route available?", criteria = {}, order = {} },
    { id = "food", type = "noul", instructions = "Is food reachable through empty cells?", criteria = {}, order = {} },
  }

  local rows, tokens = {}, 0
  for _, q in ipairs(questions) do
    local ids, markers = build_row(specials, state, q)
    tokens = tokens + #ids
    rows[#rows + 1] = { ids = ids, markers = markers, qtype = QTYPES[q.type], q = q }
  end
  local batch, marker_count = collate(rows, specials)
  local out = emb.run(batch, { outputs = { "logits", "act_logits" } })
  local logits = out.logits.data

  local probs_by_dir, frame = {}, {}
  for i, row in ipairs(rows) do
    local k = #row.markers
    local scale = cfg.temperature_by_options[temp_bucket(row.q.type, k)] or cfg.temperature[row.qtype + 1] or 1.0
    local live = {}
    for j = 1, k do live[j] = logits[(i - 1) * marker_count + j] end
    local scaled = {}
    for j = 1, k do scaled[j] = live[j] / scale end
    local probs = emb.math.softmax(scaled)
    if row.q.id == "move" then
      for j, m in ipairs(ms) do probs_by_dir[m.direction] = probs[j] end
      frame.probs = probs_by_dir
      frame.confidence = confidence(probs)
    elseif row.q.id == "risk" then
      frame.risk = probs[2]
    else
      frame.food = probs[2]
    end
  end

  local proposed = DIRS[1]
  for _, dir in ipairs(DIRS) do
    if probs_by_dir[dir] > probs_by_dir[proposed] then proposed = dir end
  end
  local executed = proposed
  if #safe > 0 and not (function()
    for _, d in ipairs(safe) do if d == proposed then return true end end
    return false
  end)() then
    executed = safe[1]
    for _, dir in ipairs(safe) do
      if probs_by_dir[dir] > probs_by_dir[executed] then executed = dir end
    end
  end

  frame.proposed = proposed
  frame.executed = executed
  frame.intervened = proposed ~= executed
  frame.safe = #safe
  frame.input_tokens = tokens
  return frame
end

-- ── the episode ─────────────────────────────────────────────────────────────
local spec = json.decode(KEYS[1])
if type(spec) ~= "table" then error("snake: KEYS[1] must be a request object", 0) end
local ticks = math.floor(tonumber(spec.ticks) or 96)
if ticks < 0 then ticks = 0 end
if ticks > MAX_TICKS then ticks = MAX_TICKS end

local specials = emb.tokenize.special_ids()
local game = load_game(spec)

local function board_of(g)
  local body = {}
  for i, cell in ipairs(g.body) do body[i] = { cell[1], cell[2] } end
  return {
    width = g.width, height = g.height, seed = g.seed,
    body = body, food = g.food and { g.food[1], g.food[2] } or nil,
    score = g.score, ticks = g.ticks, alive = g.alive, won = g.won,
    cursor = g.cursor,
  }
end

local frames, total_tokens = {}, 0
for _ = 1, ticks do
  if not game.alive or game.won then break end
  local frame = decide(specials, game)
  -- The frame shows the board the decision was made on, with the move it
  -- announced; the next frame's board is that move applied. The reference
  -- composes the same way (board first, announced action second).
  frame.board = board_of(game)
  step(game, frame.executed)
  total_tokens = total_tokens + frame.input_tokens
  frames[#frames + 1] = frame
end

return json.encode({
  frames = frames,
  board = board_of(game),
  usage = { input_tokens = total_tokens, output_tokens = 0 },
})
