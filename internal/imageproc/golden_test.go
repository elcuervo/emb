package imageproc

import (
	"encoding/binary"
	"math"
	"os"
	"testing"
)

// goldenTolerance is the maximum per-element absolute difference allowed
// between the Go pipeline and the Pillow/numpy reference. The two use different
// (but both cubic/bilinear) resize kernels and edge handling, so exact
// agreement is not expected; this is a parity check that catches channel-order,
// normalization, and crop mistakes. See testdata/generate_golden.py.
const goldenTolerance = 0.05

func loadGolden(t *testing.T, path string) []float32 {
	t.Helper()
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("reading %s: %v", path, err)
	}
	if len(raw)%4 != 0 {
		t.Fatalf("%s length %d is not a multiple of 4", path, len(raw))
	}
	out := make([]float32, len(raw)/4)
	for i := range out {
		out[i] = math.Float32frombits(binary.LittleEndian.Uint32(raw[i*4:]))
	}
	return out
}

func compareGolden(t *testing.T, got, want []float32) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("length mismatch: got %d, want %d", len(got), len(want))
	}
	maxDiff, sumDiff, argMax := 0.0, 0.0, 0
	for i := range got {
		d := math.Abs(float64(got[i] - want[i]))
		sumDiff += d
		if d > maxDiff {
			maxDiff, argMax = d, i
		}
	}
	meanDiff := sumDiff / float64(len(got))
	if maxDiff > goldenTolerance {
		t.Fatalf("golden mismatch: max |diff| = %.5f at %d (got %v, want %v), mean = %.5f, tolerance = %.5f",
			maxDiff, argMax, got[argMax], want[argMax], meanDiff, goldenTolerance)
	}
	t.Logf("golden parity ok: max |diff| = %.5f, mean |diff| = %.5f (tolerance %.5f)", maxDiff, meanDiff, goldenTolerance)
}

func TestGoldenCLIPPreprocessing(t *testing.T) {
	input, err := os.ReadFile("testdata/golden_input.png")
	if err != nil {
		t.Fatalf("reading golden input: %v", err)
	}
	want := loadGolden(t, "testdata/golden_clip.f32")

	plan := Plan{
		Input:    "pixel_values",
		Size:     16,
		Crop:     CropCenter,
		Resample: ResampleBicubic,
		Rescale:  1.0 / 255.0,
		Mean:     [3]float64{0.48145466, 0.4578275, 0.40821073},
		Std:      [3]float64{0.26862954, 0.26130258, 0.27577711},
	}
	got, err := plan.Tensor(input)
	if err != nil {
		t.Fatal(err)
	}
	compareGolden(t, got, want)
}

func TestGoldenSigLIPPreprocessing(t *testing.T) {
	input, err := os.ReadFile("testdata/golden_input.png")
	if err != nil {
		t.Fatalf("reading golden input: %v", err)
	}
	want := loadGolden(t, "testdata/golden_siglip.f32")

	plan := Plan{
		Input:    "pixel_values",
		Size:     16,
		Crop:     CropNone,
		Resample: ResampleBilinear,
		Rescale:  1.0 / 255.0,
		Mean:     [3]float64{0.5, 0.5, 0.5},
		Std:      [3]float64{0.5, 0.5, 0.5},
	}
	got, err := plan.Tensor(input)
	if err != nil {
		t.Fatal(err)
	}
	compareGolden(t, got, want)
}
