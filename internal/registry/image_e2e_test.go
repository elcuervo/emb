package registry

import (
	"encoding/binary"
	"math"
	"os"
	"path/filepath"
	"sort"
	"testing"

	"github.com/elcuervo/emb/internal/config"
	"github.com/elcuervo/emb/internal/imageproc"
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

func requireORT(t *testing.T) {
	t.Helper()
	if err := onnx.InitEnvironment(""); err != nil {
		t.Skipf("onnx runtime unavailable: %v (run inside nix develop)", err)
	}
	t.Cleanup(func() { _ = onnx.DestroyEnvironment() })
}

// newVisionImageResources resolves the plan (exercising the production
// autodetection + preprocessing), auto-selects the output tensor exactly as
// resolveModelConfig does, and opens a SINGLE named session. The gated tests
// use one session on purpose: the production pool is auto-tuned (up to
// GOMAXPROCS sessions, each with its own copy of the weights), which is far too
// heavy to open per test for a hundreds-of-MB vision export. The pool logic
// itself is covered by the fake-session unit tests.
func newVisionImageResources(t *testing.T, dir string, image *config.ImageConfig, normalize bool) (*ImageResources, imageproc.Plan, string) {
	t.Helper()
	onnxPath := filepath.Join(dir, "vision_model.onnx")

	plan, err := resolveImagePlan(config.ModelConfig{
		ONNX:      onnxPath,
		Tokenizer: filepath.Join(dir, "tokenizer.json"),
		Image:     image,
	}, "vision")
	if err != nil {
		t.Fatalf("resolveImagePlan: %v", err)
	}

	outInfo, err := onnx.GetOutputInfo(onnxPath)
	if err != nil {
		t.Fatal(err)
	}
	outputName, rank := selectOutputTensor(outInfo)

	modelData, err := os.ReadFile(onnxPath)
	if err != nil {
		t.Fatal(err)
	}
	inputNames, err := onnx.GetInputNames(onnxPath)
	if err != nil {
		t.Fatal(err)
	}
	outputNames := make([]string, 0, len(outInfo))
	for n := range outInfo {
		outputNames = append(outputNames, n)
	}
	sort.Strings(outputNames)

	sess, err := onnx.NewNamedRuntimeSessionFromBytes(modelData, inputNames, outputNames, 1, 1, onnx.ExecModeSequential)
	if err != nil {
		t.Fatalf("opening vision session: %v", err)
	}
	res := &ImageResources{
		Sessions:     []onnx.NamedSession{sess},
		Plan:         plan,
		OutputTensor: outputName,
		Pooling:      poolingForRank(rank),
		Normalize:    normalize,
		Dim:          int(outInfo[outputName].Dim),
	}
	t.Cleanup(func() {
		for _, s := range res.Sessions {
			_ = s.Close()
		}
	})
	return res, plan, outputName
}

func goldenImage(t *testing.T) []byte {
	t.Helper()
	data, err := os.ReadFile("../../internal/imageproc/testdata/golden_input.png")
	if err != nil {
		t.Fatalf("reading test image: %v", err)
	}
	return data
}

// TestImageEmbeddingEndToEnd embeds a real image through a real SigLIP2/CLIP
// vision export and asserts dimension and normalization. Gated on the fixture.
func TestImageEmbeddingEndToEnd(t *testing.T) {
	dir := visionDir(t)
	requireORT(t)

	res, _, outputName := newVisionImageResources(t, dir, &config.ImageConfig{Input: "pixel_values"}, true)
	t.Logf("vision output=%q dim=%d pooling=%s", outputName, res.Dim, res.Pooling)

	tensor, err := res.Plan.Tensor(goldenImage(t))
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

// TestSigLIP2VisionAutoconfigAndReference loads the SigLIP2 vision export with
// an EMPTY image block (so every parameter is auto-discovered) and checks:
//   - the resolved plan matches the export's preprocessor_config.json + graph
//     (input pixel_values, size 224, crop none, resample bilinear, rescale
//     1/255, mean/std 0.5);
//   - the auto-selected output is pooler_output (rank 2, dim 768);
//   - the pooled embedding matches the Python/PIL + onnxruntime reference in
//     testdata/siglip2_vision_pooler.f32 within tolerance.
//
// Gated on the fixture and the reference file. Regenerate the reference with
// testdata/generate_siglip2_reference.py.
func TestSigLIP2VisionAutoconfigAndReference(t *testing.T) {
	refPath := "testdata/siglip2_vision_pooler.f32"
	if _, err := os.Stat(refPath); err != nil {
		t.Skipf("reference fixture missing: %v", err)
	}
	dir := visionDir(t)
	requireORT(t)

	res, plan, outputName := newVisionImageResources(t, dir, &config.ImageConfig{}, false)

	// Autodiscovery: the plan is built from preprocessor_config.json + graph.
	if plan.Input != "pixel_values" {
		t.Errorf("input = %q, want pixel_values (graph-detected)", plan.Input)
	}
	if plan.Size != 224 {
		t.Errorf("size = %d, want 224 (preprocessor_config.json)", plan.Size)
	}
	if plan.Crop != imageproc.CropNone {
		t.Errorf("crop = %s, want none (no do_center_crop → SigLIP resize)", plan.Crop)
	}
	if plan.Resample != imageproc.ResampleBilinear {
		t.Errorf("resample = %s, want bilinear (resample: 2)", plan.Resample)
	}
	if plan.Rescale != 1.0/255.0 {
		t.Errorf("rescale = %v, want 1/255 (rescale_factor)", plan.Rescale)
	}
	if plan.Mean != [3]float64{0.5, 0.5, 0.5} || plan.Std != [3]float64{0.5, 0.5, 0.5} {
		t.Errorf("mean/std = %v/%v, want 0.5/0.5 (image_mean/image_std)", plan.Mean, plan.Std)
	}
	if outputName != "pooler_output" || res.Dim != 768 {
		t.Errorf("output = %q dim=%d, want pooler_output/768 (graph rank-2 preference)", outputName, res.Dim)
	}
	t.Logf("autoconfig: input=%q size=%d crop=%s resample=%s rescale=%v mean=%v std=%v output=%q dim=%d",
		plan.Input, plan.Size, plan.Crop, plan.Resample, plan.Rescale, plan.Mean, plan.Std, outputName, res.Dim)

	// Embedding parity against the Python reference.
	tensor, err := plan.Tensor(goldenImage(t))
	if err != nil {
		t.Fatalf("preprocessing: %v", err)
	}
	embs, err := res.Embed([][]float32{tensor})
	if err != nil {
		t.Fatalf("embedding: %v", err)
	}
	got := decodeFloat32LE(embs[0])
	wantRaw, err := os.ReadFile(refPath)
	if err != nil {
		t.Fatal(err)
	}
	want := decodeFloat32LE(wantRaw)
	if len(got) != len(want) {
		t.Fatalf("dim mismatch: go=%d reference=%d", len(got), len(want))
	}

	var dot, na, nb, maxAbs float64
	for i := range got {
		a, b := float64(got[i]), float64(want[i])
		dot += a * b
		na += a * a
		nb += b * b
		if d := math.Abs(a - b); d > maxAbs {
			maxAbs = d
		}
	}
	cosine := dot / (math.Sqrt(na) * math.Sqrt(nb))
	t.Logf("embedding parity vs python/PIL: dim=%d cosine=%.8f max|diff|=%.6f goNorm=%.6f pyNorm=%.6f",
		len(got), cosine, maxAbs, math.Sqrt(na), math.Sqrt(nb))

	// Tolerance accounts for the different resize kernels (x/image BiLinear vs
	// Pillow BILINEAR); see the parity caveat in the README.
	if cosine < 0.999 {
		t.Fatalf("cosine %.8f below 0.999 vs reference", cosine)
	}
	if maxAbs > 0.15 {
		t.Fatalf("max |diff| %.6f above 0.15 vs reference", maxAbs)
	}
}

func decodeFloat32LE(raw []byte) []float32 {
	out := make([]float32, len(raw)/4)
	for i := range out {
		out[i] = math.Float32frombits(binary.LittleEndian.Uint32(raw[i*4:]))
	}
	return out
}
