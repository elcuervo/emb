package script

import (
	"encoding/binary"
	"errors"
	"math"
	"strings"

	lua "github.com/yuin/gopher-lua"
)

// errPackedLength reports a packed float32 operand whose byte length is not a
// multiple of 4 (and therefore cannot be a float32 buffer).
var errPackedLength = errors.New("packed float32 buffer length must be a multiple of 4")

// looksLikeImageURL reports whether an image argument is a URL or data: URI
// rather than raw image bytes. emb never fetches remote data, so these are
// rejected with guidance instead of being decoded, mirroring EMB.IMG.
func looksLikeImageURL(b []byte) bool {
	for _, prefix := range []string{"http://", "https://", "data:"} {
		if len(b) >= len(prefix) && strings.EqualFold(string(b[:len(prefix)]), prefix) {
			return true
		}
	}
	return false
}

// imageEmbedHost implements emb.image.embed(bytes) / emb.image.embed({bytes...})
// returning pooled normalized image embeddings from the model's image branch,
// in the same space as emb.embed. Each argument is raw encoded image bytes;
// URLs are rejected.
func imageEmbedHost(ls *lua.LState, ih *ImageHost) int {
	if ih.Embed == nil {
		ls.RaiseError("emb.image.embed is unavailable for this model")
		return 0
	}

	var images [][]byte
	single := false
	switch v := ls.Get(1).(type) {
	case lua.LString:
		images = [][]byte{[]byte(v)}
		single = true
	case *lua.LTable:
		n := v.Len()
		if n == 0 {
			ls.RaiseError("emb.image.embed: empty image array")
			return 0
		}
		images = make([][]byte, n)
		for i := 1; i <= n; i++ {
			s, ok := v.RawGetInt(i).(lua.LString)
			if !ok {
				ls.RaiseError("emb.image.embed: item %d is not an image byte string", i)
				return 0
			}
			images[i-1] = []byte(s)
		}
	default:
		ls.RaiseError("emb.image.embed: expected image bytes or an array of image bytes")
		return 0
	}
	for i, b := range images {
		if looksLikeImageURL(b) {
			ls.RaiseError("emb.image.embed: argument %d looks like a URL; supply the raw encoded image bytes (the client fetches)", i+1)
			return 0
		}
	}

	packed := false
	if opts, ok := ls.Get(2).(*lua.LTable); ok {
		if b, ok := opts.RawGetString("bytes").(lua.LBool); ok {
			packed = bool(b)
		}
	}

	embs, err := ih.Embed(images)
	if err != nil {
		ls.RaiseError("emb.image.embed: %v", err)
		return 0
	}
	if len(embs) != len(images) {
		ls.RaiseError("emb.image.embed: got %d embeddings for %d images", len(embs), len(images))
		return 0
	}

	var total int64
	for _, e := range embs {
		total += int64(len(e) / 4)
	}
	if err := requestBudget(ls).charge(total); err != nil {
		ls.RaiseError("emb.image.embed: %v", err)
		return 0
	}

	if single {
		ls.Push(embedValue(ls, embs[0], packed))
		return 1
	}
	out := ls.NewTable()
	for i, e := range embs {
		out.RawSetInt(i+1, embedValue(ls, e, packed))
	}
	ls.Push(out)
	return 1
}

// embedHost implements emb.embed:
//
//	emb.embed(text)                       -> {v1, v2, ... vdim}
//	emb.embed({text1, text2, ...})        -> {{...}, {...}}
//	emb.embed(textOrArray, {bytes=true})  -> packed float32 Lua string(s)
//
// The embeddings come from the server's embedding path (pool + batcher +
// cache), so a text embedded here and by the EMB command shares one cache
// entry. Output form mirrors emb.run's float32 byte layout, so a packed vector
// is byte-compatible with emb.math.float32_bytes and with the `bytes` field of
// an emb.run input spec.
func embedHost(ls *lua.LState, h Hosts) int {
	if h.Embed == nil {
		ls.RaiseError("emb.embed is unavailable for this model")
		return 0
	}

	var texts []string
	single := false
	switch v := ls.Get(1).(type) {
	case lua.LString:
		texts = []string{string(v)}
		single = true
	case *lua.LTable:
		n := v.Len()
		if n == 0 {
			ls.RaiseError("emb.embed: empty text array")
			return 0
		}
		texts = make([]string, n)
		for i := 1; i <= n; i++ {
			s, ok := v.RawGetInt(i).(lua.LString)
			if !ok {
				ls.RaiseError("emb.embed: item %d is not a string", i)
				return 0
			}
			texts[i-1] = string(s)
		}
	default:
		ls.RaiseError("emb.embed: expected a string or an array of strings")
		return 0
	}

	packed := false
	if opts, ok := ls.Get(2).(*lua.LTable); ok {
		if b, ok := opts.RawGetString("bytes").(lua.LBool); ok {
			packed = bool(b)
		}
	}

	embs, err := h.Embed(texts)
	if err != nil {
		ls.RaiseError("emb.embed: %v", err)
		return 0
	}
	if len(embs) != len(texts) {
		ls.RaiseError("emb.embed: got %d embeddings for %d texts", len(embs), len(texts))
		return 0
	}

	// Charge the evaluation's tensor budget: the embeddings materialize into
	// the sandbox exactly like emb.run outputs do.
	var total int64
	for _, e := range embs {
		total += int64(len(e) / 4)
	}
	if err := requestBudget(ls).charge(total); err != nil {
		ls.RaiseError("emb.embed: %v", err)
		return 0
	}

	if single {
		ls.Push(embedValue(ls, embs[0], packed))
		return 1
	}
	out := ls.NewTable()
	for i, e := range embs {
		out.RawSetInt(i+1, embedValue(ls, e, packed))
	}
	ls.Push(out)
	return 1
}

// embedValue renders one embedding as either a packed little-endian float32
// Lua string or an array of Lua numbers.
func embedValue(ls *lua.LState, emb []byte, packed bool) lua.LValue {
	if packed {
		return lua.LString(emb)
	}
	n := len(emb) / 4
	t := ls.CreateTable(0, n)
	for i := 0; i < n; i++ {
		t.RawSetInt(i+1, lua.LNumber(math.Float32frombits(binary.LittleEndian.Uint32(emb[i*4:]))))
	}
	return t
}

// unpackFloat32 decodes a packed little-endian float32 Lua string into a Go
// slice. It is the inverse of embedValue's packed form and of
// emb.math.float32_bytes; the operand validators share it.
func unpackFloat32(s string) ([]float32, error) {
	raw := []byte(s)
	if len(raw)%4 != 0 {
		return nil, errPackedLength
	}
	out := make([]float32, len(raw)/4)
	for i := range out {
		out[i] = math.Float32frombits(binary.LittleEndian.Uint32(raw[i*4:]))
	}
	return out, nil
}
