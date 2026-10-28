package registry

import (
	"math"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/elcuervo/emb/internal/config"
)

// dinov3Dir resolves a downloaded DINOv3 export directory, skipping when the
// fixture is absent (mirroring visionDir). Point EMB_DINOV3_COMMUNITY or
// EMB_DINOV3_KORNIA at an export downloaded with
// `just download-dinov3-model [repo] [dir]`.
func dinov3Dir(t *testing.T, env, def, graph string) string {
	t.Helper()
	dir := os.Getenv(env)
	if dir == "" {
		dir = def
	}
	if _, err := os.Stat(filepath.Join(dir, graph)); err != nil {
		t.Skipf("DINOv3 fixture %s missing in %s (set %s or run: just download-dinov3-model %s)", graph, dir, env, def)
	}
	return dir
}

// referenceRow reads one float32 reference file, skipping when it is absent.
func referenceRow(t *testing.T, name string) []float32 {
	t.Helper()
	path := filepath.Join("testdata", name)
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Skipf("reference fixture missing: %v (run testdata/generate_dinov3_reference.py)", err)
	}
	return decodeFloat32LE(raw)
}

// cosineOfRows reports the cosine similarity of two equal-length vectors.
func cosineOfRows(t *testing.T, a, b []float32) float64 {
	t.Helper()
	if len(a) != len(b) {
		t.Fatalf("cosine operands differ: %d vs %d", len(a), len(b))
	}
	var dot, na, nb float64
	for i := range a {
		x, y := float64(a[i]), float64(b[i])
		dot += x * y
		na += x * x
		nb += y * y
	}
	if na == 0 || nb == 0 {
		t.Fatalf("zero-length vector in the comparison")
	}
	return dot / (math.Sqrt(na) * math.Sqrt(nb))
}

// materializeScriptImages writes golden_input.png once per test directory and
// returns the path, so a multi-image request can carry three distinct encodings
// of the same source image.
func materializeScriptImage(t *testing.T, name string) string {
	t.Helper()
	data, err := os.ReadFile("../../internal/imageproc/testdata/golden_input.png")
	if err != nil {
		t.Fatalf("reading test image: %v", err)
	}
	path := filepath.Join(t.TempDir(), name)
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

// TestDINOv3CommunityExport covers the onnx-community dialect: a pooled
// `pooler_output` vector and a `last_hidden_state` patch grid, with the layout
// (start 5, grid 16 at 224) read from the export's own config.json. Gated on the
// downloaded export and its generated reference.
func TestDINOv3CommunityExport(t *testing.T) {
	dir := dinov3Dir(t, "EMB_DINOV3_COMMUNITY", "../../models/dinov3-vits16", "model.onnx")
	requireORT(t)
	wantPooled := referenceRow(t, "dinov3_vits16_pooled.f32")
	wantPatch := referenceRow(t, "dinov3_vits16_patch.f32")

	entry, err := LoadModel(config.ModelConfig{
		ONNX:      filepath.Join(dir, "model.onnx"),
		Workers:   1,
		Normalize: true,
		Image: &config.ImageConfig{
			Preset: "dinov3",
			Input:  "pixel_values",
			Size:   224,
			Outputs: []config.ImageOutputConfig{
				{Name: "embedding", Tensor: "pooler_output"},
				{Name: "patches", Tensor: "last_hidden_state", Select: "tokens"},
			},
		},
	}, "dinov3")
	if err != nil {
		t.Fatalf("loading the onnx-community export: %v", err)
	}
	res, err := entry.ImageResources()
	if err != nil {
		t.Fatalf("opening the image pool: %v", err)
	}
	defer func() { entry.closeImageSessions() }()

	if len(res.Outputs) != 2 {
		t.Fatalf("outputs = %+v, want two", res.Outputs)
	}
	patches := res.Outputs[1]
	// 224 / patch_size 16 = a 14x14 grid; start = 1 CLS + 4 registers.
	if patches.Start != 5 || patches.Grid != 14 || patches.Rows != 196 {
		t.Fatalf("token layout = %+v, want start 5 grid 14 rows 196 (14x14)", patches)
	}
	if res.BatchCap != 0 {
		t.Fatalf("batch cap = %d, want unbounded for a symbolic axis", res.BatchCap)
	}

	tensor, err := res.Plan.Tensor(goldenImage(t))
	if err != nil {
		t.Fatalf("preprocessing: %v", err)
	}
	got, err := res.Embed([][]float32{tensor}, res.Outputs)
	if err != nil {
		t.Fatalf("embedding: %v", err)
	}

	pooled := decodeFloat32LE(got[0]["embedding"])
	if c := cosineOfRows(t, pooled, wantPooled); c < 0.999 {
		t.Fatalf("pooled cosine = %.6f, want >= 0.999 vs the generated reference", c)
	}
	grid := decodeFloat32LE(got[0]["patches"])
	dim := patches.Dim
	if len(grid) != patches.Rows*dim {
		t.Fatalf("grid blob = %d floats, want %d rows * %d", len(grid), patches.Rows, dim)
	}
	if c := cosineOfRows(t, grid[:dim], wantPatch); c < 0.999 {
		t.Fatalf("first patch row cosine = %.6f, want >= 0.999 vs the generated reference", c)
	}
}

// TestDINOv3KorniaExport covers the kornia dialect: a static batch-1 graph whose
// pooled vector is `descriptor` and whose grid is 441 rows of `tokens` at 336
// with start 5. Gated on the downloaded export and its generated reference.
func TestDINOv3KorniaExport(t *testing.T) {
	dir := dinov3Dir(t, "EMB_DINOV3_KORNIA", "../../models/dinov3-336", "dinov3-vits16-336.onnx")
	requireORT(t)
	wantPooled := referenceRow(t, "dinov3_vits16_336_pooled.f32")
	wantPatch := referenceRow(t, "dinov3_vits16_336_patch.f32")

	entry, err := LoadModel(config.ModelConfig{
		ONNX:      filepath.Join(dir, "dinov3-vits16-336.onnx"),
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
		t.Fatalf("loading the kornia export: %v", err)
	}
	res, err := entry.ImageResources()
	if err != nil {
		t.Fatalf("opening the image pool: %v", err)
	}
	defer func() { entry.closeImageSessions() }()

	patches := res.Outputs[1]
	if patches.Start != 5 || patches.Grid != 21 || patches.Rows != 441 {
		t.Fatalf("token layout = %+v, want start 5 grid 21 rows 441", patches)
	}
	if res.BatchCap != 1 {
		t.Fatalf("batch cap = %d, want 1 for the static-batch graph", res.BatchCap)
	}

	tensor, err := res.Plan.Tensor(goldenImage(t))
	if err != nil {
		t.Fatalf("preprocessing: %v", err)
	}
	// One image per run: the graph rejects a larger batch, which is why the
	// server caps its chunk at BatchCap.
	got, err := res.Embed([][]float32{tensor}, res.Outputs)
	if err != nil {
		t.Fatalf("embedding: %v", err)
	}

	// The reference is preprocessed by Pillow; emb resamples the 40x30 fixture
	// with x/image. A one-LSB sampling difference moves this model's cosine to
	// ~0.9979 on its own (measured), so the bound here is 0.995 for the 8.4x
	// upscale to 336 rather than the 0.999 the gentler 224 upscale reaches.
	pooled := decodeFloat32LE(got[0]["embedding"])
	if c := cosineOfRows(t, pooled, wantPooled); c < 0.995 {
		t.Fatalf("descriptor cosine = %.6f, want >= 0.995", c)
	}
	grid := decodeFloat32LE(got[0]["patches"])
	dim := patches.Dim
	if len(grid) != patches.Rows*dim {
		t.Fatalf("grid blob = %d floats, want %d * %d", len(grid), patches.Rows, dim)
	}
	if c := cosineOfRows(t, grid[:dim], wantPatch); c < 0.995 {
		t.Fatalf("first token row cosine = %.6f, want >= 0.995", c)
	}

	// A batch of two must be refused by the graph, which is the reason the chunk
	// cap exists: the static axis is the constraint, not a server convention.
	if _, err := res.Embed([][]float32{tensor, tensor}, res.Outputs); err == nil {
		t.Fatal("batch-1 graph accepted a two-image run")
	} else if !strings.Contains(err.Error(), "Expected: 1") {
		t.Fatalf("batch error = %v, want the graph's static batch of 1", err)
	}
}

// TestDINOv3DefaultsFromModelMetadata proves the onnx-community export needs no
// layout constants in config: patch_size 16 and num_register_tokens 4 resolve
// start 5 and grid 16.
func TestDINOv3DefaultsFromModelMetadata(t *testing.T) {
	dir := dinov3Dir(t, "EMB_DINOV3_COMMUNITY", "../../models/dinov3-vits16", "model.onnx")
	requireORT(t)
	plan, outputs, src, err := resolveImageResources(config.ModelConfig{
		ONNX: filepath.Join(dir, "model.onnx"),
		Image: &config.ImageConfig{
			Preset:  "dinov3",
			Input:   "pixel_values",
			Size:    224,
			Outputs: []config.ImageOutputConfig{{Name: "patches", Tensor: "last_hidden_state", Select: "tokens"}},
		},
	}, "dinov3")
	if err != nil {
		t.Fatal(err)
	}
	if plan.Size != 224 {
		t.Fatalf("size = %d", plan.Size)
	}
	if len(outputs) != 1 || outputs[0].Start != 5 || outputs[0].Grid != 14 {
		t.Fatalf("layout = %+v, want start 5 grid 14 (size 224 / patch 16) from config.json", outputs)
	}
	if !strings.Contains(src.Mean, "preprocessor_config.json") {
		t.Fatalf("mean source = %q, want the export's preprocessor_config.json", src.Mean)
	}
}
