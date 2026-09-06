package script

import (
	"encoding/json"
	"fmt"
	"math"
	"sort"

	lua "github.com/yuin/gopher-lua"

	"github.com/elcuervo/emb/internal/onnx"
	"github.com/elcuervo/emb/internal/tokenizer"
)

// Hosts binds a script evaluation to the resources it may touch: the model's
// named-tensor session and its word-aligned tokenizer. Both are optional; a
// binding with only Run lets scripts do arbitrary graph IO without
// tokenization, and vice versa. The zero value disables all host functions.
type Hosts struct {
	// Run executes one inference over named tensors and returns the graph's
	// named outputs. Nil makes emb.run unavailable.
	Run func(inputs []onnx.NamedTensor) (map[string]onnx.NamedTensor, error)
	// EncodePretokenized word-encodes an already-split word list. Nil makes
	// emb.tokenize.pretokenized unavailable.
	EncodePretokenized func(words []string, maxLen int) (ids, wordIDs []int64, err error)
}

// registerHosts installs the whitelisted emb.* and json host functions into
// the sandbox. Scripts may only reach models and tokenizers through these; no
// other host surface exists.
func registerHosts(ls *lua.LState, h Hosts) {
	emb := ls.NewTable()
	emb.RawSetString("run", ls.NewFunction(func(ls *lua.LState) int {
		return runHost(ls, h)
	}))
	tok := ls.NewTable()
	tok.RawSetString("pretokenized", ls.NewFunction(func(ls *lua.LState) int {
		return tokenizeHost(ls, h)
	}))
	tok.RawSetString("words", ls.NewFunction(tokenizeWordsHost))
	emb.RawSetString("tokenize", tok)
	ls.SetGlobal("emb", emb)

	j := ls.NewTable()
	j.RawSetString("encode", ls.NewFunction(jsonEncodeHost))
	j.RawSetString("decode", ls.NewFunction(jsonDecodeHost))
	// json.null is a unique null sentinel (the cjson.null pattern): Lua tables
	// cannot hold nil, so decoding {"a": null} stores the sentinel, and
	// encoding it reproduces null.
	j.RawSetString("null", ls.NewTable())
	ls.SetGlobal("json", j)
}

// runHost implements emb.run: a table of named inputs
//
//	{input_ids = {shape = {1, 7}, data = {101, ...}}, ...}
//
// → a table of named outputs {name = {shape = {...}, data = {...}}}. Integral
// data encodes as int64; a single fraction in a tensor's data promotes it to
// float32. Field names are processed in sorted order so identical tables map
// to identical tensor orders.
func runHost(ls *lua.LState, h Hosts) int {
	if h.Run == nil {
		ls.RaiseError("emb.run is unavailable for this model")
		return 0
	}
	arg := ls.CheckTable(1)
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
			ls.RaiseError("emb.run: input %q must be a table {shape=..., data=...}", name)
			return 0
		}
		t, err := namedTensorFromLua(spec)
		if err != nil {
			ls.RaiseError("emb.run: input %q: %v", name, err)
			return 0
		}
		t.Name = name
		inputs = append(inputs, t)
	}

	outputs, err := h.Run(inputs)
	if err != nil {
		ls.RaiseError("emb.run: %v", err)
		return 0
	}

	outNames := make([]string, 0, len(outputs))
	for n := range outputs {
		outNames = append(outNames, n)
	}
	sort.Strings(outNames)
	result := ls.NewTable()
	for _, n := range outNames {
		result.RawSetString(n, namedTensorToLua(ls, outputs[n]))
	}
	ls.Push(result)
	return 1
}

// tokenizeWordsHost implements emb.tokenize.words(text) → {words = {...},
// starts = {...}, ends = {...}}: the generic word-splitting building block
// (BertPreTokenizer rules, byte offsets, no lowercasing). Models decide their
// own casing and schema; see examples/scripts/gliner2.lua for the consumption
// pattern.
func tokenizeWordsHost(ls *lua.LState) int {
	text := ls.CheckString(1)
	words, starts, ends := tokenizer.SplitWords(text)
	result := ls.NewTable()
	wordsT := ls.NewTable()
	startsT := ls.NewTable()
	endsT := ls.NewTable()
	for i, w := range words {
		wordsT.RawSetInt(i+1, lua.LString(w))
		startsT.RawSetInt(i+1, lua.LNumber(starts[i]))
		endsT.RawSetInt(i+1, lua.LNumber(ends[i]))
	}
	result.RawSetString("words", wordsT)
	result.RawSetString("starts", startsT)
	result.RawSetString("ends", endsT)
	ls.Push(result)
	return 1
}

// tokenizeHost implements emb.tokenize.pretokenized(words, maxLen) →
// {ids = {...}, word_ids = {...}}.
func tokenizeHost(ls *lua.LState, h Hosts) int {
	if h.EncodePretokenized == nil {
		ls.RaiseError("emb.tokenize.pretokenized is unavailable for this model")
		return 0
	}
	words, ok := stringArrayFromLua(ls, 1)
	if !ok {
		ls.RaiseError("emb.tokenize.pretokenized: words must be an array of strings")
		return 0
	}
	maxLen := ls.OptInt(2, 0)
	ids, wordIDs, err := h.EncodePretokenized(words, maxLen)
	if err != nil {
		ls.RaiseError("emb.tokenize.pretokenized: %v", err)
		return 0
	}
	result := ls.NewTable()
	result.RawSetString("ids", int64ArrayToLua(ls, ids))
	result.RawSetString("word_ids", int64ArrayToLua(ls, wordIDs))
	ls.Push(result)
	return 1
}

// namedTensorFromLua reads {shape = {n...}, data = {...}}; data with any
// fractional element becomes a float32 tensor, otherwise int64.
func namedTensorFromLua(spec *lua.LTable) (onnx.NamedTensor, error) {
	var t onnx.NamedTensor
	if shapeTab, ok := spec.RawGetString("shape").(*lua.LTable); ok {
		shape, err := int64ArrayFromLua(shapeTab)
		if err != nil {
			return t, fmt.Errorf("shape: %w", err)
		}
		t.Shape = shape
	} else {
		return t, fmt.Errorf("missing shape")
	}
	dataTab, ok := spec.RawGetString("data").(*lua.LTable)
	if !ok {
		return t, fmt.Errorf("missing data")
	}
	data, err := numberArrayFromLua(dataTab)
	if err != nil {
		return t, fmt.Errorf("data: %w", err)
	}
	floats := false
	for _, n := range data {
		if math.Trunc(n) != n {
			floats = true
			break
		}
	}
	if floats {
		t.DType = onnx.TensorFloat32
		t.Float = make([]float32, len(data))
		for i, n := range data {
			t.Float[i] = float32(n)
		}
	} else {
		t.DType = onnx.TensorInt64
		t.Int64 = make([]int64, len(data))
		for i, n := range data {
			t.Int64[i] = int64(n)
		}
	}
	return t, nil
}

func namedTensorToLua(ls *lua.LState, t onnx.NamedTensor) *lua.LTable {
	out := ls.NewTable()
	shape := ls.NewTable()
	for i, d := range t.Shape {
		shape.RawSetInt(i+1, lua.LNumber(d))
	}
	out.RawSetString("shape", shape)
	data := ls.NewTable()
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

// --- json host functions ------------------------------------------------

func jsonEncodeHost(ls *lua.LState) int {
	v := luaValueToAny(ls, ls.Get(1))
	b, err := json.Marshal(v)
	if err != nil {
		ls.RaiseError("json.encode: %v", err)
		return 0
	}
	ls.Push(lua.LString(b))
	return 1
}

func jsonDecodeHost(ls *lua.LState) int {
	raw := ls.CheckString(1)
	var v any
	if err := json.Unmarshal([]byte(raw), &v); err != nil {
		ls.RaiseError("json.decode: %v", err)
		return 0
	}
	ls.Push(anyToLuaValue(ls, v))
	return 1
}

// jsonNullSentinel returns this state's json.null value (see registerHosts).
func jsonNullSentinel(ls *lua.LState) lua.LValue {
	if j, ok := ls.GetGlobal("json").(*lua.LTable); ok {
		return j.RawGetString("null")
	}
	return lua.LNil
}

// luaValueToAny converts a Lua value to a Go value for JSON encoding. Tables
// with contiguous integer keys encode as arrays; other tables encode as maps.
func luaValueToAny(ls *lua.LState, v lua.LValue) any {
	switch t := v.(type) {
	case *lua.LNilType:
		return nil
	case lua.LBool:
		return bool(t)
	case lua.LNumber:
		return float64(t)
	case lua.LString:
		return string(t)
	case *lua.LTable:
		if t == jsonNullSentinel(ls) {
			return nil
		}
		if isListTable(t) {
			arr := make([]any, 0, t.Len())
			for i := 1; i <= t.Len(); i++ {
				arr = append(arr, luaValueToAny(ls, t.RawGetInt(i)))
			}
			return arr
		}
		m := map[string]any{}
		t.ForEach(func(k, val lua.LValue) {
			if s, ok := k.(lua.LString); ok {
				m[string(s)] = luaValueToAny(ls, val)
			}
		})
		return m
	default:
		return nil
	}
}

func anyToLuaValue(ls *lua.LState, v any) lua.LValue {
	switch t := v.(type) {
	case nil:
		return lua.LNil
	case bool:
		return lua.LBool(t)
	case float64:
		return lua.LNumber(t)
	case string:
		return lua.LString(t)
	case []any:
		tbl := ls.NewTable()
		for i, e := range t {
			tbl.RawSetInt(i+1, anyToLuaValue(ls, e))
		}
		return tbl
	case map[string]any:
		tbl := ls.NewTable()
		null := jsonNullSentinel(ls)
		for k, e := range t {
			if e == nil {
				tbl.RawSetString(k, null)
			} else {
				tbl.RawSetString(k, anyToLuaValue(ls, e))
			}
		}
		return tbl
	default:
		return lua.LNil
	}
}

func isListTable(t *lua.LTable) bool {
	entries := 0
	intKeys := 0
	t.ForEach(func(k, v lua.LValue) {
		entries++
		if _, ok := k.(lua.LNumber); ok {
			intKeys++
		}
	})
	return entries > 0 && intKeys == entries && entries == t.Len()
}

// --- Lua array helpers ---------------------------------------------------

func stringArrayFromLua(ls *lua.LState, idx int) ([]string, bool) {
	t, ok := ls.Get(idx).(*lua.LTable)
	if !ok {
		return nil, false
	}
	out := make([]string, 0, t.Len())
	for i := 1; i <= t.Len(); i++ {
		s, ok := t.RawGetInt(i).(lua.LString)
		if !ok {
			return nil, false
		}
		out = append(out, string(s))
	}
	return out, true
}

func int64ArrayFromLua(t *lua.LTable) ([]int64, error) {
	out := make([]int64, 0, t.Len())
	for i := 1; i <= t.Len(); i++ {
		n, ok := t.RawGetInt(i).(lua.LNumber)
		if !ok {
			return nil, fmt.Errorf("element %d is not a number", i)
		}
		out = append(out, int64(n))
	}
	return out, nil
}

func numberArrayFromLua(t *lua.LTable) ([]float64, error) {
	out := make([]float64, 0, t.Len())
	for i := 1; i <= t.Len(); i++ {
		n, ok := t.RawGetInt(i).(lua.LNumber)
		if !ok {
			return nil, fmt.Errorf("element %d is not a number", i)
		}
		out = append(out, float64(n))
	}
	return out, nil
}

func int64ArrayToLua(ls *lua.LState, vals []int64) *lua.LTable {
	t := ls.NewTable()
	for i, v := range vals {
		t.RawSetInt(i+1, lua.LNumber(v))
	}
	return t
}
