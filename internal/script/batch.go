package script

import (
	"fmt"
	"slices"
	"sort"

	lua "github.com/yuin/gopher-lua"

	"github.com/elcuervo/emb/internal/onnx"
)

// runBatchHost implements emb.run_batch(items): an array of input-spec tables
// (each exactly like emb.run's single argument: {name = {shape, data}}).
// Items must declare the same tensor names, ranks, and dtypes; each named
// tensor is padded to the per-dimension maximum (zero-filled), concatenated
// along the batch axis, and the session is invoked ONCE. The result is an
// array of per-item output maps (same shape as emb.run's result for one item).
func runBatchHost(ls *lua.LState, h Hosts) int {
	if h.Run == nil {
		ls.RaiseError("emb.run_batch is unavailable for this model")
		return 0
	}
	itemsTab, ok := ls.Get(1).(*lua.LTable)
	if !ok {
		ls.RaiseError("emb.run_batch: expected an array of input specs, got %s", ls.Get(1).Type())
		return 0
	}
	if itemsTab.Len() == 0 {
		ls.RaiseError("emb.run_batch: empty item array")
		return 0
	}

	// Parse each item into a named-tensor set (same production rules as emb.run),
	// charging one request-wide tensor budget across all items and the merge.
	items := make([][]onnx.NamedTensor, 0, itemsTab.Len())
	budget := requestBudget(ls)
	for i := 1; i <= itemsTab.Len(); i++ {
		spec, ok := itemsTab.RawGetInt(i).(*lua.LTable)
		if !ok {
			ls.RaiseError("emb.run_batch: item %d must be a table of named inputs", i)
			return 0
		}
		inputs, err := namedInputsFromTable(ls, spec, budget)
		if err != nil {
			ls.RaiseError("emb.run_batch: item %d: %v", i, err)
			return 0
		}
		items = append(items, inputs)
	}

	// Uniformity, keyed per input NAME (inputs within an item may legitimately
	// have different ranks, e.g. GLiNER's 2-D ids + 1-D text_lengths): every
	// item must declare the same names, and each name must keep rank+dtype
	// across items.
	names := make([]string, 0, len(items[0]))
	type wantShape struct {
		rank  int
		dtype onnx.TensorType
	}
	want := make(map[string]wantShape, len(items[0]))
	for _, in := range items[0] {
		names = append(names, in.Name)
		want[in.Name] = wantShape{rank: len(in.Shape), dtype: in.DType}
	}
	sort.Strings(names)
	for _, ins := range items {
		got := make([]string, 0, len(ins))
		for _, in := range ins {
			got = append(got, in.Name)
		}
		sort.Strings(got)
		if !slices.Equal(got, names) {
			ls.RaiseError("emb.run_batch: items must declare the same input names (have %v, want %v)", got, names)
			return 0
		}
		for _, in := range ins {
			w := want[in.Name]
			if len(in.Shape) != w.rank {
				ls.RaiseError("emb.run_batch: input %q rank differs across items (have %d, want %d)", in.Name, len(in.Shape), w.rank)
				return 0
			}
			if in.DType != w.dtype {
				ls.RaiseError("emb.run_batch: input %q dtype differs across items (have %v, want %v)", in.Name, in.DType, w.dtype)
				return 0
			}
		}
	}

	merged, err := mergeBatch(items, names, budget)
	if err != nil {
		ls.RaiseError("emb.run_batch: %v", err)
		return 0
	}

	outputs, err := h.Run(merged)
	if err != nil {
		ls.RaiseError("emb.run_batch: %v", err)
		return 0
	}

	// Split outputs along the batch axis.
	n := len(items)
	result := ls.NewTable()
	outNames := make([]string, 0, len(outputs))
	for name := range outputs {
		outNames = append(outNames, name)
	}
	sort.Strings(outNames)
	for i := 0; i < n; i++ {
		itemOut := ls.NewTable()
		for _, name := range outNames {
			itemOut.RawSetString(name, sliceBatchOutput(ls, outputs[name], i, n))
		}
		result.RawSetInt(i+1, itemOut)
	}
	ls.Push(result)
	return 1
}

// namedInputsFromTable parses one emb.run-style input table into ordered
// named tensors (the same rules as runHost's argument parsing), charging the
// evaluation's tensor budget for every allocation.
func namedInputsFromTable(ls *lua.LState, arg *lua.LTable, budget *tensorBudget) ([]onnx.NamedTensor, error) {
	var names []string
	arg.ForEach(func(k, _ lua.LValue) {
		if s, ok := k.(lua.LString); ok {
			names = append(names, string(s))
		}
	})
	sort.Strings(names)
	inputs := make([]onnx.NamedTensor, 0, len(names))
	for _, name := range names {
		spec, ok := arg.RawGetString(name).(*lua.LTable)
		if !ok {
			return nil, fmt.Errorf("input %q must be a table {shape=..., data=...|fill=...}", name)
		}
		t, err := namedTensorFromLua(spec, budget)
		if err != nil {
			return nil, fmt.Errorf("input %q: %w", name, err)
		}
		t.Name = name
		inputs = append(inputs, t)
	}
	return inputs, nil
}

// mergeBatch merges per-item named tensor sets into one padded set along the
// batch axis: each named tensor keeps its input order, shapes become
// [N] + max-per-dim, data zero-filled with each item's data placed in row.
// Each item tensor must be a single batch row (Shape[0] == 1); when an item's
// inner dimensions differ from the padded maximums, its data is scattered per
// row so padding lands in the correct cells (a naive contiguous copy would
// misplace it or overwrite the next item's row).
func mergeBatch(items [][]onnx.NamedTensor, names []string, budget *tensorBudget) ([]onnx.NamedTensor, error) {
	n := len(items)
	merged := make([]onnx.NamedTensor, 0, len(names))
	for _, name := range names {
		// First item defines dtype/rank (uniformity enforced by caller).
		base := items[0][0]
		for _, ins := range items {
			for _, in := range ins {
				if in.Name == name {
					base = in
				}
			}
		}
		// The merged tensor replaces the items' batch dim (0) with N and pads
		// the remaining dims (1..rank-1) to the per-dimension maximum.
		rank := rankOf(base.Shape)
		if rank == 0 {
			return nil, fmt.Errorf("input %q has an empty shape", name)
		}
		maxShape := make([]int64, rank)
		maxShape[0] = int64(n)
		for d := 1; d < rank; d++ {
			for _, ins := range items {
				for _, in := range ins {
					if in.Name != name {
						continue
					}
					if in.Shape[d] > maxShape[d] {
						maxShape[d] = in.Shape[d]
					}
				}
			}
		}

		inner := innerInt(maxShape[1:])
		// Charge the padded merged allocation against the request budget too:
		// n*inner can exceed the sum of the items' tensor sizes (padding).
		if err := budget.charge(int64(n) * int64(inner)); err != nil {
			return nil, fmt.Errorf("merged input %q: %w", name, err)
		}
		var out onnx.NamedTensor
		out.Name = name
		out.Shape = maxShape
		out.DType = base.DType
		if base.DType == onnx.TensorInt64 {
			out.Int64 = make([]int64, n*inner)
		} else {
			out.Float = make([]float32, n*inner)
		}
		for i, ins := range items {
			var in onnx.NamedTensor
			for _, cand := range ins {
				if cand.Name == name {
					in = cand
				}
			}
			if in.Shape[0] != 1 {
				return nil, fmt.Errorf("input %q item %d: batch dimension must be 1, got %d", name, i+1, in.Shape[0])
			}
			row := i * inner
			if slices.Equal(in.Shape[1:], maxShape[1:]) {
				// Exact inner layout: a single contiguous copy fills the row.
				if in.DType == onnx.TensorInt64 {
					copy(out.Int64[row:], in.Int64)
				} else {
					copy(out.Float[row:], in.Float)
				}
			} else {
				scatterRow(out, row, in, maxShape[1:])
			}
		}
		merged = append(merged, out)
	}
	return merged, nil
}

// scatterRow copies in's inner row-major data into dst's padded row layout
// (maxInner is the per-dimension maximum). in's inner dims are <= maxInner; a
// cell at item-index (i1, i2, …) maps to merged offset (i1*maxStride1 + …).
func scatterRow(out onnx.NamedTensor, dstStart int, in onnx.NamedTensor, maxInner []int64) {
	n := len(in.Shape) - 1 // inner dims
	if n == 0 {
		if in.DType == onnx.TensorInt64 {
			out.Int64[dstStart] = in.Int64[0]
		} else {
			out.Float[dstStart] = in.Float[0]
		}
		return
	}
	maxStrides := make([]int, n)
	srcStrides := make([]int, n)
	stride := 1
	for k := n - 1; k >= 0; k-- {
		maxStrides[k] = stride
		stride *= int(maxInner[k])
	}
	stride = 1
	for k := n - 1; k >= 0; k-- {
		srcStrides[k] = stride
		stride *= int(in.Shape[k+1])
	}
	cur := make([]int, n)
	for {
		src, dst := 0, 0
		for k := range cur {
			src += cur[k] * srcStrides[k]
			dst += cur[k] * maxStrides[k]
		}
		if in.DType == onnx.TensorInt64 {
			out.Int64[dstStart+dst] = in.Int64[src]
		} else {
			out.Float[dstStart+dst] = in.Float[src]
		}
		k := n - 1
		for k >= 0 {
			cur[k]++
			if cur[k] < int(in.Shape[k+1]) {
				break
			}
			cur[k] = 0
			k--
		}
		if k < 0 {
			break
		}
	}
}

// sliceBatchOutput extracts item i's data (0-based) from a batched output.
// It validates that the output carries the expected batch dimension and data
// length before slicing, so a graph output without a leading dim of size n
// (e.g. a pooled or scalar result) raises a Lua error instead of panicking.
func sliceBatchOutput(ls *lua.LState, t onnx.NamedTensor, i, n int) *lua.LTable {
	out := ls.NewTable()
	shape := t.Shape
	if len(shape) == 0 {
		ls.RaiseError("emb.run_batch: output has an empty shape (no batch dimension)")
		return out
	}
	if shape[0] != int64(n) {
		ls.RaiseError("emb.run_batch: output shape %v has batch dimension %d, want %d", shape, shape[0], n)
		return out
	}
	inner := 1
	for _, d := range shape[1:] {
		inner *= int(d)
	}
	if (t.DType == onnx.TensorInt64 && len(t.Int64) < n*inner) || (t.DType != onnx.TensorInt64 && len(t.Float) < n*inner) {
		ls.RaiseError("emb.run_batch: output %q data has %d elements, want at least %d", t.Name, len(t.Int64)+len(t.Float), n*inner)
		return out
	}
	batchShape := append([]int64{1}, shape[1:]...)
	shapeTab := ls.NewTable()
	for k, d := range batchShape {
		shapeTab.RawSetInt(k+1, lua.LNumber(d))
	}
	out.RawSetString("shape", shapeTab)
	data := ls.NewTable()
	start := i * inner
	switch t.DType {
	case onnx.TensorInt64:
		for j := 0; j < inner; j++ {
			data.RawSetInt(j+1, lua.LNumber(t.Int64[start+j]))
		}
	default:
		for j := 0; j < inner; j++ {
			data.RawSetInt(j+1, lua.LNumber(t.Float[start+j]))
		}
	}
	out.RawSetString("data", data)
	return out
}

func innerInt(dims []int64) int {
	n := 1
	for _, d := range dims {
		n *= int(d)
	}
	return n
}

func rankOf(shape []int64) int { return len(shape) }
