package script

import (
	"fmt"
	"math"
	"strings"

	lua "github.com/yuin/gopher-lua"
)

// similarityMetrics are the emb.similarity metrics, all "higher is more
// similar".
var similarityMetrics = []string{"cosine", "dot"}

// distanceMetrics are the emb.distance metrics, all "lower is closer".
var distanceMetrics = []string{"l2", "l2sq", "cosine"}

// vectorOperand decodes one similarity/distance operand: either a Lua array of
// numbers or a packed little-endian float32 string (the form emb.embed with
// {bytes=true} and emb.math.float32_bytes produce).
func vectorOperand(v lua.LValue, name string) ([]float32, error) {
	switch t := v.(type) {
	case *lua.LTable:
		vals, err := numberArrayFromLua(t)
		if err != nil {
			return nil, fmt.Errorf("%s: %v", name, err)
		}
		if len(vals) == 0 {
			return nil, fmt.Errorf("%s: empty vector", name)
		}
		out := make([]float32, len(vals))
		for i, x := range vals {
			out[i] = float32(x)
		}
		return out, nil
	case lua.LString:
		out, err := unpackFloat32(string(t))
		if err != nil {
			return nil, fmt.Errorf("%s: %v", name, err)
		}
		if len(out) == 0 {
			return nil, fmt.Errorf("%s: empty vector", name)
		}
		return out, nil
	default:
		return nil, fmt.Errorf("%s: expected an array of numbers or a packed float32 string", name)
	}
}

// metricArg reads the optional metric argument (position 3), defaulting to def
// and validating against allowed.
func metricArg(ls *lua.LState, def string, allowed []string) (string, error) {
	v := ls.Get(3)
	if v == lua.LNil {
		return def, nil
	}
	s, ok := v.(lua.LString)
	if !ok {
		return "", fmt.Errorf("metric must be a string")
	}
	metric := string(s)
	for _, a := range allowed {
		if metric == a {
			return metric, nil
		}
	}
	return "", fmt.Errorf("unknown metric %q (want %s)", metric, strings.Join(allowed, ", "))
}

// operands decodes the two vector operands and enforces equal lengths, naming
// the function in any error.
func operands(ls *lua.LState, fn string) ([]float32, []float32, error) {
	a, err := vectorOperand(ls.Get(1), "first operand")
	if err != nil {
		return nil, nil, fmt.Errorf("%s: %v", fn, err)
	}
	b, err := vectorOperand(ls.Get(2), "second operand")
	if err != nil {
		return nil, nil, fmt.Errorf("%s: %v", fn, err)
	}
	if len(a) != len(b) {
		return nil, nil, fmt.Errorf("%s: length mismatch: %d vs %d", fn, len(a), len(b))
	}
	return a, b, nil
}

// similarityHost implements emb.similarity(a, b [, metric]): higher is more
// similar. Metrics: cosine (default), dot.
func similarityHost(ls *lua.LState) int {
	a, b, err := operands(ls, "emb.similarity")
	if err != nil {
		ls.RaiseError("%v", err)
		return 0
	}
	metric, err := metricArg(ls, "cosine", similarityMetrics)
	if err != nil {
		ls.RaiseError("emb.similarity: %v", err)
		return 0
	}
	var out float64
	switch metric {
	case "cosine":
		out = cosineSimilarity(a, b)
	case "dot":
		out = dotProduct(a, b)
	}
	ls.Push(lua.LNumber(out))
	return 1
}

// distanceHost implements emb.distance(a, b [, metric]): lower is closer.
// Metrics: l2 (default), l2sq, cosine (= 1 - cosine similarity).
func distanceHost(ls *lua.LState) int {
	a, b, err := operands(ls, "emb.distance")
	if err != nil {
		ls.RaiseError("%v", err)
		return 0
	}
	metric, err := metricArg(ls, "l2", distanceMetrics)
	if err != nil {
		ls.RaiseError("emb.distance: %v", err)
		return 0
	}
	var out float64
	switch metric {
	case "l2":
		out = math.Sqrt(squaredL2(a, b))
	case "l2sq":
		out = squaredL2(a, b)
	case "cosine":
		out = 1 - cosineSimilarity(a, b)
	}
	ls.Push(lua.LNumber(out))
	return 1
}

// dotProduct accumulates in float64 so long vectors do not lose precision.
func dotProduct(a, b []float32) float64 {
	var sum float64
	for i := range a {
		sum += float64(a[i]) * float64(b[i])
	}
	return sum
}

// cosineSimilarity returns dot/(|a||b|), or 0 when either operand has zero
// magnitude (matching the convention used by the shipped examples).
func cosineSimilarity(a, b []float32) float64 {
	var dot, na, nb float64
	for i := range a {
		x, y := float64(a[i]), float64(b[i])
		dot += x * y
		na += x * x
		nb += y * y
	}
	denom := math.Sqrt(na) * math.Sqrt(nb)
	if denom == 0 {
		return 0
	}
	return dot / denom
}

func squaredL2(a, b []float32) float64 {
	var sum float64
	for i := range a {
		d := float64(a[i]) - float64(b[i])
		sum += d * d
	}
	return sum
}
