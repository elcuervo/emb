package pipeline

import (
	"testing"
	"time"

	"github.com/elcuervo/emb/internal/onnx"
)

// TestPoolResponseCarriesThePhases covers the attribution contract on the
// unbatched path: the request's response carries the time it waited for a
// worker, the time it waited at the CPU gate and its own run, each equal to what
// the model's cumulative counters recorded for that same request.
func TestPoolResponseCarriesThePhases(t *testing.T) {
	const gateWait = 10 * time.Millisecond
	p, err := NewPool(func() (onnx.Session, error) {
		return &admittingSession{wait: gateWait}, nil
	}, fakeTok{}, 1, 2, 16, false, "mean", 0, 32, 0, 0)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = p.Close() }()

	before := p.Stats()
	resp, err := p.Embed([]string{"hello"})
	if err != nil {
		t.Fatal(err)
	}
	after := p.Stats()

	// The counters hold microseconds, so the comparison is at that resolution.
	if got, want := resp.AdmissionWait.Microseconds(), after.AdmissionWaitUs-before.AdmissionWaitUs; got != want {
		t.Errorf("response admission wait = %dus, model recorded %dus", got, want)
	}
	if got, want := resp.Run.Microseconds(), after.RunUs-before.RunUs; got != want {
		t.Errorf("response run = %dus, model recorded %dus", got, want)
	}
	if got, want := resp.DispatchWait.Microseconds(), after.DispatchWaitUs-before.DispatchWaitUs; got != want {
		t.Errorf("response dispatch wait = %dus, model recorded %dus", got, want)
	}

	// The gate wait is not run time: a run that returned instantly is not
	// charged the ten milliseconds it waited before it could start.
	if resp.Run > time.Millisecond {
		t.Errorf("run absorbed the admission wait: run=%s admit=%s", resp.Run, resp.AdmissionWait)
	}
	if resp.AdmissionWait != gateWait {
		t.Errorf("admission wait = %s, want %s", resp.AdmissionWait, gateWait)
	}
}

// TestBatcherResponseCarriesThePhases covers the same contract on the batched
// path, where every item of a batch waits for and shares that batch's run.
func TestBatcherResponseCarriesThePhases(t *testing.T) {
	const gateWait = 10 * time.Millisecond
	b := NewBatcher(&admittingSession{wait: gateWait}, fakeTok{}, 2, 16, false, "mean", 50, 8, 0, 0)
	defer func() { _ = b.Close() }()

	beforeDispatch, beforeAdmit, beforeRun := b.dispatchWaitUs.Load(), b.admissionWaitUs.Load(), b.totalLat.Load()
	resp, err := b.Embed([]string{"hello"})
	if err != nil {
		t.Fatal(err)
	}

	// The counters hold microseconds, so the comparison is at that resolution.
	if got, want := resp.DispatchWait.Microseconds(), b.dispatchWaitUs.Load()-beforeDispatch; got != want {
		t.Errorf("response dispatch wait = %dus, batch recorded %dus", got, want)
	}
	if got, want := resp.AdmissionWait.Microseconds(), b.admissionWaitUs.Load()-beforeAdmit; got != want {
		t.Errorf("response admission wait = %dus, batch recorded %dus", got, want)
	}
	if got, want := resp.Run.Microseconds(), b.totalLat.Load()-beforeRun; got != want {
		t.Errorf("response run = %dus, batch recorded %dus", got, want)
	}
	if resp.Run > time.Millisecond {
		t.Errorf("run absorbed the admission wait: run=%s admit=%s", resp.Run, resp.AdmissionWait)
	}
}

// TestResponsePhasesSumWithinTheRequest keeps the attribution honest: what a
// request reports about itself never exceeds the wall time its caller waited.
func TestResponsePhasesSumWithinTheRequest(t *testing.T) {
	b := NewBatcher(&recordingSession{dim: 2, sleep: 2 * time.Millisecond}, fakeTok{}, 2, 16, false, "mean", 20, 8, 0, 0)
	defer func() { _ = b.Close() }()

	start := time.Now()
	resp, err := b.Embed([]string{"hello", "world"})
	if err != nil {
		t.Fatal(err)
	}
	elapsed := time.Since(start)

	if resp.Run <= 0 {
		t.Errorf("a completed run reported no run time: %+v", resp)
	}
	if sum := resp.DispatchWait + resp.AdmissionWait + resp.Run; sum > elapsed {
		t.Errorf("phases %s exceed the request's %s", sum, elapsed)
	}
}
