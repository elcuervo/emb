package script

import (
	"encoding/binary"
	"fmt"
	"math"
	"sort"
	"strings"

	lua "github.com/yuin/gopher-lua"

	"github.com/elcuervo/emb/internal/onnx"
)

// maxOutputBytes bounds one materialized output tensor in bytes. Packed and
// array forms both materialize the whole tensor, so a single output may not
// exceed this regardless of the element budget (which bounds the total across
// an evaluation).
const maxOutputBytes = 256 << 20

// runOptions is the optional trailing argument of emb.run / emb.run_batch:
//
//	{ bytes = true }              packed little-endian outputs
//	{ outputs = {"a", "b"} }      materialize only these graph outputs
//
// Both may be combined. Absent fields preserve the historical behaviour
// (per-element `data` arrays for every output).
type runOptions struct {
	packed     bool
	outputs    []string
	hasOutputs bool
}

func parseRunOptions(ls *lua.LState, idx int) (runOptions, error) {
	var opts runOptions
	v := ls.Get(idx)
	if v == lua.LNil {
		return opts, nil
	}
	t, ok := v.(*lua.LTable)
	if !ok {
		return opts, fmt.Errorf("options must be a table {bytes = ..., outputs = {...}}")
	}
	if b, ok := t.RawGetString("bytes").(lua.LBool); ok {
		opts.packed = bool(b)
	}
	if o := t.RawGetString("outputs"); o != lua.LNil {
		ot, ok := o.(*lua.LTable)
		if !ok {
			return opts, fmt.Errorf("outputs must be an array of output names")
		}
		if ot.Len() == 0 {
			return opts, fmt.Errorf("outputs must name at least one output")
		}
		opts.hasOutputs = true
		for i := 1; i <= ot.Len(); i++ {
			s, ok := ot.RawGetInt(i).(lua.LString)
			if !ok {
				return opts, fmt.Errorf("outputs item %d is not a string", i)
			}
			opts.outputs = append(opts.outputs, string(s))
		}
	}
	return opts, nil
}

// selectOutputNames returns the outputs to materialize, validating requested
// names against what the graph produced.
func selectOutputNames(outputs map[string]onnx.NamedTensor, opts runOptions) ([]string, error) {
	if opts.hasOutputs {
		for _, n := range opts.outputs {
			if _, ok := outputs[n]; !ok {
				available := make([]string, 0, len(outputs))
				for k := range outputs {
					available = append(available, k)
				}
				sort.Strings(available)
				return nil, fmt.Errorf("unknown output %q (available: %s)", n, strings.Join(available, ", "))
			}
		}
		return opts.outputs, nil
	}
	names := make([]string, 0, len(outputs))
	for n := range outputs {
		names = append(names, n)
	}
	sort.Strings(names)
	return names, nil
}

// dtypeWidth returns the byte width of a tensor dtype: 4 for float32, 8 for
// int64. It is the single source of the 4/8 constants used by the packed
// bytes decoder, the renderer, packTensor, and chargeOutputs.
func dtypeWidth(dt onnx.TensorType) int {
	if dt == onnx.TensorInt64 {
		return 8
	}
	return 4
}

// chargeOutputs charges every selected output against the evaluation's tensor
// budget (packed and array forms alike) and enforces the per-tensor byte cap.
// Outputs used to be unmetered; metering both forms keeps the packed path from
// becoming a way around the budget.
func chargeOutputs(ls *lua.LState, outputs map[string]onnx.NamedTensor, names []string) error {
	budget := requestBudget(ls)
	for _, n := range names {
		t := outputs[n]
		count, err := shapeElementCount(t.Shape)
		if err != nil {
			return fmt.Errorf("output %q: %w", n, err)
		}
		width := int64(dtypeWidth(t.DType))
		if count > int64(maxOutputBytes)/width {
			return fmt.Errorf("output %q is over the %d-byte output cap (%d elements × %d bytes)", n, maxOutputBytes, count, width)
		}
		if err := budget.charge(count); err != nil {
			return fmt.Errorf("output %q: %w", n, err)
		}
	}
	return nil
}

// dtypeName is the wire name of a tensor dtype in a packed spec.
func dtypeName(dt onnx.TensorType) string {
	if dt == onnx.TensorInt64 {
		return "i64"
	}
	return "f32"
}

// packTensor serializes a tensor as little-endian raw elements in a single
// allocation: the packed form emb.run accepts on input and
// emb.math.float32_bytes produces. No per-element Lua table is built.
func packTensor(t onnx.NamedTensor) []byte {
	width := dtypeWidth(t.DType)
	if t.DType == onnx.TensorInt64 {
		buf := make([]byte, width*len(t.Int64))
		for i, v := range t.Int64 {
			binary.LittleEndian.PutUint64(buf[i*8:], uint64(v))
		}
		return buf
	}
	buf := make([]byte, width*len(t.Float))
	for i, v := range t.Float {
		binary.LittleEndian.PutUint32(buf[i*4:], math.Float32bits(v))
	}
	return buf
}

// renderTensor is the single output-table builder for every returned tensor:
//
//	{shape = {...}, data = {...}, dtype = "f32"|"i64"}   default (array) form
//	{shape = {...}, bytes = <string>, dtype = "f32"|"i64"} packed form
//
// Both forms carry dtype, so every documented output is already a valid
// emb.run input spec: no dtype is re-inferred from element values (an
// all-integral float32 output would otherwise infer i64 and mismatch the
// session). emb.run, emb.run_batch slices, and the packed form all render
// through here, so the two forms cannot drift.
func renderTensor(ls *lua.LState, t onnx.NamedTensor, packed bool) *lua.LTable {
	out := ls.NewTable()
	out.RawSetString("shape", shapeTable(ls, t.Shape))
	out.RawSetString("dtype", lua.LString(dtypeName(t.DType)))
	if packed {
		out.RawSetString("bytes", lua.LString(packTensor(t)))
		return out
	}
	n := len(t.Float)
	if t.DType == onnx.TensorInt64 {
		n = len(t.Int64)
	}
	data := ls.CreateTable(0, n)
	switch t.DType {
	case onnx.TensorInt64:
		for i, v := range t.Int64 {
			data.RawSetInt(i+1, lua.LNumber(v))
		}
	default:
		for i, v := range t.Float {
			data.RawSetInt(i+1, lua.LNumber(v))
		}
	}
	out.RawSetString("data", data)
	return out
}

func shapeTable(ls *lua.LState, shape []int64) *lua.LTable {
	t := ls.NewTable()
	for i, d := range shape {
		t.RawSetInt(i+1, lua.LNumber(d))
	}
	return t
}
