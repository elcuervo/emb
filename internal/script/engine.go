// Package script implements the Redis-style script surface for emb:
// sandboxed Lua evaluation over config-mounted models, with KEYS/ARGV
// semantics, content-addressed reply caching, and Lua→RESP2 conversion.
package script

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	lua "github.com/yuin/gopher-lua"
	"github.com/yuin/gopher-lua/parse"
)

// DefaultMaxScriptBytes bounds a script's source size. Mirrors the spirit of
// Redis's lua-time-limit guardrails: a cap that scripts never legitimately
// approach, enforced before compilation so pathological payloads cost nothing.
const DefaultMaxScriptBytes = 64 * 1024

// DefaultDeadline bounds each evaluation's wall-clock time. Scripts are pure
// compute (see sandbox), so real workloads finish in microseconds.
const DefaultDeadline = 30 * time.Second

// ErrScriptTooLarge is returned when a script exceeds the size cap.
var ErrScriptTooLarge = errors.New("script exceeds size limit")

// Compile validates that `source` parses as a Lua chunk without executing it.
// EMB.SCRIPT LOAD uses it to reject invalid scripts before caching. It wraps
// the source exactly like the evaluators (wrapSource), so LOAD accepts exactly
// what EVAL/EVSHA will run (a raw chunk permits top-level `return`, which the
// function wrapper does not).
func Compile(source string) error {
	ls := lua.NewState(lua.Options{SkipOpenLibs: true})
	defer ls.Close()
	if _, err := ls.LoadString(wrapSource(source)); err != nil {
		return fmt.Errorf("compiling script: %w", err)
	}
	return nil
}

// ErrDeadlineExceeded is returned when an evaluation runs past its deadline.
var ErrDeadlineExceeded = errors.New("script deadline exceeded")

// EvalOptions configures a single evaluation.
type EvalOptions struct {
	// Deadline bounds wall-clock execution; zero uses DefaultDeadline.
	Deadline time.Duration
	// MaxScriptBytes bounds source size; zero uses DefaultMaxScriptBytes.
	MaxScriptBytes int
}

// Eval compiles and runs `source` in a fresh sandboxed Lua state with KEYS and
// ARGV globals set from the given strings, and returns the script's return
// value (the last value of the script body, Redis-style). Each invocation
// creates a fresh state: no global state leaks between evaluations, and a
// runaway script kills only its own request.
func Eval(source string, keys, argv []string, opts EvalOptions) (lua.LValue, error) {
	return EvalWithHosts(source, keys, argv, Hosts{}, opts)
}

// EvalWithHosts is Eval with the whitelisted emb.* / json host functions
// bound from `hosts` (see Hosts). Each invocation compiles the source fresh;
// hot paths should use Compiler.Eval to reuse the compiled prototype.
func EvalWithHosts(source string, keys, argv []string, hosts Hosts, opts EvalOptions) (lua.LValue, error) {
	if opts.MaxScriptBytes <= 0 {
		opts.MaxScriptBytes = DefaultMaxScriptBytes
	}
	if len(source) > opts.MaxScriptBytes {
		return lua.LNil, ErrScriptTooLarge
	}
	if opts.Deadline <= 0 {
		opts.Deadline = DefaultDeadline
	}

	chunk, err := parse.Parse(strings.NewReader(wrapSource(source)), "<string>")
	if err != nil {
		return lua.LNil, fmt.Errorf("compiling script: %w", err)
	}
	proto, err := lua.Compile(chunk, "<string>")
	if err != nil {
		return lua.LNil, fmt.Errorf("compiling script: %w", err)
	}
	return runProto(proto, keys, argv, hosts, opts)
}

// runProto runs a compiled (wrapped) script in a fresh sandboxed state with
// KEYS/ARGV globals and the given deadline; identical semantics to
// EvalWithHosts, minus the parse+compile step.
func runProto(proto *lua.FunctionProto, keys, argv []string, hosts Hosts, opts EvalOptions) (lua.LValue, error) {
	if opts.Deadline <= 0 {
		opts.Deadline = DefaultDeadline
	}

	ls := lua.NewState(lua.Options{SkipOpenLibs: true})
	defer ls.Close()
	openSandboxLibs(ls)
	registerHosts(ls, hosts)

	// KEYS/ARGV globals: the wrapper also passes them as function args so
	// explicit `function(KEYS, ARGV)` signatures work too.
	ls.SetGlobal("KEYS", stringTable(ls, keys))
	ls.SetGlobal("ARGV", stringTable(ls, argv))

	ctx, cancel := context.WithTimeout(context.Background(), opts.Deadline)
	defer cancel()
	ls.SetContext(ctx)

	fn := ls.NewFunctionFromProto(proto)
	ls.Push(fn)
	if err := ls.PCall(0, lua.MultRet, nil); err != nil {
		if ctx.Err() == context.DeadlineExceeded {
			return lua.LNil, ErrDeadlineExceeded
		}
		return lua.LNil, fmt.Errorf("executing script: %w", err)
	}
	if ls.GetTop() == 0 {
		return lua.LNil, nil
	}
	return ls.Get(-1), nil
}

// openSandboxLibs opens the libraries and strips everything outside the
// curated subset: os, io, debug, package/require, file loaders, channels and
// coroutines are removed after the fact, and non-deterministic math functions
// are dropped (scripts must be pure compute so replies are cacheable).
func openSandboxLibs(ls *lua.LState) {
	ls.OpenLibs()
	for _, name := range []string{
		lua.OsLibName, lua.IoLibName, lua.DebugLibName, lua.LoadLibName,
		"channel", "coroutine", "require", "dofile", "loadfile", "module",
	} {
		ls.SetGlobal(name, lua.LNil)
	}
	// Determinism: no rand(), no os, nothing with hidden state.
	if mathTab, ok := ls.GetGlobal("math").(*lua.LTable); ok {
		mathTab.RawSetString("random", lua.LNil)
		mathTab.RawSetString("randomseed", lua.LNil)
	}
}

// stringTable builds a Lua array table of strings (indexes 1..n).
func stringTable(ls *lua.LState, items []string) *lua.LTable {
	t := ls.NewTable()
	for i, s := range items {
		t.RawSetInt(i+1, lua.LString(s))
	}
	return t
}
