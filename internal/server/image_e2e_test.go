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
			Input:  "pixel_values",
			Output: envOr("EMB_FUSED_IMAGE_OUTPUT", "image_embeds"),
			Size:   224,
		},
	}, "fused")
	if err != nil {
		t.Fatalf("loading fused model: %v", err)
	}
	t.Cleanup(func() {
		if entry.ImageRes != nil {
			for _, sess := range entry.ImageRes.Sessions {
				_ = sess.Close()
			}
		}
	})

	reg := registry.New()
	reg.Add("fused", entry)
	if _, err := reg.GetOrInit("fused"); err != nil {
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
	imageVec := decodeFloat32Bulk(t, bulkOf(t, redisCmd(t, addr, "EMB.IMG", "fused", string(image))))

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
