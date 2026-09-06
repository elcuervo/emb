package script

import (
	"math"

	lua "github.com/yuin/gopher-lua"
)

// registerMath installs the emb.math baseline: the common post-processing
// primitives for classification / QA / reranker / span models, so scripts
// share one implementation instead of redefining five functions each.
// sigmoid is vectorized (array in → array out) so hot per-candidate loops
// cross the Go/Lua boundary once per array, not per element.
func registerMath(emb *lua.LTable, ls *lua.LState) {
	m := ls.NewTable()
	m.RawSetString("sigmoid", ls.NewFunction(mathSigmoid))
	m.RawSetString("softmax", ls.NewFunction(mathSoftmax))
	m.RawSetString("argmax", ls.NewFunction(mathArgmax))
	emb.RawSetString("math", m)
}

// mathSigmoid implements emb.math.sigmoid(x): a number in, a number out; an
// array in, the element-wise sigmoid array out. Empty arrays error.
func mathSigmoid(ls *lua.LState) int {
	v := ls.Get(1)
	switch t := v.(type) {
	case lua.LNumber:
		ls.Push(lua.LNumber(sigmoid(float64(t))))
		return 1
	case *lua.LTable:
		vals, err := numberArrayFromLua(t)
		if err != nil {
			ls.RaiseError("emb.math.sigmoid: %v", err)
			return 0
		}
		if len(vals) == 0 {
			ls.RaiseError("emb.math.sigmoid: empty array")
			return 0
		}
		out := ls.NewTable()
		for i, x := range vals {
			out.RawSetInt(i+1, lua.LNumber(sigmoid(x)))
		}
		ls.Push(out)
		return 1
	default:
		ls.RaiseError("emb.math.sigmoid: expected number or array, got %s", v.Type())
		return 0
	}
}

// mathSoftmax implements emb.math.softmax(vals): a numerically stable
// softmax (subtract the max before exponentiating) over the input array.
func mathSoftmax(ls *lua.LState) int {
	vals, err := numberArrayFromLua(ls.CheckTable(1))
	if err != nil {
		ls.RaiseError("emb.math.softmax: %v", err)
		return 0
	}
	if len(vals) == 0 {
		ls.RaiseError("emb.math.softmax: empty array")
		return 0
	}
	max := vals[0]
	for _, x := range vals[1:] {
		if x > max {
			max = x
		}
	}
	sum := 0.0
	exps := make([]float64, len(vals))
	for i, x := range vals {
		exps[i] = math.Exp(x - max)
		sum += exps[i]
	}
	out := ls.NewTable()
	for i, e := range exps {
		out.RawSetInt(i+1, lua.LNumber(e/sum))
	}
	ls.Push(out)
	return 1
}

// mathArgmax implements emb.math.argmax(vals): the 1-based index and value
// of the first maximum element (multi-return: index, value).
func mathArgmax(ls *lua.LState) int {
	vals, err := numberArrayFromLua(ls.CheckTable(1))
	if err != nil {
		ls.RaiseError("emb.math.argmax: %v", err)
		return 0
	}
	if len(vals) == 0 {
		ls.RaiseError("emb.math.argmax: empty array")
		return 0
	}
	best, bestVal := 1, vals[0]
	for i, x := range vals[1:] {
		if x > bestVal {
			best, bestVal = i+2, x
		}
	}
	ls.Push(lua.LNumber(best))
	ls.Push(lua.LNumber(bestVal))
	return 2
}

func sigmoid(x float64) float64 {
	return 1 / (1 + math.Exp(-x))
}
