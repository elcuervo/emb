package server

import (
	"bytes"
	"fmt"
	"image"
	"image/color"
	"image/gif"
	"image/png"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/elcuervo/emb/internal/config"
	"github.com/elcuervo/emb/internal/imageproc"
	"github.com/elcuervo/emb/internal/onnx"
	"github.com/elcuervo/emb/internal/registry"
)

// fakeImageSession is a named-tensor session that returns a deterministic
// [batch, dim] output and records the batch shape it was asked to run.
type fakeImageSession struct {
	mu        sync.Mutex
	calls     int
	lastShape []int64
	shapes    [][]int64
	dim       int
	err       error
	// outBatch, when > 0, overrides the batch dimension of the returned outputs
	// (shape[0] and row count) so a test can model a graph whose output batch
	// disagrees with the images it was run with. 0 returns the requested batch.
	outBatch int
	// tensors, when set, replaces the default single [batch, dim] tensor: each
	// spec is emitted with the requested batch and rows per image, so a test can
	// model a graph with a pooled vector and a token grid.
	tensors []fakeTensorSpec
}

// fakeTensorSpec describes one tensor a fakeImageSession emits: its name and how
// many rows each image yields (1 = a pooled vector, grid*grid = a token grid).
type fakeTensorSpec struct {
	name string
	rows int
}

func (f *fakeImageSession) RunNamed(inputs []onnx.NamedTensor) (map[string]onnx.NamedTensor, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls++
	if f.err != nil {
		return nil, f.err
	}
	if len(inputs) != 1 {
		return nil, nil
	}
	shape := make([]int64, len(inputs[0].Shape))
	copy(shape, inputs[0].Shape)
	f.lastShape = shape
	f.shapes = append(f.shapes, shape)
	n := 1
	for _, d := range shape {
		n *= int(d)
	}
	// n is the tensor element count; batch is shape[0].
	batch := int(shape[0])
	if f.outBatch > 0 {
		batch = f.outBatch
	}
	emitted := f.tensors
	if len(emitted) == 0 {
		emitted = []fakeTensorSpec{{name: "image_embeds", rows: 1}}
	}
	result := make(map[string]onnx.NamedTensor, len(emitted))
	for _, spec := range emitted {
		rows := max(spec.rows, 1)
		out := make([]float32, batch*rows*f.dim)
		for i := range out {
			out[i] = float32(i%f.dim) + 0.5
		}
		shape := []int64{int64(batch), int64(f.dim)}
		if rows > 1 {
			shape = []int64{int64(batch), int64(rows), int64(f.dim)}
		}
		result[spec.name] = onnx.NamedTensor{
			Name:  spec.name,
			Shape: shape,
			DType: onnx.TensorFloat32,
			Float: out,
		}
	}
	return result, nil
}

func (f *fakeImageSession) Close() error { return nil }

func (f *fakeImageSession) callCount() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.calls
}

func (f *fakeImageSession) shape() []int64 {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]int64(nil), f.lastShape...)
}

func (f *fakeImageSession) runShapes() [][]int64 {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := make([][]int64, len(f.shapes))
	for i, s := range f.shapes {
		out[i] = append([]int64(nil), s...)
	}
	return out
}

const testImageDim = 4

func testImagePlan(size int) imageproc.Plan {
	return imageproc.Plan{
		Input:    "pixel_values",
		Size:     size,
		Crop:     imageproc.CropNone,
		Resample: imageproc.ResampleBicubic,
		Rescale:  1,
		Mean:     [3]float64{},
		Std:      [3]float64{1, 1, 1},
	}
}

// serveImage starts a server with image models imgA/imgB and a text-only model
// "textonly", returning the address and the fake sessions by model name.
func serveImage(t *testing.T, cacheConfig string, opts ...Option) (string, *Server, map[string]*fakeImageSession) {
	t.Helper()
	reg := registry.New()
	sessions := map[string]*fakeImageSession{}
	for _, name := range []string{"imgA", "imgB"} {
		fake := &fakeImageSession{dim: testImageDim}
		sessions[name] = fake
		reg.Add(name, &registry.ModelEntry{
			Name: name,
			Dim:  testImageDim,
			ImageRes: &registry.ImageResources{
				Sessions: []onnx.NamedSession{fake},
				Plan:     testImagePlan(4),
				Outputs:  []registry.ImageOutput{{Name: "embedding", Tensor: "image_embeds", Select: "none", Rows: 1, Dim: testImageDim}},
			},
		})
	}
	reg.Add("textonly", &registry.ModelEntry{Name: "textonly", Dim: testImageDim})

	// depthlike carries a 2-D map output: rows×cols one image, whose only reply
	// form is the packed float32 bulk.
	depth := &fakeImageSession{dim: testImageDim, tensors: []fakeTensorSpec{{name: "predicted_depth", rows: 2}}}
	sessions["depthlike"] = depth
	reg.Add("depthlike", &registry.ModelEntry{
		Name: "depthlike",
		Dim:  testImageDim,
		ImageRes: &registry.ImageResources{
			Sessions: []onnx.NamedSession{depth},
			Plan:     testImagePlan(4),
			Outputs:  []registry.ImageOutput{{Name: "depth", Tensor: "predicted_depth", Select: "map", Rows: 2, Dim: testImageDim}},
		},
	})

	addr := getFreeAddr()
	srv := New(addr, reg, "", cacheConfig, nil, opts...)
	go srv.ListenAndServe()
	t.Cleanup(func() { srv.Close() })
	time.Sleep(50 * time.Millisecond)
	return addr, srv, sessions
}

// bulkOrArray returns the named output's value from an EMB.IMG reply map, for
// callers that expect a nested envelope rather than a bulk.
func bulkOrArray(t *testing.T, tok respToken, name string) respToken {
	t.Helper()
	elems := arrayOf(t, tok)
	for i := 0; i+1 < len(elems); i += 2 {
		if got, ok := elems[i].val.(string); ok && got == name {
			return elems[i+1]
		}
	}
	t.Fatalf("reply %#v has no %q output", tok, name)
	return respToken{}
}

// imageValueOf returns the named output's blob from an EMB.IMG reply map (a flat
// name/value array under RESP2).
func imageValueOf(t *testing.T, tok respToken, name string) string {
	t.Helper()
	elems := arrayOf(t, tok)
	for i := 0; i+1 < len(elems); i += 2 {
		if got, ok := elems[i].val.(string); ok && got == name {
			return bulkOf(t, elems[i+1])
		}
	}
	t.Fatalf("reply %#v has no %q output", tok, name)
	return ""
}

func solidImagePNG(t *testing.T, c color.Color) string {
	t.Helper()
	img := image.NewNRGBA(image.Rect(0, 0, 4, 4))
	for y := 0; y < 4; y++ {
		for x := 0; x < 4; x++ {
			img.Set(x, y, c)
		}
	}
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		t.Fatal(err)
	}
	return buf.String()
}

// gifWithBinaryComment returns a valid GIF whose bytes contain NUL, 0xFF, CR
// and LF (inside a comment extension), exercising RESP binary safety end to end.
func gifWithBinaryComment(t *testing.T) string {
	t.Helper()
	pal := color.Palette{color.Black, color.White}
	img := image.NewPaletted(image.Rect(0, 0, 2, 2), pal)
	img.SetColorIndex(0, 0, 1)
	var buf bytes.Buffer
	if err := gif.Encode(&buf, img, nil); err != nil {
		t.Fatal(err)
	}
	raw := buf.Bytes()
	// Comment extension (0x21 0xFE) with a 4-byte sub-block containing the
	// hostile sequences, then the block terminator; insert before the trailer.
	comment := []byte{0x21, 0xFE, 0x04, 0x00, 0xFF, 0x0D, 0x0A, 0x00}
	out := append([]byte{}, raw[:len(raw)-1]...)
	out = append(out, comment...)
	out = append(out, 0x3B)

	for _, b := range []byte{0x00, 0xFF, 0x0D, 0x0A} {
		if bytes.IndexByte(out, b) < 0 {
			t.Fatalf("test GIF does not contain byte %#x", b)
		}
	}
	return string(out)
}

func TestEMBIMGArityAndModelErrors(t *testing.T) {
	addr, _, _ := serveImage(t, "")
	pngBytes := solidImagePNG(t, color.NRGBA{R: 10, G: 20, B: 30, A: 255})

	if got := errorOf(t, redisCmd(t, addr, "EMB.IMG", "imgA")); !strings.Contains(got, "wrong number") {
		t.Fatalf("missing image args = %q", got)
	}
	if got := errorOf(t, redisCmd(t, addr, "EMB.IMG", "nope", pngBytes)); !strings.Contains(got, "not found") {
		t.Fatalf("unknown model = %q", got)
	}
	if got := errorOf(t, redisCmd(t, addr, "EMB.IMG", "textonly", pngBytes)); !strings.Contains(got, "no image configuration") {
		t.Fatalf("text-only model = %q", got)
	}
	if got := errorOf(t, redisCmd(t, addr, "EMB.IMG", "imgA", "https://example.com/cat.jpg")); !strings.Contains(got, "fetch") {
		t.Fatalf("URL rejection = %q", got)
	}
	if got := errorOf(t, redisCmd(t, addr, "EMB.IMG", "imgA", "data:image/png;base64,AAAA")); !strings.Contains(got, "fetch") {
		t.Fatalf("data: URL rejection = %q", got)
	}
}

func TestEMBIMGKeyedMap(t *testing.T) {
	addr, _, sessions := serveImage(t, "")
	pngBytes := solidImagePNG(t, color.NRGBA{R: 10, G: 20, B: 30, A: 255})

	// A single image replies one map keyed by output name (a flat name/value
	// array under RESP2), never a bare bulk.
	elems := arrayOf(t, redisCmd(t, addr, "EMB.IMG", "imgA", pngBytes))
	if len(elems) != 2 || elems[0].kind != "bulk" || elems[0].val != "embedding" {
		t.Fatalf("single image reply = %#v, want a one-key map named embedding", elems)
	}
	if got := len(bulkOf(t, elems[1])); got != testImageDim*4 {
		t.Fatalf("embedding length = %d, want %d", got, testImageDim*4)
	}
	if sessions["imgA"].callCount() != 1 {
		t.Fatalf("session calls = %d, want 1", sessions["imgA"].callCount())
	}
}

func TestEMBIMGBinarySafety(t *testing.T) {
	addr, _, _ := serveImage(t, "")
	gifBytes := gifWithBinaryComment(t)

	elems := arrayOf(t, redisCmd(t, addr, "EMB.IMG", "imgA", gifBytes))
	if len(elems) != 2 || len(bulkOf(t, elems[1])) != testImageDim*4 {
		t.Fatalf("binary-content gif reply = %#v (bytes corrupted or undecodable?)", elems)
	}
}

func TestEMBIMGKeywordPosition(t *testing.T) {
	addr, _, _ := serveImage(t, "")
	pngBytes := solidImagePNG(t, color.NRGBA{R: 1, G: 2, B: 3, A: 255})

	// VALUES at position 2 is the reply format → a one-key map whose value is a
	// 3-pair RESP2 envelope (one row of the model dimension).
	raw := string(respCommand("EMB.IMG", "imgA", "VALUES", pngBytes))
	c := dial(t, addr)
	c.Write([]byte(raw))
	resp := readRESP(t, c)
	if !strings.HasPrefix(resp, "*2\r\n$9\r\nembedding\r\n*6\r\n") {
		t.Fatalf("VALUES reply = %q, want a one-key map with a 3-pair envelope", resp)
	}
	if !strings.Contains(resp, "$5\r\nshape\r\n*2\r\n:1\r\n:4\r\n") {
		t.Fatalf("VALUES shape wrong: %q", resp)
	}

	// A trailing VALUES is an image argument, not a format keyword: two slots,
	// the first keyed and the second a failed (null) slot.
	elems := arrayOf(t, redisCmd(t, addr, "EMB.IMG", "imgA", pngBytes, "VALUES"))
	if len(elems) != 2 || elems[0].kind != "array" || elems[1].kind != "bulk" || elems[1].val != nil {
		t.Fatalf("trailing VALUES should be a (failed) image slot: %#v", elems)
	}
}

func TestEMBIMGBatchedSingleRun(t *testing.T) {
	addr, _, sessions := serveImage(t, "")
	a := solidImagePNG(t, color.NRGBA{R: 1, A: 255})
	b := solidImagePNG(t, color.NRGBA{G: 2, A: 255})
	d := solidImagePNG(t, color.NRGBA{B: 3, A: 255})

	elems := arrayOf(t, redisCmd(t, addr, "EMB.IMG", "imgA", a, b, d))
	if len(elems) != 3 {
		t.Fatalf("reply slots = %d, want 3", len(elems))
	}
	for i, e := range elems {
		if e.kind != "array" || len(e.val.([]respToken)) != 2 {
			t.Fatalf("slot %d = %#v, want a one-key map", i, e)
		}
	}
	fake := sessions["imgA"]
	if fake.callCount() != 1 {
		t.Fatalf("session runs = %d, want exactly 1 batched run", fake.callCount())
	}
	shape := fake.shape()
	if len(shape) != 4 || shape[0] != 3 || shape[1] != 3 || shape[2] != 4 || shape[3] != 4 {
		t.Fatalf("batch shape = %v, want [3 3 4 4]", shape)
	}
}

// TestEMBIMGBoundedBatchedRuns proves a command larger than one chunk is still
// fully processed, using multiple bounded runs instead of one unbounded batch.
func TestEMBIMGBoundedBatchedRuns(t *testing.T) {
	addr, _, sessions := serveImage(t, "")
	imgs := make([]string, maxImageBatchImages+1)
	for i := range imgs {
		imgs[i] = solidImagePNG(t, color.NRGBA{R: uint8(i % 256), G: 7, A: 255})
	}

	tok := redisCmd(t, addr, append([]string{"EMB.IMG", "imgA"}, imgs...)...)
	elems := arrayOf(t, tok)
	if len(elems) != len(imgs) {
		t.Fatalf("reply slots = %d, want %d", len(elems), len(imgs))
	}
	for i, e := range elems {
		if e.kind != "array" || e.val == nil {
			t.Fatalf("slot %d = %#v, want a keyed map", i, e)
		}
	}

	fake := sessions["imgA"]
	if got, want := fake.callCount(), 2; got != want {
		t.Fatalf("session runs = %d, want %d bounded runs", got, want)
	}
	for i, shape := range fake.runShapes() {
		if len(shape) != 4 || shape[0] < 1 || shape[0] > maxImageBatchImages {
			t.Fatalf("run %d shape = %v, want batch within [1 %d]", i, shape, maxImageBatchImages)
		}
	}
}

// TestEMBIMGBatchDimensionMismatch proves an output whose batch dimension
// disagrees with the images in the chunk is refused, naming both dimensions,
// before any row is pooled: a short output previously read past the tensor.
func TestEMBIMGBatchDimensionMismatch(t *testing.T) {
	for _, tc := range []struct {
		name     string
		outBatch int
		images   int
	}{
		{"short output", 1, 2},
		{"long output", 3, 2},
	} {
		t.Run(tc.name, func(t *testing.T) {
			reg := registry.New()
			fake := &fakeImageSession{dim: testImageDim, outBatch: tc.outBatch}
			reg.Add("imgA", &registry.ModelEntry{
				Name: "imgA",
				Dim:  testImageDim,
				ImageRes: &registry.ImageResources{
					Sessions: []onnx.NamedSession{fake},
					Plan:     testImagePlan(4),
					Outputs:  []registry.ImageOutput{{Name: "embedding", Tensor: "image_embeds", Select: "none", Rows: 1, Dim: testImageDim}},
				},
			})
			entry, err := reg.Resolve("imgA")
			if err != nil {
				t.Fatalf("Resolve: %v", err)
			}
			ir, err := entry.ImageResources()
			if err != nil || ir == nil {
				t.Fatalf("ImageResources = %v, %v", ir, err)
			}
			images := make([][]byte, tc.images)
			for i := range images {
				images[i] = []byte(solidImagePNG(t, color.NRGBA{R: uint8(i + 1), A: 255}))
			}

			srv := New(getFreeAddr(), reg, "", "", nil)
			results, _, errs := srv.embedImages("imgA", ir, ir.Outputs, images)

			want := fmt.Sprintf("image output %q has batch dimension %d, want %d",
				"embedding", tc.outBatch, tc.images)
			for i, e := range errs {
				if e == nil {
					t.Fatalf("slot %d: no error, want %q", i, want)
				}
				if e.Error() != want {
					t.Fatalf("slot %d error = %q, want %q", i, e.Error(), want)
				}
			}
			for i, r := range results {
				if r != nil {
					t.Fatalf("slot %d returned an embedding despite the refusal", i)
				}
			}
		})
	}
}

func TestEMBIMGTruncation(t *testing.T) {
	addr, _, sessions := serveImage(t, "", WithMaxImages(2))
	a := solidImagePNG(t, color.NRGBA{R: 1, A: 255})
	b := solidImagePNG(t, color.NRGBA{G: 2, A: 255})
	d := solidImagePNG(t, color.NRGBA{B: 3, A: 255})

	tok := redisCmd(t, addr, "EMB.IMG", "imgA", a, b, d)
	elems := arrayOf(t, tok)
	if len(elems) != 3 {
		t.Fatalf("truncated reply slots = %d, want 3", len(elems))
	}
	if elems[0].kind != "array" || elems[1].kind != "array" || elems[2].kind != "bulk" || elems[2].val != nil {
		t.Fatalf("overflow slot should be null: %#v", elems)
	}
	if fake := sessions["imgA"]; fake.callCount() != 1 {
		t.Fatalf("session runs = %d, want 1", fake.callCount())
	}
	if got := statsIntField(t, dial(t, addr), "truncated_images"); got != 1 {
		t.Fatalf("truncated_images = %d, want 1", got)
	}
}

func TestEMBIMGOneBadImageDoesNotFailCommand(t *testing.T) {
	addr, _, sessions := serveImage(t, "")
	good := solidImagePNG(t, color.NRGBA{R: 1, A: 255})
	good2 := solidImagePNG(t, color.NRGBA{G: 2, A: 255})

	tok := redisCmd(t, addr, "EMB.IMG", "imgA", good, "not an image", good2)
	elems := arrayOf(t, tok)
	if len(elems) != 3 {
		t.Fatalf("slots = %d, want 3", len(elems))
	}
	if elems[0].kind != "array" || elems[2].kind != "array" {
		t.Fatalf("good images should succeed: %#v", elems)
	}
	if elems[1].val != nil {
		t.Fatalf("bad image slot should be null: %#v", elems[1])
	}
	if fake := sessions["imgA"]; fake.callCount() != 1 {
		t.Fatalf("session runs = %d, want 1 (one batched run over the good images)", fake.callCount())
	}
}

func TestEMBIMGValuesRESP3(t *testing.T) {
	addr, _, _ := serveImage(t, "")
	pngBytes := solidImagePNG(t, color.NRGBA{R: 9, A: 255})

	c := dial(t, addr)
	c.Write([]byte("*2\r\n$5\r\nHELLO\r\n$1\r\n3\r\n"))
	if resp := readRESP(t, c); !strings.HasPrefix(resp, "%") {
		t.Fatalf("HELLO 3 = %q", resp)
	}
	c.Write(respCommand("EMB.IMG", "imgA", "VALUES", pngBytes))
	resp := readRESP(t, c)
	if !strings.HasPrefix(resp, "%1\r\n$9\r\nembedding\r\n%3\r\n") {
		t.Fatalf("RESP3 VALUES envelope = %q, want a one-key map holding a 3-field envelope", resp)
	}
	if !strings.Contains(resp, "dtype") || !strings.Contains(resp, "shape") || !strings.Contains(resp, "values") {
		t.Fatalf("RESP3 VALUES envelope missing fields: %q", resp)
	}
}

func TestEMBIMGMULTITwoModelsAndPartialFailure(t *testing.T) {
	addr, _, _ := serveImage(t, "")
	a := solidImagePNG(t, color.NRGBA{R: 1, A: 255})
	b := solidImagePNG(t, color.NRGBA{G: 2, A: 255})

	tok := redisCmd(t, addr, "EMB.IMGMULTI", "imgA", a, "imgB", b)
	elems := arrayOf(t, tok)
	if len(elems) != 2 || elems[0].kind != "array" || elems[1].kind != "array" {
		t.Fatalf("two-model IMGMULTI = %#v", elems)
	}

	tok = redisCmd(t, addr, "EMB.IMGMULTI", "imgA", a, "imgB", "bad", "imgA", b)
	elems = arrayOf(t, tok)
	if len(elems) != 3 || elems[0].kind != "array" || elems[1].val != nil || elems[2].kind != "array" {
		t.Fatalf("partial failure IMGMULTI = %#v", elems)
	}
}

func TestEMBIMGMULTITruncationAndFormat(t *testing.T) {
	addr, _, _ := serveImage(t, "", WithMaxImages(1))
	a := solidImagePNG(t, color.NRGBA{R: 1, A: 255})
	b := solidImagePNG(t, color.NRGBA{G: 2, A: 255})

	tok := redisCmd(t, addr, "EMB.IMGMULTI", "imgA", a, "imgB", b)
	elems := arrayOf(t, tok)
	if len(elems) != 2 || elems[0].kind != "array" || elems[1].val != nil {
		t.Fatalf("truncated IMGMULTI = %#v", elems)
	}
	if got := statsIntField(t, dial(t, addr), "truncated_images"); got != 1 {
		t.Fatalf("truncated_images = %d, want 1 (IMGMULTI overflow counts as images)", got)
	}

	// VALUES at position 1 is the reply format; each pair is a model-tagged envelope.
	raw := string(respCommand("EMB.IMGMULTI", "VALUES", "imgA", a, "imgB", b))
	c := dial(t, addr)
	c.Write([]byte(raw))
	resp := readRESP(t, c)
	if !strings.HasPrefix(resp, "*2\r\n*4\r\n") {
		t.Fatalf("IMGMULTI VALUES reply = %q, want 2 model-tagged maps (model + one output)", resp)
	}
	if !strings.Contains(resp, "$5\r\nmodel\r\n") {
		t.Fatalf("IMGMULTI VALUES envelope missing model key: %q", resp)
	}
}

func TestEMBIMGMULTIStatsCountEachPair(t *testing.T) {
	addr, _, _ := serveImage(t, "")
	a := solidImagePNG(t, color.NRGBA{R: 1, A: 255})
	b := solidImagePNG(t, color.NRGBA{G: 2, A: 255})

	redisCmd(t, addr, "EMB.IMGMULTI", "imgA", a, "imgB", b)
	if got := statsIntField(t, dial(t, addr), "image_requests"); got != 2 {
		t.Fatalf("image_requests = %d, want 2 (one per pair)", got)
	}
}

func TestMaxCommandBytesRefusesDeclaredBulkBeforeBuffering(t *testing.T) {
	addr, _, _ := serveImage(t, "", WithMaxCommandBytes(1024))
	c := dial(t, addr)
	// Declare a 5 MB bulk header but never send its payload: the server must
	// reject from the header alone, i.e. before buffering the payload.
	c.Write([]byte("*3\r\n$7\r\nEMB.IMG\r\n$4\r\nimgA\r\n$5000000\r\n"))
	resp := readRESP(t, c)
	if !strings.HasPrefix(resp, "-ERR") || !strings.Contains(resp, "maximum") {
		t.Fatalf("oversized declared bulk = %q, want an error mentioning the maximum", resp)
	}
}

func TestMaxCommandBytesTotalCommand(t *testing.T) {
	addr, _, _ := serveImage(t, "", WithMaxCommandBytes(80))
	// Two 40-byte bulks are each under the 80-byte per-bulk cap, but the
	// command's cumulative RESP bytes (4 header + 13 + 10 + 46 + 46 = 119) are
	// not. Only up to the second bulk's header is sent, so a rejection can only
	// come from the reader's aggregate guard, before that payload is buffered.
	arg := strings.Repeat("x", 40)
	c := dial(t, addr)
	c.Write([]byte("*4\r\n$7\r\nEMB.IMG\r\n$4\r\nimgA\r\n$40\r\n" + arg + "\r\n$40\r\n"))
	resp := readRESP(t, c)
	if !strings.HasPrefix(resp, "-ERR") || !strings.Contains(resp, "maximum") {
		t.Fatalf("oversized command = %q, want an error mentioning the maximum", resp)
	}
}

func TestConfigCommandAndImageCaps(t *testing.T) {
	addr, _, _ := serveImage(t, "", WithMaxCommandBytes(2048), WithMaxImageBytes(1024))

	getConfig := func(name string) string {
		elems := arrayOf(t, redisCmd(t, addr, "CONFIG", "GET", name))
		if len(elems) != 2 {
			t.Fatalf("CONFIG GET %s returned %d elements", name, len(elems))
		}
		return bulkOf(t, elems[1])
	}

	if got := getConfig("max_command_bytes"); got != "2048" {
		t.Fatalf("max_command_bytes = %q, want 2048", got)
	}
	if got := getConfig("max_image_bytes"); got != "1024" {
		t.Fatalf("max_image_bytes = %q, want 1024", got)
	}

	// Invalid values are rejected and leave the active value unchanged.
	for _, bad := range []string{"-1", "abc", "1.5"} {
		if got := errorOf(t, redisCmd(t, addr, "CONFIG", "SET", "max_command_bytes", bad)); !strings.Contains(got, "non-negative") {
			t.Fatalf("CONFIG SET max_command_bytes %q = %q, want non-negative error", bad, got)
		}
	}
	if got := getConfig("max_command_bytes"); got != "2048" {
		t.Fatalf("failed CONFIG SET changed the value to %q", got)
	}

	// Valid updates apply live.
	if got := redisCmd(t, addr, "CONFIG", "SET", "max_command_bytes", "4096"); got.kind != "status" {
		t.Fatalf("CONFIG SET max_command_bytes = %#v", got)
	}
	if got := getConfig("max_command_bytes"); got != "4096" {
		t.Fatalf("max_command_bytes after set = %q, want 4096", got)
	}
	if got := redisCmd(t, addr, "CONFIG", "SET", "max_image_bytes", "4096"); got.kind != "status" {
		t.Fatalf("CONFIG SET max_image_bytes = %#v", got)
	}
	if got := getConfig("max_image_bytes"); got != "4096" {
		t.Fatalf("max_image_bytes after set = %q, want 4096", got)
	}
}

func TestEMBHELPDocumentsImageCommands(t *testing.T) {
	addr, _, _ := serveImage(t, "")
	help := bulkOf(t, redisCmd(t, addr, "EMB.HELP"))
	for _, want := range []string{"EMB.IMG", "EMB.IMGMULTI", "emb.image", "OUTPUTS"} {
		if !strings.Contains(help, want) {
			t.Fatalf("EMB.HELP missing %q", want)
		}
	}
}

func TestImageCacheHitAndMiss(t *testing.T) {
	addr, _, sessions := serveImage(t, "1MB")
	a := solidImagePNG(t, color.NRGBA{R: 1, A: 255})

	redisCmd(t, addr, "EMB.IMG", "imgA", a)
	if c := sessions["imgA"].callCount(); c != 1 {
		t.Fatalf("first request runs = %d, want 1", c)
	}
	// Identical bytes hit the content-addressed key: no decode, no inference.
	redisCmd(t, addr, "EMB.IMG", "imgA", a)
	if c := sessions["imgA"].callCount(); c != 1 {
		t.Fatalf("identical bytes should hit the cache; runs = %d, want 1", c)
	}
	// Different bytes are a different key: a miss.
	b := solidImagePNG(t, color.NRGBA{G: 2, A: 255})
	redisCmd(t, addr, "EMB.IMG", "imgA", b)
	if c := sessions["imgA"].callCount(); c != 2 {
		t.Fatalf("changed bytes should miss; runs = %d, want 2", c)
	}
}

func TestImageCacheFlushIsModelScoped(t *testing.T) {
	addr, srv, sessions := serveImage(t, "1MB")
	a := solidImagePNG(t, color.NRGBA{R: 1, A: 255})
	b := solidImagePNG(t, color.NRGBA{G: 2, A: 255})

	redisCmd(t, addr, "EMB.IMG", "imgA", a)
	redisCmd(t, addr, "EMB.IMG", "imgB", b)
	if srv.cache == nil {
		t.Fatal("cache not configured")
	}
	if _, ok := srv.cache.Get(imageCacheKey("imgA", []byte(a), "embedding")); !ok {
		t.Fatal("imgA entry missing before flush")
	}
	if _, ok := srv.cache.Get(imageCacheKey("imgB", []byte(b), "embedding")); !ok {
		t.Fatal("imgB entry missing before flush")
	}

	if got := redisCmd(t, addr, "EMB.CACHE.FLUSH", "imgA"); got.kind != "int" || got.val.(int) != 1 {
		t.Fatalf("flush imgA = %#v, want integer 1", got)
	}
	if _, ok := srv.cache.Get(imageCacheKey("imgA", []byte(a), "embedding")); ok {
		t.Fatal("imgA image entry survived a model-scoped flush")
	}
	if _, ok := srv.cache.Get(imageCacheKey("imgB", []byte(b), "embedding")); !ok {
		t.Fatal("imgB image entry was removed by flushing imgA")
	}

	// imgA recomputes; imgB still hits.
	redisCmd(t, addr, "EMB.IMG", "imgA", a)
	redisCmd(t, addr, "EMB.IMG", "imgB", b)
	if c := sessions["imgA"].callCount(); c != 2 {
		t.Fatalf("imgA runs after flush = %d, want 2", c)
	}
	if c := sessions["imgB"].callCount(); c != 1 {
		t.Fatalf("imgB runs = %d, want 1 (still cached)", c)
	}
}

func TestImageCacheByteBudgetAndEviction(t *testing.T) {
	// Image entries use the same byte budget and LRU/eviction accounting as
	// text entries; a budget too small for the working set evicts the LRU tail.
	c := NewCache(400)
	val := make([]byte, 8)
	for i := 0; i < 20; i++ {
		c.Set(imageCacheKey("m", []byte{byte(i)}, "embedding"), val)
	}
	st := c.Stats()
	if st.Evictions == 0 {
		t.Fatal("expected image entries to be evicted under the byte budget")
	}
	if st.Entries == 0 || st.Entries >= 20 {
		t.Fatalf("entries = %d, want a bounded non-empty working set", st.Entries)
	}
	if st.CurBytes > st.MaxBytes {
		t.Fatalf("cache over budget: %d > %d", st.CurBytes, st.MaxBytes)
	}
}

// TestEMBIMGStaticBatchCap proves a graph that declares batch 1 is fed one
// image per run and still serves every slot of a multi-image request.
func TestEMBIMGStaticBatchCap(t *testing.T) {
	addr, _, sessions := serveImageBatchCap(t, 1)
	a := solidImagePNG(t, color.NRGBA{R: 1, A: 255})
	b := solidImagePNG(t, color.NRGBA{G: 2, A: 255})
	d := solidImagePNG(t, color.NRGBA{B: 3, A: 255})

	tok := redisCmd(t, addr, "EMB.IMG", "imgA", a, b, d)
	elems := arrayOf(t, tok)
	if len(elems) != 3 {
		t.Fatalf("reply slots = %d, want 3", len(elems))
	}
	for i, e := range elems {
		m := arrayOf(t, e)
		if len(m) != 2 || m[0].val != "embedding" || len(bulkOf(t, m[1])) != testImageDim*4 {
			t.Fatalf("slot %d = %#v, want a one-key map with a %d-byte embedding", i, e, testImageDim*4)
		}
	}
	fake := sessions["imgA"]
	if fake.callCount() != 3 {
		t.Fatalf("session runs = %d, want one per image (3)", fake.callCount())
	}
	for i, shape := range fake.runShapes() {
		if shape[0] != 1 {
			t.Fatalf("run %d batch = %d, want 1", i, shape[0])
		}
	}
}

// TestEMBIMGStaticBatchPadding proves a request shorter than a graph's declared
// batch is padded to it (the fixed-shape input accepts nothing else) and the
// padded results never reach a slot.
func TestEMBIMGStaticBatchPadding(t *testing.T) {
	addr, _, sessions := serveImageBatchCap(t, 4)
	a := solidImagePNG(t, color.NRGBA{R: 1, A: 255})
	b := solidImagePNG(t, color.NRGBA{G: 2, A: 255})
	d := solidImagePNG(t, color.NRGBA{B: 3, A: 255})

	elems := arrayOf(t, redisCmd(t, addr, "EMB.IMG", "imgA", a, b, d))
	if len(elems) != 3 {
		t.Fatalf("reply slots = %d, want 3", len(elems))
	}
	for i, e := range elems {
		m := arrayOf(t, e)
		if len(m) != 2 || m[0].val != "embedding" || len(bulkOf(t, m[1])) != testImageDim*4 {
			t.Fatalf("slot %d = %#v, want a one-key map with a %d-byte embedding", i, e, testImageDim*4)
		}
	}
	fake := sessions["imgA"]
	if fake.callCount() != 1 {
		t.Fatalf("session runs = %d, want 1", fake.callCount())
	}
	if shape := fake.runShapes()[0]; shape[0] != 4 {
		t.Fatalf("run batch = %d, want the graph's declared 4", shape[0])
	}
}

// serveImageBatchCap starts a server whose imgA model declares a static batch
// axis, so tests can observe the per-run batching the cap forces.
func serveImageBatchCap(t *testing.T, batchCap int) (string, *Server, map[string]*fakeImageSession) {
	t.Helper()
	reg := registry.New()
	fake := &fakeImageSession{dim: testImageDim}
	reg.Add("imgA", &registry.ModelEntry{
		Name: "imgA",
		Dim:  testImageDim,
		ImageRes: &registry.ImageResources{
			Sessions: []onnx.NamedSession{fake},
			Plan:     testImagePlan(4),
			BatchCap: batchCap,
			Outputs:  []registry.ImageOutput{{Name: "embedding", Tensor: "image_embeds", Select: "none", Rows: 1, Dim: testImageDim}},
		},
	})
	addr := getFreeAddr()
	srv := New(addr, reg, "", "", nil)
	go srv.ListenAndServe()
	t.Cleanup(func() { srv.Close() })
	time.Sleep(50 * time.Millisecond)
	return addr, srv, map[string]*fakeImageSession{"imgA": fake}
}

// TestTextOnImageOnlyModelNamesMissingTokenizer proves an image-only model
// loads without a tokenizer, serves the image surface, and answers a text
// command with an error naming the model and the tokenizer file it wanted.
func TestTextOnImageOnlyModelNamesMissingTokenizer(t *testing.T) {
	const model = "../../models/minilm/model.onnx"
	if _, err := os.Stat(model); err != nil {
		t.Skipf("test model not present: %v (run: just download-model)", err)
	}
	if !ortOK {
		t.Skip("onnx runtime unavailable (run inside nix develop)")
	}

	// A graph with no tokenizer.json beside it: the inferred tokenizer path
	// does not exist, which is exactly an image-only export.
	dir := t.TempDir()
	abs, err := filepath.Abs(model)
	if err != nil {
		t.Fatal(err)
	}
	graph := filepath.Join(dir, "model.onnx")
	if err := os.Symlink(abs, graph); err != nil {
		t.Fatalf("symlinking model: %v", err)
	}

	entry, err := registry.LoadModel(config.ModelConfig{
		ONNX: graph,
		Image: &config.ImageConfig{
			Input:   "pixel_values",
			Size:    4,
			Outputs: []config.ImageOutputConfig{{Name: "embedding", Tensor: "last_hidden_state"}},
		},
	}, "imgonly")
	if err != nil {
		t.Fatalf("loading image-only model without a tokenizer: %v", err)
	}
	reg := registry.New()
	reg.Add("imgonly", entry)
	t.Cleanup(func() { _ = reg.Close() })

	addr := getFreeAddr()
	srv := New(addr, reg, "", "", nil)
	go srv.ListenAndServe()
	t.Cleanup(func() { srv.Close() })
	time.Sleep(50 * time.Millisecond)

	// The load is asynchronous, so the first text command can race it; the
	// failure under test is the one reported once loading has finished.
	got := errorOf(t, redisCmd(t, addr, "EMB", "imgonly", "hello"))
	for i := 0; i < 100 && strings.Contains(got, "initializing"); i++ {
		time.Sleep(10 * time.Millisecond)
		got = errorOf(t, redisCmd(t, addr, "EMB", "imgonly", "hello"))
	}
	if !strings.Contains(got, "imgonly") || !strings.Contains(got, "tokenizer") {
		t.Fatalf("text on an image-only model = %q, want the model and the missing tokenizer named", got)
	}
}

// serveImageMulti starts a server whose "dinov3" model serves two outputs — a
// pooled vector and a 3x3 token grid — and a second model with only the pooled
// one.
func serveImageMulti(t *testing.T, cacheConfig string) (string, *Server, map[string]*fakeImageSession) {
	t.Helper()
	reg := registry.New()
	sessions := map[string]*fakeImageSession{}
	pooled := registry.ImageOutput{Name: "embedding", Tensor: "image_embeds", Select: "none", Rows: 1, Dim: testImageDim}
	grid := registry.ImageOutput{Name: "patches", Tensor: "tokens", Select: "tokens", Start: 1, Grid: 3, Rows: 9, Dim: testImageDim}

	two := &fakeImageSession{dim: testImageDim, tensors: []fakeTensorSpec{
		{name: "image_embeds", rows: 1},
		{name: "tokens", rows: 10}, // 1 CLS + 9 patch rows
	}}
	sessions["dinov3"] = two
	reg.Add("dinov3", &registry.ModelEntry{
		Name: "dinov3",
		Dim:  testImageDim,
		ImageRes: &registry.ImageResources{
			Sessions: []onnx.NamedSession{two},
			Plan:     testImagePlan(4),
			Outputs:  []registry.ImageOutput{pooled, grid},
		},
	})

	only := &fakeImageSession{dim: testImageDim}
	sessions["pooled"] = only
	reg.Add("pooled", &registry.ModelEntry{
		Name: "pooled",
		Dim:  testImageDim,
		ImageRes: &registry.ImageResources{
			Sessions: []onnx.NamedSession{only},
			Plan:     testImagePlan(4),
			Outputs:  []registry.ImageOutput{pooled},
		},
	})

	addr := getFreeAddr()
	srv := New(addr, reg, "", cacheConfig, nil)
	go srv.ListenAndServe()
	t.Cleanup(func() { srv.Close() })
	time.Sleep(50 * time.Millisecond)
	return addr, srv, sessions
}

// TestEMBIMGMapRefusesValues proves a map output's only reply form is the
// packed bulk: a VALUES request is refused before the cache or a session run, for
// both image commands, and the refusal names the form to use.
func TestEMBIMGMapRefusesValues(t *testing.T) {
	addr, _, sessions := serveImage(t, "")
	png := solidImagePNG(t, color.NRGBA{R: 9, A: 255})

	for _, args := range [][]string{
		{"EMB.IMG", "depthlike", "VALUES", png},
		{"EMB.IMGMULTI", "VALUES", "depthlike", png},
	} {
		tok := redisCmd(t, addr, args...)
		if tok.kind != "error" {
			t.Fatalf("%s VALUES = %#v, want an error refusing the map", args[0], tok)
		}
		if msg, _ := tok.val.(string); !strings.Contains(msg, "BLOB") {
			t.Fatalf("%s VALUES error = %q, want it to name BLOB", args[0], msg)
		}
	}
	if got := sessions["depthlike"].callCount(); got != 0 {
		t.Fatalf("the refused map ran %d session(s), want none", got)
	}

	// BLOB is the form: rows×cols float32, no envelope.
	blob := imageValueOf(t, redisCmd(t, addr, "EMB.IMG", "depthlike", "BLOB", png), "depth")
	if len(blob) != 2*testImageDim*4 {
		t.Fatalf("map blob = %d bytes, want %d", len(blob), 2*testImageDim*4)
	}
}

// TestEMBIMGPartialCacheMissIsNull proves a failed miss cannot leave the map a
// partial cache hit filled. One output is cached; the other's preprocessing then
// fails, because the byte cap was lowered after the first call. Before the fix
// the incomplete map reached the VALUES writer, which sliced a nil blob and
// panicked the handler; a failed image is a null in both reply formats.
func TestEMBIMGPartialCacheMissIsNull(t *testing.T) {
	addr, srv, sessions := serveImageMulti(t, "16MB")
	png := solidImagePNG(t, color.NRGBA{R: 4, A: 255})

	// Cache one of the two products by asking for it alone.
	if got := imageValueOf(t, redisCmd(t, addr, "EMB.IMG", "dinov3", "OUTPUTS", "embedding", png), "embedding"); len(got) == 0 {
		t.Fatal("the pooled product was not cached")
	}
	calls := sessions["dinov3"].callCount()

	// The same bytes no longer preprocess, so the second product cannot be
	// computed and the cached first one must not be answered on its own.
	srv.maxImageBytes.Store(8)

	for _, format := range []string{"BLOB", "VALUES"} {
		tok := redisCmd(t, addr, "EMB.IMG", "dinov3", format, "OUTPUTS", "embedding,patches", png)
		if tok.kind != "bulk" || tok.val != nil {
			t.Fatalf("%s: failed image = %#v, want a null rather than a partial map", format, tok)
		}
	}
	if got := sessions["dinov3"].callCount(); got != calls {
		t.Fatalf("the refused image ran %d extra session(s), want none", got-calls)
	}
}

// TestEMBIMGOutputsKeyword covers the OUTPUTS positions, the all-outputs
// default, the unknown-name error, and a URL in the first image slot after it.
func TestEMBIMGOutputsKeyword(t *testing.T) {
	addr, _, sessions := serveImageMulti(t, "")
	png := solidImagePNG(t, color.NRGBA{R: 4, A: 255})

	// No OUTPUTS → every configured output, in configured order.
	elems := arrayOf(t, redisCmd(t, addr, "EMB.IMG", "dinov3", png))
	if len(elems) != 4 || elems[0].val != "embedding" || elems[2].val != "patches" {
		t.Fatalf("default projection = %#v, want both outputs", elems)
	}

	// OUTPUTS right after the model.
	elems = arrayOf(t, redisCmd(t, addr, "EMB.IMG", "dinov3", "OUTPUTS", "patches", png))
	if len(elems) != 2 || elems[0].val != "patches" {
		t.Fatalf("OUTPUTS patch projection = %#v", elems)
	}

	// OUTPUTS after the reply-format keyword.
	elems = arrayOf(t, redisCmd(t, addr, "EMB.IMG", "dinov3", "BLOB", "OUTPUTS", "embedding", png))
	if len(elems) != 2 || elems[0].val != "embedding" {
		t.Fatalf("BLOB OUTPUTS projection = %#v", elems)
	}

	// Repeated names are deduped into one entry.
	elems = arrayOf(t, redisCmd(t, addr, "EMB.IMG", "dinov3", "OUTPUTS", "patches,patches", png))
	if len(elems) != 2 {
		t.Fatalf("deduped projection = %#v, want one output", elems)
	}

	// An unknown name fails the command before any image is decoded, naming the
	// configured outputs.
	got := errorOf(t, redisCmd(t, addr, "EMB.IMG", "dinov3", "OUTPUTS", "nope", png))
	for _, want := range []string{"nope", "embedding", "patches"} {
		if !strings.Contains(got, want) {
			t.Fatalf("unknown output error %q should mention %q", got, want)
		}
	}

	// A URL in the first image slot after OUTPUTS is still caught by the guard.
	if got := errorOf(t, redisCmd(t, addr, "EMB.IMG", "dinov3", "OUTPUTS", "embedding", "https://example.com/x.png")); !strings.Contains(got, "fetch") {
		t.Fatalf("URL after OUTPUTS = %q, want the fetch guidance", got)
	}

	if c := sessions["dinov3"].callCount(); c != 4 {
		t.Fatalf("runs = %d, want 4 (the deduped projection shares one run)", c)
	}
}

// TestEMBIMGMultiOutputKeyedShapes asserts the exact wire shapes of the keyed
// reply under both protocols: one map with two keys, an array of two maps, and
// the token envelope's grid.
func TestEMBIMGMultiOutputKeyedShapes(t *testing.T) {
	addr, _, sessions := serveImageMulti(t, "")
	a := solidImagePNG(t, color.NRGBA{R: 1, A: 255})
	b := solidImagePNG(t, color.NRGBA{G: 2, A: 255})

	// RESP2: one image → flat name/value array, in configured order, with the
	// two values being a dim*4 blob and a 9*dim*4 grid blob.
	elems := arrayOf(t, redisCmd(t, addr, "EMB.IMG", "dinov3", a))
	if len(elems) != 4 {
		t.Fatalf("two-output map = %d elements, want 4", len(elems))
	}
	if got := len(bulkOf(t, elems[1])); got != testImageDim*4 {
		t.Fatalf("embedding = %d bytes, want %d", got, testImageDim*4)
	}
	if got := len(bulkOf(t, elems[3])); got != 9*testImageDim*4 {
		t.Fatalf("patches = %d bytes, want %d", got, 9*testImageDim*4)
	}

	// RESP2: several images → an array of two such maps.
	tok := redisCmd(t, addr, "EMB.IMG", "dinov3", a, b)
	slotElems := arrayOf(t, tok)
	if len(slotElems) != 2 {
		t.Fatalf("two images = %d slots, want 2", len(slotElems))
	}
	for i, slot := range slotElems {
		if slot.kind != "array" || len(slot.val.([]respToken)) != 4 {
			t.Fatalf("slot %d = %#v, want a two-key map", i, slot)
		}
	}

	// RESP3: a real map keyed by output name, with a raw blob value.
	c := dial(t, addr)
	defer c.Close()
	c.Write([]byte("*2\r\n$5\r\nHELLO\r\n$1\r\n3\r\n"))
	if resp := readRESP(t, c); !strings.HasPrefix(resp, "%") {
		t.Fatalf("HELLO 3 = %q", resp)
	}
	c.Write(respCommand("EMB.IMG", "dinov3", "OUTPUTS", "patches", a))
	resp := readRESPComplete(t, c)
	wantBlob := fmt.Sprintf("%%1\r\n$7\r\npatches\r\n$%d\r\n", 9*testImageDim*4)
	if !strings.HasPrefix(resp, wantBlob) {
		t.Fatalf("RESP3 keyed map = %q, want %q", resp[:min(len(resp), 80)], wantBlob)
	}

	// VALUES (RESP2) carries the token envelope's shape and grid.
	envelope := arrayOf(t, bulkOrArray(t, redisCmd(t, addr, "EMB.IMG", "dinov3", "VALUES", "OUTPUTS", "patches", a), "patches"))
	fields := map[string]respToken{}
	for i := 0; i+1 < len(envelope); i += 2 {
		fields[bulkOf(t, envelope[i])] = envelope[i+1]
	}
	if shape := arrayOf(t, fields["shape"]); shape[0].val != 9 || shape[1].val != testImageDim {
		t.Fatalf("token shape = %#v, want [9 %d]", shape, testImageDim)
	}
	if grid := arrayOf(t, fields["grid"]); grid[0].val != 3 || grid[1].val != 3 {
		t.Fatalf("token grid = %#v, want [3 3]", grid)
	}

	// Four commands, one session run each: the two-output map, the two-image
	// array, and the two narrowed projections.
	if c := sessions["dinov3"].callCount(); c != 4 {
		t.Fatalf("runs = %d, want 4 (one per command)", c)
	}
}

// TestEMBIMGPerOutputCache proves each output is cached on its own: narrowing
// the projection after a full request performs no inference, while asking for
// the other output does its own run.
func TestEMBIMGPerOutputCache(t *testing.T) {
	addr, srv, sessions := serveImageMulti(t, "1MB")
	png := solidImagePNG(t, color.NRGBA{R: 7, A: 255})

	redisCmd(t, addr, "EMB.IMG", "dinov3", png)
	if c := sessions["dinov3"].callCount(); c != 1 {
		t.Fatalf("first request runs = %d, want 1", c)
	}
	// Same bytes, same outputs → every entry hits.
	redisCmd(t, addr, "EMB.IMG", "dinov3", png)
	if c := sessions["dinov3"].callCount(); c != 1 {
		t.Fatalf("repeat request runs = %d, want 1 (per-output hits)", c)
	}
	// A different output of the same bytes is a different entry, so it is
	// already cached and needs no second run either.
	redisCmd(t, addr, "EMB.IMG", "dinov3", "OUTPUTS", "patches", png)
	if c := sessions["dinov3"].callCount(); c != 1 {
		t.Fatalf("narrowed request runs = %d, want 1", c)
	}

	for _, out := range []string{"embedding", "patches"} {
		if _, ok := srv.cache.Get(imageCacheKey("dinov3", []byte(png), out)); !ok {
			t.Fatalf("%s entry missing from the cache", out)
		}
	}

	// Flushing the model removes every output's entry.
	if got := redisCmd(t, addr, "EMB.CACHE.FLUSH", "dinov3"); got.kind != "int" || got.val.(int) != 2 {
		t.Fatalf("flush = %#v, want 2 entries", got)
	}
}

// TestEMBIMGTwoOutputRequestCountedOnce covers request accounting: one command
// is one image request however many outputs it asks for.
func TestEMBIMGTwoOutputRequestCountedOnce(t *testing.T) {
	addr, _, _ := serveImageMulti(t, "")
	png := solidImagePNG(t, color.NRGBA{R: 5, A: 255})

	redisCmd(t, addr, "EMB.IMG", "dinov3", png)
	if got := statsIntField(t, dial(t, addr), "image_requests"); got != 1 {
		t.Fatalf("image_requests = %d, want 1 for one command", got)
	}
}

// TestEMBIMGMULTIKeyedMapsPerPair asserts the EMB.IMGMULTI wire shape: one keyed
// map per pair, with the pair's model beside its outputs under VALUES, and the
// projection keyword at its fixed position.
func TestEMBIMGMULTIKeyedMapsPerPair(t *testing.T) {
	addr, _, _ := serveImageMulti(t, "")
	a := solidImagePNG(t, color.NRGBA{R: 1, A: 255})
	b := solidImagePNG(t, color.NRGBA{G: 2, A: 255})

	elems := arrayOf(t, redisCmd(t, addr, "EMB.IMGMULTI", "OUTPUTS", "embedding", "dinov3", a, "pooled", b))
	if len(elems) != 2 {
		t.Fatalf("pairs = %d, want 2", len(elems))
	}
	for i, pair := range elems {
		if m := arrayOf(t, pair); len(m) != 2 || m[0].val != "embedding" {
			t.Fatalf("pair %d = %#v, want a one-key embedding map", i, pair)
		}
	}

	// A name one model does not configure fails only that pair.
	elems = arrayOf(t, redisCmd(t, addr, "EMB.IMGMULTI", "OUTPUTS", "patches", "dinov3", a, "pooled", b))
	if elems[1].val != nil {
		t.Fatalf("pooled pair = %#v, want null for an unconfigured output", elems[1])
	}

	// VALUES carries the pair's model beside its outputs.
	raw := string(respCommand("EMB.IMGMULTI", "VALUES", "OUTPUTS", "embedding", "pooled", b))
	c := dial(t, addr)
	defer c.Close()
	c.Write([]byte(raw))
	resp := readRESPComplete(t, c)
	if !strings.HasPrefix(resp, "*1\r\n*4\r\n$5\r\nmodel\r\n$6\r\npooled\r\n") {
		t.Fatalf("IMGMULTI VALUES map = %q, want model beside the output", resp)
	}
}

// TestEMBINFOImageOutputs proves EMB.INFO lists every configured image output
// with its shape, without running inference and without a notion of a default.
func TestEMBINFOImageOutputs(t *testing.T) {
	addr, _, sessions := serveImageMulti(t, "")
	c := dial(t, addr)
	defer c.Close()

	c.Write(respCommand("EMB.INFO", "dinov3"))
	info := replyPairs(t, parseRESP(t, readRESPComplete(t, c)))

	outputs := arrayOf(t, info["image_outputs"])
	if len(outputs) != 4 {
		t.Fatalf("image_outputs = %#v, want two named entries", outputs)
	}
	pooled := replyPairs(t, outputs[1])
	if pooled["tensor"].val != "image_embeds" || pooled["select"].val != "none" || intOf(t, pooled["rows"]) != 1 || intOf(t, pooled["dim"]) != testImageDim {
		t.Fatalf("pooled output = %#v", pooled)
	}
	grid := replyPairs(t, outputs[3])
	if grid["tensor"].val != "tokens" || grid["select"].val != "tokens" || intOf(t, grid["rows"]) != 9 {
		t.Fatalf("token output = %#v", grid)
	}
	if g := arrayOf(t, grid["grid"]); intOf(t, g[0]) != 3 || intOf(t, g[1]) != 3 {
		t.Fatalf("token grid = %#v, want [3 3]", g)
	}
	if c := sessions["dinov3"].callCount(); c != 0 {
		t.Fatalf("EMB.INFO ran %d inferences, want none", c)
	}
}
