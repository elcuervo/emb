package registry

import (
	"fmt"
	"runtime"
	"sync"

	"github.com/elcuervo/emb/internal/config"
	"github.com/elcuervo/emb/internal/onnx"
	"github.com/elcuervo/emb/internal/pipeline"
	"github.com/elcuervo/emb/internal/tokenizer"
)

type ModelEntry struct {
	Pool  *pipeline.Pool
	Dim   int
	Name  string
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

func LoadModel(cfg config.ModelConfig, name string) (*ModelEntry, error) {
	if err := cfg.Validate(); err != nil {
		return nil, fmt.Errorf("validating model %q: %w", name, err)
	}

	tok, err := tokenizer.NewHFTokenizer(cfg.Tokenizer)
	if err != nil {
		return nil, fmt.Errorf("loading tokenizer for %q: %w", name, err)
	}

	numWorkers := runtime.GOMAXPROCS(0)

	sessionFactory := func() (onnx.Session, error) {
		return onnx.NewRuntimeSession(
			cfg.ONNX,
			[]string{"input_ids", "attention_mask", "token_type_ids"},
			[]string{"last_hidden_state"},
			cfg.Dim,
		)
	}

	pool, err := pipeline.NewPool(sessionFactory, tok, numWorkers, cfg.Dim, cfg.MaxLength, cfg.Normalize)
	if err != nil {
		tok.Close()
		return nil, fmt.Errorf("creating pool for %q: %w", name, err)
	}

	return &ModelEntry{
		Pool: pool,
		Dim:  cfg.Dim,
		Name: name,
	}, nil
}

func (r *Registry) Add(name string, entry *ModelEntry) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.models[name] = entry
}

func (r *Registry) Get(name string) (*ModelEntry, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	entry, ok := r.models[name]
	return entry, ok
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
	for name, entry := range r.models {
		entry.Pool.Close()
		delete(r.models, name)
	}
	return nil
}
