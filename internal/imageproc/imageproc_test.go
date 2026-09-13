package imageproc

import (
	"bytes"
	"encoding/binary"
	"hash/crc32"
	"image"
	"image/color"
	"image/gif"
	"image/jpeg"
	"image/png"
	"os"
	"testing"
)

func solidPNG(t *testing.T, w, h int, c color.Color) []byte {
	t.Helper()
	img := image.NewNRGBA(image.Rect(0, 0, w, h))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			img.Set(x, y, c)
		}
	}
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		t.Fatalf("encoding png: %v", err)
	}
	return buf.Bytes()
}

func TestDecodeFormats(t *testing.T) {
	src := image.NewNRGBA(image.Rect(0, 0, 3, 5))
	for y := 0; y < 5; y++ {
		for x := 0; x < 3; x++ {
			src.Set(x, y, color.NRGBA{R: uint8(x * 60), G: uint8(y * 40), B: 200, A: 255})
		}
	}

	var pngBuf, jpegBuf, gifBuf bytes.Buffer
	if err := png.Encode(&pngBuf, src); err != nil {
		t.Fatal(err)
	}
	if err := jpeg.Encode(&jpegBuf, src, nil); err != nil {
		t.Fatal(err)
	}
	if err := gif.Encode(&gifBuf, src, nil); err != nil {
		t.Fatal(err)
	}
	webpBytes, err := os.ReadFile("testdata/sample.webp")
	if err != nil {
		t.Fatalf("reading webp fixture: %v", err)
	}

	cases := []struct {
		name string
		data []byte
		w, h int
	}{
		{"png", pngBuf.Bytes(), 3, 5},
		{"jpeg", jpegBuf.Bytes(), 3, 5},
		{"gif", gifBuf.Bytes(), 3, 5},
		{"webp", webpBytes, 0, 0}, // dimensions asserted below from the header
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			cfg, format, err := DecodeConfig(tc.data, 0)
			if err != nil {
				t.Fatalf("DecodeConfig: %v", err)
			}
			if format != tc.name {
				t.Fatalf("format = %q, want %q", format, tc.name)
			}
			img, err := Decode(tc.data, 0)
			if err != nil {
				t.Fatalf("Decode: %v", err)
			}
			if img.Bounds().Dx() != cfg.Width || img.Bounds().Dy() != cfg.Height {
				t.Fatalf("decoded %dx%d, header said %dx%d", img.Bounds().Dx(), img.Bounds().Dy(), cfg.Width, cfg.Height)
			}
			if tc.w != 0 && (cfg.Width != tc.w || cfg.Height != tc.h) {
				t.Fatalf("dimensions = %dx%d, want %dx%d", cfg.Width, cfg.Height, tc.w, tc.h)
			}
		})
	}
}

// pngWithDimensions rewrites a valid 1x1 PNG's IHDR dimensions (and its CRC)
// so the header declares w×h without the file containing those pixels. This is
// the hostile-dimension fixture: DecodeConfig must reject it from the header.
func pngWithDimensions(t *testing.T, w, h uint32) []byte {
	t.Helper()
	base := solidPNG(t, 1, 1, color.NRGBA{A: 255})
	data := make([]byte, len(base))
	copy(data, base)
	binary.BigEndian.PutUint32(data[16:20], w)
	binary.BigEndian.PutUint32(data[20:24], h)
	crc := crc32.ChecksumIEEE(data[12:29])
	binary.BigEndian.PutUint32(data[29:33], crc)
	return data
}

func TestDecodePixelCapRejectsHostileHeader(t *testing.T) {
	hostile := pngWithDimensions(t, 100000, 100000)
	const cap = int64(32 * 1024 * 1024)

	if _, _, err := DecodeConfig(hostile, cap); err == nil {
		t.Fatal("DecodeConfig accepted a 10^10-pixel header under a 33.5MP cap")
	}
	if _, err := Decode(hostile, cap); err == nil {
		t.Fatal("Decode accepted a 10^10-pixel header under a 33.5MP cap")
	}

	// Without a cap the header parses but Decode will fail on the truncated
	// pixel stream; the important part is DecodeConfig does not allocate.
	cfg, _, err := DecodeConfig(hostile, 0)
	if err != nil {
		t.Fatalf("uncapped DecodeConfig: %v", err)
	}
	if cfg.Width != 100000 || cfg.Height != 100000 {
		t.Fatalf("header dimensions = %dx%d", cfg.Width, cfg.Height)
	}
}

func TestDecodeRejectsUndecodable(t *testing.T) {
	if _, err := Decode([]byte("not an image"), 0); err == nil {
		t.Fatal("expected error for non-image bytes")
	}
}

func basePlan(size int) Plan {
	return Plan{
		Input:    "pixel_values",
		Size:     size,
		Crop:     CropNone,
		Resample: ResampleBicubic,
		Rescale:  1,
		Mean:     [3]float64{},
		Std:      [3]float64{1, 1, 1},
	}
}

func TestTensorShapeAndChannelOrder(t *testing.T) {
	data := solidPNG(t, 4, 4, color.NRGBA{R: 255, G: 0, B: 0, A: 255})
	plan := basePlan(2)
	tensor, err := plan.Tensor(data)
	if err != nil {
		t.Fatal(err)
	}
	if len(tensor) != 3*2*2 {
		t.Fatalf("len = %d, want 12", len(tensor))
	}
	hw := 4
	for i := 0; i < hw; i++ {
		if tensor[i] != 255 { // R plane
			t.Fatalf("R[%d] = %v, want 255", i, tensor[i])
		}
		if tensor[hw+i] != 0 || tensor[2*hw+i] != 0 {
			t.Fatalf("G/B[%d] = %v/%v, want 0", i, tensor[hw+i], tensor[2*hw+i])
		}
	}
	if got := plan.Shape(); got[0] != 1 || got[1] != 3 || got[2] != 2 || got[3] != 2 {
		t.Fatalf("shape = %v", got)
	}
}

func TestTensorRescaleAndNormalize(t *testing.T) {
	// Mid-gray (128) with rescale 1/255 ≈ 0.50196, mean 0.5, std 0.5.
	data := solidPNG(t, 2, 2, color.NRGBA{R: 128, G: 128, B: 128, A: 255})
	plan := basePlan(2)
	plan.Rescale = 1.0 / 255.0
	plan.Mean = [3]float64{0.5, 0.5, 0.5}
	plan.Std = [3]float64{0.5, 0.5, 0.5}
	tensor, err := plan.Tensor(data)
	if err != nil {
		t.Fatal(err)
	}
	want := float32((128.0/255.0 - 0.5) / 0.5)
	for i, v := range tensor {
		if v != want {
			t.Fatalf("tensor[%d] = %v, want %v", i, v, want)
		}
	}
}

func TestDeterminism(t *testing.T) {
	data := solidPNG(t, 8, 6, color.NRGBA{R: 10, G: 200, B: 30, A: 255})
	plan := basePlan(5)
	plan.Rescale = 1.0 / 255.0
	plan.Mean = [3]float64{0.5, 0.5, 0.5}
	plan.Std = [3]float64{0.5, 0.5, 0.5}

	first, err := plan.Tensor(data)
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 5; i++ {
		again, err := plan.Tensor(data)
		if err != nil {
			t.Fatal(err)
		}
		if len(again) != len(first) {
			t.Fatalf("length changed: %d vs %d", len(again), len(first))
		}
		for j := range first {
			if again[j] != first[j] {
				t.Fatalf("run %d element %d differs: %v vs %v", i, j, again[j], first[j])
			}
		}
	}
}

func TestGrayscaleAndRGBAConversion(t *testing.T) {
	// Grayscale 128 → RGB (128,128,128).
	gray := image.NewGray(image.Rect(0, 0, 3, 3))
	for y := 0; y < 3; y++ {
		for x := 0; x < 3; x++ {
			gray.SetGray(x, y, color.Gray{Y: 128})
		}
	}
	var grayBuf bytes.Buffer
	if err := png.Encode(&grayBuf, gray); err != nil {
		t.Fatal(err)
	}
	plan := basePlan(2)
	tensor, err := plan.Tensor(grayBuf.Bytes())
	if err != nil {
		t.Fatal(err)
	}
	hw := 4
	for i := 0; i < hw; i++ {
		if tensor[i] != 128 || tensor[hw+i] != 128 || tensor[2*hw+i] != 128 {
			t.Fatalf("gray[%d] = %v/%v/%v, want 128", i, tensor[i], tensor[hw+i], tensor[2*hw+i])
		}
	}

	// RGBA with alpha: RGB values are preserved, not premultiplied.
	rgba := image.NewNRGBA(image.Rect(0, 0, 2, 2))
	for y := 0; y < 2; y++ {
		for x := 0; x < 2; x++ {
			rgba.SetNRGBA(x, y, color.NRGBA{R: 200, G: 100, B: 50, A: 64})
		}
	}
	var rgbaBuf bytes.Buffer
	if err := png.Encode(&rgbaBuf, rgba); err != nil {
		t.Fatal(err)
	}
	tensor, err = plan.Tensor(rgbaBuf.Bytes())
	if err != nil {
		t.Fatal(err)
	}
	if tensor[0] != 200 || tensor[hw] != 100 || tensor[2*hw] != 50 {
		t.Fatalf("RGBA RGB = %v/%v/%v, want 200/100/50", tensor[0], tensor[hw], tensor[2*hw])
	}
}

func TestCenterCropShortestEdge(t *testing.T) {
	// 100x50 center-cropped to 20: resize to 40x20 (shortest edge 20), then
	// crop to 20x20.
	plan := basePlan(20)
	plan.Crop = CropCenter
	if w, h := plan.targetSize(100, 50); w != 40 || h != 20 {
		t.Fatalf("targetSize(100,50) = %dx%d, want 40x20", w, h)
	}
	// Portrait: 50x100 → 20x40.
	if w, h := plan.targetSize(50, 100); w != 20 || h != 40 {
		t.Fatalf("targetSize(50,100) = %dx%d, want 20x40", w, h)
	}
	// No crop: always size×size.
	plan.Crop = CropNone
	if w, h := plan.targetSize(100, 50); w != 20 || h != 20 {
		t.Fatalf("targetSize no-crop = %dx%d, want 20x20", w, h)
	}
}

func TestTensorCenterCropProducesSize(t *testing.T) {
	data := solidPNG(t, 100, 50, color.NRGBA{R: 1, G: 2, B: 3, A: 255})
	plan := basePlan(20)
	plan.Crop = CropCenter
	tensor, err := plan.Tensor(data)
	if err != nil {
		t.Fatal(err)
	}
	if len(tensor) != 3*20*20 {
		t.Fatalf("len = %d, want %d", len(tensor), 3*20*20)
	}
}

func TestByteCap(t *testing.T) {
	data := solidPNG(t, 4, 4, color.NRGBA{R: 1, A: 255})
	plan := basePlan(2)
	plan.MaxBytes = int64(len(data) - 1)
	if _, err := plan.Tensor(data); err == nil {
		t.Fatal("expected byte-cap error")
	}
}

func TestPixelCapOnRealImage(t *testing.T) {
	data := solidPNG(t, 20, 20, color.NRGBA{R: 1, A: 255})
	plan := basePlan(2)
	plan.MaxPixels = 100
	if _, err := plan.Tensor(data); err == nil {
		t.Fatal("expected pixel-cap error")
	}
}
