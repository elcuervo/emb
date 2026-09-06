package script

import (
	"os"
	"testing"

	"github.com/elcuervo/emb/internal/onnx"
	"github.com/elcuervo/emb/internal/tokenizer"
)

// realTokenizerHosts binds the minilm tokenizer (regular + offsets + word
// capabilities) so example scripts run end-to-end minus real inference.
func realTokenizerHosts(t *testing.T) Hosts {
	t.Helper()
	rt, err := tokenizer.NewTokenizer("../../models/minilm/tokenizer.json", false)
	if err != nil {
		t.Skipf("test tokenizer not present: %v (run: just download-model)", err)
	}
	t.Cleanup(func() { _ = rt.Close() })
	return Hosts{
		EncodePlain:        rt.EncodeOffsets,
		EncodePair:         rt.EncodePairOffsets,
		EncodePretokenized: rt.EncodePretokenized,
	}
}

// evalExample runs one example script with the given run closure and returns
// the script's reply bytes (hash replies are flat field/value pair arrays).
func evalExample(t *testing.T, file string, keys, argv []string, run func([]onnx.NamedTensor) (map[string]onnx.NamedTensor, error)) string {
	t.Helper()
	src, err := os.ReadFile("../../examples/scripts/" + file)
	if err != nil {
		t.Fatal(err)
	}
	hosts := realTokenizerHosts(t)
	hosts.Run = run
	v, err := EvalWithHosts(string(src), keys, argv, hosts, EvalOptions{})
	if err != nil {
		t.Fatalf("%s: %v", file, err)
	}
	encoded, err := EncodeReply(v)
	if err != nil {
		t.Fatalf("%s: %v", file, err)
	}
	return string(encoded)
}

func TestExampleSST2(t *testing.T) {
	// Canned logits favor class 2 => POSITIVE.
	run := func(_ []onnx.NamedTensor) (map[string]onnx.NamedTensor, error) {
		return map[string]onnx.NamedTensor{
			"logits": {Name: "logits", Shape: []int64{1, 2}, DType: onnx.TensorFloat32, Float: []float32{-1.5, 1.5}},
		}, nil
	}
	reply := evalExample(t, "sst2.lua", []string{"this film is great"}, []string{"NEGATIVE", "POSITIVE"}, run)
	// sst2 returns {label, confidence, scores}: label must be POSITIVE.
	if !containsReply(reply, "\"POSITIVE\"") && !containsReply(reply, "POSITIVE") {
		t.Fatalf("expected POSITIVE label in %q", reply)
	}
	if containsReply(reply, "NEGATIVE") {
		t.Fatalf("unexpected NEGATIVE in %q", reply)
	}
}

func TestExampleQA(t *testing.T) {
	rt, err := tokenizer.NewTokenizer("../../models/minilm/tokenizer.json", false)
	if err != nil {
		t.Skip(err)
	}
	defer rt.Close()
	// Token id for the answer string "1976".
	ids, _, _, err := rt.EncodeOffsets("1976", 512)
	if err != nil {
		t.Fatal(err)
	}
	var targetID int64
	for _, id := range ids {
		if id > 0 && id != 101 && id != 102 { // skip [CLS]/[SEP]
			targetID = id
			break
		}
	}
	if targetID == 0 {
		t.Fatal("no target token found")
	}

	question := "when was the Mac launched"
	context := "Apple launched the Mac in 1976."
	run := func(inputs []onnx.NamedTensor) (map[string]onnx.NamedTensor, error) {
		var pair []int64
		for _, in := range inputs {
			if in.Name == "input_ids" {
				pair = in.Int64
			}
		}
		n := len(pair)
		start := make([]float32, n)
		end := make([]float32, n)
		for i := range start {
			start[i], end[i] = -1, -1
		}
		// Peak start+end at the LAST occurrence of the "1976" token.
		for i, id := range pair {
			if id == targetID {
				start[i] = 3
				end[i] = 3
			}
		}
		return map[string]onnx.NamedTensor{
			"start_logits": {Name: "start_logits", Shape: []int64{1, int64(n)}, DType: onnx.TensorFloat32, Float: start},
			"end_logits":   {Name: "end_logits", Shape: []int64{1, int64(n)}, DType: onnx.TensorFloat32, Float: end},
		}, nil
	}
	reply := evalExample(t, "qa.lua", []string{question, context}, nil, run)
	// The answer surfaced from the context via offsets must include 1976.
	if !containsReply(reply, "1976") {
		t.Fatalf("expected answer 1976 in %q", reply)
	}
}

func TestExampleRerank(t *testing.T) {
	calls := 0
	logitsByCall := []float32{-1, 1, 2}
	run := func(_ []onnx.NamedTensor) (map[string]onnx.NamedTensor, error) {
		v := logitsByCall[calls%len(logitsByCall)]
		calls++
		return map[string]onnx.NamedTensor{
			"logits": {Name: "logits", Shape: []int64{1, 1}, DType: onnx.TensorFloat32, Float: []float32{v}},
		}, nil
	}
	reply := evalExample(t, "rerank.lua", []string{"what is the capital of france"}, []string{"Paris", "Lyon", "Nice"}, run)
	// sigmoid(2) > sigmoid(1) > sigmoid(-1), so Nice ranks first, Lyon second,
	// Paris third. The reply is an array of {rank, doc, score} hashes in order.
	if !containsReply(reply, "Nice") || !containsReply(reply, "Paris") {
		t.Fatalf("missing docs in %q", reply)
	}
	if idxNice, idxParis := indexOf(reply, "Nice"), indexOf(reply, "Paris"); idxNice > idxParis {
		t.Fatalf("Nice should rank before Paris: %q", reply)
	}
}

func containsReply(reply, needle string) bool {
	return indexOf(reply, needle) >= 0
}

func indexOf(reply, needle string) int {
	for i := 0; i+len(needle) <= len(reply); i++ {
		if reply[i:i+len(needle)] == needle {
			return i
		}
	}
	return -1
}
