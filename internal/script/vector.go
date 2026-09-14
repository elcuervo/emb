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

// operands decodes the two vector operands through the shared decoder and
// enforces equal lengths, naming the function in any error. policy selects the
// empty-operand behavior: similarity/distance require a vector, while
// emb.math.dot/l2 accept the empty operand.
func operands(ls *lua.LState, fn string, policy emptyOperandPolicy) ([]float64, []float64, error) {
	a, err := mathOperand(ls.Get(1), "first operand", policy)
	if err != nil {
		return nil, nil, fmt.Errorf("%s: %v", fn, err)
	}
	b, err := mathOperand(ls.Get(2), "second operand", policy)
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
	a, b, err := operands(ls, "emb.similarity", emptyRejected)
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
	a, b, err := operands(ls, "emb.distance", emptyRejected)
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
func dotProduct(a, b []float64) float64 {
	var sum float64
	for i := range a {
		sum += a[i] * b[i]
	}
	return sum
}

// cosineSimilarity returns dot/(|a||b|), or 0 when either operand has zero
// magnitude (matching the convention used by the shipped examples).
func cosineSimilarity(a, b []float64) float64 {
	var dot, na, nb float64
	for i := range a {
		x, y := a[i], b[i]
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

func squaredL2(a, b []float64) float64 {
	var sum float64
	for i := range a {
		d := a[i] - b[i]
		sum += d * d
	}
	return sum
}
