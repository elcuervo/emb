package onnx

import (
	"encoding/binary"
	"math"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// Minimal ONNX protobuf writer, enough to emit a two-weight graph whose
// initializer lives in a sidecar file. Hand-built so the test needs no ONNX
// Python toolchain and no downloaded model: the point is only that session
// creation resolves the sidecar from the graph's directory.
func pbVarint(dst []byte, v uint64) []byte {
	for v >= 0x80 {
		dst = append(dst, byte(v)|0x80)
		v >>= 7
	}
	return append(dst, byte(v))
}

func pbTag(dst []byte, field, wire int) []byte { return pbVarint(dst, uint64(field<<3|wire)) }

func pbVarintField(dst []byte, field int, v uint64) []byte {
	return pbVarint(pbTag(dst, field, 0), v)
}

func pbBytesField(dst []byte, field int, b []byte) []byte {
	return append(pbVarint(pbTag(dst, field, 2), uint64(len(b))), b...)
}

func pbStringField(dst []byte, field int, s string) []byte {
	return pbBytesField(dst, field, []byte(s))
}

// valueInfo encodes ValueInfoProto{name, tensor(float, dims)}.
func valueInfo(name string, dims ...int64) []byte {
	var shape []byte
	for _, d := range dims {
		dim := pbVarintField(nil, 1, uint64(d)) // Dimension.dim_value
		shape = append(shape, pbBytesField(nil, 1, dim)...)
	}
	tensor := pbVarintField(nil, 1, 1)                      // Tensor.elem_type = FLOAT
	tensor = append(tensor, pbBytesField(nil, 2, shape)...) // Tensor.shape
	typ := pbBytesField(nil, 1, tensor)                     // TypeProto.tensor_type
	out := pbStringField(nil, 1, name)
	return append(out, pbBytesField(nil, 2, typ)...)
}

// externalInitModelBytes returns an ONNX graph reading one FLOAT[2] initializer
// "w" from sidecar (location recorded inside the graph) and adding it to the
// FLOAT[2] input "x". Working directory is irrelevant to it.
func externalInitModelBytes(sidecar string) []byte {
	entry := func(k, v string) []byte {
		return pbStringField(pbStringField(nil, 1, k), 2, v)
	}

	var tensor []byte
	tensor = append(tensor, pbBytesField(nil, 1, pbVarint(nil, 2)[:])...) // dims [2] (packed)
	tensor = pbVarintField(tensor, 2, 1)                                  // data_type FLOAT
	tensor = pbStringField(tensor, 8, "w")                                // name
	for _, e := range [][2]string{{"location", sidecar}, {"offset", "0"}, {"length", "8"}} {
		tensor = append(tensor, pbBytesField(nil, 13, entry(e[0], e[1]))...) // external_data
	}
	tensor = pbVarintField(tensor, 14, 1) // data_location EXTERNAL

	var node []byte
	node = pbStringField(node, 1, "w")   // input
	node = pbStringField(node, 1, "x")   // input
	node = pbStringField(node, 2, "y")   // output
	node = pbStringField(node, 3, "add") // name
	node = pbStringField(node, 4, "Add")

	var graph []byte
	graph = append(graph, pbBytesField(nil, 1, node)...)               // node
	graph = pbStringField(graph, 2, "external")                        // name
	graph = append(graph, pbBytesField(nil, 5, tensor)...)             // initializer
	graph = append(graph, pbBytesField(nil, 11, valueInfo("x", 2))...) // input
	graph = append(graph, pbBytesField(nil, 12, valueInfo("y", 2))...) // output

	model := pbVarintField(nil, 1, 8) // ir_version
	model = append(model, pbBytesField(nil, 7, graph)...)
	opset := pbVarintField(nil, 2, 17) // OperatorSetIdProto.version (domain "")
	return append(model, pbBytesField(nil, 8, opset)...)
}

// writeExternalDataModel lays a graph and its sidecar in one directory and
// returns the graph path.
func writeExternalDataModel(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	graphPath := filepath.Join(dir, "model.onnx")

	sidecar := make([]byte, 8)
	binary.LittleEndian.PutUint32(sidecar[0:], math.Float32bits(1.5))
	binary.LittleEndian.PutUint32(sidecar[4:], math.Float32bits(-2.5))
	if err := os.WriteFile(graphPath+".data", sidecar, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(graphPath, externalInitModelBytes("model.onnx.data"), 0o644); err != nil {
		t.Fatal(err)
	}
	return graphPath
}

// TestExternalInitializersResolveBesideGraph proves a from-bytes session reads a
// sidecar initializer from the model's directory: the working directory is a
// different temp directory, and the graph still loads and returns the weights.
func TestExternalInitializersResolveBesideGraph(t *testing.T) {
	if err := InitEnvironment(""); err != nil {
		t.Fatal(err)
	}
	defer func() { _ = DestroyEnvironment() }()

	graphPath := writeExternalDataModel(t)
	data, err := os.ReadFile(graphPath)
	if err != nil {
		t.Fatal(err)
	}

	wd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(t.TempDir()); err != nil {
		t.Fatal(err)
	}
	defer func() { _ = os.Chdir(wd) }()

	sess, err := NewNamedRuntimeSessionFromBytes(data, graphPath, []string{"x"}, []string{"y"}, 1, 2, ExecModeSequential, true)
	if err != nil {
		t.Fatalf("session from bytes with sidecar: %v", err)
	}
	defer func() { _ = sess.Close() }()

	outs, err := sess.RunNamed([]NamedTensor{{Name: "x", Shape: []int64{2}, DType: TensorFloat32, Float: []float32{1, 1}}})
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	got := outs["y"].Float
	if len(got) != 2 || got[0] != 2.5 || got[1] != -1.5 {
		t.Fatalf("x + external initializer = %v, want [2.5 -1.5]", got)
	}

	// Without the model path the same bytes fail, so the folder entry is what
	// resolved the sidecar rather than a coincidentally matching working
	// directory.
	if _, err := NewNamedRuntimeSessionFromBytes(data, "", []string{"x"}, []string{"y"}, 1, 2, ExecModeSequential, true); err == nil {
		t.Fatal("session built without the model path loaded a sidecar graph, want a failure")
	}
}

// TestExternalInitializersMissingIsNamed proves the failure names the model and
// the directory searched instead of surfacing a bare ORT message.
func TestExternalInitializersMissingIsNamed(t *testing.T) {
	if err := InitEnvironment(""); err != nil {
		t.Fatal(err)
	}
	defer func() { _ = DestroyEnvironment() }()

	graphPath := writeExternalDataModel(t)
	if err := os.Remove(graphPath + ".data"); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(graphPath)
	if err != nil {
		t.Fatal(err)
	}

	_, err = NewNamedRuntimeSessionFromBytes(data, graphPath, []string{"x"}, []string{"y"}, 1, 2, ExecModeSequential, true)
	if err == nil {
		t.Fatal("session without its sidecar loaded, want an error")
	}
	if !strings.Contains(err.Error(), graphPath) || !strings.Contains(err.Error(), filepath.Dir(graphPath)) {
		t.Fatalf("error %q must name the model path and its directory", err)
	}
}
