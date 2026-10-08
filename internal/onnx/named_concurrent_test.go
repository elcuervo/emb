package onnx

import (
	"fmt"
	"sort"
	"strconv"
	"strings"
	"sync"
	"testing"
)

// namedInputsForSeq builds a single-sequence named input set with the given
// sequence length, for exercising concurrent runs across shapes.
func namedInputsForSeq(inputNames []string, seq int) []NamedTensor {
	out := make([]NamedTensor, 0, len(inputNames))
	for _, name := range inputNames {
		data := make([]int64, seq)
		switch name {
		case "input_ids":
			for i := range data {
				data[i] = 101
			}
		case "attention_mask":
			for i := range data {
				data[i] = 1
			}
		default:
			// token_type_ids and any other integral input: zeros.
		}
		out = append(out, NamedTensor{Name: name, Shape: []int64{1, int64(seq)}, DType: TensorInt64, Int64: data})
	}
	return out
}

// namedOutputsKey renders an output set as a stable text, so two runs can be
// compared regardless of map iteration order.
func namedOutputsKey(out map[string]NamedTensor) string {
	names := make([]string, 0, len(out))
	for n := range out {
		names = append(names, n)
	}
	sort.Strings(names)
	var b strings.Builder
	for _, n := range names {
		t := out[n]
		fmt.Fprintf(&b, "%s|%v|%d|", n, t.Shape, t.DType)
		switch t.DType {
		case TensorInt64:
			for _, v := range t.Int64 {
				b.WriteString(strconv.FormatInt(v, 10))
				b.WriteByte(',')
			}
		case TensorFloat32:
			for _, v := range t.Float {
				b.WriteString(strconv.FormatFloat(float64(v), 'g', -1, 32))
				b.WriteByte(',')
			}
		case TensorBool:
			for _, v := range t.Bool {
				b.WriteString(strconv.FormatBool(v))
				b.WriteByte(',')
			}
		}
	}
	return b.String()
}

func runNamedKey(t *testing.T, sess *NamedRuntimeSession, inputs []NamedTensor) string {
	t.Helper()
	out, err := sess.RunNamed(inputs)
	if err != nil {
		t.Fatalf("RunNamed: %v", err)
	}
	return namedOutputsKey(out)
}

// TestNamedSessionConcurrentRunsMatchSerial runs distinct sequence lengths on
// one session concurrently and checks each reply equals its serial reply. Run
// under -race to prove the session has no shared mutable state.
func TestNamedSessionConcurrentRunsMatchSerial(t *testing.T) {
	if err := InitEnvironment(""); err != nil {
		t.Fatal(err)
	}
	defer func() { _ = DestroyEnvironment() }()

	data := namedTestModel(t)
	inputNames, outNames := namedTestInputs(t)
	sess, err := NewNamedRuntimeSessionFromBytes(data, inputNames, outNames, 1, 2, ExecModeSequential, true)
	if err != nil {
		t.Fatalf("creating named session: %v", err)
	}
	defer func() { _ = sess.Close() }()

	seqs := []int{4, 5, 6, 7, 8}

	serial := make(map[int]string, len(seqs))
	for _, seq := range seqs {
		serial[seq] = runNamedKey(t, sess, namedInputsForSeq(inputNames, seq))
	}

	for round := 0; round < 8; round++ {
		var wg sync.WaitGroup
		keys := make([]string, len(seqs))
		for i, seq := range seqs {
			wg.Add(1)
			go func(i, seq int) {
				defer wg.Done()
				keys[i] = runNamedKey(t, sess, namedInputsForSeq(inputNames, seq))
			}(i, seq)
		}
		wg.Wait()
		for i, seq := range seqs {
			if keys[i] != serial[seq] {
				t.Fatalf("round %d seq %d: concurrent reply differs from serial", round, seq)
			}
		}
	}
}

// TestSessionOptionsAllowSpinning checks that allow_spinning=false sets the ORT
// session config entry, while the default leaves it unset.
func TestSessionOptionsAllowSpinning(t *testing.T) {
	if err := InitEnvironment(""); err != nil {
		t.Fatal(err)
	}
	defer func() { _ = DestroyEnvironment() }()

	off, err := newSessionOptions(1, 2, ExecModeSequential, false)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = off.Destroy() }()
	has, err := off.HasSessionConfigEntry("session.intra_op.allow_spinning")
	if err != nil {
		t.Fatal(err)
	}
	if !has {
		t.Fatal("allow_spinning=false did not set session.intra_op.allow_spinning")
	}
	v, err := off.GetSessionConfigEntry("session.intra_op.allow_spinning")
	if err != nil {
		t.Fatal(err)
	}
	if v != "0" {
		t.Fatalf("session.intra_op.allow_spinning = %q, want \"0\"", v)
	}

	on, err := newSessionOptions(1, 2, ExecModeSequential, true)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = on.Destroy() }()
	has, err = on.HasSessionConfigEntry("session.intra_op.allow_spinning")
	if err != nil {
		t.Fatal(err)
	}
	if has {
		t.Fatal("allow_spinning=true should leave ORT's default entry unset")
	}
}
