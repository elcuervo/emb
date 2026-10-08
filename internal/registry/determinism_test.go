package registry

import (
	"errors"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/elcuervo/emb/internal/config"
	"github.com/elcuervo/emb/internal/onnx"
	"github.com/elcuervo/emb/internal/tokenizer"
)

// initORTOnce initializes the ORT shared library once for this test binary
// (production main does this at startup; test binaries must do it themselves).
// Tests skip when the library is unavailable (run inside nix develop).
var initORTOnce sync.Once
var initORTErr error

func initORT(t *testing.T) {
	t.Helper()
	initORTOnce.Do(func() { initORTErr = onnx.InitEnvironment("") })
	if initORTErr != nil {
		t.Skipf("onnx runtime unavailable: %v (run inside nix develop)", initORTErr)
	}
}

// registryTok is a deterministic fake tokenizer: one id per rune, truncated to
// maxLength, mask all ones.
type registryTok struct{}

func (registryTok) Encode(text string, maxLength int) ([]int64, []int64, error) {
	n := len(text)
	if n > maxLength {
		n = maxLength
	}
	ids := make([]int64, n)
	mask := make([]int64, n)
	for i := 0; i < n; i++ {
		ids[i] = int64(i + 1)
		mask[i] = 1
	}
	return ids, mask, nil
}

func (registryTok) Close() error { return nil }

var _ tokenizer.Tokenizer = registryTok{}

// fakeSession is a batch-invariant (position-independent values) session whose
// values shift with batch size when sensitive is set, emulating a
// dynamic-quantized graph. Session counting happens in the injected factory.
type fakeSession struct {
	sensitive bool
}

func (s *fakeSession) Run(inputIDs, attnMask []int64, batchSize, seqLen, dim int) ([]float32, error) {
	hidden := make([]float32, batchSize*seqLen*dim)
	for b := 0; b < batchSize; b++ {
		for t := 0; t < seqLen; t++ {
			for d := 0; d < dim; d++ {
				v := float32(t*10 + d)
				if s.sensitive {
					v += float32(batchSize)
				}
				hidden[b*seqLen*dim+t*dim+d] = v
			}
		}
	}
	return hidden, nil
}

func (s *fakeSession) Close() error { return nil }

var _ onnx.Session = (*fakeSession)(nil)

// modelConfigOnMinilm returns a config that resolves and validates against the
// vendored minilm fixture (metadata-only reads; inference sessions are faked
// through the newRuntimeSession seam), with the given batching timeout. It uses
// the fp32 export CI downloads (models/minilm), not the local int8 weights, so
// the tests execute on a clean runner.
func modelConfigOnMinilm(timeoutMS int, workers int) config.ModelConfig {
	t := timeoutMS
	return config.ModelConfig{
		ONNX:         "../../models/minilm/model.onnx",
		Tokenizer:    "../../models/minilm/tokenizer.json",
		Quantize:     "off",
		OutputTensor: "last_hidden_state",
		Pooling:      "mean",
		Normalize:    true,
		MaxLength:    128,
		Dim:          384,
		Workers:      workers,
		Batching:     config.BatchingConfig{Timeout: &t},
	}
}

// openRegistry creates a registry with the faked tokenizer/session seams and a
// loaded model for cfg, returning the registry, the model name, and the
// session-open counter (probe opens + pool opens).
func openRegistry(t *testing.T, cfg config.ModelConfig, sensitive bool) (*Registry, string, *atomic.Int64) {
	t.Helper()
	initORT(t)
	origTok, origRT := newTokenizer, newRuntimeSession
	t.Cleanup(func() {
		newTokenizer, newRuntimeSession = origTok, origRT
	})
	newTokenizer = func(string, bool) (tokenizer.Tokenizer, error) { return registryTok{}, nil }

	var opens atomic.Int64
	newRuntimeSession = func(data []byte, inputNames, outputNames []string, dim int, rank, intraOpThreads, interOpThreads, execMode int, allowSpinning bool) (onnx.Session, error) {
		opens.Add(1)
		return &fakeSession{sensitive: sensitive}, nil
	}

	entry, err := LoadModel(cfg, "probe")
	if err != nil {
		t.Fatalf("LoadModel: %v", err)
	}
	reg := New()
	reg.Add("probe", entry)
	return reg, "probe", &opens
}

func TestEnsurePoolProbePassKeepsBatching(t *testing.T) {
	reg, name, _ := openRegistry(t, modelConfigOnMinilm(1, 1), false)
	_, err := reg.GetOrInit(name)
	if err != nil {
		t.Fatal(err)
	}
	entry := reg.models[name]
	verdict, reason := entry.BatchDeterminism, entry.BatchDeterminismReason
	if verdict != "passed" || reason != "passed" {
		t.Fatalf("verdict=%q reason=%q, want passed/passed", verdict, reason)
	}
	st := entry.Pool.Stats()
	if st.BatchingTimeout != 1 || st.NumWorkers != 1 {
		t.Fatalf("batcher pool expected (timeout=1, workers=1), got timeout=%d workers=%d", st.BatchingTimeout, st.NumWorkers)
	}
}

func TestEnsurePoolProbeFailureDegradesToWorkerPool(t *testing.T) {
	reg, name, opens := openRegistry(t, modelConfigOnMinilm(1, 2), true)
	_, err := reg.GetOrInit(name)
	if err != nil {
		t.Fatal(err)
	}
	entry := reg.models[name]
	verdict, reason := entry.BatchDeterminism, entry.BatchDeterminismReason
	if verdict != "failed" || reason != "dql_batch_dependence" {
		t.Fatalf("verdict=%q reason=%q, want failed/dql_batch_dependence", verdict, reason)
	}
	st := entry.Pool.Stats()
	// Degraded pool equals the explicit worker-pool path: no batching window,
	// cfg.Workers sessions.
	if st.BatchingTimeout != 0 || st.NumWorkers != 2 {
		t.Fatalf("degraded pool expected worker pool (timeout=0, workers=2), got timeout=%d workers=%d", st.BatchingTimeout, st.NumWorkers)
	}
	// Probe session + 2 worker sessions.
	if got := opens.Load(); got != 3 {
		t.Fatalf("session opens = %d, want 3 (1 probe + 2 workers)", got)
	}
}

func TestEnsurePoolSkipsProbeWhenBatchingDisabled(t *testing.T) {
	reg, name, opens := openRegistry(t, modelConfigOnMinilm(0, 2), true)
	_, err := reg.GetOrInit(name)
	if err != nil {
		t.Fatal(err)
	}
	entry := reg.models[name]
	verdict, reason := entry.BatchDeterminism, entry.BatchDeterminismReason
	if verdict != "untested" || reason != "untested" {
		t.Fatalf("verdict=%q reason=%q, want untested/untested", verdict, reason)
	}
	st := entry.Pool.Stats()
	if st.BatchingTimeout != 0 || st.NumWorkers != 2 {
		t.Fatalf("worker pool expected (timeout=0, workers=2), got timeout=%d workers=%d", st.BatchingTimeout, st.NumWorkers)
	}
	// No probe session: only the 2 worker sessions open.
	if got := opens.Load(); got != 2 {
		t.Fatalf("session opens = %d, want 2 (no probe)", got)
	}
}

// TestEnsurePoolProbeRunsOnce proves the verdict is computed at most once per
// model lifetime: repeated GetOrInit (which short-circuits on the loaded flag)
// opens no additional sessions and never re-runs the probe.
func TestEnsurePoolProbeRunsOnce(t *testing.T) {
	reg, name, opens := openRegistry(t, modelConfigOnMinilm(1, 1), false)
	for i := 0; i < 3; i++ {
		if _, err := reg.GetOrInit(name); err != nil {
			t.Fatal(err)
		}
	}
	// 1 probe session + 1 batcher session, once; repeated GetOrInit adds none.
	if got := opens.Load(); got != 2 {
		t.Fatalf("session opens = %d, want 2 (probe once + batcher session once)", got)
	}
	entry := reg.models[name]
	if verdict := entry.BatchDeterminism; verdict != "passed" {
		t.Fatalf("verdict=%q, want passed", verdict)
	}
}

// errSession fails every run, mimicking an incompatible graph (e.g. a static
// sequence-length constraint the probe texts violate).
type errSession struct{}

func (s *errSession) Run(inputIDs, attnMask []int64, batchSize, seqLen, dim int) ([]float32, error) {
	return nil, errors.New("shape mismatch")
}

func (s *errSession) Close() error { return nil }

var _ onnx.Session = (*errSession)(nil)

// TestEnsurePoolProbeErrorClassifiesSeparately proves a probe failure that is
// not batch dependence still degrades batching, but with the probe_error
// reason rather than being mislabeled as dynamic-quantization dependence.
func TestEnsurePoolProbeErrorClassifiesSeparately(t *testing.T) {
	initORT(t)
	origTok, origRT := newTokenizer, newRuntimeSession
	t.Cleanup(func() { newTokenizer, newRuntimeSession = origTok, origRT })
	newTokenizer = func(string, bool) (tokenizer.Tokenizer, error) { return registryTok{}, nil }

	var opens atomic.Int64
	newRuntimeSession = func(data []byte, inputNames, outputNames []string, dim int, rank, intraOpThreads, interOpThreads, execMode int, allowSpinning bool) (onnx.Session, error) {
		opens.Add(1)
		return &errSession{}, nil
	}

	entry, err := LoadModel(modelConfigOnMinilm(1, 1), "probe")
	if err != nil {
		t.Fatal(err)
	}
	reg := New()
	reg.Add("probe", entry)
	if _, err := reg.GetOrInit("probe"); err != nil {
		t.Fatal(err)
	}
	verdict, reason := entry.BatchDeterminism, entry.BatchDeterminismReason
	if verdict != "failed" || reason != "probe_error" {
		t.Fatalf("verdict=%q reason=%q, want failed/probe_error", verdict, reason)
	}
	if st := entry.Pool.Stats(); st.BatchingTimeout != 0 {
		t.Fatalf("expected degraded pool (timeout 0), got %d", st.BatchingTimeout)
	}
}
