package registry

import (
	"encoding/binary"
	"math"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/elcuervo/emb/internal/config"
	"github.com/elcuervo/emb/internal/imageproc"
	"github.com/elcuervo/emb/internal/onnx"
)

func TestDetectImageInputFromInfos(t *testing.T) {
	infos := []onnx.InputInfo{
		{Name: "input_ids", Rank: 2, Dimensions: []int64{-1, -1}},
		{Name: "pixel_values", Rank: 4, Dimensions: []int64{-1, 3, 224, 224}},
	}
	name, size, found := detectImageInputFromInfos(infos)
	if !found || name != "pixel_values" || size != 224 {
		t.Fatalf("detected (%q, %d, %v), want (pixel_values, 224, true)", name, size, found)
	}

	// No conventional name: any 3-channel rank-4 input wins.
	name, size, found = detectImageInputFromInfos([]onnx.InputInfo{
		{Name: "input_ids", Rank: 2, Dimensions: []int64{-1, -1}},
		{Name: "image", Rank: 4, Dimensions: []int64{-1, 3, 384, 384}},
	})
	if !found || name != "image" || size != 384 {
		t.Fatalf("detected (%q, %d, %v), want (image, 384, true)", name, size, found)
	}

	// Fully dynamic spatial dims: detected but no static size.
	_, size, found = detectImageInputFromInfos([]onnx.InputInfo{
		{Name: "pixel_values", Rank: 4, Dimensions: []int64{-1, 3, -1, -1}},
	})
	if !found || size != 0 {
		t.Fatalf("dynamic-size detection = (size %d, found %v), want (0, true)", size, found)
	}

	// Text-only graph: nothing detected.
	if _, _, found := detectImageInputFromInfos([]onnx.InputInfo{{Name: "input_ids", Rank: 2}}); found {
		t.Fatal("text-only graph should not detect an image input")
	}
}

func TestStaticSquareSize(t *testing.T) {
	if got := staticSquareSize([]int64{-1, 3, 224, 224}); got != 224 {
		t.Fatalf("staticSquareSize = %d, want 224", got)
	}
	if got := staticSquareSize([]int64{-1, 3, 224, 256}); got != 0 {
		t.Fatalf("non-square size = %d, want 0", got)
	}
	if got := staticSquareSize([]int64{-1, 3, -1, -1}); got != 0 {
		t.Fatalf("dynamic size = %d, want 0", got)
	}
}

func TestPreprocessorSizeShapes(t *testing.T) {
	cases := []struct {
		raw  string
		want int
	}{
		{`224`, 224},
		{`{"shortest_edge": 256}`, 256},
		{`{"height": 384, "width": 384}`, 384},
		{`{"height": 300, "width": 400}`, 0},
	}
	for _, tc := range cases {
		if got, _ := preprocessorSize([]byte(tc.raw)); got != tc.want {
			t.Fatalf("preprocessorSize(%s) = %d, want %d", tc.raw, got, tc.want)
		}
	}
}

func TestPreprocessorResampleMapping(t *testing.T) {
	cases := map[string]imageproc.Resample{
		`0`: imageproc.ResampleNearest,
		`1`: imageproc.ResampleLanczos,
		`2`: imageproc.ResampleBilinear,
		`3`: imageproc.ResampleBicubic,
	}
	for raw, want := range cases {
		got, ok := preprocessorResample([]byte(raw))
		if !ok || got != want {
			t.Fatalf("preprocessorResample(%s) = (%v, %v), want %v", raw, got, ok, want)
		}
	}
}

func TestResolveImagePlanExplicit(t *testing.T) {
	rescale := 1.0 / 255.0
	cfg := config.ModelConfig{
		ONNX: "model.onnx",
		Image: &config.ImageConfig{
			Input:    "pixel_values",
			Size:     224,
			Crop:     "center",
			Resample: "bicubic",
			Rescale:  &rescale,
			Mean:     []float64{0.48145466, 0.4578275, 0.40821073},
			Std:      []float64{0.26862954, 0.26130258, 0.27577711},
		},
	}
	plan, err := resolveImagePlan(cfg, "clip")
	if err != nil {
		t.Fatal(err)
	}
	if plan.Input != "pixel_values" || plan.Size != 224 || plan.Crop != imageproc.CropCenter || plan.Resample != imageproc.ResampleBicubic {
		t.Fatalf("unexpected plan: %#v", plan)
	}
	if plan.Rescale != rescale {
		t.Fatalf("rescale = %v", plan.Rescale)
	}
	if plan.Mean[0] != 0.48145466 || plan.Std[2] != 0.27577711 {
		t.Fatalf("mean/std = %v/%v", plan.Mean, plan.Std)
	}
}

func TestResolveImagePlanDefaultsWhenOmitted(t *testing.T) {
	// Explicit input+size, everything else omitted → documented defaults.
	cfg := config.ModelConfig{
		ONNX:  "model.onnx",
		Image: &config.ImageConfig{Input: "pixel_values", Size: 16},
	}
	plan, err := resolveImagePlan(cfg, "siglip")
	if err != nil {
		t.Fatal(err)
	}
	if plan.Crop != imageproc.CropNone || plan.Resample != imageproc.ResampleBicubic {
		t.Fatalf("defaults: crop=%v resample=%v", plan.Crop, plan.Resample)
	}
	if plan.Rescale != 1.0/255.0 {
		t.Fatalf("default rescale = %v", plan.Rescale)
	}
	if plan.Mean != [3]float64{0.5, 0.5, 0.5} || plan.Std != [3]float64{0.5, 0.5, 0.5} {
		t.Fatalf("default mean/std = %v/%v", plan.Mean, plan.Std)
	}
}

func TestResolveImagePlanPreprocessorConfigAndPrecedence(t *testing.T) {
	dir := t.TempDir()
	onnxPath := filepath.Join(dir, "model.onnx")
	if err := os.WriteFile(onnxPath, []byte("dummy"), 0644); err != nil {
		t.Fatal(err)
	}
	pp := `{
		"do_center_crop": true,
		"size": {"shortest_edge": 224},
		"resample": 3,
		"rescale_factor": 0.00392156862745098,
		"image_mean": [0.5, 0.5, 0.5],
		"image_std": [0.5, 0.5, 0.5]
	}`
	if err := os.WriteFile(filepath.Join(dir, "preprocessor_config.json"), []byte(pp), 0644); err != nil {
		t.Fatal(err)
	}

	// Input set explicitly so graph detection is not attempted.
	cfg := config.ModelConfig{ONNX: onnxPath, Image: &config.ImageConfig{Input: "pixel_values"}}
	plan, err := resolveImagePlan(cfg, "detected")
	if err != nil {
		t.Fatal(err)
	}
	if plan.Size != 224 || plan.Crop != imageproc.CropCenter || plan.Resample != imageproc.ResampleBicubic {
		t.Fatalf("detected plan: %#v", plan)
	}
	if plan.Mean[0] != 0.5 {
		t.Fatalf("detected mean = %v", plan.Mean)
	}

	// Explicit mean/crop override the detected values.
	explicitRescale := 0.5
	cfg.Image = &config.ImageConfig{
		Input:   "pixel_values",
		Crop:    "none",
		Rescale: &explicitRescale,
		Mean:    []float64{0.1, 0.2, 0.3},
		Std:     []float64{0.4, 0.5, 0.6},
	}
	plan, err = resolveImagePlan(cfg, "override")
	if err != nil {
		t.Fatal(err)
	}
	if plan.Crop != imageproc.CropNone {
		t.Fatalf("explicit crop did not win: %v", plan.Crop)
	}
	if plan.Rescale != 0.5 {
		t.Fatalf("explicit rescale did not win: %v", plan.Rescale)
	}
	if plan.Mean != [3]float64{0.1, 0.2, 0.3} || plan.Std != [3]float64{0.4, 0.5, 0.6} {
		t.Fatalf("explicit mean/std did not win: %v/%v", plan.Mean, plan.Std)
	}
	// Size still comes from the preprocessor config.
	if plan.Size != 224 {
		t.Fatalf("size = %d, want 224 from preprocessor config", plan.Size)
	}
}

func TestResolveImagePlanMissingSizeErrors(t *testing.T) {
	cfg := config.ModelConfig{
		ONNX:  filepath.Join(t.TempDir(), "missing.onnx"),
		Image: &config.ImageConfig{Input: "pixel_values"},
	}
	_, err := resolveImagePlan(cfg, "no-size")
	if err == nil || !strings.Contains(err.Error(), "image.size") {
		t.Fatalf("expected image.size error, got %v", err)
	}
}

// TestResolveImagePlanRejectsInvalidResolvedValues covers values that bypass
// config.validateImageConfig: preprocessor_config.json metadata (std <= 0,
// negative rescale) and explicit std, which the resolved-plan boundary must
// reject rather than silently coercing or emitting non-finite tensors.
func TestResolveImagePlanRejectsInvalidResolvedValues(t *testing.T) {
	cases := []struct {
		name string
		pp   string
		img  *config.ImageConfig
		want string
	}{
		{
			name: "preprocessor zero std",
			pp:   `{"size": 224, "image_mean": [0.5, 0.5, 0.5], "image_std": [0, 0.5, 0.5]}`,
			img:  &config.ImageConfig{Input: "pixel_values"},
			want: "std",
		},
		{
			name: "preprocessor negative rescale",
			pp:   `{"size": 224, "rescale_factor": -1}`,
			img:  &config.ImageConfig{Input: "pixel_values"},
			want: "rescale",
		},
		{
			name: "explicit zero std",
			pp:   `{"size": 224}`,
			img: &config.ImageConfig{
				Input: "pixel_values",
				Mean:  []float64{0.5, 0.5, 0.5},
				Std:   []float64{0, 0, 0},
			},
			want: "std",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			onnxPath := filepath.Join(dir, "model.onnx")
			if err := os.WriteFile(onnxPath, []byte("dummy"), 0644); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(filepath.Join(dir, "preprocessor_config.json"), []byte(tc.pp), 0644); err != nil {
				t.Fatal(err)
			}
			_, err := resolveImagePlan(config.ModelConfig{ONNX: onnxPath, Image: tc.img}, tc.name)
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("expected error mentioning %q, got %v", tc.want, err)
			}
		})
	}
}

func TestResolveImagePlanPreprocessorConfigSizeObject(t *testing.T) {
	dir := t.TempDir()
	onnxPath := filepath.Join(dir, "model.onnx")
	os.WriteFile(onnxPath, []byte("dummy"), 0644)
	os.WriteFile(filepath.Join(dir, "preprocessor_config.json"),
		[]byte(`{"size": {"height": 256, "width": 256}, "resample": 2}`), 0644)

	cfg := config.ModelConfig{ONNX: onnxPath, Image: &config.ImageConfig{Input: "pixel_values"}}
	plan, err := resolveImagePlan(cfg, "m")
	if err != nil {
		t.Fatal(err)
	}
	if plan.Size != 256 || plan.Resample != imageproc.ResampleBilinear {
		t.Fatalf("plan = %#v", plan)
	}
}

func TestImageResourcesNilWithoutImageBlock(t *testing.T) {
	entry := &ModelEntry{Name: "text", cfg: config.ModelConfig{}}
	res, err := entry.ImageResources()
	if err != nil {
		t.Fatal(err)
	}
	if res != nil {
		t.Fatalf("expected nil image resources for a text-only model, got %#v", res)
	}
}

func TestCheckImagePairing(t *testing.T) {
	if err := checkImagePairing("m", "image_embeds", 512, 512); err != nil {
		t.Fatalf("matching dims rejected: %v", err)
	}
	err := checkImagePairing("m", "image_embeds", 512, 768)
	if err == nil {
		t.Fatal("mismatched dims accepted")
	}
	msg := err.Error()
	if !strings.Contains(msg, "512") || !strings.Contains(msg, "768") {
		t.Fatalf("error must name both dimensions, got %q", msg)
	}
}

type fakeNamedSession struct {
	calls   int
	inputs  []onnx.NamedTensor
	outputs map[string]onnx.NamedTensor
	err     error
}

func (f *fakeNamedSession) RunNamed(inputs []onnx.NamedTensor) (map[string]onnx.NamedTensor, error) {
	f.calls++
	f.inputs = inputs
	return f.outputs, f.err
}

func (f *fakeNamedSession) Close() error { return nil }

func TestImageResourcesEmbedSingleRunBatched(t *testing.T) {
	const dim = 3
	plan := imageproc.Plan{Input: "pixel_values", Size: 2, Rescale: 1, Std: [3]float64{1, 1, 1}}
	fake := &fakeNamedSession{
		outputs: map[string]onnx.NamedTensor{
			"image_embeds": {
				Name:  "image_embeds",
				Shape: []int64{2, dim},
				DType: onnx.TensorFloat32,
				Float: []float32{1, 2, 3, 4, 5, 6},
			},
		},
	}
	res := &ImageResources{
		Sessions:     []onnx.NamedSession{fake},
		Plan:         plan,
		OutputTensor: "image_embeds",
		Pooling:      "none",
		Normalize:    false,
		Dim:          dim,
	}

	elem := plan.ElementCount()
	a := make([]float32, elem)
	b := make([]float32, elem)
	got, err := res.Embed([][]float32{a, b})
	if err != nil {
		t.Fatal(err)
	}
	if fake.calls != 1 {
		t.Fatalf("session runs = %d, want exactly 1", fake.calls)
	}
	if len(fake.inputs) != 1 {
		t.Fatalf("expected 1 named input, got %d", len(fake.inputs))
	}
	shape := fake.inputs[0].Shape
	if len(shape) != 4 || shape[0] != 2 || shape[1] != 3 || shape[2] != 2 || shape[3] != 2 {
		t.Fatalf("input shape = %v, want [2 3 2 2]", shape)
	}
	if len(got) != 2 || len(got[0]) != dim*4 || len(got[1]) != dim*4 {
		t.Fatalf("got %d embeddings", len(got))
	}
}

func TestImageResourcesMeanPoolNormalizes(t *testing.T) {
	plan := imageproc.Plan{Input: "pixel_values", Size: 2, Rescale: 1, Std: [3]float64{1, 1, 1}}
	// Two patches of dim 2: [1,1] and [3,3] → mean [2,2], L2-normalized.
	fake := &fakeNamedSession{
		outputs: map[string]onnx.NamedTensor{
			"last_hidden_state": {
				Name:  "last_hidden_state",
				Shape: []int64{1, 2, 2},
				DType: onnx.TensorFloat32,
				Float: []float32{1, 1, 3, 3},
			},
		},
	}
	res := &ImageResources{Sessions: []onnx.NamedSession{fake}, Plan: plan, OutputTensor: "last_hidden_state", Pooling: "mean", Normalize: true, Dim: 2}
	got, err := res.Embed([][]float32{make([]float32, plan.ElementCount())})
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || len(got[0]) != 8 {
		t.Fatalf("got %v", got)
	}
	// sqrt(8) ≈ 2.828427; each component 2/2.828427 ≈ 0.7071.
	first := float32FromLE(got[0][0:4])
	if first < 0.707 || first > 0.708 {
		t.Fatalf("normalized first component = %v, want ~0.7071", first)
	}
}

func float32FromLE(b []byte) float32 {
	return math.Float32frombits(binary.LittleEndian.Uint32(b))
}
