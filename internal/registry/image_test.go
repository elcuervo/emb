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
	plan, _, err := resolveImagePlan(cfg, "clip")
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
	plan, _, err := resolveImagePlan(cfg, "siglip")
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
	plan, _, err := resolveImagePlan(cfg, "detected")
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
	plan, _, err = resolveImagePlan(cfg, "override")
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
	_, _, err := resolveImagePlan(cfg, "no-size")
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
			_, _, err := resolveImagePlan(config.ModelConfig{ONNX: onnxPath, Image: tc.img}, tc.name)
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
	plan, _, err := resolveImagePlan(cfg, "m")
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
	outputs := []ImageOutput{{Name: "embedding", Tensor: "image_embeds", Select: "none", Rows: 1, Dim: dim}}
	res := &ImageResources{
		Sessions:  []onnx.NamedSession{fake},
		Plan:      plan,
		Outputs:   outputs,
		Normalize: false,
	}

	elem := plan.ElementCount()
	a := make([]float32, elem)
	b := make([]float32, elem)
	got, err := res.Embed([][]float32{a, b}, outputs)
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
	if len(got) != 2 || len(got[0]["embedding"]) != dim*4 || len(got[1]["embedding"]) != dim*4 {
		t.Fatalf("got %d results", len(got))
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
	outputs := []ImageOutput{{Name: "embedding", Tensor: "last_hidden_state", Select: "mean", Rows: 1, Dim: 2}}
	res := &ImageResources{Sessions: []onnx.NamedSession{fake}, Plan: plan, Outputs: outputs, Normalize: true}
	got, err := res.Embed([][]float32{make([]float32, plan.ElementCount())}, outputs)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || len(got[0]["embedding"]) != 8 {
		t.Fatalf("got %v", got)
	}
	// sqrt(8) ≈ 2.828427; each component 2/2.828427 ≈ 0.7071.
	first := float32FromLE(got[0]["embedding"][0:4])
	if first < 0.707 || first > 0.708 {
		t.Fatalf("normalized first component = %v, want ~0.7071", first)
	}
}

func float32FromLE(b []byte) float32 {
	return math.Float32frombits(binary.LittleEndian.Uint32(b))
}

// TestStaticBatchCap covers the graph-declared batch axis: a positive value
// caps a chunk, a symbolic axis leaves it unbounded.
func TestStaticBatchCap(t *testing.T) {
	cases := []struct {
		name  string
		infos []onnx.InputInfo
		input string
		want  int
	}{
		{"static batch 1", []onnx.InputInfo{{Name: "input", Rank: 4, Dimensions: []int64{1, 3, 336, 336}}}, "input", 1},
		{"static batch 4", []onnx.InputInfo{{Name: "pixel_values", Rank: 4, Dimensions: []int64{4, 3, 224, 224}}}, "pixel_values", 4},
		{"symbolic batch", []onnx.InputInfo{{Name: "pixel_values", Rank: 4, Dimensions: []int64{-1, 3, -1, -1}}}, "pixel_values", 0},
		{"input not in graph", []onnx.InputInfo{{Name: "input_ids", Rank: 2, Dimensions: []int64{1, 128}}}, "pixel_values", 0},
	}
	for _, c := range cases {
		if got := staticBatchCap(c.infos, c.input); got != c.want {
			t.Errorf("%s: cap = %d, want %d", c.name, got, c.want)
		}
	}
}

// imageOutputFixture builds a config with one declared output, so output
// resolution tests state only the fields they exercise.
func imageOutputFixture(outputs ...config.ImageOutputConfig) config.ModelConfig {
	return config.ModelConfig{
		Dim:   4,
		Image: &config.ImageConfig{Input: "pixel_values", Size: 32, Outputs: outputs},
	}
}

// dinov3OutputInfo is the pair of tensors a DINOv3-family export publishes.
func dinov3OutputInfo() map[string]onnx.OutputInfo {
	return map[string]onnx.OutputInfo{
		"last_hidden_state": {Name: "last_hidden_state", Rank: 3, Dim: 4},
		"pooler_output":     {Name: "pooler_output", Rank: 2, Dim: 4},
	}
}

// TestResolveImageOutputs covers the load-time output rules: a missing tensor, a
// rank that contradicts the selector, a duplicate name, an unknown selector, and
// a grid that cannot divide the configured size.
func TestResolveImageOutputs(t *testing.T) {
	cases := []struct {
		name string
		outs []config.ImageOutputConfig
		info map[string]onnx.OutputInfo
		md   modelImageMetadata
		want string
	}{
		{
			name: "missing tensor",
			outs: []config.ImageOutputConfig{{Name: "embedding", Tensor: "nope"}},
			want: `tensor "nope" not found`,
		},
		{
			name: "tokens on a rank-2 tensor",
			outs: []config.ImageOutputConfig{{Name: "embedding", Tensor: "pooler_output", Select: "tokens", Grid: 2}},
			want: "selects tokens from a rank-2 tensor",
		},
		{
			name: "none on a rank-3 tensor",
			outs: []config.ImageOutputConfig{{Name: "embedding", Tensor: "last_hidden_state", Select: "none"}},
			want: "selects none from a rank-3 tensor",
		},
		{
			name: "duplicate name",
			outs: []config.ImageOutputConfig{
				{Name: "embedding", Tensor: "pooler_output"},
				{Name: "embedding", Tensor: "last_hidden_state", Select: "cls"},
			},
			want: `duplicate image output name "embedding"`,
		},
		{
			name: "unknown select",
			outs: []config.ImageOutputConfig{{Name: "embedding", Tensor: "pooler_output", Select: "pool"}},
			want: `unknown select "pool"`,
		},
		{
			name: "grid does not divide size",
			outs: []config.ImageOutputConfig{{Name: "patches", Tensor: "last_hidden_state", Select: "tokens", Grid: 7}},
			want: "does not divide image.size",
		},
		{
			name: "no outputs",
			want: "image.outputs must declare at least one named output",
		},
		{
			name: "tokens without any grid source",
			outs: []config.ImageOutputConfig{{Name: "patches", Tensor: "last_hidden_state", Select: "tokens"}},
			md:   modelImageMetadata{}, // no patch_size to fall back on
			want: "has no grid",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			info := tc.info
			if info == nil {
				info = dinov3OutputInfo()
			}
			cfg := imageOutputFixture(tc.outs...)
			_, err := resolveImageOutputs(cfg, "m", 32, imagePreset{}, false, info, tc.md)
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("error = %v, want it to mention %q", err, tc.want)
			}
		})
	}
}

// TestResolveImageOutputsLayout covers the resolved shape of both dialects: a
// pooled vector from a rank-2 tensor and a token grid whose start/grid come from
// the model's own config.json, with explicit values winning.
func TestResolveImageOutputsLayout(t *testing.T) {
	cfg := imageOutputFixture(
		config.ImageOutputConfig{Name: "embedding", Tensor: "pooler_output"},
		config.ImageOutputConfig{Name: "patches", Tensor: "last_hidden_state", Select: "tokens"},
	)
	md := modelImageMetadata{PatchSize: 16, NumRegisterTokens: 4}
	outs, err := resolveImageOutputs(cfg, "m", 32, imagePreset{}, false, dinov3OutputInfo(), md)
	if err != nil {
		t.Fatal(err)
	}
	if len(outs) != 2 {
		t.Fatalf("got %d outputs, want 2", len(outs))
	}
	if outs[0].Name != "embedding" || outs[0].Select != "none" || outs[0].Rows != 1 || outs[0].Dim != 4 {
		t.Fatalf("pooled output = %+v", outs[0])
	}
	p := outs[1]
	// size 32 / patch_size 16 = a 2x2 grid; start = 1 CLS + 4 registers.
	if p.Select != "tokens" || p.Start != 5 || p.Grid != 2 || p.Rows != 4 {
		t.Fatalf("token output = %+v, want start 5 grid 2 rows 4", p)
	}

	// Explicit start and grid win over the model metadata.
	start := 1
	cfg.Image.Outputs[1].Start = &start
	cfg.Image.Outputs[1].Grid = 4
	outs, err = resolveImageOutputs(cfg, "m", 32, imagePreset{}, false, dinov3OutputInfo(), md)
	if err != nil {
		t.Fatal(err)
	}
	if got := outs[1]; got.Start != 1 || got.Grid != 4 || got.Rows != 16 {
		t.Fatalf("explicit layout = %+v, want start 1 grid 4 rows 16", got)
	}
}

// TestLoadModelImageMetadata reads the layout a DINOv3 export declares about
// itself, beside the graph or one directory up.
func TestLoadModelImageMetadata(t *testing.T) {
	dir := t.TempDir()
	sub := filepath.Join(dir, "onnx")
	if err := os.MkdirAll(sub, 0o755); err != nil {
		t.Fatal(err)
	}
	graph := filepath.Join(sub, "model.onnx")
	if err := os.WriteFile(graph, []byte("dummy"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "config.json"),
		[]byte(`{"patch_size": 16, "num_register_tokens": 4, "hidden_size": 384}`), 0o644); err != nil {
		t.Fatal(err)
	}
	md := loadModelImageMetadata(graph)
	if md.PatchSize != 16 || md.NumRegisterTokens != 4 {
		t.Fatalf("metadata = %+v, want patch_size 16 registers 4", md)
	}

	// A graph with no config.json yields the zero metadata rather than failing.
	md = loadModelImageMetadata(filepath.Join(t.TempDir(), "model.onnx"))
	if md.PatchSize != 0 || md.NumRegisterTokens != 0 {
		t.Fatalf("metadata without config.json = %+v, want zero", md)
	}
}

// TestImagePresets covers each family profile: the constants it supplies and
// the fact that an explicit key still wins over it.
func TestImagePresets(t *testing.T) {
	cases := []struct {
		preset   string
		mean     [3]float64
		std      [3]float64
		rescale  float64
		resample imageproc.Resample
		crop     imageproc.CropMode
	}{
		{"clip", [3]float64{0.48145466, 0.4578275, 0.40821073}, [3]float64{0.26862954, 0.26130258, 0.27577711}, 1.0 / 255.0, imageproc.ResampleBicubic, imageproc.CropCenter},
		{"siglip", [3]float64{0.5, 0.5, 0.5}, [3]float64{0.5, 0.5, 0.5}, 1.0 / 255.0, imageproc.ResampleBicubic, imageproc.CropNone},
		{"imagenet", [3]float64{0.485, 0.456, 0.406}, [3]float64{0.229, 0.224, 0.225}, 1.0 / 255.0, imageproc.ResampleBicubic, imageproc.CropNone},
		{"dinov3", [3]float64{0.485, 0.456, 0.406}, [3]float64{0.229, 0.224, 0.225}, 1.0 / 255.0, imageproc.ResampleBilinear, imageproc.CropNone},
	}
	for _, c := range cases {
		t.Run(c.preset, func(t *testing.T) {
			cfg := config.ModelConfig{Image: &config.ImageConfig{Input: "input", Size: 32, Preset: c.preset}}
			plan, src, err := resolveImagePlan(cfg, "m")
			if err != nil {
				t.Fatal(err)
			}
			if plan.Mean != c.mean || plan.Std != c.std || plan.Rescale != c.rescale ||
				plan.Resample != c.resample || plan.Crop != c.crop {
				t.Fatalf("%s plan = %+v", c.preset, plan)
			}
			if src.Mean != "preset:"+c.preset || src.Crop != "preset:"+c.preset {
				t.Fatalf("%s sources = %+v, want the preset named", c.preset, src)
			}
		})
	}

	// An explicit key overrides the preset field it names, leaving the rest.
	cfg := config.ModelConfig{Image: &config.ImageConfig{
		Input: "input", Size: 32, Preset: "dinov3", Resample: "bicubic", Crop: "center",
	}}
	plan, src, err := resolveImagePlan(cfg, "m")
	if err != nil {
		t.Fatal(err)
	}
	if plan.Resample != imageproc.ResampleBicubic || plan.Crop != imageproc.CropCenter {
		t.Fatalf("explicit keys did not win: %+v", plan)
	}
	if plan.Mean != [3]float64{0.485, 0.456, 0.406} {
		t.Fatalf("preset mean lost: %v", plan.Mean)
	}
	if src.Resample != "explicit" || src.Crop != "explicit" || src.Mean != "preset:dinov3" {
		t.Fatalf("sources = %+v", src)
	}

	// An unknown preset is a load error naming the valid ones.
	bad := config.ModelConfig{Image: &config.ImageConfig{Input: "input", Size: 32, Preset: "nope"}}
	if _, _, err := resolveImagePlan(bad, "m"); err == nil || !strings.Contains(err.Error(), "clip, siglip, imagenet, dinov3") {
		t.Fatalf("unknown preset error = %v", err)
	}
}

// TestResolveImageOutputsPairing covers the dual-encoder rule applied to every
// output: one output disagreeing with the text dimension fails the load naming
// that output.
func TestResolveImageOutputsPairing(t *testing.T) {
	info := map[string]onnx.OutputInfo{
		"pooler_output":     {Name: "pooler_output", Rank: 2, Dim: 4},
		"last_hidden_state": {Name: "last_hidden_state", Rank: 3, Dim: 8},
	}
	cfg := imageOutputFixture(
		config.ImageOutputConfig{Name: "embedding", Tensor: "pooler_output"},
		config.ImageOutputConfig{Name: "patches", Tensor: "last_hidden_state", Select: "cls"},
	)
	_, err := resolveImageOutputs(cfg, "m", 32, imagePreset{}, false, info, modelImageMetadata{})
	if err == nil {
		t.Fatal("expected a pairing error for the 8-dimensional output")
	}
	for _, want := range []string{"patches", "4", "8", "pairing"} {
		if !strings.Contains(err.Error(), want) {
			t.Fatalf("error %q should name %q", err, want)
		}
	}
}

// TestImageResourcesTokenGridValidation covers the run-time row check: a token
// tensor whose row count disagrees with the configured start and grid is refused
// naming both counts, and no partial grid is returned.
func TestImageResourcesTokenGridValidation(t *testing.T) {
	plan := imageproc.Plan{Input: "pixel_values", Size: 2, Rescale: 1, Std: [3]float64{1, 1, 1}}
	outputs := []ImageOutput{{Name: "patches", Tensor: "last_hidden_state", Select: "tokens", Start: 1, Grid: 2, Rows: 4, Dim: 2}}

	// 5 rows per image, start 1 → 4 rows, which matches this grid.
	ok := &fakeNamedSession{outputs: map[string]onnx.NamedTensor{
		"last_hidden_state": {
			Name:  "last_hidden_state",
			Shape: []int64{1, 5, 2},
			DType: onnx.TensorFloat32,
			Float: []float32{9, 9, 1, 2, 3, 4, 5, 6, 7, 8},
		},
	}}
	res := &ImageResources{Sessions: []onnx.NamedSession{ok}, Plan: plan, Outputs: outputs, Normalize: false}
	got, err := res.Embed([][]float32{make([]float32, plan.ElementCount())}, outputs)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || len(got[0]["patches"]) != 4*2*4 {
		t.Fatalf("grid blob = %d bytes, want 4 rows * 2 dims * 4", len(got[0]["patches"]))
	}

	// A short sequence must be refused instead of returning a partial grid.
	short := &fakeNamedSession{outputs: map[string]onnx.NamedTensor{
		"last_hidden_state": {
			Name:  "last_hidden_state",
			Shape: []int64{1, 4, 2},
			DType: onnx.TensorFloat32,
			Float: make([]float32, 8),
		},
	}}
	res = &ImageResources{Sessions: []onnx.NamedSession{short}, Plan: plan, Outputs: outputs}
	_, err = res.Embed([][]float32{make([]float32, plan.ElementCount())}, outputs)
	if err == nil {
		t.Fatal("expected a row-count refusal")
	}
	for _, want := range []string{"3 rows", "want 4"} {
		if !strings.Contains(err.Error(), want) {
			t.Fatalf("error %q should name %q", err, want)
		}
	}
}
