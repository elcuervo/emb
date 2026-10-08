-- clap_audio.lua: a packed log-mel -> the CLAP audio tower's L2-normalized 512-d
-- embedding. This is the one preloaded preset the bridge admits binary for
-- besides the image preset, because the mel the browser computes is exactly the
-- tensor the graph wants: `input_features` [1, 1, 1001, 64] float32, computed
-- outside the graph per the model's own preprocessor_config.json.
--
--   EMB.EVSHA clap-audio <sha1> 1 <packed f32 mel bytes>
--   -> { shape = {1,512}, bytes = <float32>, dtype = "f32" }

local BINS, FRAMES = 64, 1001

local function normalize(bytes)
  local n = emb.math.norm(bytes)
  if n > 0 then
    return emb.math.float32_bytes(emb.math.scale(bytes, 1.0 / n))
  end
  return bytes
end

local r = emb.run({
  input_features = { shape = { 1, 1, FRAMES, BINS }, bytes = KEYS[1], dtype = "f32" },
}, { outputs = { "audio_embeds" }, bytes = true })

return { shape = r.audio_embeds.shape, bytes = normalize(r.audio_embeds.bytes), dtype = "f32" }
