package script

import (
	"strconv"
	"testing"
	"time"
)

func TestCompilerReusesProto(t *testing.T) {
	c := NewCompiler()
	const src = "return KEYS[1] .. '|' .. ARGV[1]"
	v1, err := c.Eval("m", src, []string{"a"}, []string{"b"}, Hosts{}, EvalOptions{})
	if err != nil {
		t.Fatal(err)
	}
	v2, err := c.Eval("m", src, []string{"a"}, []string{"b"}, Hosts{}, EvalOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if v1.String() != v2.String() {
		t.Fatalf("identical source produced different results: %q vs %q", v1.String(), v2.String())
	}
	if got := c.Compiles.Load(); got != 1 {
		t.Fatalf("expected 1 compile for repeated source, got %d", got)
	}
}

func TestCompilerPerModel(t *testing.T) {
	c := NewCompiler()
	const src = "return 1"
	if _, err := c.Eval("a", src, nil, nil, Hosts{}, EvalOptions{}); err != nil {
		t.Fatal(err)
	}
	if _, err := c.Eval("b", src, nil, nil, Hosts{}, EvalOptions{}); err != nil {
		t.Fatal(err)
	}
	if got := c.Compiles.Load(); got != 2 {
		t.Fatalf("per-model compile expected 2, got %d", got)
	}
}

func TestCompilerPerSource(t *testing.T) {
	c := NewCompiler()
	if _, err := c.Eval("m", "return 1", nil, nil, Hosts{}, EvalOptions{}); err != nil {
		t.Fatal(err)
	}
	if _, err := c.Eval("m", "return 2", nil, nil, Hosts{}, EvalOptions{}); err != nil {
		t.Fatal(err)
	}
	if got := c.Compiles.Load(); got != 2 {
		t.Fatalf("per-source compile expected 2, got %d", got)
	}
}

func TestCompilerFlush(t *testing.T) {
	c := NewCompiler()
	const src = "return 1"
	if _, err := c.Eval("a", src, nil, nil, Hosts{}, EvalOptions{}); err != nil {
		t.Fatal(err)
	}
	if _, err := c.Eval("b", src, nil, nil, Hosts{}, EvalOptions{}); err != nil {
		t.Fatal(err)
	}
	c.Flush("a")
	if _, err := c.Eval("a", src, nil, nil, Hosts{}, EvalOptions{}); err != nil {
		t.Fatal(err)
	}
	// a recompiled (back to its own compile), b's entry survived: total 3.
	if got := c.Compiles.Load(); got != 3 {
		t.Fatalf("expected 3 compiles after flush of one model, got %d", got)
	}
	c.Flush("")
	if _, err := c.Eval("a", src, nil, nil, Hosts{}, EvalOptions{}); err != nil {
		t.Fatal(err)
	}
	if got := c.Compiles.Load(); got != 4 {
		t.Fatalf("expected compile after global flush, got %d", got)
	}
}

func TestCompilerCompileErrorNotCached(t *testing.T) {
	c := NewCompiler()
	if _, err := c.Eval("m", "this is not lua (", nil, nil, Hosts{}, EvalOptions{}); err == nil {
		t.Fatal("expected compile error")
	}
	// A broken source must not poison the cache for later valid runs.
	if _, err := c.Eval("m", "return 1", nil, nil, Hosts{}, EvalOptions{}); err != nil {
		t.Fatal(err)
	}
	if got := c.Compiles.Load(); got != 1 {
		t.Fatalf("expected exactly 1 successful compile, got %d", got)
	}
}

func TestCompilerProtoCacheBounded(t *testing.T) {
	// EMB.EVAL feeds unique inline scripts per request; the prototype cache
	// must stay bounded instead of growing without limit.
	c := NewCompiler()
	c.max = 8
	for i := 0; i < 100; i++ {
		if _, err := c.Eval("m", "return '"+string(rune('a'+i%26))+"' .. "+strconv.Itoa(i), nil, nil, Hosts{}, EvalOptions{}); err != nil {
			t.Fatal(err)
		}
	}
	c.mu.Lock()
	n := len(c.protos["m"])
	c.mu.Unlock()
	if n != c.max {
		t.Fatalf("expected proto cache capped at %d, got %d", c.max, n)
	}
}

func TestPrecompileWarmsCache(t *testing.T) {
	c := NewCompiler()
	const src = "return KEYS[1] .. '|' .. ARGV[1]"
	if err := c.Precompile("m", src); err != nil {
		t.Fatal(err)
	}
	// Precompile should have compiled the proto.
	if got := c.Compiles.Load(); got != 1 {
		t.Fatalf("expected 1 compile after Precompile, got %d", got)
	}
	// Eval with the same source should reuse the cached proto.
	if _, err := c.Eval("m", src, []string{"a"}, []string{"b"}, Hosts{}, EvalOptions{}); err != nil {
		t.Fatal(err)
	}
	if got := c.Compiles.Load(); got != 1 {
		t.Fatalf("expected 1 compile (reused), got %d", got)
	}
}

func TestPrecompileRejectsOversized(t *testing.T) {
	c := NewCompiler()
	if err := c.Precompile("m", "return 1"); err != nil {
		t.Fatal(err)
	}
	if err := c.Precompile("m", "return 1"); err != nil {
		// repeated precompile of same source is idempotent (cache hit)
		t.Fatal(err)
	}
	big := make([]byte, DefaultMaxScriptBytes+1)
	if err := c.Precompile("m", string(big)); err != ErrScriptTooLarge {
		t.Fatalf("expected ErrScriptTooLarge, got %v", err)
	}
}

func TestPrecompileRejectsInvalidLua(t *testing.T) {
	c := NewCompiler()
	if err := c.Precompile("m", "this is not lua ("); err == nil {
		t.Fatal("expected compile error")
	}
}

func TestCompilerBudgetAndSandbox(t *testing.T) {
	c := NewCompiler()
	if _, err := c.Eval("m", "return 1", nil, nil, Hosts{}, EvalOptions{MaxScriptBytes: 3}); err != ErrScriptTooLarge {
		t.Fatalf("expected ErrScriptTooLarge, got %v", err)
	}
	// Deadlines still apply to cached protos.
	if _, err := c.Eval("m", "local x = 0 while true do x = x + 1 end", nil, nil, Hosts{}, EvalOptions{Deadline: 50 * time.Millisecond}); err != ErrDeadlineExceeded {
		t.Fatalf("expected ErrDeadlineExceeded, got %v", err)
	}
	// Sandbox still strips os/io on cached protos.
	if _, err := c.Eval("m", "return os.getenv('HOME')", nil, nil, Hosts{}, EvalOptions{}); err == nil {
		t.Fatal("expected sandbox error on cached proto")
	}
}
