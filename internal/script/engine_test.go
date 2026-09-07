package script

import (
	"strings"
	"testing"
	"time"

	lua "github.com/yuin/gopher-lua"
)

func TestEvalReturnsValue(t *testing.T) {
	v, err := Eval("return {first = 1, second = 'two'}", nil, nil, EvalOptions{})
	if err != nil {
		t.Fatal(err)
	}
	tbl, ok := v.(*lua.LTable)
	if !ok {
		t.Fatalf("expected table, got %T", v)
	}
	if got := tbl.RawGetString("second"); got.String() != "two" {
		t.Fatalf("expected second=two, got %v", got)
	}
}

func TestEvalKeysArgv(t *testing.T) {
	v, err := Eval("return KEYS[1] .. '|' .. ARGV[2]",
		[]string{"hello world"}, []string{"a", "b", "c"}, EvalOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if v.String() != "hello world|b" {
		t.Fatalf("unexpected result %q", v.String())
	}
}

func TestEvalExplicitSignature(t *testing.T) {
	v, err := Eval("local function f() return #KEYS + #ARGV end return f()",
		[]string{"k1", "k2"}, []string{"a1", "a2", "a3"}, EvalOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if v.String() != "5" {
		t.Fatalf("expected 5, got %q", v.String())
	}
}

func TestEvalCompileError(t *testing.T) {
	if _, err := Eval("this is not lua (", nil, nil, EvalOptions{}); err == nil {
		t.Fatal("expected compile error")
	} else if !strings.Contains(err.Error(), "compiling") {
		t.Fatalf("expected compile error, got %v", err)
	}
}

func TestEvalRuntimeError(t *testing.T) {
	if _, err := Eval("return nil.x", nil, nil, EvalOptions{}); err == nil {
		t.Fatal("expected runtime error")
	}
}

func TestSandboxNoOsIo(t *testing.T) {
	for _, src := range []string{
		"return os.getenv('HOME')",
		"return os.execute('ls')",
		"return io.open('/etc/passwd')",
		"return require('io')",
		"return module('os')",
		"return module('io')",
		"return coroutine.running()",
	} {
		if _, err := Eval(src, nil, nil, EvalOptions{}); err == nil {
			t.Fatalf("script %q should have failed in the sandbox", src)
		}
	}
}

func TestSandboxNoRandom(t *testing.T) {
	if _, err := Eval("return math.random()", nil, nil, EvalOptions{}); err == nil {
		t.Fatal("math.random must be unavailable (determinism)")
	}
}

func TestEvalScriptTooLarge(t *testing.T) {
	if _, err := Eval(strings.Repeat("-- pad\n", 100), nil, nil, EvalOptions{MaxScriptBytes: 100}); err != ErrScriptTooLarge {
		t.Fatalf("expected ErrScriptTooLarge, got %v", err)
	}
}

func TestEvalInfiniteLoopDeadline(t *testing.T) {
	src := "local x = 0 while true do x = x + 1 end"
	start := time.Now()
	if _, err := Eval(src, nil, nil, EvalOptions{Deadline: 200 * time.Millisecond}); err != ErrDeadlineExceeded {
		t.Fatalf("expected ErrDeadlineExceeded, got %v", err)
	}
	if elapsed := time.Since(start); elapsed > 5*time.Second {
		t.Fatalf("deadline took too long to fire: %v", elapsed)
	}
}

func TestEvalDeepRecursionFails(t *testing.T) {
	// Tail recursion is a tight loop in gopher-lua: the call-stack limit does
	// not bound it, so the wall-clock deadline is the guard. Keep the deadline
	// short to keep the unit suite fast.
	src := "local function f(n) return f(n + 1) end return f(0)"
	start := time.Now()
	if _, err := Eval(src, nil, nil, EvalOptions{Deadline: 200 * time.Millisecond}); err == nil {
		t.Fatal("expected recursion loop to fail")
	}
	if elapsed := time.Since(start); elapsed > 5*time.Second {
		t.Fatalf("recursion loop took too long to bound: %v", elapsed)
	}
}

func TestEvalIsolationAcrossCalls(t *testing.T) {
	// No global state may leak between evaluations: the second call runs in a
	// fresh state, so the (nil) global reads as LNil instead of the value the
	// first call wrote.
	if _, err := Eval("leaked_global = 1 return leaked_global", nil, nil, EvalOptions{}); err != nil {
		t.Fatal(err)
	}
	v, err := Eval("return leaked_global", nil, nil, EvalOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if v != lua.LNil {
		t.Fatalf("global leaked between evaluations: %v", v)
	}
}
