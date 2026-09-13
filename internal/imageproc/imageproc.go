// Package imageproc implements deterministic server-side image preprocessing:
// decode (PNG/JPEG/GIF/WebP) → resize → optional center crop → rescale →
// per-channel normalize → a float32 tensor in [1, 3, H, W] RGB channel-first
// layout. It is the host capability behind EMB.IMG and emb.image.preprocess.
//
// Preprocessing is intentionally pure compute (no network, no time): identical
// bytes and configuration always produce byte-identical tensors, which is what
// makes image replies cacheable.
package imageproc

import (
	"bytes"
	"errors"
	"fmt"
	"image"
	"image/color"
	_ "image/gif"
	_ "image/jpeg"
	_ "image/png"
	"math"

	xdraw "golang.org/x/image/draw"
	_ "golang.org/x/image/webp"
)

// CropMode selects how a decoded image is fitted to the configured size.
type CropMode int

const (
	// CropNone resizes directly to size×size (aspect ratio not preserved).
	CropNone CropMode = iota
	// CropCenter resizes the shortest edge to size (preserving aspect ratio)
	// then center-crops to size×size.
	CropCenter
)

func (c CropMode) String() string {
	if c == CropCenter {
		return "center"
	}
	return "none"
}

// ParseCrop resolves a config crop keyword. An empty string defaults to none.
func ParseCrop(s string) (CropMode, error) {
	switch s {
	case "", "none":
		return CropNone, nil
	case "center":
		return CropCenter, nil
	default:
		return CropNone, fmt.Errorf("unknown crop mode %q", s)
	}
}

// Resample selects the resize interpolation.
type Resample int

const (
	ResampleBicubic Resample = iota
	ResampleBilinear
	ResampleNearest
	ResampleLanczos
)

func (r Resample) String() string {
	switch r {
	case ResampleBilinear:
		return "bilinear"
	case ResampleNearest:
		return "nearest"
	case ResampleLanczos:
		return "lanczos"
	default:
		return "bicubic"
	}
}

// ParseResample resolves a config resample keyword. An empty string defaults
// to bicubic.
func ParseResample(s string) (Resample, error) {
	switch s {
	case "", "bicubic":
		return ResampleBicubic, nil
	case "bilinear":
		return ResampleBilinear, nil
	case "nearest":
		return ResampleNearest, nil
	case "lanczos":
		return ResampleLanczos, nil
	default:
		return ResampleBicubic, fmt.Errorf("unknown resample mode %q", s)
	}
}

// Plan is an immutable, fully-resolved preprocessing plan for one model. All
// fields are set at model load; the zero Config is not a valid plan (Size,
// Rescale, Mean and Std must be resolved).
type Plan struct {
	// Input is the ONNX input tensor name the pixel tensor is bound to.
	Input string
	// Size is the target square edge length.
	Size int
	// Crop is the fitting mode.
	Crop CropMode
	// Resample is the resize interpolation.
	Resample Resample
	// Rescale multiplies raw [0,255] pixel values before normalization.
	Rescale float64
	// Mean and Std are the per-channel (RGB) normalization constants.
	Mean [3]float64
	Std  [3]float64
	// MaxBytes bounds one image argument (0 = unlimited), enforced before decode.
	MaxBytes int64
	// MaxPixels bounds the decoded pixel count (0 = unlimited), checked from the
	// image header before the full decode allocates.
	MaxPixels int64
}

// ErrTooLarge reports an image that exceeds the configured byte or pixel cap.
var ErrTooLarge = errors.New("image exceeds size limit")

// DecodeConfig reads the image header without decoding pixels, returning the
// dimensions and format name. It enforces the pixel cap so a hostile header
// cannot force a huge allocation by the subsequent full decode.
func DecodeConfig(data []byte, maxPixels int64) (image.Config, string, error) {
	cfg, format, err := image.DecodeConfig(bytes.NewReader(data))
	if err != nil {
		return image.Config{}, "", fmt.Errorf("decoding image: %w", err)
	}
	if cfg.Width <= 0 || cfg.Height <= 0 {
		return image.Config{}, "", fmt.Errorf("invalid image dimensions %dx%d", cfg.Width, cfg.Height)
	}
	if maxPixels > 0 && int64(cfg.Width)*int64(cfg.Height) > maxPixels {
		return image.Config{}, "", fmt.Errorf("%w: %dx%d (%d pixels) exceeds cap of %d pixels",
			ErrTooLarge, cfg.Width, cfg.Height, int64(cfg.Width)*int64(cfg.Height), maxPixels)
	}
	return cfg, format, nil
}

// Decode fully decodes an image after checking the pixel cap from its header.
// maxPixels <= 0 disables the cap.
func Decode(data []byte, maxPixels int64) (image.Image, error) {
	if _, _, err := DecodeConfig(data, maxPixels); err != nil {
		return nil, err
	}
	img, format, err := image.Decode(bytes.NewReader(data))
	if err != nil {
		return nil, fmt.Errorf("decoding image: %w", err)
	}
	if img.Bounds().Dx() <= 0 || img.Bounds().Dy() <= 0 {
		return nil, fmt.Errorf("decoded %s image has no pixels", format)
	}
	return img, nil
}

// Tensor decodes raw image bytes and returns the channel-first float32 tensor
// (length 3*Size*Size) for this plan. It enforces the plan's byte and pixel
// caps.
func (p Plan) Tensor(data []byte) ([]float32, error) {
	if p.MaxBytes > 0 && int64(len(data)) > p.MaxBytes {
		return nil, fmt.Errorf("%w: %d bytes exceeds cap of %d bytes", ErrTooLarge, len(data), p.MaxBytes)
	}
	img, err := Decode(data, p.MaxPixels)
	if err != nil {
		return nil, err
	}
	return p.TensorFromImage(img), nil
}

// TensorFromImage applies the plan to an already-decoded image. It is
// deterministic: the same image bytes and plan always produce identical
// output.
func (p Plan) TensorFromImage(src image.Image) []float32 {
	img := toNRGBA(src)
	img = p.resize(img)
	if p.Crop == CropCenter {
		img = centerCrop(img, p.Size)
	}
	if img.Rect.Dx() != p.Size || img.Rect.Dy() != p.Size {
		// Defensive: a mis-sized result would panic indexing in normalize.
		img = p.resizeDirect(img, p.Size, p.Size)
	}
	return p.normalize(img)
}

// Shape returns the ONNX shape of the tensor this plan produces.
func (p Plan) Shape() []int64 { return []int64{1, 3, int64(p.Size), int64(p.Size)} }

// ElementCount returns the number of float32 elements a tensor holds.
func (p Plan) ElementCount() int { return 3 * p.Size * p.Size }

// resize fits src to the plan's target dimensions.
func (p Plan) resize(src *image.NRGBA) *image.NRGBA {
	w, h := src.Rect.Dx(), src.Rect.Dy()
	tw, th := p.targetSize(w, h)
	return p.resizeDirect(src, tw, th)
}

// targetSize returns the resized dimensions before any center crop: the
// shortest edge maps to Size (preserving aspect ratio) for center crop, and
// Size×Size directly otherwise.
func (p Plan) targetSize(w, h int) (int, int) {
	if p.Crop != CropCenter {
		return p.Size, p.Size
	}
	shorter, longer := min(w, h), max(w, h)
	newShort := p.Size
	// Match the reference processors' integer truncation for the longer edge.
	newLong := int(float64(newShort) * float64(longer) / float64(shorter))
	if newLong < newShort {
		newLong = newShort
	}
	if w <= h {
		return newShort, newLong
	}
	return newLong, newShort
}

func (p Plan) resizeDirect(src *image.NRGBA, w, h int) *image.NRGBA {
	dst := image.NewNRGBA(image.Rect(0, 0, w, h))
	p.interpolator().Scale(dst, dst.Bounds(), src, src.Bounds(), xdraw.Src, nil)
	return dst
}

func (p Plan) interpolator() xdraw.Interpolator {
	switch p.Resample {
	case ResampleNearest:
		return xdraw.NearestNeighbor
	case ResampleBilinear:
		return xdraw.BiLinear
	case ResampleLanczos:
		return lanczosInterpolator
	default:
		return xdraw.CatmullRom
	}
}

// lanczosInterpolator is a standard 3-lobe Lanczos kernel (the same window
// PIL's Image.LANCZOS uses in spirit), built on x/image/draw's kernel scaler.
var lanczosInterpolator = &xdraw.Kernel{
	Support: 3,
	At: func(t float64) float64 {
		if t == 0 {
			return 1
		}
		x := math.Pi * t
		return 3 * math.Sin(x) * math.Sin(x/3) / (x * x)
	},
}

// centerCrop returns the centered size×size region of img.
func centerCrop(img *image.NRGBA, size int) *image.NRGBA {
	w, h := img.Rect.Dx(), img.Rect.Dy()
	left := (w - size) / 2
	top := (h - size) / 2
	dst := image.NewNRGBA(image.Rect(0, 0, size, size))
	for y := 0; y < size; y++ {
		copy(
			dst.Pix[y*dst.Stride:(y+1)*dst.Stride],
			img.Pix[(top+y)*img.Stride+left*4:(top+y)*img.Stride+left*4+size*4],
		)
	}
	return dst
}

// normalize converts straight-alpha NRGBA pixels to a channel-first
// [3, size, size] float32 tensor, applying rescale then per-channel
// (value-mean)/std normalization.
func (p Plan) normalize(img *image.NRGBA) []float32 {
	size := p.Size
	hw := size * size
	out := make([]float32, 3*hw)
	for y := 0; y < size; y++ {
		row := y * img.Stride
		base := y * size
		for x := 0; x < size; x++ {
			i := row + x*4
			j := base + x
			out[j] = normalizeChannel(img.Pix[i], 0, p)
			out[hw+j] = normalizeChannel(img.Pix[i+1], 1, p)
			out[2*hw+j] = normalizeChannel(img.Pix[i+2], 2, p)
		}
	}
	return out
}

func normalizeChannel(v uint8, channel int, p Plan) float32 {
	x := float64(v) * p.Rescale
	std := p.Std[channel]
	if std == 0 {
		std = 1
	}
	return float32((x - p.Mean[channel]) / std)
}

// toNRGBA converts any image to a straight-alpha NRGBA with a zero-origin
// rect, preserving RGB values (alpha is carried but ignored by normalization).
// An already-converted NRGBA is returned as-is.
func toNRGBA(src image.Image) *image.NRGBA {
	if n, ok := src.(*image.NRGBA); ok && n.Rect.Min == (image.Point{}) {
		return n
	}
	b := src.Bounds()
	dst := image.NewNRGBA(image.Rect(0, 0, b.Dx(), b.Dy()))
	for y := 0; y < b.Dy(); y++ {
		row := y * dst.Stride
		for x := 0; x < b.Dx(); x++ {
			c := color.NRGBAModel.Convert(src.At(b.Min.X+x, b.Min.Y+y)).(color.NRGBA)
			dst.Pix[row+x*4] = c.R
			dst.Pix[row+x*4+1] = c.G
			dst.Pix[row+x*4+2] = c.B
			dst.Pix[row+x*4+3] = c.A
		}
	}
	return dst
}
