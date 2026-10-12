package registry

import (
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/elcuervo/emb/internal/onnx"
)

func TestCalibrationCandidatesRespectProcessBudget(t *testing.T) {
	for _, candidate := range calibrationCandidates(5, 4, 0, 0, false) {
		if candidate[0] < 1 || candidate[1] < 1 || candidate[0]*candidate[1] > 5 {
			t.Fatalf("invalid candidate %v for budget 5", candidate)
		}
		if candidate[0] > 4 {
			t.Fatalf("candidate %v exceeds the memory-estimate worker cap 4", candidate)
		}
	}
	got := calibrationCandidates(4, 4, 2, 0, true)
	if len(got) != 1 || got[0] != [2]int{1, 2} {
		t.Fatalf("batched calibration with explicit width should only test one session at width 2, got %v", got)
	}
	// The batcher owns embedding inference, so a determinism probe that kept
	// batching (batched=true) may not screen batch-sized worker counts; the
	// unbatched path may.
	for _, candidate := range calibrationCandidates(4, 4, 0, 0, true) {
		if candidate[0] != 1 {
			t.Fatalf("batched candidate %v has more than one session", candidate)
		}
	}
	if got := calibrationCandidates(4, 4, 0, 0, false); len(got) < 2 {
		t.Fatalf("unbatched calibration should screen multiple worker counts, got %v", got)
	}
	// A dimension the operator set explicitly is fixed for that dimension only.
	explicitWidth := calibrationCandidates(4, 4, 2, 0, false)
	for _, candidate := range explicitWidth {
		if candidate[1] != 2 {
			t.Fatalf("explicit width candidate %v changed the width", candidate)
		}
	}
	explicitWorkers := calibrationCandidates(6, 6, 0, 3, false)
	for _, candidate := range explicitWorkers {
		if candidate[0] != 3 {
			t.Fatalf("explicit worker candidate %v changed the session count", candidate)
		}
	}
}

// calBatchProbe records the shape of every Run a fake session serves.
type calBatchRun struct{ batch, seqLen int }

type calBatchProbe struct {
	mu   sync.Mutex
	runs []calBatchRun
}

func (p *calBatchProbe) record(batch, seqLen int) {
	p.mu.Lock()
	p.runs = append(p.runs, calBatchRun{batch: batch, seqLen: seqLen})
	p.mu.Unlock()
}

func (p *calBatchProbe) seen() []calBatchRun {
	p.mu.Lock()
	defer p.mu.Unlock()
	return append([]calBatchRun(nil), p.runs...)
}

// calFakeSession is a model-free stand-in for one ORT session: it sleeps for a
// per-candidate duration (so the wall-clock scorer can be driven
// deterministically) and returns a deterministic vector.
type calFakeSession struct {
	dur   time.Duration
	out   []float32
	err   error
	probe *calBatchProbe
}

func (s calFakeSession) Run(_, _ []int64, batch, seqLen, _ int) ([]float32, error) {
	if s.probe != nil {
		s.probe.record(batch, seqLen)
	}
	if s.err != nil {
		return nil, s.err
	}
	if s.dur > 0 {
		time.Sleep(s.dur)
	}
	if s.out == nil {
		return []float32{1, 2, 3}, nil
	}
	return append([]float32(nil), s.out...), nil
}

func (calFakeSession) Close() error { return nil }

var _ onnx.Session = calFakeSession{}

// withCalibrationSeams swaps the calibration clock and CPU-accounting seams for
// the duration of one test. A nil argument keeps the real seam.
func withCalibrationSeams(t *testing.T, cpu func() (uint64, uint64), now func() time.Time) {
	t.Helper()
	prevCPU, prevNow := calibrationCPUTimes, calibrationNow
	t.Cleanup(func() { calibrationCPUTimes, calibrationNow = prevCPU, prevNow })
	if cpu != nil {
		calibrationCPUTimes = cpu
	}
	if now != nil {
		calibrationNow = now
	}
}

// liveCPU reports a monotonically growing, non-zero CPU delta so candidates are
// never rejected for missing accounting.
func liveCPU() func() (uint64, uint64) {
	var usec uint64
	return func() (uint64, uint64) {
		usec += 500_000
		return usec, 0
	}
}

func runCalibration(t *testing.T, sessions func(threads int) calFakeSession, budget, maxWorkers, configuredThreads, configuredWorkers int, batched bool, batchRows int) calibratedLayout {
	t.Helper()
	factory := func(threads int) (onnx.Session, error) { return sessions(threads), nil }
	return calibrateLayout(factory, registryTok{}, 3, 32, budget, maxWorkers, configuredThreads, configuredWorkers, batched, batchRows)
}

// A constant per-run cost with parallel sessions means throughput scales with
// the session count, so the scorer must pick the widest pool the budget allows.
func TestCalibrationScoresThroughput(t *testing.T) {
	withCalibrationSeams(t, liveCPU(), nil)
	layout := runCalibration(t, func(int) calFakeSession { return calFakeSession{dur: 5 * time.Millisecond} }, 4, 4, 0, 0, false, 1)
	if layout.Workers != 4 || layout.Threads != 1 || layout.Source != "calibrated" {
		t.Fatalf("layout = %+v, want the measured 4-session/1-thread winner", layout)
	}
}

// Equal measured throughput on a batched pool keeps the wider width: the trial
// serves a smaller batch than the coalescing pool, so the measurement
// understates the benefit of threads. The duration is long enough that the
// scores are genuinely equal (a 1ms sleep is equal only on average, and the tie
// rule must not be decided by sleep jitter).
func TestCalibrationPrefersWiderOnBatchedTie(t *testing.T) {
	withCalibrationSeams(t, liveCPU(), nil)
	layout := runCalibration(t, func(int) calFakeSession { return calFakeSession{dur: 20 * time.Millisecond} }, 4, 4, 0, 0, true, 1)
	if layout.Threads != 4 || layout.Workers != 1 || layout.Source != "calibrated" {
		t.Fatalf("layout = %+v, want the widest layout inside the noise band", layout)
	}
}

// Equal measured throughput on an unbatched pool keeps the lower-resource
// layout, which is measured at its real shape. Session counts are explicit so
// the widths actually compete.
func TestCalibrationPrefersLowerResourceOnUnbatchedTie(t *testing.T) {
	withCalibrationSeams(t, liveCPU(), nil)
	layout := runCalibration(t, func(int) calFakeSession { return calFakeSession{dur: 20 * time.Millisecond} }, 8, 8, 0, 2, false, 1)
	if layout.Workers != 2 || layout.Threads != 1 || layout.Source != "calibrated" {
		t.Fatalf("layout = %+v, want the narrowest width at the measured session count", layout)
	}
}

func TestCalibrationRejectsOutputMismatch(t *testing.T) {
	withCalibrationSeams(t, liveCPU(), nil)
	layout := runCalibration(t, func(threads int) calFakeSession {
		if threads > 1 {
			return calFakeSession{dur: time.Millisecond, out: []float32{9, 9, 9}}
		}
		return calFakeSession{dur: time.Millisecond, out: []float32{1, 2, 3}}
	}, 2, 2, 0, 0, true, 1)
	if layout.Workers != 1 || layout.Threads != 1 {
		t.Fatalf("layout = %+v, want the correct baseline after rejecting the mismatch", layout)
	}
}

func TestCalibrationRejectsP95Regression(t *testing.T) {
	withCalibrationSeams(t, liveCPU(), nil)
	layout := runCalibration(t, func(threads int) calFakeSession {
		dur := time.Millisecond
		if threads > 1 {
			dur = 50 * time.Millisecond // far past 2x the baseline p95
		}
		return calFakeSession{dur: dur}
	}, 2, 2, 0, 0, true, 1)
	if layout.Workers != 1 || layout.Threads != 1 {
		t.Fatalf("layout = %+v, want the baseline after rejecting the slow tail", layout)
	}
}

func TestCalibrationFallsBackWhenBaselineFails(t *testing.T) {
	withCalibrationSeams(t, liveCPU(), nil)
	layout := runCalibration(t, func(int) calFakeSession { return calFakeSession{err: errors.New("run failed")} }, 4, 4, 0, 0, false, 1)
	if layout.Source != "fallback" || layout.Reason != "baseline_failed" || layout.Workers != 1 || layout.Threads != 1 {
		t.Fatalf("layout = %+v, want the 1-worker/1-thread fallback with baseline_failed", layout)
	}
}

func TestCalibrationFallsBackWithoutCPUAccounting(t *testing.T) {
	withCalibrationSeams(t, func() (uint64, uint64) { return 0, 0 }, nil)
	layout := runCalibration(t, func(int) calFakeSession { return calFakeSession{dur: time.Millisecond} }, 4, 4, 0, 0, false, 1)
	if layout.Source != "fallback" || layout.Reason != "cpu_accounting_unavailable" || layout.Workers != 1 || layout.Threads != 1 {
		t.Fatalf("layout = %+v, want the fallback reported for unavailable CPU accounting", layout)
	}
}

func TestCalibrationFallsBackOnDeadline(t *testing.T) {
	base := time.Unix(0, 0)
	calls := 0
	now := func() time.Time {
		calls++
		if calls == 1 {
			return base
		}
		return base.Add(time.Hour) // every candidate is past the nominal deadline
	}
	withCalibrationSeams(t, liveCPU(), now)
	layout := runCalibration(t, func(int) calFakeSession { return calFakeSession{dur: time.Millisecond} }, 4, 4, 0, 0, false, 1)
	if layout.Source != "fallback" || layout.Reason != "calibration_deadline" || layout.Workers != 1 || layout.Threads != 1 {
		t.Fatalf("layout = %+v, want the deadline fallback", layout)
	}
}

func TestCalibrationFallsBackOnInvalidFixture(t *testing.T) {
	withCalibrationSeams(t, liveCPU(), nil)
	factory := func(int) (onnx.Session, error) { return calFakeSession{}, nil }
	layout := calibrateLayout(factory, emptyTok{}, 3, 0, 4, 4, 0, 0, false, 1)
	if layout.Source != "fallback" || layout.Reason != "invalid_limits" {
		t.Fatalf("layout = %+v, want invalid_limits fallback", layout)
	}
	layout = calibrateLayout(factory, emptyTok{}, 3, 32, 4, 4, 0, 0, false, 1)
	if layout.Source != "fallback" || layout.Reason == "" || layout.Workers != 1 || layout.Threads != 1 {
		t.Fatalf("layout = %+v, want a reported fallback for an unusable tokenizer", layout)
	}
}

func TestCalibrationKeepsExplicitDimensions(t *testing.T) {
	withCalibrationSeams(t, liveCPU(), nil)
	width := runCalibration(t, func(int) calFakeSession { return calFakeSession{dur: time.Millisecond} }, 4, 4, 2, 0, true, 1)
	if width.Threads != 2 || width.Workers != 1 {
		t.Fatalf("layout = %+v, want the explicit width 2 honored", width)
	}
	workers := runCalibration(t, func(int) calFakeSession { return calFakeSession{dur: time.Millisecond} }, 6, 6, 0, 3, false, 1)
	if workers.Workers != 3 {
		t.Fatalf("layout = %+v, want the explicit 3 workers honored", workers)
	}
}

// A single candidate (an explicit dimension, or a one-slot budget) needs no
// trial: the answer is the only option, so no real model run is spent on it.
func TestCalibrationSkipsTrialForASingleCandidate(t *testing.T) {
	withCalibrationSeams(t, liveCPU(), nil)
	probe := &calBatchProbe{}
	layout := runCalibration(t, func(threads int) calFakeSession {
		return calFakeSession{dur: time.Millisecond, probe: probe}
	}, 4, 4, 2, 0, true, 1)
	if layout.Threads != 2 || layout.Workers != 1 || layout.Source != "calibrated" || layout.Reason != "single_candidate" {
		t.Fatalf("layout = %+v, want the explicit width returned without a trial", layout)
	}
	if got := probe.seen(); len(got) != 0 {
		t.Fatalf("single-candidate calibration ran %d model runs, want none", len(got))
	}
}

// The trial must feed the batch the pool serves, and the batch shape must be
// able to change which width wins: wider is worse for one row (thread sync)
// and better for a coalesced batch.
func TestCalibrationUsesTheServedBatch(t *testing.T) {
	withCalibrationSeams(t, liveCPU(), nil)
	// 4 rows per run: width divides the time. One row: width multiplies it.
	dur := func(threads, batch int) time.Duration {
		if batch > 1 {
			return time.Duration(8/threads) * time.Millisecond
		}
		return time.Duration(threads) * 2 * time.Millisecond
	}

	single := &calBatchProbe{}
	singleLayout := runCalibration(t, func(threads int) calFakeSession {
		return calFakeSession{dur: dur(threads, 1), probe: single}
	}, 4, 4, 0, 0, true, 1)
	if singleLayout.Threads != 1 {
		t.Fatalf("single-row layout = %+v, want width 1", singleLayout)
	}
	for _, run := range single.seen() {
		if run.batch != 1 {
			t.Fatalf("single-row calibration ran batch=%d", run.batch)
		}
	}

	batched := &calBatchProbe{}
	batchedLayout := runCalibration(t, func(threads int) calFakeSession {
		return calFakeSession{dur: dur(threads, calibrationBatchRows), probe: batched}
	}, 4, 4, 0, 0, true, calibrationBatchRows)
	if batchedLayout.Threads != 4 {
		t.Fatalf("batched layout = %+v, want the width-4 winner", batchedLayout)
	}
	seen := batched.seen()
	if len(seen) == 0 {
		t.Fatal("batched calibration ran no model runs")
	}
	for _, run := range seen {
		if want := fixtureBatch(run.seqLen, calibrationBatchRows); run.batch != want {
			t.Fatalf("batched calibration ran batch=%d for seqLen=%d, want %d", run.batch, run.seqLen, want)
		}
	}
}

// The trial batch shrinks with fixture length so one long fixture cannot spend
// the whole deadline on a single run.
func TestFixtureBatchBoundsRowsByTokens(t *testing.T) {
	for _, tc := range []struct{ seqLen, maxRows, want int }{
		{8, calibrationBatchRows, calibrationBatchRows},
		{64, calibrationBatchRows, 4},
		{128, calibrationBatchRows, 2},
		{256, calibrationBatchRows, 1},
		{2048, calibrationBatchRows, 1},
		{100, 1, 1},
		{0, calibrationBatchRows, 1},
	} {
		if got := fixtureBatch(tc.seqLen, tc.maxRows); got != tc.want {
			t.Errorf("fixtureBatch(%d, %d) = %d, want %d", tc.seqLen, tc.maxRows, got, tc.want)
		}
	}
}

// A candidate's reported rows count the rows of each coalesced batch, not just
// the runs, so the score is per-row throughput rather than per-run latency.
func TestCalibrationCandidateCountsRowsPerBatch(t *testing.T) {
	withCalibrationSeams(t, liveCPU(), nil)
	inputs, err := calibrationInputs(registryTok{}, 32)
	if err != nil {
		t.Fatal(err)
	}
	factory := func(int) (onnx.Session, error) { return calFakeSession{}, nil }
	res, _, ok := runCalibrationCandidate(factory, inputs, 3, 1, 1, calibrationBatchRows)
	if !ok {
		t.Fatal("candidate trial failed")
	}
	want := 0
	for _, input := range inputs {
		want += 3 * fixtureBatch(len(input.ids), calibrationBatchRows)
	}
	if res.Rows != want {
		t.Fatalf("Rows = %d, want %d (repeats × tiled rows)", res.Rows, want)
	}
}

// The selection rule is pure, so it is tested with exact scores instead of
// slept-through trials: the branch that matters is narrow enough that sleep
// jitter could decide it.
func TestChooseCalibratedLayoutRule(t *testing.T) {
	// Candidates are ordered as calibrateLayout orders them: the serial baseline
	// first, then the largest layout down.
	batchedCandidates := [][2]int{{1, 1}, {1, 4}, {1, 2}}
	fallback := calibratedLayout{Workers: 1, Threads: 1, Source: "fallback", Reason: "no_valid_candidate"}

	// width 4 is 3.1% above the baseline and width 2 is 3.2% above width 4, both
	// inside the 5% band, while width 2 is 6.4% above the baseline: the wider
	// incumbent must keep the pick rather than be displaced by the narrower
	// candidate's comparison against the baseline.
	got, score := chooseCalibratedLayout(map[[2]int]float64{
		{1, 1}: 100, {1, 4}: 103.1, {1, 2}: 106.4,
	}, batchedCandidates, true, fallback)
	if got.Threads != 4 || got.Workers != 1 || got.Source != "calibrated" {
		t.Fatalf("layout = %+v, want the wider width 4 kept inside the band", got)
	}
	if score != 103.1 {
		t.Fatalf("score = %v, want the kept layout's own score 103.1", score)
	}

	// Outside the band the higher score wins, whichever layout it belongs to.
	if got, _ := chooseCalibratedLayout(map[[2]int]float64{
		{1, 1}: 100, {1, 4}: 103.1, {1, 2}: 120,
	}, batchedCandidates, true, fallback); got.Threads != 2 {
		t.Fatalf("layout = %+v, want width 2 to win outside the band", got)
	}

	// An unbatched pool keeps the lower-resource layout inside the band.
	unbatched := [][2]int{{2, 1}, {2, 4}, {2, 2}}
	if got, _ := chooseCalibratedLayout(map[[2]int]float64{
		{2, 1}: 100, {2, 4}: 103, {2, 2}: 103,
	}, unbatched, false, fallback); got.Threads != 1 || got.Workers != 2 {
		t.Fatalf("layout = %+v, want the narrowest width at the measured session count", got)
	}

	// Nothing measured keeps the fallback and reports no score.
	if got, score := chooseCalibratedLayout(map[[2]int]float64{}, batchedCandidates, true, fallback); got != fallback || score >= 0 {
		t.Fatalf("empty scores = %+v/%v, want the fallback and a negative score", got, score)
	}
}

// emptyTok returns no tokens, which calibrationInputs must reject.
type emptyTok struct{}

func (emptyTok) Encode(string, int) ([]int64, []int64, error) { return nil, nil, nil }
func (emptyTok) Close() error                                 { return nil }
