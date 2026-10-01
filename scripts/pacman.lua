-- pacman.lua — a second decision loop run where the model runs.
--
-- The sibling of snake.lua, and deliberately not a port of it: the board is a
-- fixed maze, the pieces are a player and four ghosts, and the rules are
-- pellets, power pellets and collisions instead of a body and a Hamiltonian
-- cycle. What it shares is the contract — one `EMB.EVSHA` returns a whole
-- bounded episode of frames, so the browser never pays a round trip per frame,
-- and the returned game chains into the next call.
--
--   EMB.EVSHA laya <pacman.lua sha1> 1 '<request-json>'
--
--   KEYS[1] = {
--     "ticks": 120,                -- episode length, 0..400 (bounded)
--     "game": {...} | nil,         -- continue this game, or nil for a new one
--     "seed": 7
--   }
--
--   -> { "frames": [ {board, probs, proposed, executed, intervened, danger,
--                     clear, input_tokens}, ... ],
--        "game": {...},           -- the game the next episode resumes from
--        "usage": { "input_tokens": N, "output_tokens": 0 } }
--
-- The model picks a direction among the legal ones; the rules own everything
-- else. The checkpoint the sandbox serves is a random-weight miniature, so a
-- planner vetoes a move that walks into a live ghost — exactly as snake's
-- shield does. The game is the mechanism's demo, not the model's judgment.

local DIRS = { "UP", "DOWN", "LEFT", "RIGHT" }
local VEC = { UP = { 0, -1 }, DOWN = { 0, 1 }, LEFT = { -1, 0 }, RIGHT = { 1, 0 } }
local REVERSE = { UP = "DOWN", DOWN = "UP", LEFT = "RIGHT", RIGHT = "LEFT" }

local MAX_OPTION_TOKENS = 48
local MAX_TICKS = 400
local DEFAULT_SEED = 7
local DEFAULT_TICKS = 120

-- Decisions without a pellet before the planner takes over from the model. The
-- served weights are random, so a model-only player can wander in place; the
-- demo's subject is the loop, so a stalled run is steered rather than left to
-- oscillate.
local STALL_LIMIT = 24

-- ── the maze ────────────────────────────────────────────────────────────────
-- 19 x 13, bordered, symmetric, every open cell reachable and no dead ends (a
-- dead end has one exit; the player would be trapped there). `#` wall, `.`
-- pellet, `o` power pellet. The four corners are the power pellets; the ghosts
-- spawn on the four cells just inside them and the player in the centre.
local MAZE = {
  "###################",
  "#o.......#.......o#",
  "#.##.###...###.##.#",
  "#.#.....#.#.....#.#",
  "#.#.###.#.#.###.#.#",
  "#.....#.....#.....#",
  "###.#.#.###.#.#.###",
  "#.....#.....#.....#",
  "#.#.###.#.#.###.#.#",
  "#.#.....#.#.....#.#",
  "#.##.###...###.##.#",
  "#o.......#.......o#",
  "###################",
}

local PAC_START = { 9, 5 }
local GHOST_HOMES = { { 4, 1 }, { 14, 1 }, { 4, 11 }, { 14, 11 } }
local LIVES = 3
local FRIGHT_TICKS = 24
local DOT_SCORE, POWER_SCORE, GHOST_SCORE = 10, 50, 200

-- The checkpoint envelope, identical to snake.lua's: it rides on the model
-- entry's scripts config as emb.script.config, an optional ARGV[1] overrides.
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

-- ── geometry ────────────────────────────────────────────────────────────────
local W, H = #MAZE[1], #MAZE

local function idx(x, y) return y * W + x end
local function in_bounds(x, y) return x >= 0 and x < W and y >= 0 and y < H end
local function tile_at(x, y)
  if not in_bounds(x, y) then return "#" end
  return string.sub(MAZE[y + 1], x + 1, x + 1)
end

local function new_game(seed)
  local wall, pellet, dots = {}, {}, 0
  for y = 0, H - 1 do
    for x = 0, W - 1 do
      local t = tile_at(x, y)
      local i = idx(x, y)
      if t == "#" then
        wall[i] = true
      elseif t == "." then
        pellet[i] = 1; dots = dots + 1
      elseif t == "o" then
        pellet[i] = 2; dots = dots + 1
      end
    end
  end
  local game = {
    width = W, height = H, seed = seed, cursor = (seed % 2147483647) + 1,
    wall = wall, pellet = pellet, dots = dots,
    lives = LIVES, score = 0, frightened = 0, ticks = 0, stall = 0, reversed = false,
    alive = true, won = false, ghosts = {},
  }
  game.pac = { x = PAC_START[1], y = PAC_START[2], dir = "LEFT" }
  for i, home in ipairs(GHOST_HOMES) do
    game.ghosts[i] = { x = home[1], y = home[2], dir = DIRS[((i - 1) % 4) + 1], hx = home[1], hy = home[2] }
  end
  return game
end

local function load_game(spec)
  local b = spec.game
  if type(b) ~= "table" or type(b.grid) ~= "string" then return new_game(spec.seed or DEFAULT_SEED) end
  local game = new_game(spec.seed or b.seed or DEFAULT_SEED)
  game.pellet, game.dots = {}, 0
  local grid = b.grid
  for y = 0, H - 1 do
    for x = 0, W - 1 do
      local i = idx(x, y)
      local c = string.sub(grid, i + 1, i + 1)
      if c == "1" then game.pellet[i] = 1; game.dots = game.dots + 1
      elseif c == "2" then game.pellet[i] = 2; game.dots = game.dots + 1 end
    end
  end
  game.cursor = b.cursor or game.cursor
  game.lives = b.lives or LIVES
  game.score = b.score or 0
  game.frightened = b.frightened or 0
  game.ticks = b.ticks or 0
  game.stall = b.stall or 0
  game.reversed = b.reversed == true
  game.alive = b.alive ~= false
  game.won = b.won or false
  if type(b.pac) == "table" then
    game.pac = { x = b.pac.x, y = b.pac.y, dir = b.pac.dir or "LEFT" }
  end
  if type(b.ghosts) == "table" then
    for i, gh in ipairs(b.ghosts) do
      local home = GHOST_HOMES[i] or GHOST_HOMES[1]
      game.ghosts[i] = { x = gh.x, y = gh.y, dir = gh.dir or DIRS[1], hx = home[1], hy = home[2] }
    end
  end
  return game
end

local function game_of(g)
  local grid = {}
  for y = 0, H - 1 do
    for x = 0, W - 1 do
      local i = idx(x, y)
      if g.wall[i] then grid[#grid + 1] = "#"
      elseif g.pellet[i] == 1 then grid[#grid + 1] = "1"
      elseif g.pellet[i] == 2 then grid[#grid + 1] = "2"
      else grid[#grid + 1] = "." end
    end
  end
  local ghosts = {}
  for i, gh in ipairs(g.ghosts) do
    ghosts[i] = { x = gh.x, y = gh.y, dir = gh.dir, hx = gh.hx, hy = gh.hy }
  end
  return {
    width = g.width, height = g.height, seed = g.seed, cursor = g.cursor,
    pac = { x = g.pac.x, y = g.pac.y, dir = g.pac.dir },
    ghosts = ghosts,
    grid = table.concat(grid),
    dots = g.dots, lives = g.lives, score = g.score,
    frightened = g.frightened, ticks = g.ticks, stall = g.stall,
    reversed = g.reversed == true,
    alive = g.alive, won = g.won,
  }
end

-- ── the rules ───────────────────────────────────────────────────────────────
local function is_wall(g, x, y)
  if not in_bounds(x, y) then return true end
  return g.wall[idx(x, y)] == true
end

local function legal_dirs(g)
  local out = {}
  for _, d in ipairs(DIRS) do
    if not is_wall(g, g.pac.x + VEC[d][1], g.pac.y + VEC[d][2]) then out[#out + 1] = d end
  end
  return out
end

-- BFS over non-wall tiles from the player: the distance to every reachable
-- cell, the first step of a shortest path to the nearest pellet, and that
-- pellet's distance. Manhattan distance is wrong across walls, so progress is
-- measured on this real path, never on the coordinate delta.
local function pellet_path(g)
  local dist, step = {}, {}
  local start = idx(g.pac.x, g.pac.y)
  dist[start] = 0
  local queue, head, tail = { { g.pac.x, g.pac.y } }, 1, 1
  local preferred, nearest
  while head <= tail do
    local cell = queue[head]; head = head + 1
    local i = idx(cell[1], cell[2])
    if i ~= start and g.pellet[i] and (nearest == nil or dist[i] < nearest) then
      nearest, preferred = dist[i], step[i]
    end
    for _, d in ipairs(DIRS) do
      local nx, ny = cell[1] + VEC[d][1], cell[2] + VEC[d][2]
      local ni = idx(nx, ny)
      if not is_wall(g, nx, ny) and dist[ni] == nil then
        dist[ni] = dist[i] + 1
        step[ni] = (i == start) and d or step[i]
        tail = tail + 1
        queue[tail] = { nx, ny }
      end
    end
  end
  return dist, preferred, nearest
end
local function all_pellets_reachable(g)
  local seen = { [idx(g.pac.x, g.pac.y)] = true }
  local queue, head, tail = { { g.pac.x, g.pac.y } }, 1, 1
  local found = 0
  while head <= tail do
    local cell = queue[head]; head = head + 1
    if g.pellet[idx(cell[1], cell[2])] then found = found + 1 end
    for _, d in ipairs(DIRS) do
      local nx, ny = cell[1] + VEC[d][1], cell[2] + VEC[d][2]
      if not is_wall(g, nx, ny) and not seen[idx(nx, ny)] then
        seen[idx(nx, ny)] = true
        tail = tail + 1
        queue[tail] = { nx, ny }
      end
    end
  end
  return found == g.dots
end

-- A move is unsafe when its destination sits within two tiles of a live ghost.
-- The planner can then veto a model pick that walks into one, which is what
-- keeps a random-weight model's game watchable. Two tiles, not one: the greedy
-- ghosts below are fast, and a one-tile radius leaves no room to turn.
local function live_ghost_near(g, x, y)
  for _, gh in ipairs(g.ghosts) do
    if g.frightened <= 0 then
      local d = math.abs(gh.x - x) + math.abs(gh.y - y)
      if d <= 2 then return true end
    end
  end
  return false
end

local function nearest_ghost_distance(g, x, y)
  local best
  for _, gh in ipairs(g.ghosts) do
    local d = math.abs(gh.x - x) + math.abs(gh.y - y)
    if best == nil or d < best then best = d end
  end
  return best or 999
end

-- A Lehmer LCG (m = 2^31 - 1), the same generator snake.lua uses. Here it seeds
-- the ghosts' wandering, so two episodes from two seeds are two different games.
local LCG_MOD, LCG_MUL = 2147483647, 48271
local function lcg_next(g)
  g.cursor = (g.cursor * LCG_MUL) % LCG_MOD
  return g.cursor
end

local function safe_dirs(g)
  local legal, safe = legal_dirs(g), {}
  for _, d in ipairs(legal) do
    if not live_ghost_near(g, g.pac.x + VEC[d][1], g.pac.y + VEC[d][2]) then safe[#safe + 1] = d end
  end
  if #safe > 0 then return safe end
  -- No clean escape: flee, taking the step that puts the most tiles between the
  -- player and the nearest ghost. Returning `legal` here is how a random-weight
  -- player walks straight into a chaser.
  local best, best_dist
  for _, d in ipairs(legal) do
    local dist = nearest_ghost_distance(g, g.pac.x + VEC[d][1], g.pac.y + VEC[d][2])
    if best == nil or dist > best_dist then best, best_dist = d, dist end
  end
  if best then return { best } end
  return legal
end

-- ponytail: ghosts step greedily toward the player (away when frightened) and
-- reverse only when boxed in — no BFS, no per-ghost scatter/chase timers. Looks
-- deliberate and is cheap; swap in real targeting if the chase ever reads flat.
local function ghost_step(g, gh)
  local options = {}
  for _, d in ipairs(DIRS) do
    if not is_wall(g, gh.x + VEC[d][1], gh.y + VEC[d][2]) then options[#options + 1] = d end
  end
  if #options == 0 then return end
  -- One step in four is a wander: the chase is not a straight line, and the
  -- cursor is seeded, so a restart plays a different game.
  if lcg_next(g) % 4 == 0 then
    local d = options[1 + (lcg_next(g) % #options)]
    gh.dir = d
    gh.x, gh.y = gh.x + VEC[d][1], gh.y + VEC[d][2]
    return
  end
  local forward = {}
  for _, d in ipairs(options) do
    if d ~= REVERSE[gh.dir] then forward[#forward + 1] = d end
  end
  if #forward == 0 then forward = options end
  local best, best_score
  for _, d in ipairs(forward) do
    local nx, ny = gh.x + VEC[d][1], gh.y + VEC[d][2]
    local dist = math.abs(nx - g.pac.x) + math.abs(ny - g.pac.y)
    local score = g.frightened > 0 and dist or -dist
    if best == nil or score > best_score then best, best_score = d, score end
  end
  gh.dir = best
  gh.x, gh.y = gh.x + VEC[best][1], gh.y + VEC[best][2]
end

-- Returns "lose" when a live ghost shares the player's tile, nil otherwise. A
-- frightened ghost is eaten in place and the loop keeps going.
local function resolve_collision(g)
  for _, gh in ipairs(g.ghosts) do
    if gh.x == g.pac.x and gh.y == g.pac.y then
      if g.frightened > 0 then
        g.score = g.score + GHOST_SCORE
        gh.x, gh.y, gh.dir = gh.hx, gh.hy, DIRS[1]
      else
        return "lose"
      end
    end
  end
  return nil
end

local function respawn(g)
  g.lives = g.lives - 1
  g.frightened = 0
  g.stall = 0
  g.pac = { x = PAC_START[1], y = PAC_START[2], dir = "LEFT" }
  for i, home in ipairs(GHOST_HOMES) do
    g.ghosts[i] = { x = home[1], y = home[2], dir = DIRS[((i - 1) % 4) + 1], hx = home[1], hy = home[2] }
  end
  if g.lives <= 0 then g.alive = false end
end

local function step(g, direction)
  if not g.alive or g.won then return end
  g.ticks = g.ticks + 1
  if g.frightened > 0 then g.frightened = g.frightened - 1 end

  local was = g.pac.dir
  local nx, ny = g.pac.x + VEC[direction][1], g.pac.y + VEC[direction][2]
  if is_wall(g, nx, ny) then direction = REVERSE[g.pac.dir] end
  nx, ny = g.pac.x + VEC[direction][1], g.pac.y + VEC[direction][2]
  if not is_wall(g, nx, ny) then
    g.pac.x, g.pac.y, g.pac.dir = nx, ny, direction
    g.reversed = direction == REVERSE[was]
  end

  local pi = idx(g.pac.x, g.pac.y)
  local pellet = g.pellet[pi]
  if pellet == 1 then
    g.score = g.score + DOT_SCORE; g.pellet[pi] = nil; g.dots = g.dots - 1
    g.stall = 0
  elseif pellet == 2 then
    g.score = g.score + POWER_SCORE; g.pellet[pi] = nil; g.dots = g.dots - 1
    g.frightened = FRIGHT_TICKS
    g.stall = 0
  else
    g.stall = g.stall + 1
  end
  if g.dots <= 0 then g.won = true; return end

  if resolve_collision(g) == "lose" then respawn(g); return end

  -- The ghosts move at a third of the player's pace, and stay put for the
  -- opening dozen ticks: without the head start four greedy chasers corner a
  -- random-weight player inside the first seconds, and the demo is over before
  -- the playback window is.
  if g.ticks > 12 and g.ticks % 3 == 0 then
    for _, gh in ipairs(g.ghosts) do ghost_step(g, gh) end
    if resolve_collision(g) == "lose" then respawn(g) end
  end
end

-- ── the typed questions ─────────────────────────────────────────────────────
-- The row builder is snake's, unchanged in shape: the same template, the same
-- marker discipline and the same temperature buckets, because the checkpoint is
-- the same. Only the question text differs.
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

-- ── one tick ────────────────────────────────────────────────────────────────
local function decide(specials, g)
  local legal = legal_dirs(g)
  local safe = safe_dirs(g)
  local safe_map = {}
  for _, d in ipairs(safe) do safe_map[d] = true end
  local dist, preferred, nearest = pellet_path(g)
  local reachable = all_pellets_reachable(g)

  local state = "Pellets left: " .. g.dots
    .. ". Nearest pellet " .. (nearest or 0) .. " tiles away."
    .. " All pellets reachable: " .. (reachable and "yes" or "no") .. "."
    .. " Ghosts frightened: " .. (g.frightened > 0 and "yes" or "no") .. "."

  -- Only the legal directions carry markers, so the softmax is over real
  -- choices and the criteria map is built from `legal` alone.
  local criteria, order = {}, {}
  for i, d in ipairs(legal) do
    local nx, ny = g.pac.x + VEC[d][1], g.pac.y + VEC[d][2]
    local desc
    if not safe_map[d] then desc = "Unsafe. A ghost waits there."
    elseif g.pellet[idx(nx, ny)] == 2 then desc = "Safe. Eat the power pellet."
    elseif g.pellet[idx(nx, ny)] then desc = "Safe. Eat a pellet."
    elseif d == preferred then desc = "Safe. Best route to a pellet."
    else desc = "Safe. Longer route." end
    criteria[d] = desc
    order[d] = i
  end

  local questions = {
    { id = "move", type = "choice", instructions = "Choose the best legal move toward the pellets and away from the ghosts.", criteria = criteria, order = order },
    { id = "danger", type = "noul", instructions = "Is a live ghost within one tile of the player?", criteria = {}, order = {} },
    { id = "clear", type = "noul", instructions = "Are all remaining pellets still reachable?", criteria = {}, order = {} },
  }

  local rows, tokens = {}, 0
  for _, q in ipairs(questions) do
    local ids, markers = build_row(specials, state, q)
    tokens = tokens + #ids
    rows[#rows + 1] = { ids = ids, markers = markers, qtype = QTYPES[q.type], q = q }
  end
  local batch, marker_count = collate(rows, specials)
  local out, inference_ms = emb.run(batch, { outputs = { "logits", "act_logits" } })
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
      for j, d in ipairs(legal) do probs_by_dir[d] = probs[j] end
      frame.probs = probs_by_dir
    elseif row.q.id == "danger" then
      frame.danger = probs[2]
    else
      frame.clear = probs[2]
    end
  end

  local proposed
  for _, d in ipairs(legal) do
    if proposed == nil or (probs_by_dir[d] or 0) > (probs_by_dir[proposed] or 0) then proposed = d end
  end
  proposed = proposed or DIRS[1]

  -- The planner's pool: safe moves minus an immediate reversal when another
  -- safe move exists (the left/right attractor), narrowed to the moves that
  -- step onto a shortest path to the nearest pellet when any such move is safe.
  -- ponytail: the planner is greedy — BFS to the *nearest* pellet, not a full
  -- tour, so it can still walk into a pocket while a farther pellet waits. The
  -- reversal veto and the stall watchdog keep that from reading as a loop; swap
  -- in a whole-maze tour (or a learned policy) if a run ever deadlocks.
  local rev = REVERSE[g.pac.dir]
  local pool = {}
  for _, d in ipairs(safe) do
    local ni = idx(g.pac.x + VEC[d][1], g.pac.y + VEC[d][2])
    local on_path = nearest ~= nil and dist[ni] ~= nil and dist[ni] < nearest
    -- No reversal while another safe move exists, and never two reversals in a
    -- row: that single rule is what a two-cell left/right loop cannot survive.
    if not (d == rev and (#safe > 1 or g.reversed)) then
      pool[#pool + 1] = { d = d, on_path = on_path }
    end
  end
  if #pool == 0 then
    -- Boxed in: take the farthest-from-ghost legal move other than the reversal
    -- just made, so the loop breaks even when every forward cell is threatened.
    local best, best_dist
    for _, d in ipairs(legal) do
      if d ~= rev or #legal == 1 then
        local gd = nearest_ghost_distance(g, g.pac.x + VEC[d][1], g.pac.y + VEC[d][2])
        if best == nil or gd > best_dist then best, best_dist = d, gd end
      end
    end
    pool[#pool + 1] = { d = best or legal[1] }
  end
  local progress = {}
  for _, c in ipairs(pool) do if c.on_path then progress[#progress + 1] = c end end
  local choices = #progress > 0 and progress or pool

  local executed = proposed
  local allowed = false
  for _, c in ipairs(choices) do if c.d == proposed then allowed = true end end
  if not allowed then
    executed = choices[1].d
    for _, c in ipairs(choices) do
      if (probs_by_dir[c.d] or 0) > (probs_by_dir[executed] or 0) then executed = c.d end
    end
  end
  -- Stalled: the model's picks are not clearing pellets, so the planner takes
  -- the shortest path until one is eaten.
  if g.stall >= STALL_LIMIT and preferred and safe_map[preferred] then
    executed = preferred
  end

  frame.proposed = proposed
  frame.executed = executed
  frame.intervened = proposed ~= executed
  frame.input_tokens = tokens
  frame.inference_ms = inference_ms
  return frame
end

-- ── the episode ─────────────────────────────────────────────────────────────
local spec = json.decode(KEYS[1])
if type(spec) ~= "table" then error("pacman: KEYS[1] must be a request object", 0) end
local ticks = math.floor(tonumber(spec.ticks) or DEFAULT_TICKS)
if ticks < 0 then ticks = 0 end
if ticks > MAX_TICKS then ticks = MAX_TICKS end

local specials = emb.tokenize.special_ids()
local game = load_game(spec)

local frames, total_tokens, total_inference = {}, 0, 0
for _ = 1, ticks do
  if not game.alive or game.won then break end
  local frame = decide(specials, game)
  frame.board = game_of(game)
  step(game, frame.executed)
  total_tokens = total_tokens + frame.input_tokens
  total_inference = total_inference + frame.inference_ms
  frames[#frames + 1] = frame
end

return json.encode({
  frames = frames,
  game = game_of(game),
  usage = { input_tokens = total_tokens, output_tokens = 0, inference_ms = total_inference },
})
