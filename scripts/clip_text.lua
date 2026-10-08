-- clip_text.lua: text in, a packed CLIP text embedding out.
--
-- The fused CLIP export keeps its text and image branches in one graph and
-- demands every input for either run, so a text-only run still has to hand the
-- graph a `pixel_values` tensor. It is built host-side as a constant (zeroed)
-- rather than as a Lua table of 150,528 elements, exactly as zeroshot.lua does
-- for the mirror-image case.
--
--   EMB.EVSHA clip <sha1> 1 "thin horizontal bands"
--   -> { { shape = {1,512}, bytes = <little-endian float32>, dtype = "f32" } }
--
-- One value per key, because a multi-text evaluation returns one value per
-- text. A per-text output depends only on its own text, so the reply cache
-- stays correct for a repeated key.

local SIDE = 224

local zero = { shape = { 1, 3, SIDE, SIDE }, fill = 0, dtype = "f32" }

local out = {}
for i = 1, #KEYS do
  local enc = emb.tokenize.encode(KEYS[i], 77)
  local r = emb.run({
    input_ids = { shape = { 1, #enc.ids }, data = enc.ids },
    attention_mask = { shape = { 1, #enc.mask }, data = enc.mask },
    pixel_values = zero,
  }, { outputs = { "text_embeds" }, bytes = true })
  out[i] = {
    shape = r.text_embeds.shape,
    bytes = r.text_embeds.bytes,
    dtype = r.text_embeds.dtype,
  }
end

-- The server's own contract: a one-text call returns the value itself, and a
-- multi-text call returns one value per text. Returning a one-element array
-- here would read as a list of its own fields on the wire.
if #KEYS == 1 then
  return out[1]
end
return out
