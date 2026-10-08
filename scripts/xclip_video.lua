-- xclip_video.lua: 8 packed RGB frames -> the X-CLIP video tower's L2-normalized
-- 512-d embedding. The export declares fixed axes, so the clip is exactly
-- [1, 8, 3, 224, 224] float32 (scale [0,1]; mean/std are inside the graph).
-- This is a build-only preset: a 4.8 MB tensor is far past the sandbox's
-- per-request byte cap, so the browser never sends one.
--
--   EMB.EVSHA xclip-video <sha1> 1 <packed f32 pixel bytes>

local FRAMES, SIDE = 8, 224

local function normalize(bytes)
  local n = emb.math.norm(bytes)
  if n > 0 then
    return emb.math.float32_bytes(emb.math.scale(bytes, 1.0 / n))
  end
  return bytes
end

local r = emb.run({
  pixel_values = { shape = { 1, FRAMES, 3, SIDE, SIDE }, bytes = KEYS[1], dtype = "f32" },
}, { outputs = { "video_embeds" }, bytes = true })

return { shape = r.video_embeds.shape, bytes = normalize(r.video_embeds.bytes), dtype = "f32" }
