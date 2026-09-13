package registry

import (
	"encoding/binary"
	"math"
	"os"
	"path/filepath"
	"testing"

	"github.com/elcuervo/emb/internal/config"
	"github.com/elcuervo/emb/internal/onnx"
)

// visionDir resolves a real vision-export fixture directory. It skips when the
// fixture is absent, mirroring the GLiNER gated tests; point EMB_VISION_MODEL
// at an export downloaded with `just download-vision-model`.
func visionDir(t *testing.T) string {
	t.Helper()
	dir := os.Getenv("EMB_VISION_MODEL")
	if dir == "" {
		dir = "../../models/siglip2-vision"
	}
	for _, f := range []string{"vision_model.onnx", "tokenizer.json"} {
		if _, err := os.Stat(filepath.Join(dir, f)); err != nil {
			t.Skipf("vision fixture %s missing in %s (set EMB_VISION_MODEL or run: just download-vision-model)", f, dir)
		}
	}
	return dir
}

// TestImageEmbeddingEndToEnd embeds a real image through a real SigLIP2/CLIP
// vision export and asserts dimension and normalization. Gated on the fixture.
func TestImageEmbeddingEndToEnd(t *testing.T) {
	dir := visionDir(t)
	if err := onnx.InitEnvironment(""); err != nil {
		t.Skipf("onnx runtime unavailable: %v (run inside nix develop)", err)
	}
	t.Cleanup(func() { _ = onnx.DestroyEnvironment() })

	entry, err := LoadModel(config.ModelConfig{
		ONNX:      filepath.Join(dir, "vision_model.onnx"),
		Tokenizer: filepath.Join(dir, "tokenizer.json"),
		Pooling:   "none",
		Normalize: true,
		Image:     &config.ImageConfig{Input: "pixel_values"},
	}, "vision")
	if err != nil {
		t.Fatalf("loading vision model: %v", err)
	}
	t.Cleanup(entry.closeImageSessions)

	res, err := entry.ImageResources()
	if err != nil {
		t.Fatal(err)
	}
	if res == nil {
		t.Fatal("expected image resources for a model with an image block")
	}
	data, err := os.ReadFile("../../internal/imageproc/testdata/golden_input.png")
	if err != nil {
		t.Fatalf("reading test image: %v", err)
	}
	tensor, err := res.Plan.Tensor(data)
	if err != nil {
		t.Fatalf("preprocessing: %v", err)
	}
	embs, err := res.Embed([][]float32{tensor})
	if err != nil {
		t.Fatalf("embedding: %v", err)
	}
	if len(embs) != 1 {
		t.Fatalf("got %d embeddings, want 1", len(embs))
	}
	if res.Dim <= 0 || len(embs[0]) != res.Dim*4 {
		t.Fatalf("embedding bytes = %d, want dim %d * 4", len(embs[0]), res.Dim)
	}
	if res.Normalize {
		var sumSq float64
		for i := 0; i < res.Dim; i++ {
			v := math.Float32frombits(binary.LittleEndian.Uint32(embs[0][i*4:]))
			sumSq += float64(v) * float64(v)
		}
		if math.Abs(sumSq-1) > 1e-3 {
			t.Fatalf("normalized embedding L2 norm^2 = %f, want ~1", sumSq)
		}
	}
}
