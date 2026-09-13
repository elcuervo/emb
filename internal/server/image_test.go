package server

import (
	"bytes"
	"image"
	"image/color"
	"image/gif"
	"image/png"
	"strings"
	"sync"
	"testing"
	"time"

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
	out := make([]float32, batch*f.dim)
	for i := range out {
		out[i] = float32(i%f.dim) + 0.5
	}
	return map[string]onnx.NamedTensor{
		"image_embeds": {
			Name:  "image_embeds",
			Shape: []int64{int64(batch), int64(f.dim)},
			DType: onnx.TensorFloat32,
			Float: out,
		},
	}, nil
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
				Sessions:     []onnx.NamedSession{fake},
				Plan:         testImagePlan(4),
				OutputTensor: "image_embeds",
				Pooling:      "none",
				Dim:          testImageDim,
			},
		})
	}
	reg.Add("textonly", &registry.ModelEntry{Name: "textonly", Dim: testImageDim})

	addr := getFreeAddr()
	srv := New(addr, reg, "", cacheConfig, nil, opts...)
	go srv.ListenAndServe()
	t.Cleanup(func() { srv.Close() })
	time.Sleep(50 * time.Millisecond)
	return addr, srv, sessions
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

func TestEMBIMGSingleBulk(t *testing.T) {
	addr, _, sessions := serveImage(t, "")
	pngBytes := solidImagePNG(t, color.NRGBA{R: 10, G: 20, B: 30, A: 255})

	tok := redisCmd(t, addr, "EMB.IMG", "imgA", pngBytes)
	if tok.kind != "bulk" {
		t.Fatalf("single image reply = %#v, want bulk", tok)
	}
	if len(bulkOf(t, tok)) != testImageDim*4 {
		t.Fatalf("embedding length = %d, want %d", len(bulkOf(t, tok)), testImageDim*4)
	}
	if sessions["imgA"].callCount() != 1 {
		t.Fatalf("session calls = %d, want 1", sessions["imgA"].callCount())
	}
}

func TestEMBIMGBinarySafety(t *testing.T) {
	addr, _, _ := serveImage(t, "")
	gifBytes := gifWithBinaryComment(t)

	tok := redisCmd(t, addr, "EMB.IMG", "imgA", gifBytes)
	if tok.kind != "bulk" {
		t.Fatalf("binary-content gif reply = %#v (bytes corrupted or undecodable?)", tok)
	}
}

func TestEMBIMGKeywordPosition(t *testing.T) {
	addr, _, _ := serveImage(t, "")
	pngBytes := solidImagePNG(t, color.NRGBA{R: 1, G: 2, B: 3, A: 255})

	// VALUES at position 2 is the reply format → a 6-element RESP2 envelope.
	raw := string(respCommand("EMB.IMG", "imgA", "VALUES", pngBytes))
	c := dial(t, addr)
	c.Write([]byte(raw))
	resp := readRESP(t, c)
	if !strings.HasPrefix(resp, "*6\r\n") {
		t.Fatalf("VALUES reply = %q, want 6-element envelope", resp)
	}
	if !strings.Contains(resp, "$5\r\nshape\r\n*2\r\n:1\r\n:4\r\n") {
		t.Fatalf("VALUES shape wrong: %q", resp)
	}

	// A trailing VALUES is an image argument, not a format keyword.
	tok := redisCmd(t, addr, "EMB.IMG", "imgA", pngBytes, "VALUES")
	elems := arrayOf(t, tok)
	if len(elems) != 2 || elems[0].kind != "bulk" || elems[1].kind != "bulk" || elems[1].val != nil {
		t.Fatalf("trailing VALUES should be a (failed) image slot: %#v", elems)
	}
}

func TestEMBIMGBatchedSingleRun(t *testing.T) {
	addr, _, sessions := serveImage(t, "")
	a := solidImagePNG(t, color.NRGBA{R: 1, A: 255})
	b := solidImagePNG(t, color.NRGBA{G: 2, A: 255})
	d := solidImagePNG(t, color.NRGBA{B: 3, A: 255})

	tok := redisCmd(t, addr, "EMB.IMG", "imgA", a, b, d)
	elems := arrayOf(t, tok)
	if len(elems) != 3 {
		t.Fatalf("reply slots = %d, want 3", len(elems))
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
		if e.kind != "bulk" || e.val == nil {
			t.Fatalf("slot %d = %#v, want a bulk embedding", i, e)
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
	if elems[0].kind != "bulk" || elems[1].kind != "bulk" || elems[2].kind != "bulk" || elems[2].val != nil {
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
	if elems[0].kind != "bulk" || elems[2].kind != "bulk" {
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
	if !strings.HasPrefix(resp, "%3\r\n") {
		t.Fatalf("RESP3 VALUES envelope = %q, want a 3-field map", resp)
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
	if len(elems) != 2 || elems[0].kind != "bulk" || elems[1].kind != "bulk" {
		t.Fatalf("two-model IMGMULTI = %#v", elems)
	}

	tok = redisCmd(t, addr, "EMB.IMGMULTI", "imgA", a, "imgB", "bad", "imgA", b)
	elems = arrayOf(t, tok)
	if len(elems) != 3 || elems[0].kind != "bulk" || elems[1].val != nil || elems[2].kind != "bulk" {
		t.Fatalf("partial failure IMGMULTI = %#v", elems)
	}
}

func TestEMBIMGMULTITruncationAndFormat(t *testing.T) {
	addr, _, _ := serveImage(t, "", WithMaxImages(1))
	a := solidImagePNG(t, color.NRGBA{R: 1, A: 255})
	b := solidImagePNG(t, color.NRGBA{G: 2, A: 255})

	tok := redisCmd(t, addr, "EMB.IMGMULTI", "imgA", a, "imgB", b)
	elems := arrayOf(t, tok)
	if len(elems) != 2 || elems[0].kind != "bulk" || elems[1].val != nil {
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
	if !strings.HasPrefix(resp, "*2\r\n*8\r\n") {
		t.Fatalf("IMGMULTI VALUES reply = %q, want 2 model-tagged envelopes", resp)
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
	for _, want := range []string{"EMB.IMG", "EMB.IMGMULTI", "emb.image"} {
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
	if _, ok := srv.cache.Get(imageCacheKey("imgA", []byte(a))); !ok {
		t.Fatal("imgA entry missing before flush")
	}
	if _, ok := srv.cache.Get(imageCacheKey("imgB", []byte(b))); !ok {
		t.Fatal("imgB entry missing before flush")
	}

	if got := redisCmd(t, addr, "EMB.CACHE.FLUSH", "imgA"); got.kind != "int" || got.val.(int) != 1 {
		t.Fatalf("flush imgA = %#v, want integer 1", got)
	}
	if _, ok := srv.cache.Get(imageCacheKey("imgA", []byte(a))); ok {
		t.Fatal("imgA image entry survived a model-scoped flush")
	}
	if _, ok := srv.cache.Get(imageCacheKey("imgB", []byte(b))); !ok {
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
		c.Set(imageCacheKey("m", []byte{byte(i)}), val)
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
