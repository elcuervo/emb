package script

import (
	"encoding/binary"
	"errors"
	"strings"
	"testing"

	"github.com/elcuervo/emb/internal/imageproc"
	"github.com/elcuervo/emb/internal/onnx"
)

func testImagePlan(size int) imageproc.Plan {
	return imageproc.Plan{
		Input:    "pixel_values",
		Size:     size,
		Crop:     imageproc.CropCenter,
		Resample: imageproc.ResampleBicubic,
		Rescale:  1.0 / 255.0,
		Mean:     [3]float64{0.5, 0.5, 0.5},
		Std:      [3]float64{0.5, 0.5, 0.5},
	}
}

func TestImagePreprocessHostFeedsRun(t *testing.T) {
	plan := testImagePlan(2)
	want := make([]float32, plan.ElementCount())
	for i := range want {
		want[i] = float32(i) * 0.25
	}

	var got onnx.NamedTensor
	hosts := Hosts{
		Run: func(inputs []onnx.NamedTensor) (map[string]onnx.NamedTensor, error) {
			got = inputs[0]
			return map[string]onnx.NamedTensor{
				"image_embeds": {Name: "image_embeds", Shape: []int64{1, 3}, DType: onnx.TensorFloat32, Float: []float32{7, 8, 9}},
			}, nil
		},
		Image: &ImageHost{
			Plan: func() (imageproc.Plan, error) { return plan, nil },
			Preprocess: func(data []byte) ([]float32, error) {
				if string(data) != "fake-image-bytes" {
					t.Fatalf("preprocess received %q", data)
				}
				return want, nil
			},
		},
	}

	src := `
local spec = emb.image.preprocess(KEYS[1])
local out = emb.run({[spec.input] = {shape = spec.shape, bytes = spec.bytes, dtype = spec.dtype}})
return out.image_embeds.data[1]
`
	v, err := EvalWithHosts(src, []string{"fake-image-bytes"}, nil, hosts, EvalOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if v.String() != "7" {
		t.Fatalf("script result = %v, want 7", v)
	}
	if got.Name != "pixel_values" {
		t.Fatalf("run input name = %q", got.Name)
	}
	if len(got.Shape) != 4 || got.Shape[0] != 1 || got.Shape[1] != 3 || got.Shape[2] != 2 || got.Shape[3] != 2 {
		t.Fatalf("run input shape = %v, want [1 3 2 2]", got.Shape)
	}
	if got.DType != onnx.TensorFloat32 || len(got.Float) != len(want) {
		t.Fatalf("run input dtype/len = %v/%d", got.DType, len(got.Float))
	}
	for i := range want {
		if got.Float[i] != want[i] {
			t.Fatalf("run input[%d] = %v, want %v", i, got.Float[i], want[i])
		}
	}
}

func TestImageInfoHost(t *testing.T) {
	plan := testImagePlan(4)
	hosts := Hosts{Image: &ImageHost{Plan: func() (imageproc.Plan, error) { return plan, nil }, Preprocess: func([]byte) ([]float32, error) { return nil, nil }}}
	v, err := EvalWithHosts(`local i = emb.image.info(); return i.input .. ":" .. i.size .. ":" .. i.crop .. ":" .. i.resample`, nil, nil, hosts, EvalOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if v.String() != "pixel_values:4:center:bicubic" {
		t.Fatalf("emb.image.info = %q", v.String())
	}
}

func TestImageHostAbsentWithoutImageBlock(t *testing.T) {
	_, err := EvalWithHosts(`return emb.image.preprocess("x")`, nil, nil, Hosts{}, EvalOptions{})
	if err == nil || !strings.Contains(err.Error(), "preprocess") {
		t.Fatalf("expected emb.image error for a text-only model, got %v", err)
	}
}

func TestImagePreprocessRespectsCaps(t *testing.T) {
	plan := testImagePlan(2)
	plan.MaxBytes = 3
	hosts := Hosts{Image: &ImageHost{Plan: func() (imageproc.Plan, error) { return plan, nil }, Preprocess: plan.Tensor}}
	_, err := EvalWithHosts(`return emb.image.preprocess("way too long to be an image")`, nil, nil, hosts, EvalOptions{})
	if err == nil || !strings.Contains(err.Error(), "limit") {
		t.Fatalf("expected byte-cap error, got %v", err)
	}
}

func TestRunPackedBytesFloatRoundTrip(t *testing.T) {
	var got onnx.NamedTensor
	hosts := Hosts{
		Run: func(inputs []onnx.NamedTensor) (map[string]onnx.NamedTensor, error) {
			got = inputs[0]
			return map[string]onnx.NamedTensor{
				"out": {Name: "out", Shape: []int64{1}, DType: onnx.TensorFloat32, Float: []float32{1}},
			}, nil
		},
	}
	src := `
local packed = emb.math.float32_bytes({1.5, -2.5, 3.25})
local out = emb.run({x = {shape = {3}, bytes = packed, dtype = "f32"}})
return out.out.data[1]
`
	if _, err := EvalWithHosts(src, nil, nil, hosts, EvalOptions{}); err != nil {
		t.Fatal(err)
	}
	if len(got.Float) != 3 || got.Float[0] != 1.5 || got.Float[1] != -2.5 || got.Float[2] != 3.25 {
		t.Fatalf("packed float tensor = %v", got.Float)
	}
	if got.DType != onnx.TensorFloat32 {
		t.Fatalf("dtype = %v, want f32", got.DType)
	}
}

func TestRunPackedBytesInt64(t *testing.T) {
	var got onnx.NamedTensor
	hosts := Hosts{
		Run: func(inputs []onnx.NamedTensor) (map[string]onnx.NamedTensor, error) {
			got = inputs[0]
			return map[string]onnx.NamedTensor{
				"out": {Name: "out", Shape: []int64{1}, DType: onnx.TensorFloat32, Float: []float32{1}},
			}, nil
		},
	}
	var raw []byte
	for _, v := range []int64{101, -7} {
		raw = binary.LittleEndian.AppendUint64(raw, uint64(v))
	}
	// Drive the bytes form through a KEYS element, since a Lua source literal
	// cannot safely hold NUL bytes.
	src := `local out = emb.run({ids = {shape = {2}, bytes = KEYS[1], dtype = "i64"}}); return 1`
	if _, err := EvalWithHosts(src, []string{string(raw)}, nil, hosts, EvalOptions{}); err != nil {
		t.Fatal(err)
	}
	if got.DType != onnx.TensorInt64 || len(got.Int64) != 2 || got.Int64[0] != 101 || got.Int64[1] != -7 {
		t.Fatalf("packed int64 tensor = %v (%v)", got.Int64, got.DType)
	}
}

func TestRunPackedBytesLengthMismatch(t *testing.T) {
	hosts := Hosts{Run: func([]onnx.NamedTensor) (map[string]onnx.NamedTensor, error) { return nil, nil }}
	_, err := EvalWithHosts(`return emb.run({x = {shape = {2}, bytes = "abcde", dtype = "f32"}})`, nil, nil, hosts, EvalOptions{})
	if err == nil || !strings.Contains(err.Error(), "does not match") {
		t.Fatalf("expected length-mismatch error, got %v", err)
	}
}

func TestRunPackedBytesMutuallyExclusive(t *testing.T) {
	hosts := Hosts{Run: func([]onnx.NamedTensor) (map[string]onnx.NamedTensor, error) { return nil, nil }}
	if _, err := EvalWithHosts(`return emb.run({x = {shape = {1}, data = {1}, bytes = "AAAA", dtype = "f32"}})`, nil, nil, hosts, EvalOptions{}); err == nil || !strings.Contains(err.Error(), "mutually exclusive") {
		t.Fatalf("data+bytes should error, got %v", err)
	}
	if _, err := EvalWithHosts(`return emb.run({x = {shape = {1}}})`, nil, nil, hosts, EvalOptions{}); err == nil || !strings.Contains(err.Error(), "exactly one") {
		t.Fatalf("missing data/fill/bytes should error, got %v", err)
	}
	if _, err := EvalWithHosts(`return emb.run({x = {shape = {1}, bytes = "AAAA"}})`, nil, nil, hosts, EvalOptions{}); err == nil || !strings.Contains(err.Error(), "dtype") {
		t.Fatalf("bytes without dtype should error, got %v", err)
	}
}

// scriptImageHost serves two outputs for emb.image.embed tests: a pooled vector
// and a 2x2 token grid, both on a dim-2 model.
func scriptImageHost(t *testing.T) *ImageHost {
	t.Helper()
	return &ImageHost{
		Plan:       func() (imageproc.Plan, error) { return testImagePlan(2), nil },
		Preprocess: func([]byte) ([]float32, error) { return nil, nil },
		Outputs: func() ([]ImageOutputInfo, error) {
			return []ImageOutputInfo{
				{Name: "embedding", Tensor: "pooler_output", Select: "none", Rows: 1, Dim: 2},
				{Name: "patches", Tensor: "last_hidden_state", Select: "tokens", Rows: 4, Grid: 2, Dim: 2},
			}, nil
		},
		Embed: func(images [][]byte, outputs []string) ([]map[string][]byte, error) {
			want := outputs
			if len(want) == 0 {
				want = []string{"embedding", "patches"}
			}
			out := make([]map[string][]byte, len(images))
			for i := range images {
				values := make(map[string][]byte, len(want))
				for _, name := range want {
					switch name {
					case "embedding":
						values[name] = make([]byte, 2*4)
					case "patches":
						values[name] = make([]byte, 4*2*4)
					default:
						return nil, errors.New(`unknown image output "` + name + `"; configured outputs: embedding, patches`)
					}
				}
				out[i] = values
			}
			return out, nil
		},
	}
}

// TestImageEmbedKeyedTable covers emb.image.embed's keyed result: every
// configured output under its own key, one value when narrowed, and an error
// listing the configured outputs for an unknown name.
func TestImageEmbedKeyedTable(t *testing.T) {
	hosts := Hosts{Image: scriptImageHost(t)}

	v, err := EvalWithHosts(`
local e = emb.image.embed("img")
local n = 0
for name, vec in pairs(e) do n = n + #vec end
return tostring(n) .. ":" .. tostring(#e.embedding) .. ":" .. tostring(#e.patches)`,
		nil, nil, hosts, EvalOptions{})
	if err != nil {
		t.Fatal(err)
	}
	// 2 pooled values + 4*2 patch values, keyed so both are addressable.
	if v.String() != "10:2:8" {
		t.Fatalf("keyed table = %q, want 10 values across a 2-vector and an 8-value grid", v.String())
	}

	// The narrowed form returns that one value, not a table.
	v, err = EvalWithHosts(`local e = emb.image.embed("img", {output = "patches"}); return tostring(type(e)) .. ":" .. tostring(#e)`,
		nil, nil, hosts, EvalOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if v.String() != "table:8" {
		t.Fatalf("narrowed form = %q, want a table of 8 grid values", v.String())
	}

	// An array of images yields one keyed table per image.
	v, err = EvalWithHosts(`local e = emb.image.embed({"img", "img2"}); return tostring(#e) .. ":" .. tostring(#e[1].patches) .. ":" .. tostring(#e[2].embedding)`,
		nil, nil, hosts, EvalOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if v.String() != "2:8:2" {
		t.Fatalf("array form = %q, want two keyed tables", v.String())
	}

	// An unknown output name raises an error listing the configured ones.
	_, err = EvalWithHosts(`return emb.image.embed("img", {output = "nope"})`, nil, nil, hosts, EvalOptions{})
	if err == nil {
		t.Fatal("unknown output name accepted")
	}
	for _, want := range []string{"nope", "embedding", "patches"} {
		if !strings.Contains(err.Error(), want) {
			t.Fatalf("error %q should mention %q", err, want)
		}
	}
}

// TestImageInfoReportsOutputs covers emb.image.info's output list: name, tensor,
// select, rows, dim, and grid for a token output.
func TestImageInfoReportsOutputs(t *testing.T) {
	hosts := Hosts{Image: scriptImageHost(t)}
	v, err := EvalWithHosts(`
local i = emb.image.info()
return tostring(#i.outputs) .. ":" .. i.outputs[1].name .. ":" .. i.outputs[1].tensor
    .. ":" .. i.outputs[2].select .. ":" .. tostring(i.outputs[2].rows) .. ":" .. tostring(i.outputs[2].grid)`,
		nil, nil, hosts, EvalOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if v.String() != "2:embedding:pooler_output:tokens:4:2" {
		t.Fatalf("emb.image.info outputs = %q", v.String())
	}
}
