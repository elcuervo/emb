package registry

import (
	"crypto/sha256"
	"encoding/hex"
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

	"github.com/elcuervo/emb/internal/config"
	"github.com/elcuervo/emb/internal/hfhub"
	"github.com/elcuervo/emb/internal/onnx"
	"github.com/elcuervo/emb/internal/pipeline"
	"github.com/elcuervo/emb/internal/tokenizer"
)

type ModelEntry struct {
	Pool *pipeline.Pool
	Dim  int
	Name string

	// Quantization is "int8" when quantized weights were resolved, else "fp32".
	Quantization string
	// ModelSize is the on-disk size of the resolved weights file.
	ModelSize int64

	once    sync.Once
	cfg     config.ModelConfig
	loaded  atomic.Bool
	loadErr error
	// Fingerprints are expensive for large ONNX files. Persistence computes one
	// lazily per model and all periodic/manual saves reuse it.
	fingerprintOnce sync.Once
	fingerprint     string
	fingerprintErr  error
}

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
	cores := runtime.GOMAXPROCS(0)
	if cores <= 2 {
		return 1
	}
	return cores - 2
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

func (e *ModelEntry) ensurePool() error {
	if e.loaded.Load() {
		return nil
	}

	log.Printf("  loading model %q (dim=%d, max_length=%d)...", e.Name, e.cfg.Dim, e.cfg.MaxLength)

	cfg := e.cfg
	tok, err := tokenizer.NewTokenizer(cfg.Tokenizer, cfg.PadOutput)
	if err != nil {
		return fmt.Errorf("loading tokenizer for %q: %w", e.Name, err)
	}

	numWorkers := cfg.Workers
	if numWorkers <= 0 {
		numWorkers = autoTuneWorkers(cfg.ONNX, 0)
	}

	outInfo, err := onnx.GetOutputInfo(cfg.ONNX)
	if err != nil {
		_ = tok.Close()
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
		_ = tok.Close()
		return fmt.Errorf("reading input names for %q: %w", e.Name, err)
	}
	log.Printf("  %s: inputs=%v output=%q rank=%d", e.Name, inputNames, cfg.OutputTensor, out.Rank)

	modelData, err := os.ReadFile(cfg.ONNX)
	if err != nil {
		_ = tok.Close()
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
		return onnx.NewRuntimeSessionFromBytes(
			modelData,
			inputNames,
			[]string{cfg.OutputTensor},
			cfg.Dim,
			out.Rank,
			intraThreads,
			cfg.InterOpThreads,
			execMode,
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
	pool, err := pipeline.NewPool(sessionFactory, tok, numWorkers, cfg.Dim, cfg.MaxLength, cfg.Normalize, cfg.Pooling, timeoutMS, cfg.Batching.MaxBatch, maxBatchTokens, tokenizeWorkers)
	if err != nil {
		_ = tok.Close()
		return fmt.Errorf("creating pool for %q: %w", e.Name, err)
	}

	e.Pool = pool
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
	log.Printf("  downloading %s from %s...", name, cfg.ModelRepo)
	preferQuantized := cfg.Quantize != "off" && cfg.Quantize != ""
	if err := hfhub.New().DownloadModel(cfg.ModelRepo, dir, preferQuantized); err != nil {
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
			return nil, err
		}
	}

	return entry, nil
}

func (r *Registry) GetOrInit(name string) (*ModelEntry, error) {
	r.mu.RLock()
	entry, ok := r.models[name]
	r.mu.RUnlock()
	if !ok {
		return nil, fmt.Errorf("model '%s' not found", name)
	}

	if entry.Pool == nil {
		entry.once.Do(func() {
			entry.loadErr = entry.ensurePool()
		})
		if entry.loadErr != nil {
			return nil, entry.loadErr
		}
	}
	return entry, nil
}

func (r *Registry) Add(name string, entry *ModelEntry) {
	r.mu.Lock()
	defer r.mu.Unlock()
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
	defer r.mu.RUnlock()
	list := make([]*ModelEntry, 0, len(r.models))
	for _, entry := range r.models {
		list = append(list, entry)
	}
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

// FingerprintState reports every configured model: a verified fingerprint for
// loaded models and an unloaded placeholder for lazy (never-loaded) ones.
// Restore uses it to decide which snapshot entries can be admitted now and
// which must be quarantined until their model's first load, without reading
// every large ONNX file at startup for the sake of the cache.
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
		if entry.Pool != nil {
			total += entry.Pool.Stats().Errors
		}
	}
	return total
}

func (r *Registry) Close() error {
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, entry := range r.models {
		if entry.Pool != nil {
			_ = entry.Pool.Close()
		}
	}
	clear(r.models)
	return nil
}
