package script

import (
	"fmt"
	"math"
	"sort"

	lua "github.com/yuin/gopher-lua"
)

// mathOperand decodes a math operand: either an array of Lua numbers or a
// packed little-endian float32 string (what emb.run {bytes=true} and
// emb.math.float32_bytes produce). It accepts an empty operand; individual
// operations enforce their own non-empty requirements.
func mathOperand(v lua.LValue) ([]float64, error) {
	switch t := v.(type) {
	case *lua.LTable:
		vals, err := numberArrayFromLua(t)
		if err != nil {
			return nil, err
		}
		return vals, nil
	case lua.LString:
		fs, err := unpackFloat32(string(t))
		if err != nil {
			return nil, err
		}
		out := make([]float64, len(fs))
		for i, f := range fs {
			out[i] = float64(f)
		}
		return out, nil
	default:
		return nil, fmt.Errorf("expected an array of numbers or a packed float32 string, got %s", v.Type())
	}
}

// mathPairwise implements emb.math.dot / cosine / l2 over two operands.
func mathPairwise(ls *lua.LState, op string) int {
	a, b, err := operands(ls, "emb.math."+op)
	if err != nil {
		ls.RaiseError("%v", err)
		return 0
	}
	var out float64
	switch op {
	case "dot":
		out = dotProduct(a, b)
	case "cosine":
		out = cosineSimilarity(a, b)
	case "l2":
		out = math.Sqrt(squaredL2(a, b))
	}
	ls.Push(lua.LNumber(out))
	return 1
}

// mathNorm implements emb.math.norm(a): the L2 norm.
func mathNorm(ls *lua.LState) int {
	a, err := vectorOperand(ls.Get(1), "operand")
	if err != nil {
		ls.RaiseError("emb.math.norm: %v", err)
		return 0
	}
	var sum float64
	for _, x := range a {
		sum += float64(x) * float64(x)
	}
	ls.Push(lua.LNumber(math.Sqrt(sum)))
	return 1
}

// shapeOf validates and returns a shape table as []int, erroring on a negative
// or non-integer dimension.
func shapeOf(ls *lua.LState, v lua.LValue, fn string) ([]int, error) {
	t, ok := v.(*lua.LTable)
	if !ok {
		return nil, fmt.Errorf("%s: shape must be an array", fn)
	}
	shape := make([]int, t.Len())
	for i := 1; i <= t.Len(); i++ {
		n, ok := t.RawGetInt(i).(lua.LNumber)
		if !ok || n < 0 || n != lua.LNumber(int(n)) {
			return nil, fmt.Errorf("%s: shape dimension %d is not a non-negative integer", fn, i)
		}
		shape[i-1] = int(n)
	}
	return shape, nil
}

func product(shape []int) int {
	n := 1
	for _, d := range shape {
		n *= d
	}
	return n
}

// mathShape3 validates a {batch, seq, dim} shape against an operand length. It
// requires every dimension to be positive: a zero dimension makes the element
// product zero, so a tiny operand could satisfy a plain count check while the
// loops below iterate an attacker-chosen number of times (CWE-400). The product
// is computed with overflow guards so a huge shape cannot wrap into a match.
func mathShape3(fn string, shape []int, n int) (batch, seq, dim int, err error) {
	if len(shape) != 3 {
		return 0, 0, 0, fmt.Errorf("%s: shape must be {batch, seq, dim}, got %v", fn, shape)
	}
	batch, seq, dim = shape[0], shape[1], shape[2]
	if batch < 1 || seq < 1 || dim < 1 {
		return 0, 0, 0, fmt.Errorf("%s: shape dimensions must be positive, got %v", fn, shape)
	}
	if batch > math.MaxInt/seq || batch*seq > math.MaxInt/dim {
		return 0, 0, 0, fmt.Errorf("%s: shape %v overflows the element count", fn, shape)
	}
	if elems := batch * seq * dim; elems != n {
		return 0, 0, 0, fmt.Errorf("%s: shape %v needs %d elements, got %d", fn, shape, elems, n)
	}
	return batch, seq, dim, nil
}

// floatTable renders a slice as a Lua array of numbers.
func floatTable(ls *lua.LState, vals []float64) *lua.LTable {
	t := ls.CreateTable(0, len(vals))
	for i, v := range vals {
		t.RawSetInt(i+1, lua.LNumber(v))
	}
	return t
}

// l2NormalizeInPlace normalizes a vector to unit length (no-op on a zero
// vector, which stays all zeros rather than producing NaNs).
func l2NormalizeInPlace(v []float64) {
	var norm float64
	for _, x := range v {
		norm += x * x
	}
	norm = math.Sqrt(norm)
	if norm == 0 {
		return
	}
	for i := range v {
		v[i] /= norm
	}
}

// mathMeanPool implements emb.math.mean_pool(hidden, shape, mask): masked mean
// over the sequence axis followed by L2 normalization, returning one vector
// per batch row. shape is {batch, seq, dim}; mask is a flat batch*seq array of
// 0/1 values.
func mathMeanPool(ls *lua.LState) int {
	const fn = "emb.math.mean_pool"
	hidden, err := mathOperand(ls.Get(1))
	if err != nil {
		ls.RaiseError("%s: %v", fn, err)
		return 0
	}
	shape, err := shapeOf(ls, ls.Get(2), fn)
	if err != nil {
		ls.RaiseError("%v", err)
		return 0
	}
	if len(shape) != 3 {
		ls.RaiseError("%s: shape must be {batch, seq, dim}, got %v", fn, shape)
		return 0
	}
	mask, err := mathOperand(ls.Get(3))
	if err != nil {
		ls.RaiseError("%s: mask: %v", fn, err)
		return 0
	}
	batch, seq, dim, err := mathShape3(fn, shape, len(hidden))
	if err != nil {
		ls.RaiseError("%v", err)
		return 0
	}
	if len(mask) != batch*seq {
		ls.RaiseError("%s: mask needs %d values (batch*seq), got %d", fn, batch*seq, len(mask))
		return 0
	}
	out := ls.NewTable()
	for b := 0; b < batch; b++ {
		vec := make([]float64, dim)
		n := 0
		for s := 0; s < seq; s++ {
			if mask[b*seq+s] == 0 {
				continue
			}
			n++
			base := (b*seq + s) * dim
			for d := 0; d < dim; d++ {
				vec[d] += hidden[base+d]
			}
		}
		if n == 0 {
			ls.RaiseError("%s: row %d has an all-zero mask", fn, b+1)
			return 0
		}
		for d := range vec {
			vec[d] /= float64(n)
		}
		l2NormalizeInPlace(vec)
		out.RawSetInt(b+1, floatTable(ls, vec))
	}
	ls.Push(out)
	return 1
}

// mathCLS implements emb.math.cls(hidden, shape): the first sequence position
// of each batch row, L2 normalized.
func mathCLS(ls *lua.LState) int {
	const fn = "emb.math.cls"
	hidden, err := mathOperand(ls.Get(1))
	if err != nil {
		ls.RaiseError("%s: %v", fn, err)
		return 0
	}
	shape, err := shapeOf(ls, ls.Get(2), fn)
	if err != nil {
		ls.RaiseError("%v", err)
		return 0
	}
	if len(shape) != 3 || shape[1] < 1 {
		ls.RaiseError("%s: shape must be {batch, seq>=1, dim}, got %v", fn, shape)
		return 0
	}
	batch, seq, dim, err := mathShape3(fn, shape, len(hidden))
	if err != nil {
		ls.RaiseError("%v", err)
		return 0
	}
	out := ls.NewTable()
	for b := 0; b < batch; b++ {
		vec := make([]float64, dim)
		copy(vec, hidden[b*seq*dim:b*seq*dim+dim])
		l2NormalizeInPlace(vec)
		out.RawSetInt(b+1, floatTable(ls, vec))
	}
	ls.Push(out)
	return 1
}

// mathTopk implements emb.math.topk(values, k): the k largest values as
// {index=..., value=...} tables in descending value order, 1-based indices.
func mathTopk(ls *lua.LState) int {
	const fn = "emb.math.topk"
	vals, err := mathOperand(ls.Get(1))
	if err != nil {
		ls.RaiseError("%s: %v", fn, err)
		return 0
	}
	k := int(ls.CheckNumber(2))
	if k < 1 {
		ls.RaiseError("%s: k must be >= 1, got %d", fn, k)
		return 0
	}
	if k > len(vals) {
		k = len(vals)
	}
	idx := make([]int, len(vals))
	for i := range idx {
		idx[i] = i
	}
	// Stable descending sort by value; ties keep the lower index first.
	sort.SliceStable(idx, func(a, b int) bool { return vals[idx[a]] > vals[idx[b]] })
	out := ls.NewTable()
	for i := 0; i < k; i++ {
		entry := ls.NewTable()
		entry.RawSetString("index", lua.LNumber(idx[i]+1))
		entry.RawSetString("value", lua.LNumber(vals[idx[i]]))
		out.RawSetInt(i+1, entry)
	}
	ls.Push(out)
	return 1
}

// mathGather implements emb.math.gather(values, indices): values[indices[i]],
// with 1-based indices.
func mathGather(ls *lua.LState) int {
	const fn = "emb.math.gather"
	vals, err := mathOperand(ls.Get(1))
	if err != nil {
		ls.RaiseError("%s: %v", fn, err)
		return 0
	}
	idx, err := mathOperand(ls.Get(2))
	if err != nil {
		ls.RaiseError("%s: indices: %v", fn, err)
		return 0
	}
	out := make([]float64, len(idx))
	for i, x := range idx {
		j := int(x) - 1
		if j < 0 || j >= len(vals) {
			ls.RaiseError("%s: index %d out of range (len %d)", fn, int(x), len(vals))
			return 0
		}
		out[i] = vals[j]
	}
	ls.Push(floatTable(ls, out))
	return 1
}

// mathSlice implements emb.math.slice(tensor, shape, offset, length): `length`
// elements starting at the 1-based element `offset` of the flattened tensor.
// shape is validated against the operand's element count.
func mathSlice(ls *lua.LState) int {
	const fn = "emb.math.slice"
	vals, err := mathOperand(ls.Get(1))
	if err != nil {
		ls.RaiseError("%s: %v", fn, err)
		return 0
	}
	shape, err := shapeOf(ls, ls.Get(2), fn)
	if err != nil {
		ls.RaiseError("%v", err)
		return 0
	}
	if len(vals) != product(shape) {
		ls.RaiseError("%s: shape %v needs %d elements, got %d", fn, shape, product(shape), len(vals))
		return 0
	}
	offset := int(ls.CheckNumber(3))
	length := int(ls.CheckNumber(4))
	// Validate by subtraction: offset-1+length can overflow for large positive
	// inputs and wrap negative, letting an attacker-selected allocation through.
	start := offset - 1
	if offset < 1 || length < 0 || start > len(vals) || length > len(vals)-start {
		ls.RaiseError("%s: offset %d length %d out of range (len %d)", fn, offset, length, len(vals))
		return 0
	}
	out := make([]float64, length)
	copy(out, vals[start:start+length])
	ls.Push(floatTable(ls, out))
	return 1
}

// mathScale implements emb.math.scale(values, factor): element-wise product.
func mathScale(ls *lua.LState) int {
	vals, err := mathOperand(ls.Get(1))
	if err != nil {
		ls.RaiseError("emb.math.scale: %v", err)
		return 0
	}
	factor := float64(ls.CheckNumber(2))
	out := make([]float64, len(vals))
	for i, v := range vals {
		out[i] = v * factor
	}
	ls.Push(floatTable(ls, out))
	return 1
}

// mathAdd implements emb.math.add(a, b): element-wise sum.
func mathAdd(ls *lua.LState) int {
	a, err := mathOperand(ls.Get(1))
	if err != nil {
		ls.RaiseError("emb.math.add: %v", err)
		return 0
	}
	b, err := mathOperand(ls.Get(2))
	if err != nil {
		ls.RaiseError("emb.math.add: %v", err)
		return 0
	}
	if len(a) != len(b) {
		ls.RaiseError("emb.math.add: length mismatch: %d vs %d", len(a), len(b))
		return 0
	}
	out := make([]float64, len(a))
	for i := range a {
		out[i] = a[i] + b[i]
	}
	ls.Push(floatTable(ls, out))
	return 1
}
