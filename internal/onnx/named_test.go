package onnx

import (
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
