package server

import (
	"fmt"
	"image/color"
	"net"
	"runtime"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/elcuervo/emb/internal/config"
	"github.com/elcuervo/emb/internal/onnx"
	"github.com/elcuervo/emb/internal/registry"
)

// runShapeScript runs the graph and returns one output dimension, opening the
// model's named-tensor script sessions. It exercises the minimal emb.run path.
const runShapeScript = `local e = emb.tokenize.encode(KEYS[1], 8)
local out = emb.run({input_ids = {shape = {1, #e.ids}, data = e.ids},
  attention_mask = {shape = {1, #e.mask}, data = e.mask},
  token_type_ids = {shape = {1, #e.ids}, fill = 0, dtype = "i64"}})
return #out.last_hidden_state.shape`

// serveScriptModel starts a server with the minilm fixture under a custom model
// config, so script_workers / batching / image semantics can be exercised.
func serveScriptModel(t *testing.T, modelCfg config.ModelConfig, cacheCfg string) (string, *Server) {
	t.Helper()
	if !ortOK {
		t.Skip("onnx runtime unavailable (run inside nix develop)")
	}
	modelCfg.ONNX = "../../models/minilm/model.onnx"
	modelCfg.Tokenizer = "../../models/minilm/tokenizer.json"
	if modelCfg.Dim == 0 {
		modelCfg.Dim = 384
	}
	if modelCfg.MaxLength == 0 {
		modelCfg.MaxLength = 128
	}
	if modelCfg.Pooling == "" {
		modelCfg.Pooling = "mean"
	}
	reg := registry.New()
	entry, err := registry.LoadModel(modelCfg, "test")
	if err != nil {
		t.Skipf("script test model not present: %v (run: just download-model)", err)
	}
	reg.Add("test", entry)

	addr := getFreeAddr()
	srv := New(addr, reg, "", cacheCfg, nil)
	go srv.ListenAndServe()
	t.Cleanup(func() { srv.Close() })
	time.Sleep(50 * time.Millisecond)
	return addr, srv
}

// TestScriptSessionsBoundedByBatcherPoolObservable verifies the auto-tuned
// bound is exactly one session under the batching default, observable through
// EMB.INFO's script footprint.
func TestScriptSessionsBoundedByBatcherPoolObservable(t *testing.T) {
	addr, _ := serveScriptTest(t, "")
	c := dial(t, addr)
	defer c.Close()

	if got := doCmd(t, c, "EMB.EVAL", "test", runShapeScript, "1", "hello"); !strings.HasPrefix(got, ":") {
		t.Fatalf("emb.run evaluation = %q", got)
	}
	info := statsFields(t, redisCmd(t, addr, "EMB.INFO", "test"))
	if got := intOf(t, info["script_sessions"]); got != 1 {
		t.Fatalf("script_sessions = %d, want exactly 1 (batcher pool holds one)", got)
	}
}

// TestScriptWorkersOverrideOpensFour verifies an explicit script_workers is
// honoured verbatim (not clamped to the batcher pool's one session).
func TestScriptWorkersOverrideOpensFour(t *testing.T) {
	addr, _ := serveScriptModel(t, config.ModelConfig{ScriptWorkers: 4}, "")
	c := dial(t, addr)
	defer c.Close()

	if got := doCmd(t, c, "EMB.EVAL", "test", runShapeScript, "1", "hello"); !strings.HasPrefix(got, ":") {
		t.Fatalf("emb.run evaluation = %q", got)
	}
	info := statsFields(t, redisCmd(t, addr, "EMB.INFO", "test"))
	if got := intOf(t, info["script_sessions"]); got != 4 {
		t.Fatalf("script_sessions = %d, want 4 (explicit override)", got)
	}
}

// imageModelConfig is minilm reconfigured with an image block so the image
// surface is present but its sessions are never needed by non-image scripts.
func imageModelConfig() config.ModelConfig {
	return config.ModelConfig{
		Image: &config.ImageConfig{
			Input:  "pixel_values",
			Size:   4,
			Output: "last_hidden_state",
			Std:    []float64{1, 1, 1},
		},
	}
}

// TestImageResourcesLazyForNonImageScript verifies a script that never touches
// emb.image.* leaves the model's image sessions unopened and reported as zero.
func TestImageResourcesLazyForNonImageScript(t *testing.T) {
	addr, srv := serveScriptModel(t, imageModelConfig(), "")
	entry, err := srv.reg.Resolve("test")
	if err != nil {
		t.Fatal(err)
	}
	if !entry.HasImageSurface() {
		t.Fatal("image-configured model has no image surface")
	}
	c := dial(t, addr)
	defer c.Close()

	if got := doCmd(t, c, "EMB.EVAL", "test", "return 1", "1", "x"); got != ":1\r\n" {
		t.Fatalf("constant evaluation = %q", got)
	}
	if entry.ImageRes != nil {
		t.Fatal("image sessions were opened by a non-image script")
	}
	info := statsFields(t, redisCmd(t, addr, "EMB.INFO", "test"))
	if got := intOf(t, info["script_sessions"]); got != 0 {
		t.Fatalf("script_sessions = %d, want 0", got)
	}
	if got := intOf(t, info["image_sessions"]); got != 0 {
		t.Fatalf("image_sessions = %d, want 0", got)
	}
}

// TestImageInfoDoesNotOpenSessions verifies emb.image.info reports the
// configured plan while creating no image session.
func TestImageInfoDoesNotOpenSessions(t *testing.T) {
	addr, srv := serveScriptModel(t, imageModelConfig(), "")
	entry, err := srv.reg.Resolve("test")
	if err != nil {
		t.Fatal(err)
	}
	c := dial(t, addr)
	defer c.Close()

	got := doCmd(t, c, "EMB.EVAL", "test", `local i = emb.image.info(); return i.input .. ":" .. i.size`, "1", "x")
	if got != "$14\r\npixel_values:4\r\n" {
		t.Fatalf("emb.image.info = %q", got)
	}
	if entry.ImageRes != nil || entry.ImageFootprint() != 0 {
		t.Fatalf("emb.image.info opened image sessions (res=%v footprint=%d)", entry.ImageRes, entry.ImageFootprint())
	}
}

// scriptedImageEmbed is a script whose only work is a single emb.image.embed;
// distinct variants defeat the script-reply cache while sharing the image
// cache.
func scriptedImageEmbed(variant int) string {
	return "return #emb.image.embed(KEYS[1]) + " + strconv.Itoa(variant)
}

// TestScriptedImageEmbedSharesNativeCache verifies EMB.IMG followed by a
// scripted emb.image.embed of the same bytes performs no second inference.
func TestScriptedImageEmbedSharesNativeCache(t *testing.T) {
	addr, _, sessions := serveImage(t, "1MB")
	png := solidImagePNG(t, color.White)

	if got := bulkOf(t, redisCmd(t, addr, "EMB.IMG", "imgA", png)); len(got) == 0 {
		t.Fatal("EMB.IMG returned an empty embedding")
	}
	if c := sessions["imgA"].callCount(); c != 1 {
		t.Fatalf("EMB.IMG runs = %d, want 1", c)
	}
	redisCmd(t, addr, "EMB.EVAL", "imgA", scriptedImageEmbed(0), "1", png)
	if c := sessions["imgA"].callCount(); c != 1 {
		t.Fatalf("scripted embed after EMB.IMG ran inference again (runs=%d, want 1)", c)
	}
}

// TestNativeImageHitsScriptedCache verifies the reverse direction: an image
// embedded by a script is a cache hit for EMB.IMG.
func TestNativeImageHitsScriptedCache(t *testing.T) {
	addr, _, sessions := serveImage(t, "1MB")
	png := solidImagePNG(t, color.White)

	redisCmd(t, addr, "EMB.EVAL", "imgA", scriptedImageEmbed(0), "1", png)
	if c := sessions["imgA"].callCount(); c != 1 {
		t.Fatalf("scripted embed runs = %d, want 1", c)
	}
	if got := bulkOf(t, redisCmd(t, addr, "EMB.IMG", "imgA", png)); len(got) == 0 {
		t.Fatal("EMB.IMG returned an empty embedding")
	}
	if c := sessions["imgA"].callCount(); c != 1 {
		t.Fatalf("EMB.IMG after scripted embed ran inference again (runs=%d, want 1)", c)
	}
}

// TestRepeatedScriptedImageEmbedInfersOnce verifies two scripted embeddings of
// the same bytes invoke the image branch once.
func TestRepeatedScriptedImageEmbedInfersOnce(t *testing.T) {
	addr, _, sessions := serveImage(t, "1MB")
	png := solidImagePNG(t, color.White)

	redisCmd(t, addr, "EMB.EVAL", "imgA", scriptedImageEmbed(0), "1", png)
	redisCmd(t, addr, "EMB.EVAL", "imgA", scriptedImageEmbed(1), "1", png)
	if c := sessions["imgA"].callCount(); c != 1 {
		t.Fatalf("repeated scripted embeds ran inference %d times, want 1", c)
	}
}

// TestScriptedImageCacheStaysWithinBudget verifies the scripted image path
// writes through the same bounded LRU as EMB.IMG: many distinct images never
// exceed the configured cache budget.
func TestScriptedImageCacheStaysWithinBudget(t *testing.T) {
	addr, srv, _ := serveImage(t, "1KB")
	if srv.cache == nil {
		t.Fatal("cache not configured")
	}
	for i := 0; i < 200; i++ {
		png := solidImagePNG(t, color.NRGBA{R: uint8(i), G: uint8(i * 7), B: uint8(i * 13), A: 255})
		redisCmd(t, addr, "EMB.EVAL", "imgA", scriptedImageEmbed(0), "1", png)
	}
	stats := srv.cache.Stats()
	if stats.MaxBytes <= 0 {
		t.Fatalf("cache max bytes = %d, want > 0", stats.MaxBytes)
	}
	if stats.CurBytes > stats.MaxBytes {
		t.Fatalf("cache bytes %d exceed the configured maximum %d", stats.CurBytes, stats.MaxBytes)
	}
	if stats.Evictions == 0 {
		t.Fatal("expected evictions for 200 distinct images in a 1KB cache")
	}
}

// TestScriptedEvaluationsNoLeak is the scripted analogue of the server leak
// gate: thousands of scripted evaluations across persistent connections leave
// goroutines at baseline and heap flat after GC. It uses a constant script so
// the gate measures the interpreter/host path, not inference.
func TestScriptedEvaluationsNoLeak(t *testing.T) {
	addr, _ := serveScriptTest(t, "")
	const script = `return #KEYS + #ARGV`
	conns := make([]net.Conn, 3)
	for i := range conns {
		conns[i] = dial(t, addr)
	}
	eval := func(c net.Conn) {
		t.Helper()
		c.Write(respCommand("EMB.EVAL", "test", script, "1", "leak-probe", "arg"))
		readRESP(t, c)
	}

	eval(conns[0])
	runtime.GC()
	baseGoroutines := runtime.NumGoroutine()
	baseHeap := registry.HeapInUseBytes()

	const reqs = 3000
	for i := 0; i < reqs; i++ {
		eval(conns[i%len(conns)])
	}

	runtime.GC()
	gotGoroutines := runtime.NumGoroutine()
	gotHeap := registry.HeapInUseBytes()
	if gotGoroutines > baseGoroutines+3 {
		t.Errorf("goroutines grew across %d scripted evaluations: %d -> %d", reqs, baseGoroutines, gotGoroutines)
	}
	if growth := int64(gotHeap) - int64(baseHeap); growth > 8<<20 {
		t.Errorf("heap grew %d bytes across %d scripted evaluations (leak?)", growth, reqs)
	}
}

// TestScriptedLifecycleNoAccumulation verifies creating, exercising and closing
// a scripted model repeatedly does not accumulate goroutines or heap.
func TestScriptedLifecycleNoAccumulation(t *testing.T) {
	if !ortOK {
		t.Skip("onnx runtime unavailable (run inside nix develop)")
	}
	cycle := func() {
		reg := registry.New()
		entry, err := registry.LoadModel(config.ModelConfig{
			ONNX:      "../../models/minilm/model.onnx",
			Tokenizer: "../../models/minilm/tokenizer.json",
			Dim:       384,
			MaxLength: 128,
			Pooling:   "mean",
		}, "test")
		if err != nil {
			t.Skipf("script test model not present: %v (run: just download-model)", err)
		}
		reg.Add("test", entry)
		if _, err := entry.ScriptResources(); err != nil {
			t.Fatal(err)
		}
		if err := reg.Close(); err != nil {
			t.Fatal(err)
		}
	}

	cycle() // establish a warm baseline (lazily initialized runtime state)
	runtime.GC()
	baseGoroutines := runtime.NumGoroutine()
	baseHeap := registry.HeapInUseBytes()

	for i := 0; i < 3; i++ {
		cycle()
	}
	runtime.GC()
	gotGoroutines := runtime.NumGoroutine()
	gotHeap := registry.HeapInUseBytes()
	if gotGoroutines > baseGoroutines+2 {
		t.Errorf("goroutines accumulated across lifecycles: %d -> %d", baseGoroutines, gotGoroutines)
	}
	if growth := int64(gotHeap) - int64(baseHeap); growth > 16<<20 {
		t.Errorf("heap grew %d bytes across 3 model lifecycles", growth)
	}
}

// TestScriptedFailureModesReleaseResources verifies each classic failure mode
// leaves the model usable and the output-tensor cache bounded, with no growth
// across repetition.
func TestScriptedFailureModesReleaseResources(t *testing.T) {
	addr, srv := serveScriptTest(t, "")
	entry, err := srv.reg.Resolve("test")
	if err != nil {
		t.Fatal(err)
	}
	// Open the scripted session up front so its cache is observable.
	res, err := entry.ScriptResources()
	if err != nil {
		t.Fatal(err)
	}
	sess := res.Sessions()[0]

	modes := []struct {
		name   string
		script string
	}{
		{"tensor budget exceeded", `local out = emb.run({x = {shape = {1, 1000000000}, fill = 1}}); return 1`},
		{"graph error", `local out = emb.run({input_ids = {shape = {1, 2}, dtype = "f32", data = {1.5, 2.5}}}); return 1`},
		{"unknown output", unknownOutputScript},
		{"post-run script error", strings.Replace(runShapeScript, "return #out.last_hidden_state.shape", `error("boom")`, 1)},
	}
	c := dial(t, addr)
	defer c.Close()
	for _, m := range modes {
		t.Run(m.name, func(t *testing.T) {
			before := cachedOutputSets(t, sess)
			for i := 0; i < 3; i++ {
				got := doCmd(t, c, "EMB.EVAL", "test", m.script, "1", "hello")
				if !strings.HasPrefix(got, "-ERR") {
					t.Fatalf("failure mode reply = %q, want an error", got)
				}
			}
			after := cachedOutputSets(t, sess)
			// At most one new shape signature may be retained by a run that
			// succeeded before the script failed; nothing accumulates over
			// repeated failures.
			if after > before+1 {
				t.Fatalf("retained output sets grew %d -> %d across repeated failures", before, after)
			}
			if after > maxCachedOutputSetAssertion {
				t.Fatalf("retained output sets %d exceed the documented cap %d", after, maxCachedOutputSetAssertion)
			}
			// The model stays usable after a failure.
			if got := doCmd(t, c, "EMB.EVAL", "test", "return 7", "1", "ok"); got != ":7\r\n" {
				t.Fatalf("evaluation after failure = %q, want :7", got)
			}
		})
	}
}

// maxCachedOutputSetAssertion mirrors onnx.maxCachedOutputShapes; the script
// session cache cap, restated here because the constant is unexported.
const maxCachedOutputSetAssertion = 4

// unknownOutputScript runs the graph successfully and then asks for an output
// that does not exist, so the failure happens after the run.
const unknownOutputScript = `local e = emb.tokenize.encode(KEYS[1], 8)
local out = emb.run({input_ids = {shape = {1, #e.ids}, data = e.ids},
  attention_mask = {shape = {1, #e.mask}, data = e.mask},
  token_type_ids = {shape = {1, #e.ids}, fill = 0, dtype = "i64"}}, {outputs = {"nonexistent"}})
return 1`

// cachedOutputSets returns the number of output-shape signatures a scripted
// session is retaining.
func cachedOutputSets(t *testing.T, sess onnx.NamedSession) int {
	t.Helper()
	s, ok := sess.(interface{ CachedOutputSets() int })
	if !ok {
		t.Fatalf("scripted session %T does not expose CachedOutputSets", sess)
	}
	return s.CachedOutputSets()
}

// TestScriptSourceCacheBounded verifies the per-model script source cache stays
// at its cap under many distinct scripts, and that each model is bounded
// independently.
func TestScriptSourceCacheBounded(t *testing.T) {
	c := newScriptCache(4)
	for i := 0; i < 12; i++ {
		c.Load("test", fmt.Sprintf("return %d", i))
		c.Load("other", fmt.Sprintf("return %d", i))
	}
	if got := len(c.by["test"]); got != 4 {
		t.Fatalf("script cache for test = %d entries, want 4", got)
	}
	if got := len(c.by["other"]); got != 4 {
		t.Fatalf("script cache for other = %d entries, want 4", got)
	}
}
