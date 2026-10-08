-- clip_embed.lua: raw image bytes in, a packed CLIP image embedding out.
--
-- The build half of the media plates: it is what turns a committed spectrogram
-- or contact sheet into the vector the shippped index stores. It is not
-- preloaded in the sandbox — no page calls it — so it stays out of the digest
-- stamper and the console ledger. The browser never sends bytes to it; only the
-- offline index tool, against a local server, does.
--
--   EMB.EVSHA clip <sha1> 1 <image bytes>
--   -> { shape = {1,512}, bytes = <little-endian float32>, dtype = "f32" }
--
-- The same host-built constant trick as zeroshot.lua: the fused graph needs the
-- text branch's inputs for the image run, and the tokens are a one-word dummy.

local spec = emb.image.preprocess(KEYS[1])

local text = emb.tokenize.encode("a", 77)
local zero = { shape = spec.shape, fill = 0, dtype = "f32" }
local pixels = { shape = spec.shape, bytes = spec.bytes, dtype = spec.dtype }

local r = emb.run({
  input_ids = { shape = { 1, #text.ids }, data = text.ids },
  attention_mask = { shape = { 1, #text.mask }, data = text.mask },
  [spec.input] = pixels,
}, { outputs = { "image_embeds" }, bytes = true })

return {
  shape = r.image_embeds.shape,
  bytes = r.image_embeds.bytes,
  dtype = r.image_embeds.dtype,
}
