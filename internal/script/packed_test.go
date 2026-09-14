package script

import (
	"encoding/binary"
	"math"
	"strings"
	"testing"

	"github.com/elcuervo/emb/internal/onnx"
)

// echoRun returns each input under "<name>_out", so a packed output can be fed
// straight back as a packed input and the round trip can be checked.
func echoRun(inputs []onnx.NamedTensor) (map[string]onnx.NamedTensor, error) {
	out := make(map[string]onnx.NamedTensor, len(inputs))
	for _, in := range inputs {
		in.Name += "_out"
		out[in.Name] = in
	}
	return out, nil
}

func multiOutputRun(name string, t onnx.NamedTensor) func([]onnx.NamedTensor) (map[string]onnx.NamedTensor, error) {
	return func([]onnx.NamedTensor) (map[string]onnx.NamedTensor, error) {
		return map[string]onnx.NamedTensor{name: t}, nil
	}
}

func evalInt(t *testing.T, src string, h Hosts) int {
	t.Helper()
	v, err := EvalWithHosts(src, nil, nil, h, EvalOptions{})
	if err != nil {
		t.Fatalf("%s: %v", src, err)
	}
	return int(luaNumF(v))
}

func TestPackedOutputRoundTrip(t *testing.T) {
	got := evalInt(t, `
local a = emb.run({x = {shape = {2, 3}, data = {1, 2, 3, 4, 5, 6}}}, {bytes = true})
local b = emb.run({x = a.x_out})
return b.x_out.data[1]`, Hosts{Run: echoRun})
	if got != 1 {
		t.Fatalf("packed round-trip first element = %d, want 1", got)
	}
}

func TestPackedOutputRoundTripPreservesAllElements(t *testing.T) {
	got := evalInt(t, `
local a = emb.run({x = {shape = {1, 4}, data = {1, 2, 3, 4}}}, {bytes = true})
local b = emb.run({x = a.x_out})
local s = 0
for i = 1, #b.x_out.data do s = s + b.x_out.data[i] end
return s`, Hosts{Run: echoRun})
	if got != 10 {
		t.Fatalf("packed round-trip sum = %d, want 10", got)
	}
}

func TestPackedByteLengthMatchesShape(t *testing.T) {
	// Fractional data infers float32; integral data infers int64.
	f32 := evalInt(t, `
local a = emb.run({x = {shape = {2, 3}, data = {1.5, 2.5, 3.5, 4.5, 5.5, 6.5}}}, {bytes = true})
return #a.x_out.bytes`, Hosts{Run: echoRun})
	if f32 != 24 {
		t.Fatalf("f32 packed length = %d, want 24", f32)
	}
	i64 := evalInt(t, `
local a = emb.run({x = {shape = {2}, data = {1, 2}, dtype = "i64"}}, {bytes = true})
return #a.x_out.bytes`, Hosts{Run: echoRun})
	if i64 != 16 {
		t.Fatalf("i64 packed length = %d, want 16", i64)
	}
}

func TestPackedOutputDtypeField(t *testing.T) {
	v, err := EvalWithHosts(`
local a = emb.run({x = {shape = {2}, data = {1, 2}, dtype = "i64"}}, {bytes = true})
return a.x_out.dtype`, nil, nil, Hosts{Run: echoRun}, EvalOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if v.String() != "i64" {
		t.Fatalf("packed dtype = %q, want i64", v.String())
	}
}

func TestPackedOutputExactBytes(t *testing.T) {
	v, err := EvalWithHosts(`
local a = emb.run({x = {shape = {2}, data = {1.5, 2}}}, {bytes = true})
return a.x_out.bytes`, nil, nil, Hosts{Run: echoRun}, EvalOptions{})
	if err != nil {
		t.Fatal(err)
	}
	raw := v.String()
	if len(raw) != 8 {
		t.Fatalf("packed bytes length = %d, want 8", len(raw))
	}
	for i, want := range []float32{1.5, 2} {
		got := math.Float32frombits(binary.LittleEndian.Uint32([]byte(raw)[i*4:]))
		if got != want {
			t.Fatalf("packed element %d = %v, want %v", i, got, want)
		}
	}
}

func TestDefaultOutputFormUnchanged(t *testing.T) {
	v, err := EvalWithHosts(`return emb.run({x = {shape = {1}, data = {7}}}).x_out.data[1]`,
		nil, nil, Hosts{Run: echoRun}, EvalOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if int(luaNumF(v)) != 7 {
		t.Fatalf("default output form changed: %v", luaNumF(v))
	}
}

func TestSelectiveOutputs(t *testing.T) {
	hosts := Hosts{Run: func([]onnx.NamedTensor) (map[string]onnx.NamedTensor, error) {
		return map[string]onnx.NamedTensor{
			"logits": {Name: "logits", Shape: []int64{1}, DType: onnx.TensorFloat32, Float: []float32{1}},
			"hidden": {Name: "hidden", Shape: []int64{4}, DType: onnx.TensorFloat32, Float: []float32{1, 2, 3, 4}},
		}, nil
	}}
	got := evalInt(t, `
local o = emb.run({x = {shape = {1}, data = {1}}}, {outputs = {"logits"}})
local n = 0
for _ in pairs(o) do n = n + 1 end
return n`, hosts)
	if got != 1 {
		t.Fatalf("selective outputs returned %d keys, want 1", got)
	}
}

func TestSelectiveOutputUnknownName(t *testing.T) {
	hosts := Hosts{Run: multiOutputRun("logits", onnx.NamedTensor{Name: "logits", Shape: []int64{1}, DType: onnx.TensorFloat32, Float: []float32{1}})}
	_, err := EvalWithHosts(`return emb.run({x = {shape = {1}, data = {1}}}, {outputs = {"nope"}})`, nil, nil, hosts, EvalOptions{})
	if err == nil {
		t.Fatal("expected an unknown-output error")
	}
	msg := err.Error()
	if !strings.Contains(msg, "unknown output") || !strings.Contains(msg, "logits") {
		t.Fatalf("unknown output error = %q", msg)
	}
}

func TestRunBatchPackedAndSelective(t *testing.T) {
	hosts := Hosts{Run: func(inputs []onnx.NamedTensor) (map[string]onnx.NamedTensor, error) {
		// Echo a [batch, 1] output plus an unused large output.
		var batch int64 = 1
		for _, in := range inputs {
			if in.Name == "ids" {
				batch = in.Shape[0]
			}
		}
		return map[string]onnx.NamedTensor{
			"logits": {Name: "logits", Shape: []int64{batch, 1}, DType: onnx.TensorFloat32, Float: []float32{1, 2}},
			"hidden": {Name: "hidden", Shape: []int64{batch, 2}, DType: onnx.TensorFloat32, Float: []float32{1, 2, 3, 4}},
		}, nil
	}}
	got := evalInt(t, `
local outs = emb.run_batch({
  {ids = {shape = {1}, data = {1}}},
  {ids = {shape = {1}, data = {2}}},
}, {bytes = true, outputs = {"logits"}})
local n, total = 0, 0
for _ in pairs(outs[1]) do n = n + 1 end
for i = 1, 2 do total = total + #outs[i].logits.bytes end
return n * 100 + total`, hosts)
	// 1 key per item (100) + 4 + 4 packed bytes = 108.
	if got != 108 {
		t.Fatalf("run_batch packed+selective = %d, want 108", got)
	}
}

func TestPackedOutputChargesBudget(t *testing.T) {
	hosts := Hosts{Run: multiOutputRun("big", onnx.NamedTensor{
		Name: "big", Shape: []int64{1, 32}, DType: onnx.TensorFloat32, Float: make([]float32, 32),
	})}
	_, err := EvalWithHosts(`return emb.run({x = {shape = {1}, data = {1}}}, {bytes = true})`,
		nil, nil, hosts, EvalOptions{MaxTensorElements: 8})
	if err == nil {
		t.Fatal("expected a budget error for an oversized packed output")
	}
	if !strings.Contains(err.Error(), "exceeds") {
		t.Fatalf("budget error = %q", err.Error())
	}
}

func TestArrayAndPackedOutputsShareBudget(t *testing.T) {
	hosts := Hosts{Run: multiOutputRun("big", onnx.NamedTensor{
		Name: "big", Shape: []int64{1, 32}, DType: onnx.TensorFloat32, Float: make([]float32, 32),
	})}
	_, err := EvalWithHosts(`return emb.run({x = {shape = {1}, data = {1}}})`,
		nil, nil, hosts, EvalOptions{MaxTensorElements: 8})
	if err == nil {
		t.Fatal("expected array outputs to be metered too")
	}
}
