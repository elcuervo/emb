package server

import (
	"encoding/binary"
	"math"
	"strconv"
	"strings"
	"testing"
)

// blobFloats decodes a float32-LE embedding blob (the BLOB reply payload).
func blobFloats(b []byte) []float64 {
	out := make([]float64, 0, len(b)/4)
	for i := 0; i+4 <= len(b); i += 4 {
		out = append(out, float64(math.Float32frombits(binary.LittleEndian.Uint32(b[i:]))))
	}
	return out
}

// valuesOf walks a parsed flat-pairs reply and returns the values array tokens.
func valuesOf(t *testing.T, tok respToken) []respToken {
	t.Helper()
	arr, ok := tok.val.([]respToken)
	if !ok || len(arr) != 6 {
		t.Fatalf("expected 6-element envelope array, got %+v", tok)
	}
	return arr[5].val.([]respToken)
}

func TestEMBFormatGrammarRESP2(t *testing.T) {
	addr := serveTest(t)
	c := dial(t, addr)
	defer c.Close()

	send := func(args ...string) string {
		c.Write(respCommand(args...))
		return readRESP(t, c)
	}

	// Default is the binary blob; explicit BLOB keyword is byte-identical.
	def := send("EMB", "test", "hello")
	if !strings.HasPrefix(def, "$16\r\n") {
		t.Fatalf("expected 16-byte bulk (dim 4 × f32), got %q", def)
	}
	if b := send("EMB", "test", "BLOB", "hello"); b != def {
		t.Fatalf("explicit BLOB differs from default: %q vs %q", b, def)
	}

	// VALUES envelopes into a flat 6-element pair array.
	v := send("EMB", "test", "VALUES", "hello")
	tok := parseRESP(t, v)
	if tok.kind != "array" {
		t.Fatalf("expected array envelope, got %q", v)
	}
	arr := tok.val.([]respToken)
	if len(arr) != 6 {
		t.Fatalf("expected 6 envelope elements, got %d in %q", len(arr), v)
	}
	if arr[0].val != "dtype" || arr[1].val != "FLOAT" {
		t.Fatalf("expected dtype FLOAT, got %v / %v", arr[0], arr[1])
	}
	shape := arr[3].val.([]respToken)
	if shape[0].val.(int) != 1 || shape[1].val.(int) != 4 {
		t.Fatalf("expected shape [1 4], got %v", shape)
	}
	vals := arr[5].val.([]respToken)
	if len(vals) != 4 {
		t.Fatalf("expected 4 values, got %v", vals)
	}

	// Single text named like the keyword embeds normally (arity guard).
	if r := send("EMB", "test", "VALUES"); !strings.HasPrefix(r, "$16\r\n") {
		t.Fatalf("single-text VALUES should embed text, got %q", r)
	}
	// A keyword in the tail is plain text.
	if r := send("EMB", "test", "hello", "VALUES", "world"); !strings.HasPrefix(r, "*3\r\n") {
		t.Fatalf("tail VALUES should be a text, got %q", r)
	}
}

func TestEMBValuesMatchesBlob(t *testing.T) {
	addr := serveTest(t)
	c := dial(t, addr)
	defer c.Close()

	blobReply := func() []byte {
		c.Write(respCommand("EMB", "test", "hello"))
		raw := readRESP(t, c)
		n := strings.Index(raw, "\r\n")
		if n < 2 || raw[0] != '$' {
			t.Fatalf("expected bulk, got %q", raw)
		}
		return []byte(raw[n+2 : len(raw)-2])
	}

	// Fidelity: VALUES floats (decimal, widened f64) round-trip to the same
	// float32 values as the BLOB path.
	c.Write(respCommand("EMB", "test", "VALUES", "hello"))
	tok := parseRESP(t, readRESP(t, c))
	arr := tok.val.([]respToken)
	vals := arr[5].val.([]respToken)
	got := make([]float64, len(vals))
	for i, v := range vals {
		f, err := strconv.ParseFloat(v.val.(string), 64)
		if err != nil {
			t.Fatalf("bad decimal value %v", v)
		}
		got[i] = f
	}
	want := blobFloats(blobReply())
	if len(got) != len(want) {
		t.Fatalf("values count %d != blob dims %d", len(got), len(want))
	}
	for i := range got {
		// Downcast comparison: the text is shortest-f64 of the widened f32.
		if float32(got[i]) != float32(want[i]) {
			t.Fatalf("dim %d: VALUES %v != BLOB %v", i, got[i], want[i])
		}
	}
}

func TestRESP3ValuesTypedDoubles(t *testing.T) {
	addr := serveTest(t)
	c := dial(t, addr)
	defer c.Close()

	c.Write(respCommand("HELLO", "3"))
	readRESP(t, c)

	c.Write(respCommand("EMB", "test", "VALUES", "hello"))
	raw := readRESP(t, c)
	if !strings.HasPrefix(raw, "%3\r\n$5\r\ndtype\r\n$5\r\nFLOAT\r\n") {
		t.Fatalf("expected RESP3 map envelope, got %q", raw)
	}
	// Values must be typed doubles: ,<d>\r\n entries.
	idx := strings.Index(raw, "$6\r\nvalues\r\n")
	rest := raw[idx+len("$6\r\nvalues\r\n"):]
	if !strings.HasPrefix(rest, "*4\r\n") {
		t.Fatalf("expected 4 values, got %q", rest)
	}
	rest = rest[len("*4\r\n"):]
	for i := 0; i < 4; i++ {
		if rest[0] != ',' {
			t.Fatalf("value %d is not a typed double: %q", i, rest)
		}
		lineEnd := strings.Index(rest, "\r\n")
		if _, err := strconv.ParseFloat(rest[1:lineEnd], 64); err != nil {
			t.Fatalf("bad double text %q", rest[:lineEnd])
		}
		rest = rest[lineEnd+2:]
	}
}

func TestEMBMULTIFormatGrammar(t *testing.T) {
	addr := serveTest(t)
	c := dial(t, addr)
	defer c.Close()

	send := func(args ...string) string {
		c.Write(respCommand(args...))
		return readRESP(t, c)
	}

	// BLOB default unchanged: array of blobs.
	r := send("EMB.MULTI", "test", "a", "test", "b")
	if !strings.HasPrefix(r, "*2\r\n$16\r\n") {
		t.Fatalf("expected array of bulks, got %q", r)
	}

	// VALUES: per-pair envelopes including model; failures stay null.
	r = send("EMB.MULTI", "VALUES", "test", "a")
	tok := parseRESP(t, r)
	arr := tok.val.([]respToken)
	if len(arr) != 1 {
		t.Fatalf("expected 1 element, got %q", r)
	}
	pair := arr[0].val.([]respToken)
	if len(pair) != 8 {
		t.Fatalf("expected 8-element envelope (model,dtype,shape,values), got %q", r)
	}
	if pair[0].val != "model" || pair[1].val != "test" {
		t.Fatalf("expected model key, got %v / %v", pair[0], pair[1])
	}

	r = send("EMB.MULTI", "VALUES", "test", "a", "nonexistent", "b")
	tok = parseRESP(t, r)
	arr = tok.val.([]respToken)
	if len(arr) != 2 || arr[0].kind != "array" || arr[1].kind != "bulk" {
		t.Fatalf("expected [envelope null], got %q", r)
	}
}

func TestEMBValuesTruncationShape(t *testing.T) {
	addr, _ := serveTestWithOptions(t, WithMaxTexts(1))
	c := dial(t, addr)
	defer c.Close()

	c.Write(respCommand("EMB", "test", "VALUES", "a", "b"))
	tok := parseRESP(t, readRESP(t, c))
	arr := tok.val.([]respToken)
	shape := arr[3].val.([]respToken)
	if shape[0].val.(int) != 1 {
		t.Fatalf("truncated VALUES should report shape[0]=1 (processed), got %v", shape)
	}
	if shape[1].val.(int) != 4 {
		t.Fatalf("expected dim 4, got %v", shape)
	}
	vals := arr[5].val.([]respToken)
	if len(vals) != 4 {
		t.Fatalf("expected 1×4 values, got %d", len(vals))
	}
}

func TestEMBHelpMentionsFormats(t *testing.T) {
	addr := serveTest(t)
	c := dial(t, addr)
	defer c.Close()

	c.Write(respCommand("EMB.HELP"))
	raw := readRESP(t, c)
	if !strings.Contains(raw, "BLOB|VALUES") {
		t.Fatalf("EMB.HELP should document BLOB|VALUES, got %q", raw)
	}
}

func debugEnvelope(t *testing.T, addr string, args ...string) string {
	t.Helper()
	c := dial(t, addr)
	defer c.Close()
	c.Write(respCommand(args...))
	return readRESP(t, c)
}

func TestEMBFormatArityErrors(t *testing.T) {
	addr := serveTest(t)
	r := debugEnvelope(t, addr, "EMB", "test", "VALUES", "a", "b")
	if !strings.HasPrefix(r, "*") {
		t.Fatalf("expected array envelope, got %q", r)
	}
	// VALUES with two texts: shape [2,4] and 8 values.
	tok := parseRESP(t, r)
	arr := tok.val.([]respToken)
	shape := arr[3].val.([]respToken)
	if shape[0].val.(int) != 2 || shape[1].val.(int) != 4 {
		t.Fatalf("expected shape [2 4], got %v", shape)
	}
	if len(arr[5].val.([]respToken)) != 8 {
		t.Fatalf("expected 8 values for 2 texts × 4 dims, got %q", r)
	}
}

func TestEMBMultiValuesMixed(t *testing.T) {
	addr := serveTest(t)
	c := dial(t, addr)
	defer c.Close()
	c.Write(respCommand("EMB.MULTI", "VALUES", "test", "a", "test", "b"))
	raw := readRESP(t, c)
	tok := parseRESP(t, raw)
	arr := tok.val.([]respToken)
	if len(arr) != 2 {
		t.Fatalf("expected 2 envelopes, got %q", raw)
	}
	for i, pair := range arr {
		p := pair.val.([]respToken)
		if p[1].val != "test" {
			t.Fatalf("envelope %d model mismatch: %v", i, p[1])
		}
		shape := p[5].val.([]respToken)
		if shape[0].val.(int) != 1 || shape[1].val.(int) != 4 {
			t.Fatalf("envelope %d shape %v", i, shape)
		}
	}
}
