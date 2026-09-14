package script

import (
	"encoding/binary"
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
		"return emb.math.softmax({})",
		"return emb.math.argmax({})",
		"return emb.math.float32_bytes({})",
	} {
		if _, err := EvalWithHosts(src, nil, nil, Hosts{}, EvalOptions{}); err == nil {
			t.Fatalf("script %q should error on empty input", src)
		}
	}
	if _, err := EvalWithHosts(`return emb.math.softmax({"a"})`, nil, nil, Hosts{}, EvalOptions{}); err == nil {
		t.Fatal("expected non-numeric error")
	}
}

// TestMathScalarSoftmaxArgmax covers the degenerate scalar forms the
// script-eval spec requires: softmax(x) == 1 and argmax(x) == (1, x).
func TestMathScalarSoftmaxArgmax(t *testing.T) {
	if got := evalNum(t, `return emb.math.softmax(5)`); got != 1 {
		t.Fatalf("softmax(5) = %v, want 1", got)
	}
	v, err := EvalWithHosts(`local i, x = emb.math.argmax(5) return i .. "|" .. x`, nil, nil, Hosts{}, EvalOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if v.String() != "1|5" {
		t.Fatalf("argmax(5) = %q, want 1|5", v.String())
	}
}

// TestMathDefinedEmptyResults pins the one empty-operand rule (design decision
// 4): an empty operand is valid exactly where the operation has a defined empty
// result.
func TestMathDefinedEmptyResults(t *testing.T) {
	empty := func(src string) {
		t.Helper()
		v, err := EvalWithHosts(src, nil, nil, Hosts{}, EvalOptions{})
		if err != nil {
			t.Fatalf("%s: %v", src, err)
		}
		tbl, ok := v.(*lua.LTable)
		if !ok || tbl.Len() != 0 {
			t.Fatalf("%s = %v, want an empty array", src, v)
		}
	}
	// Element-wise maps and selections yield the empty array.
	empty(`return emb.math.sigmoid({})`)
	empty(`return emb.math.scale({}, 2)`)
	empty(`return emb.math.add({}, {})`)
	empty(`return emb.math.topk({}, 3)`)
	empty(`return emb.math.gather({1, 2, 3}, {})`)
	empty(`return emb.math.slice({1, 2, 3}, {3}, 1, 0)`)
	// Linear reductions yield 0.
	for _, src := range []string{
		`return emb.math.dot({}, {})`,
		`return emb.math.l2({}, {})`,
		`return emb.math.norm({})`,
	} {
		if got := evalNum(t, src); got != 0 {
			t.Fatalf("%s = %v, want 0", src, got)
		}
	}
	// Operations needing an element or a non-zero denominator error.
	for _, src := range []string{
		`return emb.math.cosine({}, {})`,
		`return emb.math.softmax({})`,
		`return emb.math.argmax({})`,
		`return emb.math.float32_bytes({})`,
	} {
		if _, err := EvalWithHosts(src, nil, nil, Hosts{}, EvalOptions{}); err == nil {
			t.Fatalf("%s should error on the empty operand", src)
		}
	}
}

func TestMathFloat32BytesRoundTrip(t *testing.T) {
	// 768 floats pack to a 3072-byte string that decodes back to the exact
	// float32 values (little-endian IEEE 754, the embed path's layout).
	src := `
local t = {}
for i = 1, 768 do t[i] = i * 0.5 end
return emb.math.float32_bytes(t)`
	v, err := EvalWithHosts(src, nil, nil, Hosts{}, EvalOptions{})
	if err != nil {
		t.Fatal(err)
	}
	s, ok := v.(lua.LString)
	if !ok {
		t.Fatalf("expected LString, got %T", v)
	}
	if len(s) != 4*768 {
		t.Fatalf("expected %d bytes, got %d", 4*768, len(s))
	}
	enc := []byte(s)
	for i := 0; i < 768; i++ {
		bits := binary.LittleEndian.Uint32(enc[i*4 : i*4+4])
		got := math.Float32frombits(bits)
		if want := float32(float64(i+1) * 0.5); got != want {
			t.Fatalf("element %d = %v, want %v", i, got, want)
		}
	}
}

func TestMathFloat32BytesTwoElements(t *testing.T) {
	// A 2-element pack is exactly 8 bytes with known bit patterns.
	v, err := EvalWithHosts(`return emb.math.float32_bytes({1.5, -2.25})`, nil, nil, Hosts{}, EvalOptions{})
	if err != nil {
		t.Fatal(err)
	}
	s, ok := v.(lua.LString)
	if !ok {
		t.Fatalf("expected LString, got %T", v)
	}
	if len(s) != 8 {
		t.Fatalf("expected 8 bytes, got %d", len(s))
	}
	enc := []byte(s)
	if got := binary.LittleEndian.Uint32(enc[0:4]); got != 0x3FC00000 { // 1.5
		t.Fatalf("first element bits = %08x, want 3fc00000", got)
	}
	if got := binary.LittleEndian.Uint32(enc[4:8]); got != 0xC0100000 { // -2.25
		t.Fatalf("second element bits = %08x, want c0100000", got)
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
