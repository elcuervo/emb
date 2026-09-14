package script

import (
	"encoding/binary"
	"encoding/json"
	"fmt"
	"math"
	"sort"

	lua "github.com/yuin/gopher-lua"

	"github.com/elcuervo/emb/internal/imageproc"
	"github.com/elcuervo/emb/internal/onnx"
	"github.com/elcuervo/emb/internal/tokenizer"
)

// maxFillElements bounds the number of elements a single fill tensor may
// allocate (16M ≈ 64MB at 4 bytes/element). Fill shapes are script-controlled
// and multiplied host-side, so this keeps an unbounded allocation from a
// pathological spec from exhausting server memory.
const maxFillElements = 16 * 1024 * 1024

// maxRequestElements bounds the total tensor elements a single evaluation may
// allocate across all emb.run / emb.run_batch host calls (64M ≈ 256MB at 4
// bytes/element). Data tensors, fill tensors, and merged batch tensors all
// charge this budget, so a script cannot sidestep the per-tensor cap by
// issuing many specs (e.g. a large emb.run_batch of individually valid fills)
// or many emb.run calls in a loop.
const maxRequestElements = 64 * 1024 * 1024

// DefaultMaxRequestElements is the default per-evaluation tensor element
// budget (EvalOptions.MaxTensorElements when unset).
const DefaultMaxRequestElements = int64(maxRequestElements)

// tensorBudget tracks an evaluation's remaining tensor element allowance. It
// hangs off the evaluation context so every host call within one evaluation
// shares a single budget.
type tensorBudget struct{ remaining int64 }

type tensorBudgetCtxKey struct{}

func newTensorBudget(remaining int64) *tensorBudget { return &tensorBudget{remaining: remaining} }

// requestBudget returns the evaluation's budget from the Lua context, or a
// fresh one when the evaluation bypasses runProto (defensive; all server and
// unit paths go through runProto).
func requestBudget(ls *lua.LState) *tensorBudget {
	if ctx := ls.Context(); ctx != nil {
		if b, ok := ctx.Value(tensorBudgetCtxKey{}).(*tensorBudget); ok {
			return b
		}
	}
	return newTensorBudget(DefaultMaxRequestElements)
}

// charge consumes count elements, enforcing the per-tensor cap and the
// request-wide allowance before the caller allocates.
func (b *tensorBudget) charge(count int64) error {
	if count > maxFillElements {
		return fmt.Errorf("tensor exceeds max elements (%d)", maxFillElements)
	}
	if count > b.remaining {
		return fmt.Errorf("request exceeds total tensor element limit (%d)", maxRequestElements)
	}
	b.remaining -= count
	return nil
}

// Hosts binds a script evaluation to the resources it may touch: the model's
// named-tensor session and its tokenizer capabilities. All are optional; a
// binding with only Run lets scripts do arbitrary graph IO without
// tokenization, and vice versa. The zero value disables all host functions.
type Hosts struct {
	// Run executes one inference over named tensors and returns the graph's
	// named outputs. Nil makes emb.run unavailable.
	Run func(inputs []onnx.NamedTensor) (map[string]onnx.NamedTensor, error)
	// Embed returns pooled, normalized embeddings for the given texts through
	// the model's embedding path (the same path the EMB command uses, so
	// results share the embedding cache). Nil makes emb.embed unavailable;
	// host bindings leave it nil for models without an embedding config.
	Embed func(texts []string) ([][]byte, error)
	// EncodePretokenized word-encodes an already-split word list (emb.tokenize.pretokenized).
	EncodePretokenized func(words []string, maxLen int) (ids, wordIDs []int64, err error)
	// EncodePlain encodes a single text through the model tokenizer's own
	// pipeline with per-token byte offsets (emb.tokenize.encode).
	EncodePlain func(text string, maxLen int) (ids, mask []int64, offsets [][2]int, err error)
	// EncodePair composes the BERT-family pair template with per-part offsets
	// (emb.tokenize.encode_pair).
	EncodePair func(first, second string, maxLen int) (ids, mask []int64, offsets [][2]int, sep int, err error)
	// Image binds the model's image preprocessing plan to the sandbox
	// (emb.image.preprocess / emb.image.info). Nil leaves emb.image absent.
	Image *ImageHost
}

// ImageHost exposes a model's image preprocessing to scripts. Plan resolves
// the model's immutable preprocessing plan lazily, without opening inference
// sessions, so emb.image.info can report configuration while image sessions
// stay unopened. Preprocess decodes raw image bytes through the plan and
// returns the channel-first float32 tensor. Preprocessing is pure compute, so
// script replies remain deterministic and cacheable.
type ImageHost struct {
	Plan       func() (imageproc.Plan, error)
	Preprocess func(data []byte) ([]float32, error)
	// Embed returns pooled, normalized image embeddings for raw encoded image
	// bytes, in the model's shared text/image embedding space. Nil makes
	// emb.image.embed unavailable.
	Embed func(images [][]byte) ([][]byte, error)
}

// registerHosts installs the whitelisted emb.* and json host functions into
// the sandbox. Scripts may only reach models and tokenizers through these; no
// other host surface exists.
func registerHosts(ls *lua.LState, h Hosts) {
	emb := ls.NewTable()
	emb.RawSetString("run", ls.NewFunction(func(ls *lua.LState) int {
		return runHost(ls, h)
	}))
	// emb.embed is registered only when the model can produce embeddings, so
	// capability detection via type(emb.embed) matches what a script can call.
	if h.Embed != nil {
		emb.RawSetString("embed", ls.NewFunction(func(ls *lua.LState) int {
			return embedHost(ls, h)
		}))
	}
	emb.RawSetString("similarity", ls.NewFunction(similarityHost))
	emb.RawSetString("distance", ls.NewFunction(distanceHost))
	emb.RawSetString("API_VERSION", lua.LString(APIVersion))
	emb.RawSetString("run_batch", ls.NewFunction(func(ls *lua.LState) int {
		return runBatchHost(ls, h)
	}))
	tok := ls.NewTable()
	tok.RawSetString("pretokenized", ls.NewFunction(func(ls *lua.LState) int {
		return tokenizeHost(ls, h)
	}))
	tok.RawSetString("words", ls.NewFunction(tokenizeWordsHost))
	tok.RawSetString("encode", ls.NewFunction(func(ls *lua.LState) int {
		return encodeHost(ls, h)
	}))
	tok.RawSetString("encode_pair", ls.NewFunction(func(ls *lua.LState) int {
		return encodePairHost(ls, h)
	}))
	emb.RawSetString("tokenize", tok)
	registerMath(emb, ls)
	if h.Image != nil {
		img := ls.NewTable()
		img.RawSetString("preprocess", ls.NewFunction(func(ls *lua.LState) int {
			return imagePreprocessHost(ls, h.Image)
		}))
		img.RawSetString("info", ls.NewFunction(func(ls *lua.LState) int {
			return imageInfoHost(ls, h.Image)
		}))
		if h.Image.Embed != nil {
			img.RawSetString("embed", ls.NewFunction(func(ls *lua.LState) int {
				return imageEmbedHost(ls, h.Image)
			}))
		}
		emb.RawSetString("image", img)
	}
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
// float32. Input specs also accept fill for constant tensors (see
// namedTensorFromLua). Field names are processed in sorted order so identical
// tables map to identical tensor orders.
func runHost(ls *lua.LState, h Hosts) int {
	if h.Run == nil {
		ls.RaiseError("emb.run is unavailable for this model")
		return 0
	}
	arg := ls.CheckTable(1)
	opts, err := parseRunOptions(ls, 2)
	if err != nil {
		ls.RaiseError("emb.run: %v", err)
		return 0
	}
	var names []string
	arg.ForEach(func(k, _ lua.LValue) {
		if s, ok := k.(lua.LString); ok {
			names = append(names, string(s))
		}
	})
	sort.Strings(names)

	inputs := make([]onnx.NamedTensor, 0, len(names))
	budget := requestBudget(ls)
	for _, name := range names {
		spec, ok := arg.RawGetString(name).(*lua.LTable)
		if !ok {
			ls.RaiseError("emb.run: input %q must be a table {shape=..., data=...|fill=...}", name)
			return 0
		}
		t, err := namedTensorFromLua(spec, budget)
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

	outNames, err := selectOutputNames(outputs, opts)
	if err != nil {
		ls.RaiseError("emb.run: %v", err)
		return 0
	}
	if err := chargeOutputs(ls, outputs, outNames); err != nil {
		ls.RaiseError("emb.run: %v", err)
		return 0
	}
	result := ls.NewTable()
	for _, n := range outNames {
		result.RawSetString(n, renderTensor(ls, outputs[n], opts.packed))
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

// encodeHost implements emb.tokenize.encode(text, max_len) → {ids, mask,
// offsets}: the model tokenizer's own pipeline (special tokens included),
// with per-token byte offsets so scripts slice surface text directly.
func encodeHost(ls *lua.LState, h Hosts) int {
	if h.EncodePlain == nil {
		ls.RaiseError("emb.tokenize.encode is unavailable for this model")
		return 0
	}
	text := ls.CheckString(1)
	maxLen := ls.OptInt(2, 0)
	ids, mask, offsets, err := h.EncodePlain(text, maxLen)
	if err != nil {
		ls.RaiseError("emb.tokenize.encode: %v", err)
		return 0
	}
	result := ls.NewTable()
	result.RawSetString("ids", int64ArrayToLua(ls, ids))
	result.RawSetString("mask", int64ArrayToLua(ls, mask))
	result.RawSetString("offsets", offsetsToLua(ls, offsets))
	ls.Push(result)
	return 1
}

// encodePairHost implements emb.tokenize.encode_pair(first, second, max_len)
// → {ids, mask, offsets, sep}: the BERT-family pair template [CLS] a [SEP] b
// [SEP], with per-part byte offsets and the 1-based sep position.
func encodePairHost(ls *lua.LState, h Hosts) int {
	if h.EncodePair == nil {
		ls.RaiseError("emb.tokenize.encode_pair is unavailable for this model")
		return 0
	}
	first := ls.CheckString(1)
	second := ls.CheckString(2)
	maxLen := ls.OptInt(3, 0)
	ids, mask, offsets, sep, err := h.EncodePair(first, second, maxLen)
	if err != nil {
		ls.RaiseError("emb.tokenize.encode_pair: %v", err)
		return 0
	}
	result := ls.NewTable()
	result.RawSetString("ids", int64ArrayToLua(ls, ids))
	result.RawSetString("mask", int64ArrayToLua(ls, mask))
	result.RawSetString("offsets", offsetsToLua(ls, offsets))
	result.RawSetString("sep", lua.LNumber(sep))
	ls.Push(result)
	return 1
}

// imagePreprocessHost implements emb.image.preprocess(bytes): it decodes raw
// image bytes through the model's plan and returns the packed tensor spec
// ({shape, bytes, dtype, input}) that emb.run accepts without a per-element
// Lua table. The tensor is charged against the evaluation's tensor budget, and
// the plan's byte/pixel caps apply exactly as for EMB.IMG.
func imagePreprocessHost(ls *lua.LState, ih *ImageHost) int {
	if ih.Preprocess == nil {
		ls.RaiseError("emb.image.preprocess is unavailable for this model")
		return 0
	}
	plan, err := ih.Plan()
	if err != nil {
		ls.RaiseError("emb.image.preprocess: %v", err)
		return 0
	}
	data := ls.CheckString(1)
	tensor, err := ih.Preprocess([]byte(data))
	if err != nil {
		ls.RaiseError("emb.image.preprocess: %v", err)
		return 0
	}
	if err := requestBudget(ls).charge(int64(len(tensor))); err != nil {
		ls.RaiseError("emb.image.preprocess: %v", err)
		return 0
	}
	buf := make([]byte, 0, 4*len(tensor))
	for _, v := range tensor {
		buf = binary.LittleEndian.AppendUint32(buf, math.Float32bits(v))
	}
	shape := ls.NewTable()
	for i, d := range plan.Shape() {
		shape.RawSetInt(i+1, lua.LNumber(d))
	}
	result := ls.NewTable()
	result.RawSetString("shape", shape)
	result.RawSetString("bytes", lua.LString(buf))
	result.RawSetString("dtype", lua.LString("f32"))
	result.RawSetString("input", lua.LString(plan.Input))
	ls.Push(result)
	return 1
}

// imageInfoHost implements emb.image.info(): the model's configured
// preprocessing parameters. It resolves the plan lazily without opening image
// sessions.
func imageInfoHost(ls *lua.LState, ih *ImageHost) int {
	plan, err := ih.Plan()
	if err != nil {
		ls.RaiseError("emb.image.info: %v", err)
		return 0
	}
	result := ls.NewTable()
	result.RawSetString("input", lua.LString(plan.Input))
	result.RawSetString("size", lua.LNumber(plan.Size))
	result.RawSetString("crop", lua.LString(plan.Crop.String()))
	result.RawSetString("resample", lua.LString(plan.Resample.String()))
	result.RawSetString("rescale", lua.LNumber(plan.Rescale))
	mean := ls.NewTable()
	for i, v := range plan.Mean {
		mean.RawSetInt(i+1, lua.LNumber(v))
	}
	std := ls.NewTable()
	for i, v := range plan.Std {
		std.RawSetInt(i+1, lua.LNumber(v))
	}
	result.RawSetString("mean", mean)
	result.RawSetString("std", std)
	ls.Push(result)
	return 1
}

// offsetsToLua renders [][2]int spans as an array of {start, end} pairs.
func offsetsToLua(ls *lua.LState, offsets [][2]int) *lua.LTable {
	out := ls.NewTable()
	for i, off := range offsets {
		pair := ls.NewTable()
		pair.RawSetInt(1, lua.LNumber(off[0]))
		pair.RawSetInt(2, lua.LNumber(off[1]))
		out.RawSetInt(i+1, pair)
	}
	return out
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

// namedTensorFromLua reads an input spec in one of two forms:
//
//	{shape = {n...}, data = {...}, dtype?} — element-wise values
//	{shape = {n...}, fill = n, dtype?} — every element equal to n
//
// data with any fractional element becomes a float32 tensor, otherwise int64.
// fill constructs the constant tensor host-side from the shape alone (no Lua
// data table round-trip); a fractional fill infers float32. An explicit dtype
// overrides both inference rules (zero-filled float tensors like fused-CLIP
// pixel_values are all-integral and would misinfer as int64). fill and data
// are mutually exclusive.
func namedTensorFromLua(spec *lua.LTable, budget *tensorBudget) (onnx.NamedTensor, error) {
	var t onnx.NamedTensor
	if shapeTab, ok := spec.RawGetString("shape").(*lua.LTable); ok {
		shape, err := int64ArrayFromLua(shapeTab)
		if err != nil {
			return t, fmt.Errorf("shape: %w", err)
		}
		if len(shape) == 0 {
			return t, fmt.Errorf("shape must have at least one dimension")
		}
		t.Shape = shape
	} else {
		return t, fmt.Errorf("missing shape")
	}

	var explicitDType string
	if dtype, ok := spec.RawGetString("dtype").(lua.LString); ok {
		explicitDType = string(dtype)
		if explicitDType != "f32" && explicitDType != "i64" {
			return t, fmt.Errorf("dtype must be i64 or f32, got %q", explicitDType)
		}
	}

	dataTab, hasData := spec.RawGetString("data").(*lua.LTable)
	fillField := spec.RawGetString("fill")
	hasFill := fillField != lua.LNil
	fillVal, fillIsNumber := fillField.(lua.LNumber)
	if hasFill && !fillIsNumber {
		return t, fmt.Errorf("fill must be a number")
	}
	bytesField := spec.RawGetString("bytes")
	hasBytes := bytesField != lua.LNil
	provided := 0
	if hasData {
		provided++
	}
	if hasFill {
		provided++
	}
	if hasBytes {
		provided++
	}
	if provided > 1 {
		return t, fmt.Errorf("data, fill, and bytes are mutually exclusive")
	}
	if provided == 0 {
		return t, fmt.Errorf("spec must provide exactly one of data, fill, or bytes")
	}

	// Packed bytes form: little-endian raw elements, the inverse of
	// emb.math.float32_bytes. dtype is required (no inference from content) and
	// the length must match the shape exactly.
	if hasBytes {
		if explicitDType == "" {
			return t, fmt.Errorf("bytes requires an explicit dtype (\"f32\" or \"i64\")")
		}
		raw, ok := bytesField.(lua.LString)
		if !ok {
			return t, fmt.Errorf("bytes must be a string")
		}
		count, err := shapeElementCount(t.Shape)
		if err != nil {
			return t, fmt.Errorf("bytes: %w", err)
		}
		t.DType = dtypeFromString(explicitDType)
		width := int64(dtypeWidth(t.DType))
		if want := count * width; int64(len(raw)) != want {
			return t, fmt.Errorf("bytes length %d does not match shape element count %d × %d bytes (%d)", len(raw), count, width, want)
		}
		if err := budget.charge(count); err != nil {
			return t, fmt.Errorf("bytes: %w", err)
		}
		b := []byte(raw)
		switch t.DType {
		case onnx.TensorFloat32:
			t.Float = make([]float32, count)
			for i := range t.Float {
				t.Float[i] = math.Float32frombits(binary.LittleEndian.Uint32(b[i*4:]))
			}
		default:
			t.Int64 = make([]int64, count)
			for i := range t.Int64 {
				t.Int64[i] = int64(binary.LittleEndian.Uint64(b[i*8:]))
			}
		}
		return t, nil
	}

	// Data form: read the element array and infer int64/float32; the session
	// layer checks the count against the shape at run time.
	if hasData {
		data, err := numberArrayFromLua(dataTab)
		if err != nil {
			return t, fmt.Errorf("data: %w", err)
		}
		if err := budget.charge(int64(len(data))); err != nil {
			return t, fmt.Errorf("data: %w", err)
		}
		t.DType = dtypeFor(explicitDType, data)
		switch t.DType {
		case onnx.TensorFloat32:
			t.Float = make([]float32, len(data))
			for i, n := range data {
				t.Float[i] = float32(n)
			}
		default:
			t.Int64 = make([]int64, len(data))
			for i, n := range data {
				t.Int64[i] = int64(n)
			}
		}
		return t, nil
	}

	// Fill form: construct the constant tensor directly from the shape (the
	// count is exact by construction, so no Lua data table is ever built).
	// The shape is script-controlled, so bound the element count with the same
	// checked primitive every other allocation path uses.
	count, err := shapeElementCount(t.Shape)
	if err != nil {
		return t, err
	}
	if err := budget.charge(count); err != nil {
		return t, fmt.Errorf("fill: %w", err)
	}
	t.DType = dtypeFor(explicitDType, []float64{float64(fillVal)})
	switch t.DType {
	case onnx.TensorFloat32:
		t.Float = make([]float32, int(count))
		for i := range t.Float {
			t.Float[i] = float32(fillVal)
		}
	default:
		t.Int64 = make([]int64, int(count))
		for i := range t.Int64 {
			t.Int64[i] = int64(fillVal)
		}
	}
	return t, nil
}

// shapeElementCount returns the product of a tensor shape's dimensions with
// checked multiplication, so a script-controlled shape cannot overflow the
// element count before an allocation is sized.
func shapeElementCount(shape []int64) (int64, error) {
	count := int64(1)
	for _, d := range shape {
		if d < 0 {
			return 0, fmt.Errorf("negative shape dimension %d", d)
		}
		if d == 0 {
			return 0, nil
		}
		if count > math.MaxInt64/d {
			return 0, fmt.Errorf("shape element count overflows")
		}
		count *= d
	}
	return count, nil
}

// dtypeFor resolves a tensor's dtype: an explicit dtype wins; otherwise data
// infers int64, promoting to float32 on any fractional element, and a fill
// value infers float32 when it is fractional.
func dtypeFor(explicit string, data []float64) onnx.TensorType {
	if explicit != "" {
		return dtypeFromString(explicit)
	}
	dt := onnx.TensorInt64
	for _, n := range data {
		if math.Trunc(n) != n {
			dt = onnx.TensorFloat32
			break
		}
	}
	return dt
}

func dtypeFromString(s string) onnx.TensorType {
	if s == "f32" {
		return onnx.TensorFloat32
	}
	return onnx.TensorInt64
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
		// A Lua table cannot hold nil, so a JSON null element stores the
		// json.null sentinel exactly as the object branch does; encoding the
		// sentinel reproduces null, so arrays round-trip too.
		tbl := ls.NewTable()
		null := jsonNullSentinel(ls)
		for i, e := range t {
			if e == nil {
				tbl.RawSetInt(i+1, null)
			} else {
				tbl.RawSetInt(i+1, anyToLuaValue(ls, e))
			}
		}
		// An empty Lua table is otherwise indistinguishable from an empty
		// object, so mark the decoded array in its metatable; isListTable
		// consults the marker for entry-less tables.
		markJSONArray(ls, tbl)
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
	if entries == 0 {
		// An empty table carries no key evidence either way; only a decoded
		// JSON array is marked, so empty objects still encode as {}.
		return isMarkedJSONArray(t)
	}
	return intKeys == entries && entries == t.Len()
}

// jsonArrayMarker lives in the metatable of a table decoded from a JSON array.
// The marker never appears among the table's own keys, so it cannot leak into
// an encoded object, and it survives an empty array (which has no entries to
// distinguish it from an empty object).
const jsonArrayMarker = "__emb_json_array"

func markJSONArray(ls *lua.LState, t *lua.LTable) {
	mt := ls.NewTable()
	mt.RawSetString(jsonArrayMarker, lua.LTrue)
	t.Metatable = mt
}

func isMarkedJSONArray(t *lua.LTable) bool {
	mt, ok := t.Metatable.(*lua.LTable)
	if !ok {
		return false
	}
	return lua.LVAsBool(mt.RawGetString(jsonArrayMarker))
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
