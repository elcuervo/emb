package script

import (
	"encoding/binary"
	"math"

	lua "github.com/yuin/gopher-lua"
)

// registerMath installs the emb.math baseline: the common post-processing
// primitives for classification / QA / reranker / span models, plus the raw
// float32 packer for byte-compatible embedding replies, so scripts share one
// implementation instead of redefining five functions each. sigmoid is
// vectorized (array in → array out) so hot per-candidate loops cross the
// Go/Lua boundary once per array, not per element.
func registerMath(emb *lua.LTable, ls *lua.LState) {
	m := ls.NewTable()
	m.RawSetString("sigmoid", ls.NewFunction(mathSigmoid))
	m.RawSetString("softmax", ls.NewFunction(mathSoftmax))
	m.RawSetString("argmax", ls.NewFunction(mathArgmax))
	m.RawSetString("float32_bytes", ls.NewFunction(mathFloat32Bytes))
	m.RawSetString("dot", ls.NewFunction(func(ls *lua.LState) int { return mathPairwise(ls, "dot") }))
	m.RawSetString("cosine", ls.NewFunction(func(ls *lua.LState) int { return mathPairwise(ls, "cosine") }))
	m.RawSetString("l2", ls.NewFunction(func(ls *lua.LState) int { return mathPairwise(ls, "l2") }))
	m.RawSetString("norm", ls.NewFunction(mathNorm))
	m.RawSetString("mean_pool", ls.NewFunction(mathMeanPool))
	m.RawSetString("cls", ls.NewFunction(mathCLS))
	m.RawSetString("topk", ls.NewFunction(mathTopk))
	m.RawSetString("gather", ls.NewFunction(mathGather))
	m.RawSetString("slice", ls.NewFunction(mathSlice))
	m.RawSetString("scale", ls.NewFunction(mathScale))
	m.RawSetString("add", ls.NewFunction(mathAdd))
	emb.RawSetString("math", m)
}

// mathSigmoid implements emb.math.sigmoid(x): a number in, a number out; an
// array in, the element-wise sigmoid array out. The empty array has the
// defined result {} (element-wise map); softmax and argmax need an element.
func mathSigmoid(ls *lua.LState) int {
	v := ls.Get(1)
	if n, ok := v.(lua.LNumber); ok {
		ls.Push(lua.LNumber(sigmoid(float64(n))))
		return 1
	}
	vals, err := mathOperand(v, "operand", emptyAllowed)
	if err != nil {
		ls.RaiseError("emb.math.sigmoid: %v", err)
		return 0
	}
	out := ls.NewTable()
	for i, x := range vals {
		out.RawSetInt(i+1, lua.LNumber(sigmoid(x)))
	}
	ls.Push(out)
	return 1
}

// mathSoftmax implements emb.math.softmax(vals): a numerically stable
// softmax (subtract the max before exponentiating) over the input array. A
// single number takes the degenerate scalar form (the result is 1); an empty
// array errors, since there is no element to normalize.
func mathSoftmax(ls *lua.LState) int {
	v := ls.Get(1)
	if _, ok := v.(lua.LNumber); ok {
		ls.Push(lua.LNumber(1))
		return 1
	}
	vals, err := mathOperand(v, "operand", emptyAllowed)
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
// of the first maximum element (multi-return: index, value). A single number
// takes the degenerate scalar form (1, x); an empty array errors.
func mathArgmax(ls *lua.LState) int {
	v := ls.Get(1)
	if n, ok := v.(lua.LNumber); ok {
		ls.Push(lua.LNumber(1))
		ls.Push(lua.LNumber(n))
		return 2
	}
	vals, err := mathOperand(v, "operand", emptyAllowed)
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

// mathFloat32Bytes implements emb.math.float32_bytes(vals): packs an array of
// Lua numbers into a Lua string of 4 little-endian IEEE 754 float32 bytes per
// element (the byte layout the embed path replies with, so clients decode via
// unpack('e*')). Returning the string yields a single bulk reply instead of
// one bulk per element. Empty arrays error.
func mathFloat32Bytes(ls *lua.LState) int {
	vals, err := numberArrayFromLua(ls.CheckTable(1))
	if err != nil {
		ls.RaiseError("emb.math.float32_bytes: %v", err)
		return 0
	}
	if len(vals) == 0 {
		ls.RaiseError("emb.math.float32_bytes: empty array")
		return 0
	}
	buf := make([]byte, 0, 4*len(vals))
	for _, x := range vals {
		buf = binary.LittleEndian.AppendUint32(buf, math.Float32bits(float32(x)))
	}
	ls.Push(lua.LString(buf))
	return 1
}
