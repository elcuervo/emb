package registry

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/elcuervo/emb/internal/config"
	"github.com/elcuervo/emb/internal/hfhub"
	"github.com/elcuervo/emb/internal/onnx"
	"github.com/elcuervo/emb/internal/pipeline"
	"github.com/elcuervo/emb/internal/tokenizer"
)

// ModelEntry owns every inference resource a model has: the embedding pool,
// the scripted named-tensor sessions, the image sessions, and the single
// tokenizer shared by all of them. Ownership table (resource → owner → release
// point → bound):
//
//	named-tensor script sessions → ModelEntry → Registry.Close → effectiveEmbeddingSessions (default) or explicit script_workers
//	shared tokenizer            → ModelEntry → Registry.Close (exactly once) → 1 per model
//	image sessions              → ModelEntry → Registry.Close / closeImageSessions → image pool size
//	per-call output tensors      → onnx.NamedRuntimeSession.RunNamed → destroyed after each run → len(graph outputs)
//	script source / bytecode    → scriptCache / Compiler → eviction or EMB.SCRIPT FLUSH → per-model caps
//
// Every owner releases on close; a model whose resources were never created
// closes safely without creating anything.
type ModelEntry struct {
	Pool *pipeline.Pool
	Dim  int
	Name string

	// Quantization is "int8" when quantized weights were resolved, else "fp32".
	Quantization string
	// ModelSize is the on-disk size of the resolved weights file.
	ModelSize int64

	// BatchDeterminism and BatchDeterminismReason report the load-time
	// probe outcome: verdict "passed" (graph batch-invariant), "failed"
	// (dynamic activation quantization detected → served unbatched), or
	// "untested" (batching disabled / never loaded); reason is
	// "dql_batch_dependence" or "probe_error" on failure, else the verdict
	// itself. Written once by ensurePool before pool publication.
	BatchDeterminism       string
	BatchDeterminismReason string

	once      sync.Once
	cfg       config.ModelConfig
	poolReady atomic.Bool
	loaded    atomic.Bool
	loadErr   error

	// scripted resources (task: scripted-model path): a generic named-tensor
	// session plus the word-aligned tokenizer, created lazily on first script
	// evaluation, separate from the embedding pool.
	scriptOnce   sync.Once
	scriptRes    *ScriptResources
	scriptResErr error
	// scriptResPtr publishes scriptRes atomically, so observability can read
	// the dispatch counters without forcing a lazy open or racing the Once.
	scriptResPtr atomic.Pointer[ScriptResources]
	// scripted-evaluation counters and resource footprint, reported by
	// EMB.STATS/EMB.INFO (see ScriptStats / ScriptFootprint). Updated
	// atomically and never reset.
	scriptRequests  atomic.Int64
	scriptErrors    atomic.Int64
	scriptSessions  atomic.Int64
	scriptTokenizer atomic.Bool

	// image resources (EMB.IMG): a pool of named-tensor sessions plus the
	// immutable preprocessing plan, resolved lazily on first image request.
	// ImageRes is exported so tests can inject fake sessions; production code
	// uses ImageResources().
	ImageRes    *ImageResources
	imageOnce   sync.Once
	imageResErr error
	// imageSessions is the number of image sessions opened so far, published
	// atomically so observability (EMB.INFO) never races the lazy open.
	imageSessions atomic.Int64
	// imagePlanOnce memoizes the resolved preprocessing plan separately from the
	// sessions, so emb.image.info can report configuration without opening any
	// image session. imagePlanRes is written inside the Once (happens-before).
	imagePlanOnce sync.Once
	imagePlanRes  *imagePlanResult

	// sharedTok is the single tokenizer for this model, shared by the
	// embedding pool and the scripted path so a model never loads two. Created
	// lazily on first use and closed once by Registry.Close.
	sharedTokOnce sync.Once
	sharedTok     tokenizer.Tokenizer
	sharedTokErr  error

	// Fingerprints are expensive for large ONNX files. Persistence computes one
	// lazily per model and all periodic/manual saves reuse it.
	fingerprintOnce sync.Once
	fingerprint     string
	fingerprintErr  error
}

// RecordScriptedEvaluation bumps the model's cumulative scripted-evaluation
// counters (one per completed EMB.EVAL/EMB.EVSHA evaluation).
func (e *ModelEntry) RecordScriptedEvaluation(failed bool) {
	e.scriptRequests.Add(1)
	if failed {
		e.scriptErrors.Add(1)
	}
}

// ScriptStats returns the model's cumulative scripted evaluations and failures.
func (e *ModelEntry) ScriptStats() (requests, errors int64) {
	return e.scriptRequests.Load(), e.scriptErrors.Load()
}

// Embeddable reports whether the model can produce pooled embeddings: a
// declared dimension and a pooling mode other than "none". Models without an
// embedding configuration (e.g. GLiNER logits graphs) are not embeddable, and
// their scripts leave emb.embed unavailable rather than loading a pool that
// cannot serve the request.
func (e *ModelEntry) Embeddable() bool {
	return e.cfg.Dim > 0 && e.cfg.Pooling != "" && e.cfg.Pooling != "none"
}

// ScriptFootprint reports the model's scripted resource cost without creating
// anything: the number of named-tensor sessions opened and whether the script
// tokenizer has been loaded.
func (e *ModelEntry) ScriptFootprint() (sessions int64, tokenizer bool) {
	return e.scriptSessions.Load(), e.scriptTokenizer.Load()
}

// ImageFootprint reports the number of image sessions opened so far, without
// opening any (0 for a model that has never served an image request).
func (e *ModelEntry) ImageFootprint() int64 { return e.imageSessions.Load() }

// LoadedPool returns the safely published embedding pool, or nil until its
// construction completes. poolReady is stored after Pool, so its acquire load
// makes the pointer publication visible to concurrent observability readers.
func (e *ModelEntry) LoadedPool() *pipeline.Pool {
	if !e.poolReady.Load() {
		return nil
	}
	return e.Pool
}

// ScriptResources bundles what a scripted evaluation needs for a model: a
// pool of named-tensor sessions and the tokenizer (its plain/offsets/word-level
// capabilities are type-asserted when building the host binding).
type ScriptResources struct {
	sessions []onnx.NamedSession
	// idle hands out whichever session is free first (a buffered channel
	// seeded with every session), so a call never queues behind a busy session
	// while another sits idle.
	idle      chan onnx.NamedSession
	next      atomic.Uint64
	Tokenizer tokenizer.Tokenizer
	// SplitBatch is set for graphs with dynamic activation quantization, whose
	// output depends on batch composition and padding: emb.run_batch then runs
	// each item on its own instead of one padded run.
	SplitBatch bool

	// Dispatch observability (EMB.STATS/EMB.INFO): cumulative wait for a free
	// session, cumulative ORT run time, run count, and the live busy gauge.
	dispatchWaitUs atomic.Int64
	runUs          atomic.Int64
	runs           atomic.Int64
	busy           atomic.Int64
}

// RunNamed runs an inference on the first session that is free. Arrival order
// is the channel's FIFO handoff: a waiting call takes the next session
// returned, and a session is never skipped while idle.
func (r *ScriptResources) RunNamed(inputs []onnx.NamedTensor) (map[string]onnx.NamedTensor, error) {
	waitStart := time.Now()
	sess := <-r.idle
	r.dispatchWaitUs.Add(time.Since(waitStart).Microseconds())
	r.busy.Add(1)
	runStart := time.Now()
	out, err := sess.RunNamed(inputs)
	r.runUs.Add(time.Since(runStart).Microseconds())
	r.runs.Add(1)
	r.busy.Add(-1)
	r.idle <- sess
	return out, err
}

// DispatchCounters returns the script path's cumulative dispatch wait and ORT
// run microseconds, run count, and live busy gauge.
func (r *ScriptResources) DispatchCounters() (waitUs, runUs, runs, busy int64) {
	return r.dispatchWaitUs.Load(), r.runUs.Load(), r.runs.Load(), r.busy.Load()
}

// Session returns the next named-tensor session for a scripted run,
// round-robin across the pool (each session serializes its own runs). It stays
// for callers that need a specific instance (Close, tests); scripted dispatch
// uses RunNamed.
func (r *ScriptResources) Session() onnx.NamedSession {
	i := r.next.Add(1) - 1
	return r.sessions[i%uint64(len(r.sessions))]
}

// Sessions exposes the pool for Close and tests.
func (r *ScriptResources) Sessions() []onnx.NamedSession { return r.sessions }

func hashFile(h io.Writer, path string) error {
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	_, copyErr := io.Copy(h, f)
	closeErr := f.Close()
	if copyErr != nil {
		return copyErr
	}
	return closeErr
}

// Fingerprint identifies every model input that intentionally affects emitted
// embedding bytes. It is computed only when cache persistence asks for it.
func (e *ModelEntry) Fingerprint() (string, error) {
	e.fingerprintOnce.Do(func() {
		h := sha256.New()
		for _, path := range []string{e.cfg.ONNX, e.cfg.Tokenizer} {
			if path == "" {
				continue
			}
			if err := hashFile(h, path); err != nil {
				e.fingerprintErr = fmt.Errorf("hashing %q: %w", path, err)
				return
			}
		}
		// Scheduling and capacity settings are deliberately absent: workers,
		// batching, tokenizer workers, ORT threads, and execution mode do not
		// alter the intended embedding output.
		_, _ = fmt.Fprintf(h, "\x00output=%s\x00dim=%d\x00max_length=%d\x00pooling=%s\x00normalize=%t\x00pad_output=%t\x00quantization=%s",
			e.cfg.OutputTensor, e.Dim, e.cfg.MaxLength, e.cfg.Pooling,
			e.cfg.Normalize, e.cfg.PadOutput, e.Quantization)
		e.fingerprint = hex.EncodeToString(h.Sum(nil))
	})
	return e.fingerprint, e.fingerprintErr
}

type Registry struct {
	mu          sync.RWMutex
	models      map[string]*ModelEntry
	totalModels atomic.Int64
	closeOnce   sync.Once
	closeErr    error
}

func New() *Registry {
	return &Registry{
		models: make(map[string]*ModelEntry),
	}
}

// defaultIntraOpThreads returns the default ONNX intra-op thread count when a model
// leaves intra_op_threads unset: cores−2, reserving two cores for request
// parsing/dispatch so a busy request path cannot starve inference (and vice versa).
// Machines with ≤ 2 cores floor at 1.
func defaultIntraOpThreads() int {
	return scriptIntraOpThreads(0, 1, runtime.GOMAXPROCS(0))
}

// scriptIntraOpThreads returns the intra-op thread count for each script
// session: the explicit configured value when set, else the cores−2 budget
// divided across the sessions (at least 1). With one session it equals
// defaultIntraOpThreads, so a single-session model is unchanged.
func scriptIntraOpThreads(configured, sessions, cores int) int {
	if configured > 0 {
		return configured
	}
	threads := cores - 2
	if cores <= 2 {
		threads = 1
	}
	if sessions > 1 {
		threads = max(1, threads/sessions)
	}
	return threads
}

// threadUse is one path's share of a model's ORT intra-op thread budget.
type threadUse struct {
	path     string
	sessions int
	threads  int
}

// configuredScriptSessions returns how many scripted sessions the model will
// open, derived from configuration without opening anything. Zero means the
// model has no scripted surface configured.
func (e *ModelEntry) configuredScriptSessions() int {
	if e.cfg.ScriptWorkers <= 0 && !e.cfg.ScriptPreload && len(e.cfg.Scripts) == 0 {
		return 0
	}
	n := e.cfg.ScriptWorkers
	if n <= 0 {
		n = effectiveEmbeddingSessions(&e.cfg)
	}
	if n < 1 {
		n = 1
	}
	return n
}

// threadUses derives each path's session and per-session thread counts from
// configuration only. Script sessions use the divided default; embedding and
// image sessions use the configured (or default) intra_op_threads.
func (e *ModelEntry) threadUses() []threadUse {
	th := e.cfg.IntraOpThreads
	if th <= 0 {
		th = defaultIntraOpThreads()
	}
	var uses []threadUse
	if e.Embeddable() {
		uses = append(uses, threadUse{"embed", effectiveEmbeddingSessions(&e.cfg), th})
	}
	if n := e.configuredScriptSessions(); n > 0 {
		st := th
		if e.cfg.IntraOpThreads <= 0 && n > 1 {
			st = max(1, th/n)
		}
		uses = append(uses, threadUse{"script", n, st})
	}
	if e.cfg.Image != nil {
		n := autoTuneWorkers(e.cfg.ONNX, 0)
		if n < 1 {
			n = 1
		}
		uses = append(uses, threadUse{"image", n, th})
	}
	return uses
}

// ThreadBudgetEntry is one model/path group's contribution to the intra-op
// thread budget.
type ThreadBudgetEntry struct {
	Model    string
	Path     string
	Sessions int
	Threads  int
}

// OversubscribedThreads returns every model/path session group when the total
// configured intra-op thread budget (sessions × threads, across embedding,
// image and script) exceeds cores, or nil when it is within budget. It never
// changes configuration.
func OversubscribedThreads(entries []*ModelEntry, cores int) []ThreadBudgetEntry {
	var out []ThreadBudgetEntry
	total := 0
	for _, e := range entries {
		for _, u := range e.threadUses() {
			total += u.sessions * u.threads
			out = append(out, ThreadBudgetEntry{e.Name, u.path, u.sessions, u.threads})
		}
	}
	if total <= cores {
		return nil
	}
	return out
}

// WarnThreadBudget logs a boot warning when the configured intra-op thread
// budget exceeds the core count, naming each contributing model with its
// session and thread counts. It never changes configuration.
func WarnThreadBudget(entries []*ModelEntry, cores int) {
	overs := OversubscribedThreads(entries, cores)
	if overs == nil {
		return
	}
	parts := make([]string, 0, len(overs))
	total := 0
	for _, o := range overs {
		total += o.Sessions * o.Threads
		parts = append(parts, fmt.Sprintf("%s/%s=%d sessions × %d threads", o.Model, o.Path, o.Sessions, o.Threads))
	}
	log.Printf("WARNING: intra-op thread budget oversubscribed: %s = %d > %d cores; set intra_op_threads or reduce workers/script_workers",
		strings.Join(parts, ", "), total, cores)
}

func autoTuneWorkers(modelPath string, maxWorkers int) int {
	maxCores := runtime.GOMAXPROCS(0)
	if maxWorkers > 0 && maxWorkers < maxCores {
		maxCores = maxWorkers
	}
	mem := TotalSystemMemory()
	if mem == 0 {
		return maxCores
	}
	info, err := os.Stat(modelPath)
	if err != nil {
		return maxCores
	}
	modelSize := uint64(info.Size())
	perSession := modelSize + modelSize/5 // +20% overhead
	availMem := mem / 2
	byMem := int(availMem / perSession)
	if byMem < 1 {
		byMem = 1
	}
	if byMem > maxCores {
		byMem = maxCores
	}
	return byMem
}

// newTokenizer creates a model's tokenizer. It is a package var (wrapping the
// concrete constructor) so tests can inject a counting fake and prove the
// instance shared by the embedding pool and the scripted path is closed
// exactly once.
var newTokenizer = func(path string, padOutput bool) (tokenizer.Tokenizer, error) {
	return tokenizer.NewTokenizer(path, padOutput)
}

// sharedTokenizer returns the model's single tokenizer, creating it on first
// use. Both the embedding pool and the scripted path use this instance so a
// model never pays for two tokenizers. Ownership stays with the ModelEntry:
// callers must not close it (Registry.Close does).
func (e *ModelEntry) sharedTokenizer() (tokenizer.Tokenizer, error) {
	e.sharedTokOnce.Do(func() {
		e.sharedTok, e.sharedTokErr = newTokenizer(e.cfg.Tokenizer, e.cfg.PadOutput)
	})
	return e.sharedTok, e.sharedTokErr
}

// effectiveEmbeddingSessions returns how many inference sessions a model's
// embedding path actually holds, derived from the model config (so computing
// it never forces a pool load). A nil batching timeout means the default
// 1 ms window is on, so the pool is a batcher holding exactly one session; a
// worker pool holds its configured (or auto-tuned) worker count. It is the
// single source of truth for the auto-tuned scripted session bound.
func effectiveEmbeddingSessions(cfg *config.ModelConfig) int {
	if cfg.Batching.Timeout == nil || *cfg.Batching.Timeout > 0 {
		return 1
	}
	if cfg.Workers > 0 {
		return cfg.Workers
	}
	n := autoTuneWorkers(cfg.ONNX, 0)
	if n < 1 {
		n = 1
	}
	return n
}

// newRuntimeSession creates one inference session from weights bytes. It is a
// package var so tests can inject counting/batch-sensitive fakes and prove
// the probe runs exactly once per model lifetime.
var newRuntimeSession = func(data []byte, inputNames, outputNames []string, dim int, rank, intraOpThreads, interOpThreads, execMode int, allowSpinning bool) (onnx.Session, error) {
	return onnx.NewRuntimeSessionFromBytes(data, inputNames, outputNames, dim, rank, intraOpThreads, interOpThreads, execMode, allowSpinning)
}

func (e *ModelEntry) ensurePool() error {
	if e.loaded.Load() {
		return nil
	}

	log.Printf("  loading model %q (dim=%d, max_length=%d)...", e.Name, e.cfg.Dim, e.cfg.MaxLength)

	cfg := e.cfg
	tok, err := e.sharedTokenizer()
	if err != nil {
		return fmt.Errorf("loading tokenizer for %q: %w", e.Name, err)
	}

	numWorkers := cfg.Workers
	if numWorkers <= 0 {
		numWorkers = autoTuneWorkers(cfg.ONNX, 0)
	}

	outInfo, err := onnx.GetOutputInfo(cfg.ONNX)
	if err != nil {
		return fmt.Errorf("reading output info for %q: %w", e.Name, err)
	}
	if st, statErr := os.Stat(cfg.ONNX); statErr == nil {
		e.ModelSize = st.Size()
	}
	// Quantization label from the weight file name (int8/quantized) — the prod
	// siglip2 file is text_model_int8.onnx.
	base := filepath.Base(cfg.ONNX)
	if strings.Contains(base, "quantized") || strings.Contains(base, "int8") {
		e.Quantization = "int8"
	} else {
		e.Quantization = "fp32"
	}
	out, ok := outInfo[cfg.OutputTensor]
	if !ok {
		name, rank := selectOutputTensor(outInfo)
		available := make([]string, 0, len(outInfo))
		for n := range outInfo {
			available = append(available, n)
		}
		sort.Strings(available)
		log.Printf("  %s: output %q not found in graph (available: %v), auto-selected %q — set output_tensor to silence this", e.Name, cfg.OutputTensor, available, name)
		cfg.OutputTensor = name
		cfg.Pooling = poolingForRank(rank)
		out = outInfo[name]
	}

	inputNames, err := onnx.GetInputNames(cfg.ONNX)
	if err != nil {
		return fmt.Errorf("reading input names for %q: %w", e.Name, err)
	}
	log.Printf("  %s: inputs=%v output=%q rank=%d", e.Name, inputNames, cfg.OutputTensor, out.Rank)

	modelData, err := os.ReadFile(cfg.ONNX)
	if err != nil {
		return fmt.Errorf("reading model file for %q: %w", e.Name, err)
	}

	// Default intra_op_threads to cores−2 (reserve parse/dispatch CPU) when unset;
	// an explicit config value always wins.
	intraThreads := cfg.IntraOpThreads
	if intraThreads <= 0 {
		intraThreads = defaultIntraOpThreads()
	}

	execMode := onnx.ExecModeSequential
	if cfg.ExecutionMode == "parallel" {
		execMode = onnx.ExecModeParallel
	}

	sessionFactory := func() (onnx.Session, error) {
		return newRuntimeSession(
			modelData,
			inputNames,
			[]string{cfg.OutputTensor},
			cfg.Dim,
			out.Rank,
			intraThreads,
			cfg.InterOpThreads,
			execMode,
			cfg.AllowsSpinning(),
		)
	}

	maxBatchTokens := 0
	if cfg.Batching.MaxBatchTokens != nil {
		maxBatchTokens = *cfg.Batching.MaxBatchTokens
	}
	tokenizeWorkers := 0
	if cfg.TokenizeWorkers != nil {
		tokenizeWorkers = *cfg.TokenizeWorkers
	}
	timeoutMS := 0
	if cfg.Batching.Timeout != nil {
		timeoutMS = *cfg.Batching.Timeout
	}

	// Batch-determinism gating (default behavior, no config flag): whenever
	// batching would be enabled, verify the graph's output is independent of
	// batch composition; when it is not (dynamic activation quantization),
	// degrade to the worker pool — the deterministic single-row path the
	// explicit timeout: 0 configures today — and log a stable, greppable
	// line for operators and CI/deploy gates. "untested" only when batching
	// is off by config (nothing to gate). The verdict is computed at most
	// once per model lifetime: ensurePool runs once (GetOrInit's once / the
	// Preload path).
	if timeoutMS <= 0 {
		e.BatchDeterminism, e.BatchDeterminismReason = "untested", "untested"
	} else if err := pipeline.ProbeBatchDeterminism(sessionFactory, tok, cfg.Dim, cfg.MaxLength, cfg.Normalize, cfg.Pooling); err != nil {
		reason := "probe_error"
		if errors.Is(err, pipeline.ErrBatchDependence) {
			reason = "dql_batch_dependence"
		}
		e.BatchDeterminism, e.BatchDeterminismReason = "failed", reason
		log.Printf("  %s: batch determinism probe failed (%v) — degrading to unbatched (worker pool); batch_determinism=failed reason=%s", e.Name, err, reason)
		timeoutMS = 0
	} else {
		e.BatchDeterminism, e.BatchDeterminismReason = "passed", "passed"
	}

	pool, err := pipeline.NewPool(sessionFactory, tok, numWorkers, cfg.Dim, cfg.MaxLength, cfg.Normalize, cfg.Pooling, timeoutMS, cfg.Batching.MaxBatch, maxBatchTokens, tokenizeWorkers)
	if err != nil {
		return fmt.Errorf("creating pool for %q: %w", e.Name, err)
	}

	e.Pool = pool
	e.poolReady.Store(true)
	e.loaded.Store(true)
	workers := numWorkers
	batchInfo := ""
	if timeoutMS > 0 {
		batchInfo = fmt.Sprintf(", batching=%dms/%d/%d", timeoutMS, cfg.Batching.MaxBatch, maxBatchTokens)
		workers = 1
	}
	log.Printf("  %s: %d workers ready (detected dim=%d%s)", e.Name, workers, cfg.Dim, batchInfo)
	return nil
}

// newNamedSession creates one named-tensor session for the scripted/image
// paths. It is a package var so tests can inject a partial-construction failure
// and count that already-opened sessions are closed.
var newNamedSession = func(data []byte, inputNames, outputNames []string, intraOpThreads, interOpThreads, execMode int, allowSpinning bool) (onnx.NamedSession, error) {
	return onnx.NewNamedRuntimeSessionFromBytes(data, inputNames, outputNames, intraOpThreads, interOpThreads, execMode, allowSpinning)
}

// ScriptResources lazily opens the generic named-tensor session and the
// word-aligned tokenizer a scripted evaluation needs, mirroring the pool's
// session options (weights bytes, graph inputs/outputs, thread counts). The
// embedding pool is intentionally not required: script models may not be
// embeddable at all (e.g. GLiNER's logits graph).
func (e *ModelEntry) ScriptResources() (*ScriptResources, error) {
	e.scriptOnce.Do(func() {
		e.scriptRes, e.scriptResErr = e.openScriptResources()
		if e.scriptRes != nil {
			e.scriptResPtr.Store(e.scriptRes)
		}
	})
	return e.scriptRes, e.scriptResErr
}

// ScriptDispatch reports the script path's cumulative dispatch wait and run
// microseconds, run count and live busy gauge, without opening resources.
// Zero values mean the model has not run a scripted inference yet.
func (e *ModelEntry) ScriptDispatch() (waitUs, runUs, runs, busy int64) {
	if res := e.scriptResPtr.Load(); res != nil {
		return res.DispatchCounters()
	}
	return 0, 0, 0, 0
}

func (e *ModelEntry) openScriptResources() (*ScriptResources, error) {
	cfg := e.cfg

	// The tokenizer is shared with the embedding pool, so a model that serves
	// both EMB and scripts loads exactly one.
	tok, err := e.sharedTokenizer()
	if err != nil {
		return nil, fmt.Errorf("loading tokenizer for %q: %w", e.Name, err)
	}

	inputNames, err := onnx.GetInputNames(cfg.ONNX)
	if err != nil {
		return nil, fmt.Errorf("reading input names for %q: %w", e.Name, err)
	}
	outInfo, err := onnx.GetOutputInfo(cfg.ONNX)
	if err != nil {
		return nil, fmt.Errorf("reading output info for %q: %w", e.Name, err)
	}
	outputNames := make([]string, 0, len(outInfo))
	for n := range outInfo {
		outputNames = append(outputNames, n)
	}
	sort.Strings(outputNames)

	modelData, err := os.ReadFile(cfg.ONNX)
	if err != nil {
		return nil, fmt.Errorf("reading model file for %q: %w", e.Name, err)
	}

	execMode := onnx.ExecModeSequential
	if cfg.ExecutionMode == "parallel" {
		execMode = onnx.ExecModeParallel
	}

	// The auto-tuned default never exceeds the embedding path's real session
	// count (one for a batcher pool, one per worker otherwise), so scripting
	// cannot multiply a model's footprint beyond what the embedding path
	// already commits to. An explicitly configured script_workers is honoured
	// verbatim as an operator override (memory for parallelism), never clamped.
	numSessions := cfg.ScriptWorkers
	if numSessions <= 0 {
		numSessions = effectiveEmbeddingSessions(&cfg)
	}
	if numSessions < 1 {
		numSessions = 1
	}

	// Unset intra_op_threads divides the cores−2 budget across the sessions,
	// so workers × threads stays near the core count instead of multiplying it.
	// An explicit value is honoured verbatim.
	intraThreads := scriptIntraOpThreads(cfg.IntraOpThreads, numSessions, runtime.GOMAXPROCS(0))

	sessions := make([]onnx.NamedSession, 0, numSessions)
	for i := 0; i < numSessions; i++ {
		sess, err := newNamedSession(
			modelData, inputNames, outputNames, intraThreads, cfg.InterOpThreads, execMode, cfg.ScriptAllowsSpinning(),
		)
		if err != nil {
			for _, opened := range sessions {
				_ = opened.Close()
			}
			return nil, fmt.Errorf("creating scripted session %d for %q: %w", i, e.Name, err)
		}
		sessions = append(sessions, sess)
	}

	split := bytes.Contains(modelData, []byte("DynamicQuantizeLinear"))
	if split {
		log.Printf("  %s: dynamic activation quantization — emb.run_batch runs items one at a time", e.Name)
	}

	e.scriptSessions.Store(int64(len(sessions)))
	e.scriptTokenizer.Store(true)
	return &ScriptResources{sessions: sessions, idle: idleSessions(sessions, cfg.ScriptCallers()), Tokenizer: tok, SplitBatch: split}, nil
}

// idleSessions seeds the idle dispatch channel with every session `callers`
// times, so up to that many evaluations run concurrently on each session.
// ORT sessions are safe for concurrent Run and each call allocates its own
// outputs.
func idleSessions(sessions []onnx.NamedSession, callers int) chan onnx.NamedSession {
	if callers < 1 {
		callers = 1
	}
	idle := make(chan onnx.NamedSession, len(sessions)*callers)
	for c := 0; c < callers; c++ {
		for _, sess := range sessions {
			idle <- sess
		}
	}
	return idle
}

func downloadModel(cfg *config.ModelConfig, name string) error {
	dir := filepath.Dir(cfg.ONNX)
	if cfg.ONNX == "" {
		dir = filepath.Join("models", name)
		cfg.ONNX = filepath.Join(dir, "model.onnx")
		cfg.Tokenizer = filepath.Join(dir, "tokenizer.json")
	}
	if _, err := os.Stat(cfg.ONNX); err == nil {
		return nil
	}
	// With quantize enabled the download writes `model_quantized.onnx` next to
	// the configured fp32 path, which is never created. Without this the model
	// would re-download on every boot.
	if cfg.Quantize != "off" {
		if _, err := os.Stat(filepath.Join(dir, "model_quantized.onnx")); err == nil {
			return nil
		}
	}
	log.Printf("  downloading %s from %s...", name, cfg.ModelRepo)
	preferQuantized := cfg.Quantize != "off" && cfg.Quantize != ""
	if err := hfhub.New().DownloadModel(cfg.ModelRepo, cfg.ModelSubfolder, dir, preferQuantized); err != nil {
		return fmt.Errorf("downloading %s: %w", cfg.ModelRepo, err)
	}
	log.Printf("  downloaded %s to %s", name, dir)
	return nil
}

func selectOutputTensor(outputs map[string]onnx.OutputInfo) (string, int) {
	var rank2Name, rank3Name, firstName string
	var firstRank int
	for name, info := range outputs {
		if firstName == "" {
			firstName = name
			firstRank = info.Rank
		}
		switch info.Rank {
		case 2:
			if rank2Name == "" {
				rank2Name = name
			}
		case 3:
			if rank3Name == "" {
				rank3Name = name
			}
		}
	}
	if rank2Name != "" {
		return rank2Name, 2
	}
	if rank3Name != "" {
		return rank3Name, 3
	}
	if firstName != "" {
		return firstName, firstRank
	}
	return "last_hidden_state", 3
}

func poolingForRank(rank int) string {
	if rank == 2 {
		return "none"
	}
	return "mean"
}

func resolveModelConfig(cfg *config.ModelConfig, name string) error {
	if cfg.Tokenizer == "" && cfg.ONNX != "" {
		cfg.Tokenizer = filepath.Join(filepath.Dir(cfg.ONNX), "tokenizer.json")
	}
	if cfg.MaxLength <= 0 && cfg.ONNX != "" {
		if ml, err := onnx.InferMaxLength(filepath.Dir(cfg.ONNX)); err == nil {
			cfg.MaxLength = ml
			log.Printf("  %s: detected max_length=%d from config.json", name, cfg.MaxLength)
		} else {
			cfg.MaxLength = 512
		}
	}
	if cfg.Dim <= 0 && cfg.ONNX != "" {
		if d, err := onnx.InferDim(cfg.ONNX); err == nil {
			cfg.Dim = d
			log.Printf("  %s: detected dim=%d from ONNX graph", name, cfg.Dim)
		}
	}
	if cfg.OutputTensor == "" || cfg.Pooling == "" {
		outInfo, err := onnx.GetOutputInfo(cfg.ONNX)
		if err == nil {
			tensorName, rank := selectOutputTensor(outInfo)
			if cfg.OutputTensor == "" {
				cfg.OutputTensor = tensorName
				log.Printf("  %s: auto-detected output=%q rank=%d", name, cfg.OutputTensor, rank)
			}
			if cfg.Pooling == "" {
				cfg.Pooling = poolingForRank(rank)
				log.Printf("  %s: auto-detected pooling=%s", name, cfg.Pooling)
			}
		}
	}
	if cfg.Pooling == "" {
		cfg.Pooling = "mean"
	}
	if cfg.OutputTensor == "" {
		cfg.OutputTensor = "last_hidden_state"
	}
	// Batching is ON by default (1ms window) so every model gets the
	// performance path (token budget + async tokenization); explicit
	// `timeout: 0` opts back into the worker pool.
	if cfg.Batching.Timeout == nil {
		v := 1
		cfg.Batching.Timeout = &v
	}
	if cfg.Batching.MaxBatch <= 0 {
		cfg.Batching.MaxBatch = 32
	}
	// Unset max_batch_tokens defaults to TEI's 16384 when batching is enabled;
	// explicit 0 keeps count-only behavior.
	if *cfg.Batching.Timeout > 0 && cfg.Batching.MaxBatchTokens == nil {
		v := 16384
		cfg.Batching.MaxBatchTokens = &v
	}
	// Unset tokenize_workers defaults to min(4, cores) when batching is enabled;
	// explicit 0 keeps serial tokenize-in-run behavior.
	if *cfg.Batching.Timeout > 0 && cfg.TokenizeWorkers == nil {
		v := min(4, runtime.GOMAXPROCS(0))
		cfg.TokenizeWorkers = &v
	}
	return nil
}

// resolveQuantize normalizes the quantize setting and, when enabled, points
// cfg.ONNX at pre-quantized weights next to the current path when present.
func resolveQuantize(cfg *config.ModelConfig) error {
	if cfg.Quantize == "" {
		cfg.Quantize = "auto"
	}
	switch cfg.Quantize {
	case "auto", "on", "off":
	default:
		return fmt.Errorf("quantize must be auto|on|off, got %q", cfg.Quantize)
	}
	if cfg.Quantize == "off" || cfg.ONNX == "" {
		return nil
	}

	dir := filepath.Dir(cfg.ONNX)
	for _, cand := range []string{
		filepath.Join(dir, "model_quantized.onnx"),
		filepath.Join(dir, "onnx", "model_quantized.onnx"),
		filepath.Join(dir, "onnx", "quantized", "model.onnx"),
	} {
		if _, err := os.Stat(cand); err == nil {
			if cand != cfg.ONNX {
				log.Printf("  using int8 weights %s", cand)
				cfg.ONNX = cand
			}
			return nil
		}
	}
	if cfg.Quantize == "on" {
		return fmt.Errorf("quantize=on but no quantized weights found next to %q", cfg.ONNX)
	}
	return nil
}

func LoadModel(cfg config.ModelConfig, name string) (*ModelEntry, error) {
	if cfg.ModelRepo != "" {
		if err := downloadModel(&cfg, name); err != nil {
			return nil, err
		}
	}

	if err := resolveQuantize(&cfg); err != nil {
		return nil, err
	}

	if err := resolveModelConfig(&cfg, name); err != nil {
		return nil, err
	}

	if cfg.Quantize == "on" || cfg.Quantize == "auto" {
		if strings.Contains(cfg.ONNX, "quantized") {
			log.Printf("  %s: quantization=int8", name)
		}
	}

	if err := cfg.Validate(); err != nil {
		return nil, fmt.Errorf("validating model %q: %w", name, err)
	}

	entry := &ModelEntry{
		Name: name,
		Dim:  cfg.Dim,
		cfg:  cfg,
	}

	if cfg.Preload {
		log.Printf("  preloading model %q...", name)
		if err := entry.ensurePool(); err != nil {
			_ = entry.closeResources()
			return nil, err
		}
	}
	if cfg.ScriptPreload {
		res, err := entry.ScriptResources()
		if err != nil {
			_ = entry.closeResources()
			return nil, err
		}
		log.Printf("  preloaded scripted sessions for %q (workers=%d)", name, len(res.Sessions()))
	}
	if cfg.ImagePreload && cfg.Image != nil {
		res, err := entry.ImageResources()
		if err != nil {
			_ = entry.closeResources()
			return nil, err
		}
		logImagePlan(name, res)
	}

	return entry, nil
}

func (r *Registry) GetOrInit(name string) (*ModelEntry, error) {
	// Hold the read lock across initialization so a concurrent Close (which
	// takes the write lock) waits for the load instead of closing under it.
	r.mu.RLock()
	defer r.mu.RUnlock()
	entry, ok := r.models[name]
	if !ok {
		return nil, fmt.Errorf("model '%s' not found", name)
	}

	// Always enter Once: checking Pool before Once races the initialization
	// goroutine's publication of that pointer on simultaneous cold requests.
	entry.once.Do(func() {
		entry.loadErr = entry.ensurePool()
	})
	if entry.loadErr != nil {
		return nil, entry.loadErr
	}
	return entry, nil
}

// Resolve returns a model entry without requiring the embedding pool. Script
// evaluation uses it so non-embeddable models (e.g. GLiNER logits graphs) can
// be addressed by name.
func (r *Registry) Resolve(name string) (*ModelEntry, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	entry, ok := r.models[name]
	if !ok {
		return nil, fmt.Errorf("model '%s' not found", name)
	}
	return entry, nil
}

func (r *Registry) Add(name string, entry *ModelEntry) {
	r.mu.Lock()
	defer r.mu.Unlock()
	// Tests and embedders may register an already-constructed pool. Publish its
	// initialized state before the entry becomes visible and consume Once so a
	// later GetOrInit does not try to rebuild it from absent config paths.
	if entry.Pool != nil {
		entry.poolReady.Store(true)
		entry.once.Do(func() {})
	}
	r.models[name] = entry
}

func (r *Registry) ModelsLoaded() (loaded, total int) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	total = len(r.models)
	for _, entry := range r.models {
		if entry.loaded.Load() {
			loaded++
		}
	}
	return loaded, total
}

func (r *Registry) SetModelCount(n int) {
	r.totalModels.Store(int64(n))
}

func (r *Registry) List() []*ModelEntry {
	r.mu.RLock()
	list := make([]*ModelEntry, 0, len(r.models))
	for _, entry := range r.models {
		list = append(list, entry)
	}
	r.mu.RUnlock()
	// The registry is a map, so iteration order is randomized; sort by name so
	// EMB.MODELS (and any other listing) is deterministic across calls.
	sort.Slice(list, func(i, j int) bool { return list[i].Name < list[j].Name })
	return list
}

func (r *Registry) HasModel(name string) bool {
	r.mu.RLock()
	_, ok := r.models[name]
	r.mu.RUnlock()
	return ok
}

func (r *Registry) Fingerprints() (map[string]ModelFingerprint, error) {
	models := r.List()
	result := make(map[string]ModelFingerprint, len(models))
	for _, entry := range models {
		fingerprint, err := entry.Fingerprint()
		if err != nil {
			log.Printf("snapshot: skipping model %q fingerprint: %v", entry.Name, err)
			continue
		}
		result[entry.Name] = ModelFingerprint{Fingerprint: fingerprint, Dim: entry.Dim}
	}
	return result, nil
}

func (r *Registry) FingerprintState() map[string]ModelFingerprint {
	models := r.List()
	result := make(map[string]ModelFingerprint, len(models))
	for _, entry := range models {
		st := ModelFingerprint{Dim: entry.Dim}
		if entry.loaded.Load() {
			fp, err := entry.Fingerprint()
			if err != nil {
				log.Printf("snapshot: skipping loaded model %q fingerprint: %v", entry.Name, err)
				result[entry.Name] = st
				continue
			}
			st.Fingerprint = fp
			st.Loaded = true
		}
		result[entry.Name] = st
	}
	return result
}

type ModelFingerprint struct {
	Fingerprint string
	Dim         int
	// Loaded reports whether the fingerprint was verified from the model this
	// run. Lazy (never-loaded) models have an empty fingerprint until their
	// first load; restore quarantines their entries until then.
	Loaded bool
}

func (r *Registry) TotalErrors() int64 {
	r.mu.RLock()
	defer r.mu.RUnlock()
	var total int64
	for _, entry := range r.models {
		if pool := entry.LoadedPool(); pool != nil {
			total += pool.Stats().Errors
		}
	}
	return total
}

func (r *Registry) Close() error {
	r.closeOnce.Do(func() {
		r.mu.Lock()
		defer r.mu.Unlock()
		var closeErrs []error
		for _, entry := range r.models {
			closeErrs = append(closeErrs, entry.closeResources())
		}
		clear(r.models)
		r.closeErr = errors.Join(closeErrs...)
	})
	return r.closeErr
}

func (e *ModelEntry) closeResources() error {
	var closeErrs []error
	if pool := e.LoadedPool(); pool != nil {
		closeErrs = append(closeErrs, pool.Close())
	}
	if e.scriptRes != nil {
		for _, sess := range e.scriptRes.Sessions() {
			closeErrs = append(closeErrs, sess.Close())
		}
	}
	e.closeImageSessions()
	// The tokenizer is shared by every path and is released last, after all
	// workers and sessions that can reference it have stopped.
	if e.sharedTok != nil {
		closeErrs = append(closeErrs, e.sharedTok.Close())
	}
	return errors.Join(closeErrs...)
}
