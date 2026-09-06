package script

import (
	"math"
	"testing"

	lua "github.com/yuin/gopher-lua"
)

func TestMathSigmoidScalar(t *testing.T) {
	v, err := EvalWithHosts("return emb.math.sigmoid(0)", nil, nil, Hosts{}, EvalOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if math.Abs(luaNumF(v)-0.5) > 1e-9 {
		t.Fatalf("sigmoid(0) = %v, want 0.5", luaNumF(v))
	}
}

func TestMathSigmoidVector(t *testing.T) {
	v, err := EvalWithHosts("return emb.math.sigmoid({0, 1, -1})", nil, nil, Hosts{}, EvalOptions{})
	if err != nil {
		t.Fatal(err)
	}
	vals := luaNumList(t, v)
	if len(vals) != 3 {
		t.Fatalf("expected 3 values, got %v", vals)
	}
	if math.Abs(vals[0]-0.5) > 1e-9 {
		t.Fatalf("sigmoid(0)=%v", vals[0])
	}
	if math.Abs(vals[1]-sigmoidOf(1)) > 1e-9 {
		t.Fatalf("sigmoid(1)=%v", vals[1])
	}
	if math.Abs(vals[2]-sigmoidOf(-1)) > 1e-9 {
		t.Fatalf("sigmoid(-1)=%v", vals[2])
	}
}

func TestMathSoftmaxStable(t *testing.T) {
	// Large inputs: a naive exp overflows; the stable implementation must not.
	v, err := EvalWithHosts("return emb.math.softmax({1000, 1001, 999})", nil, nil, Hosts{}, EvalOptions{})
	if err != nil {
		t.Fatal(err)
	}
	vals := luaNumList(t, v)
	if len(vals) != 3 {
		t.Fatalf("expected 3 values, got %v", vals)
	}
	sum := 0.0
	for _, x := range vals {
		if math.IsInf(x, 0) || math.IsNaN(x) {
			t.Fatalf("softmax overflow: %v", vals)
		}
		sum += x
	}
	if math.Abs(sum-1) > 1e-6 {
		t.Fatalf("softmax sums to %v", sum)
	}
	if !(vals[1] > vals[0] && vals[1] > vals[2]) {
		t.Fatalf("expected middle element dominant: %v", vals)
	}
}

func TestMathArgmax(t *testing.T) {
	// Multi-return via Lua locals; first maximum wins (index 2 for {3,7,7,1}).
	v, err := EvalWithHosts(`local idx, val = emb.math.argmax({3, 7, 7, 1}) return idx .. "|" .. val`,
		nil, nil, Hosts{}, EvalOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if v.String() != "2|7" {
		t.Fatalf("argmax = %q, want 2|7", v.String())
	}
}

func TestMathEmptyErrors(t *testing.T) {
	for _, src := range []string{
		"return emb.math.sigmoid({})",
		"return emb.math.softmax({})",
		"return emb.math.argmax({})",
	} {
		if _, err := EvalWithHosts(src, nil, nil, Hosts{}, EvalOptions{}); err == nil {
			t.Fatalf("script %q should error on empty input", src)
		}
	}
	if _, err := EvalWithHosts(`return emb.math.softmax({"a"})`, nil, nil, Hosts{}, EvalOptions{}); err == nil {
		t.Fatal("expected non-numeric error")
	}
}

func sigmoidOf(x float64) float64 { return 1 / (1 + math.Exp(-x)) }

func luaNumF(v lua.LValue) float64 {
	if n, ok := v.(lua.LNumber); ok {
		return float64(n)
	}
	panic("expected LNumber, got " + v.Type().String())
}

func luaNumList(t *testing.T, v lua.LValue) []float64 {
	t.Helper()
	tbl, ok := v.(*lua.LTable)
	if !ok {
		t.Fatalf("expected table, got %T", v)
	}
	out := make([]float64, 0, tbl.Len())
	for i := 1; i <= tbl.Len(); i++ {
		n, ok := tbl.RawGetInt(i).(lua.LNumber)
		if !ok {
			t.Fatalf("element %d not a number", i)
		}
		out = append(out, float64(n))
	}
	return out
}
