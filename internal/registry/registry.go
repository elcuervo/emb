package registry

import (
	"fmt"
	"log"
	"os"
	"path/filepath"
	"runtime"
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

	once    sync.Once
	cfg     config.ModelConfig
	loaded  atomic.Bool
	loadErr error
}

type Registry struct {
	mu     sync.RWMutex
	models map[string]*ModelEntry
}

func New() *Registry {
	return &Registry{
		models: make(map[string]*ModelEntry),
	}
}

func autoTuneWorkers(modelPath string, maxWorkers int) int {
	maxCores := runtime.GOMAXPROCS(0)
	if maxWorkers > 0 && maxWorkers < maxCores {
		maxCores = maxWorkers
	}
	mem := totalSystemMemory()
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
	tok, err := tokenizer.NewHFTokenizer(cfg.Tokenizer)
	if err != nil {
		return fmt.Errorf("loading tokenizer for %q: %w", e.Name, err)
	}

	numWorkers := cfg.Workers
	if numWorkers <= 0 {
		numWorkers = autoTuneWorkers(cfg.ONNX, 0)
	}

	outInfo, err := onnx.GetOutputInfo(cfg.ONNX)
	if err != nil {
		tok.Close()
		return fmt.Errorf("reading output info for %q: %w", e.Name, err)
	}
	out, ok := outInfo[cfg.OutputTensor]
	if !ok {
		tok.Close()
		return fmt.Errorf("model %q: output tensor %q not found", e.Name, cfg.OutputTensor)
	}

	inputNames, err := onnx.GetInputNames(cfg.ONNX)
	if err != nil {
		tok.Close()
		return fmt.Errorf("reading input names for %q: %w", e.Name, err)
	}
	log.Printf("  %s: inputs=%v output=%q rank=%d", e.Name, inputNames, cfg.OutputTensor, out.Rank)

	sessionFactory := func() (onnx.Session, error) {
		return onnx.NewRuntimeSession(
			cfg.ONNX,
			inputNames,
			[]string{cfg.OutputTensor},
			cfg.Dim,
			out.Rank,
		)
	}

	pool, err := pipeline.NewPool(sessionFactory, tok, numWorkers, cfg.Dim, cfg.MaxLength, cfg.Normalize, cfg.Pooling, cfg.Batching.Timeout, cfg.Batching.MaxBatch)
	if err != nil {
		tok.Close()
		return fmt.Errorf("creating pool for %q: %w", e.Name, err)
	}

	e.Pool = pool
	e.loaded.Store(true)
	workers := numWorkers
	batchInfo := ""
	if cfg.Batching.Timeout > 0 {
		batchInfo = fmt.Sprintf(", batching=%dms/%d", cfg.Batching.Timeout, cfg.Batching.MaxBatch)
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
	if err := hfhub.New().DownloadModel(cfg.ModelRepo, dir); err != nil {
		return fmt.Errorf("downloading %s: %w", cfg.ModelRepo, err)
	}
	log.Printf("  downloaded %s to %s", name, dir)
	return nil
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
	if cfg.Pooling == "" {
		cfg.Pooling = "mean"
	}
	if cfg.OutputTensor == "" {
		cfg.OutputTensor = "last_hidden_state"
	}
	if cfg.Batching.Timeout <= 0 {
		cfg.Batching.Timeout = 1
	}
	if cfg.Batching.MaxBatch <= 0 {
		cfg.Batching.MaxBatch = 32
	}
	return nil
}

func LoadModel(cfg config.ModelConfig, name string) (*ModelEntry, error) {
	if cfg.ModelRepo != "" {
		if err := downloadModel(&cfg, name); err != nil {
			return nil, err
		}
	}

	if err := resolveModelConfig(&cfg, name); err != nil {
		return nil, err
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

func (r *Registry) List() []*ModelEntry {
	r.mu.RLock()
	defer r.mu.RUnlock()
	list := make([]*ModelEntry, 0, len(r.models))
	for _, entry := range r.models {
		list = append(list, entry)
	}
	return list
}

func (r *Registry) Close() error {
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, entry := range r.models {
		entry.Pool.Close()
	}
	clear(r.models)
	return nil
}
