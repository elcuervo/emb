package script

import (
	"testing"

	lua "github.com/yuin/gopher-lua"

	"github.com/elcuervo/emb/internal/onnx"
)

// fakeSession is a scripted-model session that doubles input_ids values.
type fakeSession struct {
	calls int
}

func (f *fakeSession) RunNamed(inputs []onnx.NamedTensor) (map[string]onnx.NamedTensor, error) {
	f.calls++
	var ids []int64
	var shape []int64
	for _, in := range inputs {
		if in.Name == "input_ids" {
			ids = in.Int64
			shape = in.Shape
		}
	}
	// outputs: int64 tensor (word marker) and float32 tensor (logits)
	n := int(shape[0] * shape[1])
	word := make([]int64, n)
	for i := range word {
		word[i] = int64(i) * 2
	}
	logits := make([]float32, len(word))
	for i := range logits {
		logits[i] = float32(ids[i]) * 0.5
	}
	return map[string]onnx.NamedTensor{
		"markers": {Name: "markers", Shape: shape, DType: onnx.TensorInt64, Int64: word},
		"logits":  {Name: "logits", Shape: shape, DType: onnx.TensorFloat32, Float: logits},
	}, nil
}

func (f *fakeSession) Close() error { return nil }

var _ onnx.NamedSession = (*fakeSession)(nil)

// fakeTok returns one token (and its word) per input word.
func fakeTok(words []string, maxLen int) ([]int64, []int64, error) {
	n := len(words)
	if maxLen > 0 && n > maxLen {
		n = maxLen
	}
	ids := make([]int64, n)
	wordIDs := make([]int64, n)
	for i := 0; i < n; i++ {
		ids[i] = int64(1000 + i)
		wordIDs[i] = int64(i)
	}
	return ids, wordIDs, nil
}

func hostFixture() Hosts {
	return Hosts{
		Run: func(inputs []onnx.NamedTensor) (map[string]onnx.NamedTensor, error) {
			return (&fakeSession{}).RunNamed(inputs)
		},
		EncodePretokenized: fakeTok,
	}
}

func TestHostRunRoundTrip(t *testing.T) {
	src := `
local out = emb.run({
  input_ids      = {shape = {1, 4}, data = {101, 2003, 2023, 102}},
  attention_mask = {shape = {1, 4}, data = {1, 1, 1, 1}}
})
return out.logits.data[1] .. "|" .. out.markers.data[4]`
	v, err := EvalWithHosts(src, nil, nil, hostFixture(), EvalOptions{})
	if err != nil {
		t.Fatal(err)
	}
	// logits[0] = ids[0]*0.5 = 101*0.5 = 50.5 (fractional → Lua number),
	// markers[3] = 3*2 = 6.
	if v.String() != "50.5|6" {
		t.Fatalf("unexpected result %q", v.String())
	}
}

func TestHostRunFloatData(t *testing.T) {
	// A fractional element in data promotes the tensor to float32.
	src := `
local out = emb.run({ x = {shape = {2}, data = {0.5, 1}} })
return out.x.data[1]`
	var gotDType onnx.TensorType
	v, err := EvalWithHosts(src, nil, nil, Hosts{
		Run: func(inputs []onnx.NamedTensor) (map[string]onnx.NamedTensor, error) {
			gotDType = inputs[0].DType
			return map[string]onnx.NamedTensor{
				"x": {Name: "x", Shape: []int64{2}, DType: onnx.TensorFloat32, Float: []float32{0.25, 0.5}},
			}, nil
		},
	}, EvalOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if v.String() != "0.25" {
		t.Fatalf("unexpected result %q", v.String())
	}
	if gotDType != onnx.TensorFloat32 {
		t.Fatalf("expected float32 tensor, got %v", gotDType)
	}
}

func TestHostRunValidationErrors(t *testing.T) {
	cases := []string{
		`return emb.run({ x = 42 })`,                         // input must be a spec table
		`return emb.run({ x = {data = {1}} })`,               // missing shape
		`return emb.run({ x = {shape = {1}, data = {1,2}}})`, // shape/data element mismatch
		`return emb.run()`,                                   // no args
	}
	for _, src := range cases {
		if _, err := EvalWithHosts(src, nil, nil, hostFixture(), EvalOptions{}); err == nil {
			t.Fatalf("script %q should fail in emb.run validation", src)
		}
	}
}

func TestHostTokenizePretokenized(t *testing.T) {
	src := `
local out = emb.tokenize.pretokenized({"(", "[E]", "PERSON", ")"}, 512)
return out.ids[2] .. "|" .. out.word_ids[4]`
	v, err := EvalWithHosts(src, nil, nil, hostFixture(), EvalOptions{})
	if err != nil {
		t.Fatal(err)
	}
	// ids[2] = 1001, word_ids[4] = 3
	if v.String() != "1001|3" {
		t.Fatalf("unexpected result %q", v.String())
	}
}

func TestHostTokenizeWords(t *testing.T) {
	src := `
local out = emb.tokenize.words("Apple CEO Tim Cook")
return out.words[1] .. "|" .. out.words[4] .. "|" .. out.starts[2] .. "|" .. out.ends[3]`
	v, err := EvalWithHosts(src, nil, nil, Hosts{}, EvalOptions{})
	if err != nil {
		t.Fatal(err)
	}
	// Apple|Cook|7|14
	if v.String() != "Apple|Cook|7|14" {
		t.Fatalf("unexpected result %q", v.String())
	}
}

func TestHostTokenizeWordsPunctuation(t *testing.T) {
	src := `
local out = emb.tokenize.words("Hello, world!")
return #out.words .. "|" .. out.words[2] .. "|" .. out.starts[3]`
	v, err := EvalWithHosts(src, nil, nil, Hosts{}, EvalOptions{})
	if err != nil {
		t.Fatal(err)
	}
	// 4|,|8
	if v.String() != "4|,|8" {
		t.Fatalf("unexpected result %q", v.String())
	}
}

func TestHostUnavailable(t *testing.T) {
	// Zero Hosts: emb.run and emb.tokenize are registered but error when used.
	for _, src := range []string{
		`return emb.run({x = {shape = {1}, data = {1}}})`,
		`return emb.tokenize.pretokenized({"hi"}, 512)`,
	} {
		if _, err := EvalWithHosts(src, nil, nil, Hosts{}, EvalOptions{}); err == nil {
			t.Fatalf("script %q should fail with unbound hosts", src)
		}
	}
}

func TestJSONEncodeDecode(t *testing.T) {
	src := `
local decoded = json.decode([[{"a": [1, 2.5], "b": "x", "c": null}]])
local out = json.encode(decoded)
return out`
	v, err := EvalWithHosts(src, nil, nil, Hosts{}, EvalOptions{})
	if err != nil {
		t.Fatal(err)
	}
	// encoding/json normalizes key order and numeric formats; assert the
	// round-trip structure rather than byte order.
	if v.String() != `{"a":[1,2.5],"b":"x","c":null}` {
		t.Fatalf("unexpected json result %q", v.String())
	}
}

func TestJSONLuaTableRoundTrip(t *testing.T) {
	src := `
return json.encode({PERSON = {"Tim Cook"}, score = 0.9823})`
	v, err := EvalWithHosts(src, nil, nil, Hosts{}, EvalOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if v.String() != `{"PERSON":["Tim Cook"],"score":0.9823}` {
		t.Fatalf("unexpected json result %q", v.String())
	}
}

var _ = lua.LNil
