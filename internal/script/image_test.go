package script

import (
	"encoding/binary"
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
			Plan: plan,
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
	hosts := Hosts{Image: &ImageHost{Plan: plan, Preprocess: func([]byte) ([]float32, error) { return nil, nil }}}
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
	hosts := Hosts{Image: &ImageHost{Plan: plan, Preprocess: plan.Tensor}}
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
