package server

import (
	"image/color"
	"strings"
	"testing"
	"time"

	"github.com/elcuervo/emb/internal/onnx"
	"github.com/elcuervo/emb/internal/registry"
)

// imageDispatch reads the model's image phase totals.
func imageDispatch(t *testing.T, srv *Server, model string) (pre, disp, admit, run, runs, busy int64) {
	t.Helper()
	entry, err := srv.reg.Resolve(model)
	if err != nil {
		t.Fatalf("resolve %s: %v", model, err)
	}
	return entry.ImageDispatch()
}

// TestImageAttributionCacheMiss proves a miss reports its measured phases, its
// image count, and an incremented run total.
func TestImageAttributionCacheMiss(t *testing.T) {
	addr, srv, sessions := serveImage(t, "1MB")
	sessions["imgA"].admitWait = 7 * time.Millisecond
	png := solidImagePNG(t, color.NRGBA{R: 9, A: 255})

	ev := fetchEvent(t, addr, "EMB.IMG", "imgA", png)
	if got := intOf(t, ev["texts"]); got != 1 {
		t.Errorf("image event texts = %d, want the image count 1", got)
	}
	if got := intOf(t, ev["cached_texts"]); got != 0 {
		t.Errorf("cache miss reports cached_texts = %d, want 0", got)
	}
	if got := intOf(t, ev["attributed"]); got != 1 {
		t.Errorf("cache miss reports attributed = %d, want 1", got)
	}
	if got := intOf(t, ev["preprocess_us"]); got <= 0 {
		t.Errorf("cache miss reports preprocess_us = %d, want > 0", got)
	}
	if got := intOf(t, ev["admit_us"]); got != 7000 {
		t.Errorf("cache miss reports admit_us = %d, want the session's 7000", got)
	}

	_, _, _, _, runs, _ := imageDispatch(t, srv, "imgA")
	if runs != 1 {
		t.Errorf("image runs = %d, want 1", runs)
	}
}

// TestImageAttributionFullCacheHit proves a cache-served image is attributed
// with zero phases and runs no inference.
func TestImageAttributionFullCacheHit(t *testing.T) {
	addr, srv, sessions := serveImage(t, "1MB")
	png := solidImagePNG(t, color.NRGBA{G: 9, A: 255})

	redisCmd(t, addr, "EMB.IMG", "imgA", png)
	before := sessions["imgA"].callCount()
	_, _, _, _, runsBefore, _ := imageDispatch(t, srv, "imgA")

	ev := fetchEvent(t, addr, "EMB.IMG", "imgA", png)
	if got := intOf(t, ev["cached_texts"]); got != 1 {
		t.Errorf("cache hit reports cached_texts = %d, want 1", got)
	}
	if got := intOf(t, ev["attributed"]); got != 1 {
		t.Errorf("cache hit reports attributed = %d, want 1", got)
	}
	for _, phase := range []string{"preprocess_us", "dispatch_us", "admit_us", "run_us"} {
		if got := intOf(t, ev[phase]); got != 0 {
			t.Errorf("cache hit reports %s = %d, want 0 (nothing ran)", phase, got)
		}
	}
	if got := sessions["imgA"].callCount(); got != before {
		t.Errorf("session runs = %d, want unchanged %d", got, before)
	}
	if _, _, _, _, runs, _ := imageDispatch(t, srv, "imgA"); runs != runsBefore {
		t.Errorf("image runs = %d, want unchanged %d", runs, runsBefore)
	}
}

// serveImageTwoOutputs starts a server whose imgA model has two independent
// image products, so one can be cached while the other still misses.
func serveImageTwoOutputs(t *testing.T) (string, *Server, *fakeImageSession) {
	t.Helper()
	fake := &fakeImageSession{dim: testImageDim, tensors: []fakeTensorSpec{
		{name: "image_embeds", rows: 1},
		{name: "pooled", rows: 1},
	}}
	reg := registry.New()
	reg.Add("imgA", &registry.ModelEntry{
		Name: "imgA",
		Dim:  testImageDim,
		ImageRes: &registry.ImageResources{
			Sessions: []onnx.NamedSession{fake},
			Plan:     testImagePlan(4),
			Outputs: []registry.ImageOutput{
				{Name: "embedding", Tensor: "image_embeds", Select: "none", Rows: 1, Dim: testImageDim},
				{Name: "pooled", Tensor: "pooled", Select: "none", Rows: 1, Dim: testImageDim},
			},
		},
	})
	addr := getFreeAddr()
	srv := New(addr, reg, "", "1MB", nil)
	go srv.ListenAndServe()
	t.Cleanup(func() { srv.Close() })
	time.Sleep(50 * time.Millisecond)
	return addr, srv, fake
}

// TestImageAttributionPartialCacheHit proves a partly cached image reports the
// miss's phases and does not count itself as cache-served.
func TestImageAttributionPartialCacheHit(t *testing.T) {
	addr, srv, _ := serveImageTwoOutputs(t)
	png := solidImagePNG(t, color.NRGBA{B: 9, A: 255})

	// Cache one product only.
	redisCmd(t, addr, "EMB.IMG", "imgA", "OUTPUTS", "embedding", png)
	_, _, _, _, runsBefore, _ := imageDispatch(t, srv, "imgA")

	ev := fetchEvent(t, addr, "EMB.IMG", "imgA", png)
	if got := intOf(t, ev["cached_texts"]); got != 0 {
		t.Errorf("partly cached image reports cached_texts = %d, want 0", got)
	}
	if got := intOf(t, ev["attributed"]); got != 1 {
		t.Errorf("partly cached image reports attributed = %d, want 1", got)
	}
	if got := intOf(t, ev["preprocess_us"]); got <= 0 {
		t.Errorf("partly cached image reports preprocess_us = %d, want > 0", got)
	}
	if _, _, _, _, runs, _ := imageDispatch(t, srv, "imgA"); runs != runsBefore+1 {
		t.Errorf("image runs = %d, want %d (only the miss ran)", runs, runsBefore+1)
	}
}

// TestImageAttributionMultiChunk proves a request that runs several chunks sums
// their phases and run counts.
func TestImageAttributionMultiChunk(t *testing.T) {
	addr, srv, sessions := serveImageBatchCap(t, 1)
	sessions["imgA"].admitWait = 5 * time.Millisecond
	a := solidImagePNG(t, color.NRGBA{R: 1, A: 255})
	b := solidImagePNG(t, color.NRGBA{G: 2, A: 255})
	c := solidImagePNG(t, color.NRGBA{B: 3, A: 255})

	ev := fetchEvent(t, addr, "EMB.IMG", "imgA", a, b, c)
	if got := intOf(t, ev["texts"]); got != 3 {
		t.Errorf("image event texts = %d, want 3", got)
	}
	if got := intOf(t, ev["cached_texts"]); got != 0 {
		t.Errorf("multi-chunk request reports cached_texts = %d, want 0", got)
	}
	if got := intOf(t, ev["admit_us"]); got != 15000 {
		t.Errorf("multi-chunk admit_us = %d, want three sessions' 15000", got)
	}
	if _, _, _, _, runs, _ := imageDispatch(t, srv, "imgA"); runs != 3 {
		t.Errorf("image runs = %d, want 3 (one per chunk)", runs)
	}
}

// TestInfoReportsImagePhaseTotals proves EMB.INFO carries the image keys and
// reading them opens no session.
func TestInfoReportsImagePhaseTotals(t *testing.T) {
	addr, srv, _ := serveImage(t, "1MB")
	png := solidImagePNG(t, color.White)
	redisCmd(t, addr, "EMB.IMG", "imgA", png)

	entry, err := srv.reg.Resolve("imgB")
	if err != nil {
		t.Fatal(err)
	}
	if got := entry.ImageFootprint(); got != 0 {
		t.Fatalf("imgB sessions opened before the read: %d", got)
	}
	info := replyPairs(t, redisCmd(t, addr, "EMB.INFO", "imgB"))
	for _, key := range []string{
		"image_preprocess_us", "image_dispatch_wait_us", "image_admission_wait_us",
		"image_run_us", "image_runs", "image_sessions_busy",
	} {
		if _, ok := info[key]; !ok {
			t.Errorf("EMB.INFO is missing %q", key)
		}
	}
	if got := entry.ImageFootprint(); got != 0 {
		t.Errorf("reading image totals opened %d sessions, want none", got)
	}
}

// TestStatsReportsPerModelImages proves EMB.STATS surfaces image traffic per
// model.
func TestStatsReportsPerModelImages(t *testing.T) {
	addr, _, _ := serveImage(t, "1MB")
	redisCmd(t, addr, "EMB.IMG", "imgA", solidImagePNG(t, color.White))

	stats := replyPairs(t, redisCmd(t, addr, "EMB.STATS"))
	perModel := bulkOf(t, stats["per_model_images"])
	if perModel == "" {
		t.Fatal("per_model_images is empty after image traffic")
	}
	if !containsAll(perModel, "imgA: preprocess=", " dispatch=", " admit=", " run=", " runs=1 ", " busy=") {
		t.Errorf("per_model_images = %q, want imgA's phase totals", perModel)
	}
}

// TestImageMultiAttribution proves each EMB.IMGMULTI pair event carries the
// phases that pair paid, not the whole command's.
func TestImageMultiAttribution(t *testing.T) {
	addr, srv, sessions := serveImage(t, "1MB")
	sessions["imgA"].admitWait = 3 * time.Millisecond
	png := solidImagePNG(t, color.NRGBA{R: 7, A: 255})

	ev := fetchEvent(t, addr, "EMB.IMGMULTI", "imgA", png)
	if got := intOf(t, ev["texts"]); got != 1 {
		t.Errorf("pair event texts = %d, want its one image", got)
	}
	if got := intOf(t, ev["cached_texts"]); got != 0 {
		t.Errorf("pair event cached_texts = %d, want 0", got)
	}
	if got := intOf(t, ev["attributed"]); got != 1 {
		t.Errorf("pair event attributed = %d, want 1", got)
	}
	if got := intOf(t, ev["admit_us"]); got != 3000 {
		t.Errorf("pair event admit_us = %d, want the session's 3000", got)
	}
	if _, _, _, _, runs, _ := imageDispatch(t, srv, "imgA"); runs != 1 {
		t.Errorf("image runs = %d, want 1", runs)
	}
}

func containsAll(s string, parts ...string) bool {
	for _, p := range parts {
		if !strings.Contains(s, p) {
			return false
		}
	}
	return true
}
