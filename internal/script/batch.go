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

	// Parse each item into a named-tensor set (same production rules as emb.run).
	items := make([][]onnx.NamedTensor, 0, itemsTab.Len())
	for i := 1; i <= itemsTab.Len(); i++ {
		spec, ok := itemsTab.RawGetInt(i).(*lua.LTable)
		if !ok {
			ls.RaiseError("emb.run_batch: item %d must be a table of named inputs", i)
			return 0
		}
		inputs, err := namedInputsFromTable(ls, spec)
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

	merged, err := mergeBatch(items, names)
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
// named tensors (the same rules as runHost's argument parsing).
func namedInputsFromTable(ls *lua.LState, arg *lua.LTable) ([]onnx.NamedTensor, error) {
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
		t, err := namedTensorFromLua(spec)
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
func mergeBatch(items [][]onnx.NamedTensor, names []string) ([]onnx.NamedTensor, error) {
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
			row := i * inner
			if in.DType == onnx.TensorInt64 {
				copy(out.Int64[row:], in.Int64)
			} else {
				copy(out.Float[row:], in.Float)
			}
		}
		merged = append(merged, out)
	}
	return merged, nil
}

// sliceBatchOutput extracts item i's data (0-based) from a batched output.
func sliceBatchOutput(ls *lua.LState, t onnx.NamedTensor, i, n int) *lua.LTable {
	out := ls.NewTable()
	shape := t.Shape
	inner := 1
	for _, d := range shape[1:] {
		inner *= int(d)
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
