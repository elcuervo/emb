package onnx

import (
	"math"
	"os"
	"testing"
)

func namedTestModel(t *testing.T) []byte {
	t.Helper()
	data, err := os.ReadFile("../../models/minilm/model.onnx")
	if err != nil {
		t.Skipf("test model not present: %v (run: just download-model)", err)
	}
	return data
}

func namedTestInputs(t *testing.T) (inputNames []string, outNames []string) {
	t.Helper()
	names, err := GetInputNames("../../models/minilm/model.onnx")
	if err != nil {
		t.Fatalf("reading input names: %v", err)
	}
	outInfo, err := GetOutputInfo("../../models/minilm/model.onnx")
	if err != nil {
		t.Fatalf("reading output info: %v", err)
	}
	outNames = make([]string, 0, len(outInfo))
	for n := range outInfo {
		outNames = append(outNames, n)
	}
	return names, outNames
}

// TestRunNamedMatchesRun verifies the generic named path produces the same
// output as the pooled Session path for identical inputs (batch=2, seq=4).
func TestRunNamedMatchesRun(t *testing.T) {
	if err := InitEnvironment(""); err != nil {
		t.Fatal(err)
	}
	defer func() { _ = DestroyEnvironment() }()

	data := namedTestModel(t)
	inputNames, outNames := namedTestInputs(t)

	// The embed path needs a single named output tensor; use the first.
	if len(outNames) == 0 {
		t.Fatal("model has no outputs")
	}
	dim := 384
	rank := 3

	// Baseline: existing Session path.
	sess, err := NewRuntimeSessionFromBytes(data, inputNames, outNames[:1], dim, rank, 1, 2, ExecModeSequential)
	if err != nil {
		t.Skipf("session creation failed (model mismatch?): %v", err)
	}
	defer func() { _ = sess.Close() }()

	ids := []int64{101, 2003, 2023, 102, 101, 2003, 2023, 102}
	masks := []int64{1, 1, 1, 1, 1, 1, 1, 1}
	baseline, err := sess.Run(ids, masks, 2, 4, dim)
	if err != nil {
		// Only inputs present in the graph can be bound; if the graph lacks
		// token_type_ids the baseline failure is expected and the test skips.
		t.Skipf("baseline run failed (graph may lack optional inputs): %v", err)
	}

	// Named path with the same inputs, order-independent.
	named, err := NewNamedRuntimeSessionFromBytes(data, inputNames, outNames, 1, 2, ExecModeSequential)
	if err != nil {
		t.Fatalf("creating named session: %v", err)
	}
	defer func() { _ = named.Close() }()

	var namedInputs []NamedTensor
	for _, name := range inputNames {
		switch name {
		case "input_ids":
			namedInputs = append(namedInputs, NamedTensor{Name: name, Shape: []int64{2, 4}, DType: TensorInt64, Int64: ids})
		case "attention_mask":
			namedInputs = append(namedInputs, NamedTensor{Name: name, Shape: []int64{2, 4}, DType: TensorInt64, Int64: masks})
		case "token_type_ids":
			namedInputs = append(namedInputs, NamedTensor{Name: name, Shape: []int64{2, 4}, DType: TensorInt64, Int64: make([]int64, 8)})
		default:
			t.Fatalf("unexpected graph input %q", name)
		}
	}

	// Shuffle input order to prove name-based matching.
	for i, j := 0, len(namedInputs)-1; i < j; i, j = i+1, j-1 {
		namedInputs[i], namedInputs[j] = namedInputs[j], namedInputs[i]
	}

	out, err := named.RunNamed(namedInputs)
	if err != nil {
		t.Fatalf("RunNamed: %v", err)
	}
	if len(out) != len(outNames) {
		t.Fatalf("expected %d outputs, got %d", len(outNames), len(out))
	}
	first := out[outNames[0]]
	if first.DType != TensorFloat32 {
		t.Fatalf("output %q: expected float32, got %v", outNames[0], first.DType)
	}
	got := first.Float
	if len(got) != len(baseline) {
		t.Fatalf("output %q: expected %d floats, got %d", outNames[0], len(baseline), len(got))
	}
	for i := range got {
		if got[i] != baseline[i] {
			t.Fatalf("output %q[%d]: named=%v session=%v", outNames[0], i, got[i], baseline[i])
		}
	}
}

func TestRunNamedErrors(t *testing.T) {
	if err := InitEnvironment(""); err != nil {
		t.Fatal(err)
	}
	defer func() { _ = DestroyEnvironment() }()

	data := namedTestModel(t)
	inputNames, outNames := namedTestInputs(t)

	named, err := NewNamedRuntimeSessionFromBytes(data, inputNames, outNames, 1, 2, ExecModeSequential)
	if err != nil {
		t.Fatalf("creating named session: %v", err)
	}
	defer func() { _ = named.Close() }()

	// Missing input.
	if _, err := named.RunNamed(nil); err == nil {
		t.Fatal("expected error for missing inputs")
	}
	// Duplicate input name.
	dup := []NamedTensor{
		{Name: inputNames[0], Shape: []int64{1, 1}, DType: TensorInt64, Int64: []int64{1}},
		{Name: inputNames[0], Shape: []int64{1, 1}, DType: TensorInt64, Int64: []int64{1}},
	}
	if _, err := named.RunNamed(dup); err == nil {
		t.Fatal("expected error for duplicate inputs")
	}
	// Data/size mismatch.
	wrong := []NamedTensor{
		{Name: inputNames[0], Shape: []int64{1, 2}, DType: TensorInt64, Int64: []int64{1}},
	}
	if _, err := named.RunNamed(wrong); err == nil {
		t.Fatal("expected error for shape/data mismatch")
	}
}

// TestRunNamedBoolGraph feeds the vendored tiny Laya graph's five inputs —
// including the ONNX bool marker_mask — through the named session, proving a
// bool tensor round-trips a real graph input (the enabling slice's core).
func TestRunNamedBoolGraph(t *testing.T) {
	if err := InitEnvironment(""); err != nil {
		t.Fatal(err)
	}
	defer func() { _ = DestroyEnvironment() }()

	data, err := os.ReadFile("../../testdata/laya/model.onnx")
	if err != nil {
		t.Fatalf("vendored laya model missing: %v", err)
	}
	inputNames, err := GetInputNames("../../testdata/laya/model.onnx")
	if err != nil {
		t.Fatal(err)
	}
	have := map[string]bool{}
	for _, n := range inputNames {
		have[n] = true
	}
	for _, want := range []string{"input_ids", "attention_mask", "marker_pos", "marker_mask", "qtype"} {
		if !have[want] {
			t.Fatalf("graph inputs %v missing %q", inputNames, want)
		}
	}

	sess, err := NewNamedRuntimeSessionFromBytes(data, inputNames, []string{"logits", "act_logits"}, 1, 2, ExecModeSequential)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = sess.Close() }()

	// Two rows, seq 8, two markers: a valid minimum batch for the graph.
	inputs := []NamedTensor{
		{Name: "input_ids", Shape: []int64{2, 8}, DType: TensorInt64,
			Int64: []int64{2, 16, 15, 79, 33, 3, 0, 0, 2, 18, 15, 79, 13, 3, 0, 0}},
		{Name: "attention_mask", Shape: []int64{2, 8}, DType: TensorInt64,
			Int64: []int64{1, 1, 1, 1, 1, 1, 0, 0, 1, 1, 1, 1, 1, 1, 0, 0}},
		{Name: "marker_pos", Shape: []int64{2, 2}, DType: TensorInt64,
			Int64: []int64{6, 0, 6, 0}},
		{Name: "marker_mask", Shape: []int64{2, 2}, DType: TensorBool,
			Bool: []bool{true, false, true, true}},
		{Name: "qtype", Shape: []int64{2}, DType: TensorInt64,
			Int64: []int64{0, 2}},
	}
	out, err := sess.RunNamed(inputs)
	if err != nil {
		t.Fatalf("bool-input run failed: %v", err)
	}
	logits, ok := out["logits"]
	if !ok {
		t.Fatal("no logits output")
	}
	if len(logits.Shape) != 2 || logits.Shape[0] != 2 || logits.Shape[1] != 2 {
		t.Fatalf("logits shape = %v, want [2 2]", logits.Shape)
	}
	act, ok := out["act_logits"]
	if !ok {
		t.Fatal("no act_logits output")
	}
	if len(act.Shape) != 2 || act.Shape[0] != 2 || act.Shape[1] != 2 {
		t.Fatalf("act_logits shape = %v, want [2 2]", act.Shape)
	}
	for i, v := range logits.Float {
		if v == 0 || v != v || math.IsInf(float64(v), 0) { // finite, nonzero per-row output
			t.Fatalf("logits[%d] = %v, want a finite nonzero value", i, v)
		}
	}
}
