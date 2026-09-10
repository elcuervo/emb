package script

import (
	"fmt"
	"strings"
	"sync"
	"sync/atomic"

	lua "github.com/yuin/gopher-lua"
	"github.com/yuin/gopher-lua/parse"
)

// Compiler caches compiled Lua prototypes per (model, wrapped source), so
// repeat EMB.EVSHA executions skip parsing/compiling entirely. Each
// evaluation still instantiates into a FRESH Lua state (same sandbox,
// budgets, and isolation); only the front-end parse+compile step is shared.
// The cache is bounded per model (mirroring scriptCache.max) so EMB.EVAL
// with unbounded unique inline scripts cannot grow it without limit.
const defaultMaxCompiledProtos = 1024

type Compiler struct {
	mu     sync.Mutex
	protos map[string]map[string]*lua.FunctionProto // model → wrappedSource → proto
	max    int
	// Compiles counts actual parse+compile events (test hook; server-level
	// EvalWithHosts without a compiler is unaffected).
	Compiles atomic.Int64
}

// NewCompiler returns an empty compile cache.
func NewCompiler() *Compiler {
	return &Compiler{
		protos: make(map[string]map[string]*lua.FunctionProto),
		max:    defaultMaxCompiledProtos,
	}
}

// Eval compiles-or-reuses the prototype for (model, source) and runs it in a
// fresh sandboxed state with KEYS/ARGV semantics identical to EvalWithHosts.
func (c *Compiler) Eval(model, source string, keys, argv []string, hosts Hosts, opts EvalOptions) (lua.LValue, error) {
	if opts.MaxScriptBytes <= 0 {
		opts.MaxScriptBytes = DefaultMaxScriptBytes
	}
	if len(source) > opts.MaxScriptBytes {
		return lua.LNil, ErrScriptTooLarge
	}
	proto, err := c.compile(model, source)
	if err != nil {
		return lua.LNil, fmt.Errorf("compiling script: %w", err)
	}
	return runProto(proto, keys, argv, hosts, opts)
}

// Flush drops cached prototypes for a model ("" clears all).
func (c *Compiler) Flush(model string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if model == "" {
		c.protos = make(map[string]map[string]*lua.FunctionProto)
		return
	}
	delete(c.protos, model)
}

func (c *Compiler) compile(model, source string) (*lua.FunctionProto, error) {
	wrapped := wrapSource(source)

	c.mu.Lock()
	defer c.mu.Unlock()
	perModel := c.protos[model]
	if perModel == nil {
		perModel = make(map[string]*lua.FunctionProto)
		c.protos[model] = perModel
	}
	if proto, ok := perModel[wrapped]; ok {
		return proto, nil
	}
	// Bound the cache: evict a deterministic entry (the lexicographically
	// smallest key, mirroring scriptCache) when this model's cache is full.
	if len(perModel) >= c.max {
		oldest := ""
		for k := range perModel {
			if oldest == "" || k < oldest {
				oldest = k
			}
		}
		delete(perModel, oldest)
	}
	chunk, err := parse.Parse(strings.NewReader(wrapped), "<string>")
	if err != nil {
		return nil, err
	}
	proto, err := lua.Compile(chunk, "<string>")
	if err != nil {
		return nil, err
	}
	c.Compiles.Add(1)
	perModel[wrapped] = proto
	return proto, nil
}

// wrapSource is the shared Redis-style wrapper so a script body's `return`
// yields the evaluation result.
func wrapSource(source string) string {
	return "return (function(KEYS, ARGV)\n" + source + "\nend)(KEYS, ARGV)"
}
