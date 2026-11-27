package registry

import (
	"errors"
	"runtime"
	"testing"
	"time"

	"github.com/elcuervo/emb/internal/config"
	"github.com/elcuervo/emb/internal/imageproc"
	"github.com/elcuervo/emb/internal/onnx"
	"github.com/elcuervo/emb/internal/permit"
)

// attributionGatedSession blocks every run until release is closed, and reports each
// start on started, so a concurrent Embed can be made to queue behind it.
type attributionGatedSession struct {
	started chan struct{}
	release chan struct{}
	outputs map[string]onnx.NamedTensor
}

func (g *attributionGatedSession) RunNamed(inputs []onnx.NamedTensor) (map[string]onnx.NamedTensor, error) {
	select {
	case g.started <- struct{}{}:
	default:
	}
	<-g.release
	return g.outputs, nil
}

func (g *attributionGatedSession) Close() error { return nil }

// admittedFakeSession returns a fixed admission wait from RunNamedAdmitted,
// standing in for the CPU gate wrapper.
type admittedFakeSession struct {
	*fakeNamedSession
	admission time.Duration
	err       error
}

func (f *admittedFakeSession) RunNamedAdmitted(inputs []onnx.NamedTensor) (map[string]onnx.NamedTensor, time.Duration, error) {
	// Spend the wait: runTimed subtracts it from the timed span, so an unspent
	// wait reports negative run time.
	time.Sleep(f.admission)
	out, err := f.RunNamed(inputs)
	if f.err != nil {
		err = f.err
	}
	return out, f.admission, err
}

func attributionPlan() (imageproc.Plan, []ImageOutput) {
	return imageproc.Plan{Input: "pixel_values", Size: 2, Rescale: 1, Std: [3]float64{1, 1, 1}},
		[]ImageOutput{{Name: "embedding", Tensor: "image_embeds", Select: "none", Rows: 1, Dim: 2}}
}

func attributionOutputs() map[string]onnx.NamedTensor {
	return map[string]onnx.NamedTensor{
		"image_embeds": {Name: "image_embeds", Shape: []int64{1, 2}, DType: onnx.TensorFloat32, Float: []float32{1, 2}},
	}
}

// TestImageEmbedDispatchWaitAttribution checks that a second concurrent Embed
// pays the wait for the only free session, and that the first call, which got
// it immediately, does not inherit that wait.
func TestImageEmbedDispatchWaitAttribution(t *testing.T) {
	plan, outs := attributionPlan()
	gated := &attributionGatedSession{started: make(chan struct{}, 1), release: make(chan struct{}), outputs: attributionOutputs()}
	res := &ImageResources{
		Sessions: []onnx.NamedSession{gated},
		idle:     make(chan onnx.NamedSession, 1),
		Plan:     plan,
		Outputs:  outs,
	}
	res.idle <- gated
	tensor := make([]float32, plan.ElementCount())

	type result struct {
		timing ImageRunTiming
		err    error
	}
	first := make(chan result, 1)
	go func() {
		_, timing, err := res.Embed([][]float32{tensor}, outs)
		first <- result{timing: timing, err: err}
	}()
	<-gated.started
	if _, _, _, _, _, busy := res.DispatchCounters(); busy != 1 {
		t.Fatalf("busy gauge = %d while a session is leased, want 1", busy)
	}

	second := make(chan result, 1)
	launched := make(chan struct{})
	go func() {
		close(launched)
		_, timing, err := res.Embed([][]float32{tensor}, outs)
		second <- result{timing: timing, err: err}
	}()
	<-launched
	// Let the queued call reach its receive before the session is handed back:
	// channel scheduling, not a sleep.
	runtime.Gosched()
	close(gated.release)

	got1, got2 := <-first, <-second
	if got1.err != nil || got2.err != nil {
		t.Fatalf("embed errors: %v, %v", got1.err, got2.err)
	}
	if got1.timing.Runs != 1 || got2.timing.Runs != 1 {
		t.Fatalf("runs = %d, %d, want 1, 1", got1.timing.Runs, got2.timing.Runs)
	}
	if got2.timing.DispatchWait <= 0 {
		t.Fatalf("second dispatch wait = %v, want > 0", got2.timing.DispatchWait)
	}

	pre, dispatchUs, admissionUs, runUs, runs, busy := res.DispatchCounters()
	if pre != 0 || admissionUs != 0 {
		t.Fatalf("unexpected preprocess/admission = %d/%d", pre, admissionUs)
	}
	if dispatchUs < got2.timing.DispatchWait.Microseconds() {
		t.Fatalf("dispatch total %d us < the queued call's %d us", dispatchUs, got2.timing.DispatchWait.Microseconds())
	}
	if runs != 2 || busy != 0 {
		t.Fatalf("runs/busy = %d/%d, want 2/0", runs, busy)
	}
	if runUs < got1.timing.Run.Microseconds()+got2.timing.Run.Microseconds() {
		t.Fatalf("run total %d us is below the sum of the per-call runs", runUs)
	}
}

// TestImageEmbedAdmissionWaitExcludedFromRun checks that the CPU-gate wait is
// surfaced as its own phase and subtracted from the run, not folded into it.
func TestImageEmbedAdmissionWaitExcludedFromRun(t *testing.T) {
	plan, outs := attributionPlan()
	const admission = 50 * time.Millisecond
	sess := &admittedFakeSession{fakeNamedSession: &fakeNamedSession{outputs: attributionOutputs()}, admission: admission}
	res := &ImageResources{Sessions: []onnx.NamedSession{sess}, Plan: plan, Outputs: outs}

	start := time.Now()
	_, timing, err := res.Embed([][]float32{make([]float32, plan.ElementCount())}, outs)
	elapsed := time.Since(start)
	if err != nil {
		t.Fatal(err)
	}
	if timing.AdmissionWait != admission {
		t.Fatalf("admission wait = %v, want %v", timing.AdmissionWait, admission)
	}
	if timing.Runs != 1 {
		t.Fatalf("runs = %d, want 1", timing.Runs)
	}
	// Run + admission covers exactly the span the run occupied: a run that
	// still included the gate wait would overshoot elapsed by admission.
	if diff := timing.Run + timing.AdmissionWait - elapsed; diff > 10*time.Millisecond || diff < -10*time.Millisecond {
		t.Fatalf("run+admission = %v, elapsed = %v (run %v, admission %v)", timing.Run+timing.AdmissionWait, elapsed, timing.Run, timing.AdmissionWait)
	}

	_, _, admissionUs, runUs, runs, _ := res.DispatchCounters()
	if admissionUs != admission.Microseconds() {
		t.Fatalf("admission total = %d us, want %d", admissionUs, admission.Microseconds())
	}
	if runUs != timing.Run.Microseconds() || runs != 1 {
		t.Fatalf("run total/runs = %d us/%d, want %d/1", runUs, runs, timing.Run.Microseconds())
	}
}

// TestImageEmbedAdmissionRefusalNotCounted checks that a refused admission is
// not a run: its wait is reported, but no run is counted.
func TestImageEmbedAdmissionRefusalNotCounted(t *testing.T) {
	plan, outs := attributionPlan()
	const admission = 2 * time.Millisecond
	sess := &admittedFakeSession{
		fakeNamedSession: &fakeNamedSession{outputs: attributionOutputs()},
		admission:        admission,
		err:              permit.ErrBusy,
	}
	res := &ImageResources{Sessions: []onnx.NamedSession{sess}, Plan: plan, Outputs: outs}

	_, timing, err := res.Embed([][]float32{make([]float32, plan.ElementCount())}, outs)
	if !errors.Is(err, permit.ErrBusy) {
		t.Fatalf("error = %v, want the gate's refusal", err)
	}
	if timing.Runs != 0 {
		t.Fatalf("refused run counted as %d runs", timing.Runs)
	}
	if timing.AdmissionWait != admission {
		t.Fatalf("admission wait = %v, want %v", timing.AdmissionWait, admission)
	}
	_, _, admissionUs, _, runs, _ := res.DispatchCounters()
	if runs != 0 || admissionUs != admission.Microseconds() {
		t.Fatalf("runs/admission total = %d/%d, want 0/%d", runs, admissionUs, admission.Microseconds())
	}
}

// TestImageDispatchTotalsAccumulate checks that per-call timings are summed
// into the model totals and that preprocessing accumulates separately.
func TestImageDispatchTotalsAccumulate(t *testing.T) {
	plan, outs := attributionPlan()
	fake := &fakeNamedSession{outputs: attributionOutputs()}
	res := &ImageResources{Sessions: []onnx.NamedSession{fake}, Plan: plan, Outputs: outs}

	res.AddPreprocess(3 * time.Millisecond)
	res.AddPreprocess(2 * time.Millisecond)

	tensor := [][]float32{make([]float32, plan.ElementCount())}
	var wantRunUs int64
	var prev int64
	for i := range 3 {
		_, timing, err := res.Embed(tensor, outs)
		if err != nil {
			t.Fatal(err)
		}
		if timing.DispatchWait != 0 {
			t.Fatalf("run %d dispatch wait = %v, want 0 for a test-injected pool", i, timing.DispatchWait)
		}
		wantRunUs += timing.Run.Microseconds()

		pre, _, _, runUs, runs, _ := res.DispatchCounters()
		if pre != 5000 {
			t.Fatalf("preprocess total = %d us, want 5000", pre)
		}
		if runs != int64(i+1) || runs < prev {
			t.Fatalf("run total = %d after run %d, want %d and monotonic", runs, i, i+1)
		}
		if runUs != wantRunUs {
			t.Fatalf("run total = %d us after run %d, want %d (sum of per-call runs)", runUs, i, wantRunUs)
		}
		prev = runs
	}
}

// TestImageDispatchUnopenedModel checks that reading image metrics neither
// opens sessions nor reports phantom work.
func TestImageDispatchUnopenedModel(t *testing.T) {
	entry := &ModelEntry{cfg: config.ModelConfig{Image: &config.ImageConfig{Input: "pixel_values", Size: 2}}}

	pre, dispatchUs, admissionUs, runUs, runs, busy := entry.ImageDispatch()
	if pre != 0 || dispatchUs != 0 || admissionUs != 0 || runUs != 0 || runs != 0 || busy != 0 {
		t.Fatalf("unopened image dispatch = %d,%d,%d,%d,%d,%d, want all zeros",
			pre, dispatchUs, admissionUs, runUs, runs, busy)
	}
	if entry.ImageRes != nil {
		t.Fatal("ImageDispatch opened the image pool")
	}
	if got := entry.ImageFootprint(); got != 0 {
		t.Fatalf("image footprint = %d, want 0", got)
	}
}

// TestImageDispatchPublishedAfterInjection checks that an injected pool becomes
// visible to ImageDispatch once ImageResources has returned it.
func TestImageDispatchPublishedAfterInjection(t *testing.T) {
	plan, outs := attributionPlan()
	res := &ImageResources{Sessions: []onnx.NamedSession{&fakeNamedSession{outputs: attributionOutputs()}}, Plan: plan, Outputs: outs}
	res.AddPreprocess(time.Millisecond)
	entry := &ModelEntry{ImageRes: res}

	pre, _, _, _, _, _ := entry.ImageDispatch()
	if pre != 0 {
		t.Fatalf("pre-publication dispatch = %d us, want 0", pre)
	}
	if _, err := entry.ImageResources(); err != nil {
		t.Fatal(err)
	}
	pre, _, _, _, _, _ = entry.ImageDispatch()
	if pre != time.Millisecond.Microseconds() {
		t.Fatalf("published dispatch = %d us, want %d", pre, time.Millisecond.Microseconds())
	}
}
