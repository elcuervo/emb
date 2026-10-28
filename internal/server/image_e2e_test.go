package server

import (
	"encoding/binary"
	"math"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/elcuervo/emb/internal/config"
	"github.com/elcuervo/emb/internal/registry"
)

func envOr(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

// TestCrossModalSmoke embeds one image (EMB.IMG) and several text prompts (EMB)
// on the same fused dual-encoder model, asserting the vectors share a dimension
// and that cosine similarities are finite/comparable (a known image/text pair
// ranks sensibly). Gated: skips unless EMB_FUSED_VISION_MODEL names a fused
// CLIP/SigLIP export directory containing model.onnx and tokenizer.json.
func TestCrossModalSmoke(t *testing.T) {
	dir := os.Getenv("EMB_FUSED_VISION_MODEL")
	if dir == "" {
		t.Skip("set EMB_FUSED_VISION_MODEL to a fused CLIP/SigLIP export directory")
	}
	for _, f := range []string{"model.onnx", "tokenizer.json"} {
		if _, err := os.Stat(filepath.Join(dir, f)); err != nil {
			t.Skipf("fused fixture %s missing in %s", f, dir)
		}
	}
	if !ortOK {
		t.Skip("onnx runtime unavailable (run inside nix develop)")
	}

	entry, err := registry.LoadModel(config.ModelConfig{
		ONNX:         filepath.Join(dir, "model.onnx"),
		Tokenizer:    filepath.Join(dir, "tokenizer.json"),
		OutputTensor: envOr("EMB_FUSED_TEXT_OUTPUT", "text_embeds"),
		Pooling:      "none",
		Normalize:    true,
		Image: &config.ImageConfig{
			Input:   "pixel_values",
			Outputs: []config.ImageOutputConfig{{Name: "embedding", Tensor: envOr("EMB_FUSED_IMAGE_OUTPUT", "image_embeds")}},
			Size:    224,
		},
	}, "fused")
	if err != nil {
		t.Fatalf("loading fused model: %v", err)
	}
	reg := registry.New()
	reg.Add("fused", entry)
	t.Cleanup(func() { _ = reg.Close() })
	if err := reg.Initialize("fused"); err != nil {
		t.Fatalf("warming text pool: %v", err)
	}

	addr := getFreeAddr()
	srv := New(addr, reg, "", "", nil)
	go srv.ListenAndServe()
	t.Cleanup(func() { srv.Close() })
	time.Sleep(50 * time.Millisecond)

	image, err := os.ReadFile("../../internal/imageproc/testdata/golden_input.png")
	if err != nil {
		t.Fatalf("reading test image: %v", err)
	}
	imageVec := decodeFloat32Bulk(t, imageValueOf(t, redisCmd(t, addr, "EMB.IMG", "fused", string(image)), "embedding"))

	labels := []string{"a photo of a cat", "a photo of a dog", "a photo of a car"}
	best, bestSim := "", -2.0
	for _, label := range labels {
		textVec := decodeFloat32Bulk(t, bulkOf(t, redisCmd(t, addr, "EMB", "fused", label)))
		if len(textVec) != len(imageVec) {
			t.Fatalf("text dim %d != image dim %d", len(textVec), len(imageVec))
		}
		sim := cosineSimilarity(textVec, imageVec)
		if sim > bestSim {
			best, bestSim = label, sim
		}
	}
	if best == "" || bestSim < -1.0001 || bestSim > 1.0001 {
		t.Fatalf("comparable vectors expected; best=%q sim=%v", best, bestSim)
	}
	t.Logf("cross-modal smoke: best label %q (cos=%.4f)", best, bestSim)
}

func decodeFloat32Bulk(t *testing.T, s string) []float32 {
	t.Helper()
	if len(s)%4 != 0 {
		t.Fatalf("bulk length %d is not a multiple of 4", len(s))
	}
	out := make([]float32, len(s)/4)
	for i := range out {
		out[i] = math.Float32frombits(binary.LittleEndian.Uint32([]byte(s)[i*4:]))
	}
	return out
}

func cosineSimilarity(a, b []float32) float64 {
	var dot, na, nb float64
	for i := range a {
		dot += float64(a[i]) * float64(b[i])
		na += float64(a[i]) * float64(a[i])
		nb += float64(b[i]) * float64(b[i])
	}
	if na == 0 || nb == 0 {
		return 0
	}
	return dot / (math.Sqrt(na) * math.Sqrt(nb))
}

// TestDINOv3ServerEndToEnd serves EMB.IMG from a real DINOv3 export: the keyed
// reply's sizes, the token envelope's shape and grid, and a three-image request
// against the static batch-1 graph. Gated on the downloaded kornia export and
// its generated reference.
func TestDINOv3ServerEndToEnd(t *testing.T) {
	dir := envOr("EMB_DINOV3_KORNIA", "../../models/dinov3-336")
	graph := filepath.Join(dir, "dinov3-vits16-336.onnx")
	if _, err := os.Stat(graph); err != nil {
		t.Skipf("DINOv3 fixture missing: %v (run: just download-dinov3-model kornia/dinov3 ./models/dinov3-336)", err)
	}
	if !ortOK {
		t.Skip("onnx runtime unavailable (run inside nix develop)")
	}
	ref, err := os.ReadFile("../../internal/registry/testdata/dinov3_vits16_336_pooled.f32")
	if err != nil {
		t.Skipf("reference fixture missing: %v", err)
	}

	entry, err := registry.LoadModel(config.ModelConfig{
		ONNX:      graph,
		Workers:   1,
		Normalize: true,
		Image: &config.ImageConfig{
			Preset: "dinov3",
			Input:  "input",
			Size:   336,
			Outputs: []config.ImageOutputConfig{
				{Name: "embedding", Tensor: "descriptor"},
				{Name: "patches", Tensor: "tokens", Select: "tokens", Start: new(5), Grid: 21},
			},
		},
	}, "dinov3-336")
	if err != nil {
		t.Fatalf("loading the export: %v", err)
	}
	reg := registry.New()
	reg.Add("dinov3-336", entry)
	t.Cleanup(func() { _ = reg.Close() })

	addr := getFreeAddr()
	srv := New(addr, reg, "", "1MB", nil)
	go srv.ListenAndServe()
	t.Cleanup(func() { srv.Close() })
	time.Sleep(50 * time.Millisecond)

	png := readPNG(t, "../../internal/imageproc/testdata/golden_input.png")
	const dim = 384

	// A bare request serves every configured output: one keyed map.
	one := imageValueOf(t, redisCmd(t, addr, "EMB.IMG", "dinov3-336", png), "embedding")
	if len(one) != dim*4 {
		t.Fatalf("embedding = %d bytes, want %d", len(one), dim*4)
	}
	want := decodeLE(ref)
	got := decodeLE([]byte(one))
	var dot, na, nb float64
	for i := range got {
		dot += float64(got[i]) * float64(want[i])
		na += float64(got[i]) * float64(got[i])
		nb += float64(want[i]) * float64(want[i])
	}
	if cosine := dot / (math.Sqrt(na) * math.Sqrt(nb)); cosine < 0.995 {
		t.Fatalf("server embedding cosine = %.6f, want >= 0.995 vs the reference", cosine)
	}

	// Both outputs in one map, sized 441x384 for the grid.
	both := arrayOf(t, redisCmd(t, addr, "EMB.IMG", "dinov3-336", "OUTPUTS", "embedding,patches", png))
	if len(both) != 4 || both[0].val != "embedding" || both[2].val != "patches" {
		t.Fatalf("two-output map = %#v", both)
	}
	if got := len(bulkOf(t, both[3])); got != 441*dim*4 {
		t.Fatalf("patches = %d bytes, want 441*384*4 = %d", got, 441*dim*4)
	}

	// VALUES carries the token grid's shape and grid edges.
	envelope := arrayOf(t, bulkOrArray(t, redisCmd(t, addr, "EMB.IMG", "dinov3-336", "VALUES", "OUTPUTS", "patches", png), "patches"))
	fields := map[string]respToken{}
	for i := 0; i+1 < len(envelope); i += 2 {
		fields[bulkOf(t, envelope[i])] = envelope[i+1]
	}
	if shape := arrayOf(t, fields["shape"]); shape[0].val != 441 || shape[1].val != dim {
		t.Fatalf("token shape = %#v, want [441 %d]", shape, dim)
	}
	if grid := arrayOf(t, fields["grid"]); grid[0].val != 21 || grid[1].val != 21 {
		t.Fatalf("token grid = %#v, want [21 21]", grid)
	}

	// Three images against a batch-1 graph: every slot served, one run each.
	three := arrayOf(t, redisCmd(t, addr, "EMB.IMG", "dinov3-336", "OUTPUTS", "embedding", png, png, png))
	if len(three) != 3 {
		t.Fatalf("three-image request = %d slots, want 3", len(three))
	}
	for i, slot := range three {
		m := arrayOf(t, slot)
		if len(m) != 2 || len(bulkOf(t, m[1])) != dim*4 {
			t.Fatalf("slot %d = %#v, want a %d-byte embedding", i, slot, dim*4)
		}
	}
}

// readPNG loads an image fixture as raw bytes.
func readPNG(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("reading %s: %v", path, err)
	}
	return string(data)
}

// decodeLE decodes little-endian float32 bytes.
func decodeLE(raw []byte) []float32 {
	out := make([]float32, len(raw)/4)
	for i := range out {
		out[i] = math.Float32frombits(binary.LittleEndian.Uint32(raw[i*4:]))
	}
	return out
}
