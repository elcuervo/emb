package script

import (
	"fmt"
	"strings"
	"sync/atomic"

	lua "github.com/yuin/gopher-lua"
	"github.com/yuin/gopher-lua/parse"

	"github.com/elcuervo/emb/internal/bounded"
)

// Compiler caches compiled Lua prototypes per (model, wrapped source), so
// repeat EMB.EVSHA executions skip parsing/compiling entirely. Each
// evaluation still instantiates into a FRESH Lua state (same sandbox,
// budgets, and isolation); only the front-end parse+compile step is shared.
// The cache is bounded per model (mirroring scriptCache) via bounded.Map, so
// EMB.EVAL with unbounded unique inline scripts cannot grow it without limit.
const defaultMaxCompiledProtos = 1024

type Compiler struct {
	protos *bounded.Map[*lua.FunctionProto] // model → wrappedSource → proto
	// Compiles counts actual parse+compile events (test hook; server-level
	// EvalWithHosts without a compiler is unaffected).
	Compiles atomic.Int64
}

// NewCompiler returns an empty compile cache.
func NewCompiler() *Compiler {
	return &Compiler{protos: bounded.New[*lua.FunctionProto](defaultMaxCompiledProtos)}
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

// Precompile validates source size, parses/compiles the script, and stores
// the prototype in the cache so the first EVSHA skips the compile step.
// It returns an error if the script is oversized or fails to compile.
func (c *Compiler) Precompile(model, source string) error {
	if len(source) > DefaultMaxScriptBytes {
		return ErrScriptTooLarge
	}
	_, err := c.compile(model, source)
	return err
}

// Flush drops cached prototypes for a model ("" clears all).
func (c *Compiler) Flush(model string) { c.protos.Clear(model) }

func (c *Compiler) compile(model, source string) (*lua.FunctionProto, error) {
	wrapped := wrapSource(source)
	proto, _, err := c.protos.GetOrCreate(model, wrapped, func() (*lua.FunctionProto, error) {
		chunk, err := parse.Parse(strings.NewReader(wrapped), "<string>")
		if err != nil {
			return nil, err
		}
		proto, err := lua.Compile(chunk, "<string>")
		if err != nil {
			return nil, err
		}
		c.Compiles.Add(1)
		return proto, nil
	})
	if err != nil {
		return nil, err
	}
	return proto, nil
}

// wrapSource is the shared Redis-style wrapper so a script body's `return`
// yields the evaluation result.
func wrapSource(source string) string {
	return "return (function(KEYS, ARGV)\n" + source + "\nend)(KEYS, ARGV)"
}
