package script

import (
	"bytes"
	"fmt"
	"testing"

	lua "github.com/yuin/gopher-lua"
)

// respBuffer emulates the redcon wire formats for exact byte assertions.
type respBuffer struct {
	b bytes.Buffer
}

func (r *respBuffer) WriteBulk(data []byte) {
	fmt.Fprintf(&r.b, "$%d\r\n%s\r\n", len(data), data)
}
func (r *respBuffer) WriteInt(n int) {
	fmt.Fprintf(&r.b, ":%d\r\n", n)
}
func (r *respBuffer) WriteArray(n int) {
	fmt.Fprintf(&r.b, "*%d\r\n", n)
}
func (r *respBuffer) WriteNull() {
	r.b.WriteString("$-1\r\n")
}
func (r *respBuffer) WriteError(msg string) {
	fmt.Fprintf(&r.b, "-%s\r\n", msg)
}

func convertStr(t *testing.T, src string) *respBuffer {
	t.Helper()
	v, err := Eval(src, nil, nil, EvalOptions{})
	if err != nil {
		t.Fatalf("eval %q: %v", src, err)
	}
	buf := &respBuffer{}
	if err := Convert(buf, v); err != nil {
		t.Fatalf("convert %q: %v", src, err)
	}
	return buf
}

func TestConvertStringBulk(t *testing.T) {
	buf := convertStr(t, `return "hello world"`)
	if want := "$11\r\nhello world\r\n"; buf.b.String() != want {
		t.Fatalf("got %q, want %q", buf.b.String(), want)
	}
}

func TestConvertRawBytesBulk(t *testing.T) {
	// Byte-safe strings: raw binary passes through untouched (the float32
	// embedding path relies on this).
	buf := convertStr(t, `return string.char(63, 160, 63, 160)`)
	want := "$4\r\n" + string([]byte{63, 160, 63, 160}) + "\r\n"
	if buf.b.String() != want {
		t.Fatalf("got %q, want %q", buf.b.String(), want)
	}
}

func TestConvertIntegralNumber(t *testing.T) {
	buf := convertStr(t, `return 42`)
	if want := ":42\r\n"; buf.b.String() != want {
		t.Fatalf("got %q, want %q", buf.b.String(), want)
	}
}

func TestConvertFractionalNumberBulk(t *testing.T) {
	buf := convertStr(t, `return 0.9823`)
	if want := "$6\r\n0.9823\r\n"; buf.b.String() != want {
		t.Fatalf("got %q, want %q", buf.b.String(), want)
	}
}

func TestConvertNilAndFalse(t *testing.T) {
	for _, src := range []string{
		`return nil`,
		`return false`,
		`return`,
	} {
		buf := convertStr(t, src)
		if want := "$-1\r\n"; buf.b.String() != want {
			t.Fatalf("src %q: got %q, want %q", src, buf.b.String(), want)
		}
	}
}

func TestConvertListTableToArray(t *testing.T) {
	buf := convertStr(t, `return {"a", "b", "c"}`)
	if want := "*3\r\n$1\r\na\r\n$1\r\nb\r\n$1\r\nc\r\n"; buf.b.String() != want {
		t.Fatalf("got %q, want %q", buf.b.String(), want)
	}
}

func TestConvertHashToFlatPairs(t *testing.T) {
	buf := convertStr(t, `return {PERSON = {"Tim Cook"}, ORG = {"Apple"}}`)
	// Keys sorted alphabetically: ORG first, then PERSON.
	want := "*4\r\n$3\r\nORG\r\n*1\r\n$5\r\nApple\r\n$6\r\nPERSON\r\n*1\r\n$8\r\nTim Cook\r\n"
	if buf.b.String() != want {
		t.Fatalf("got %q, want %q", buf.b.String(), want)
	}
}

func TestConvertNestedHash(t *testing.T) {
	buf := convertStr(t, `return {entities = {PERSON = {"Tim"}}}`)
	// Outer: 2-pair flat array; value of "entities" is itself a hash (1 field
	// → 2 flat elements).
	want := "*2\r\n$8\r\nentities\r\n*2\r\n$6\r\nPERSON\r\n*1\r\n$3\r\nTim\r\n"
	if buf.b.String() != want {
		t.Fatalf("got %q, want %q", buf.b.String(), want)
	}
}

func TestConvertErrTable(t *testing.T) {
	buf := convertStr(t, `return {err = "bad labels"}`)
	if want := "-bad labels\r\n"; buf.b.String() != want {
		t.Fatalf("got %q, want %q", buf.b.String(), want)
	}
}

func TestConvertErrTableRejectsCRLF(t *testing.T) {
	// A CR/LF inside the error message would splice a second RESP frame onto
	// the wire (and, once cached, be replayed verbatim), so it is rejected.
	v, err := Eval(`return {err = "bad\r\n:1"}`, nil, nil, EvalOptions{})
	if err != nil {
		t.Fatal(err)
	}
	buf := &respBuffer{}
	if err := Convert(buf, v); err == nil {
		t.Fatal("expected CR/LF error reply to be rejected")
	}
}

func TestConvertEmptyTable(t *testing.T) {
	buf := convertStr(t, `return {}`)
	if want := "*0\r\n"; buf.b.String() != want {
		t.Fatalf("got %q, want %q", buf.b.String(), want)
	}
}

func TestConvertDeterministicOrder(t *testing.T) {
	// Same table built in different insertion orders must produce identical
	// bytes (content-addressed caching).
	v1, _ := Eval(`return {a = 1, b = {2, 3}, c = "x"}`, nil, nil, EvalOptions{})
	v2, _ := Eval(`return {c = "x", b = {2, 3}, a = 1}`, nil, nil, EvalOptions{})
	b1, b2 := &respBuffer{}, &respBuffer{}
	if err := Convert(b1, v1); err != nil {
		t.Fatal(err)
	}
	if err := Convert(b2, v2); err != nil {
		t.Fatal(err)
	}
	if b1.b.String() != b2.b.String() {
		t.Fatalf("nondeterministic conversion:\n%q\n%q", b1.b.String(), b2.b.String())
	}
	want := "*6\r\n$1\r\na\r\n:1\r\n$1\r\nb\r\n*2\r\n:2\r\n:3\r\n$1\r\nc\r\n$1\r\nx\r\n"
	if b1.b.String() != want {
		t.Fatalf("got %q, want %q", b1.b.String(), want)
	}
}

func TestConvertMixedKeysError(t *testing.T) {
	v, err := Eval(`return {1, 2, x = "y"}`, nil, nil, EvalOptions{})
	if err != nil {
		t.Fatal(err)
	}
	buf := &respBuffer{}
	if err := Convert(buf, v); err == nil {
		t.Fatal("expected mixed-key table to fail conversion")
	}
}

var _ ReplyWriter = (*respBuffer)(nil)
var _ = lua.LNil
