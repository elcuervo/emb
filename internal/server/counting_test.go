package server

import (
	"math"
	"strings"
	"testing"

	"github.com/tidwall/redcon"
)

// fakeConn implements just enough of redcon.Conn for the counting wrapper to
// delegate to: a no-op sink plus a settable protocol version. Everything else
// is inherited from the nil embedded redcon.Conn (never called by the wrapper
// paths under test).
type fakeConn struct {
	redcon.Conn
	ver int
}

func (f *fakeConn) ProtocolVersion() int         { return f.ver }
func (f *fakeConn) SetProtocolVersion(v int)     { f.ver = v }
func (f *fakeConn) WriteError(string)            {}
func (f *fakeConn) WriteString(string)           {}
func (f *fakeConn) WriteBulk([]byte)             {}
func (f *fakeConn) WriteBulkString(string)       {}
func (f *fakeConn) WriteInt(int)                 {}
func (f *fakeConn) WriteInt64(int64)             {}
func (f *fakeConn) WriteUint64(uint64)           {}
func (f *fakeConn) WriteArray(int)               {}
func (f *fakeConn) WriteNull()                   {}
func (f *fakeConn) WriteRaw([]byte)              {}
func (f *fakeConn) WriteAny(interface{})         {}
func (f *fakeConn) WriteDouble(float64)          {}
func (f *fakeConn) WriteBool(bool)               {}
func (f *fakeConn) WriteBigNumber(string)        {}
func (f *fakeConn) WriteVerbatim(string, string) {}
func (f *fakeConn) WriteBlobError(string)        {}
func (f *fakeConn) WriteMap(int)                 {}
func (f *fakeConn) WriteSet(int)                 {}
func (f *fakeConn) WritePush(int)                {}
func (f *fakeConn) WriteAttribute(int)           {}

func newCounting(f *fakeConn) (*countingConn, *Server) {
	s := New("127.0.0.1:0", nil, "", "", nil)
	return &countingConn{Conn: f, s: s}, s
}

// respText mirrors the fork's encoders for the values the wrapper counts, so
// the expected sizes are cross-checked against the real wire format, not
// eyeballed.
// respOps is the reply-writing surface the counted methods share between the
// fork's *Writer and emb's countingConn (whose methods are promoted from
// redcon.Conn), so the same op drives both sides of the cross-check.
type respOps interface {
	WriteBulk([]byte)
	WriteString(string)
	WriteInt(int)
	WriteNull()
	WriteArray(int)
	WriteDouble(float64)
	WriteBool(bool)
	WriteBigNumber(string)
	WriteMap(int)
	WriteSet(int)
	WritePush(int)
	WriteAttribute(int)
	WriteVerbatim(string, string)
	WriteBlobError(string)
}

func respText(ver int, op func(respOps)) string {
	var b strings.Builder
	w := redcon.NewWriter(&b)
	w.SetProtocolVersion(ver)
	op(w)
	w.Flush()
	return b.String()
}

func TestCountingBytesExact(t *testing.T) {
	cases := []struct {
		name string
		ver  int
		w    func(respOps)
		got  int
	}{
		{"null RESP2", 2, func(c respOps) { c.WriteNull() }, 5},
		{"null RESP3", 3, func(c respOps) { c.WriteNull() }, 3},
		{"double RESP3", 3, func(c respOps) { c.WriteDouble(0.1) }, len(",0.1\r\n")},
		{"double pi RESP3", 3, func(c respOps) { c.WriteDouble(math.Pi) }, len(",3.141592653589793\r\n")},
		{"bool RESP3", 3, func(c respOps) { c.WriteBool(true) }, 4},
		{"bignum RESP3", 3, func(c respOps) { c.WriteBigNumber("3492890328409238509324850943850943825024385") }, len("(3492890328409238509324850943850943825024385\r\n")},
		{"map header RESP3", 3, func(c respOps) { c.WriteMap(7) }, len("%7\r\n")},
		{"set header RESP3", 3, func(c respOps) { c.WriteSet(2) }, len("~2\r\n")},
		{"push header RESP3", 3, func(c respOps) { c.WritePush(1) }, len(">1\r\n")},
		{"attribute header RESP3", 3, func(c respOps) { c.WriteAttribute(0) }, len("|0\r\n")},
		{"verbatim RESP3", 3, func(c respOps) { c.WriteVerbatim("txt", "hello") }, len("=9\r\ntxt:hello\r\n")},
		{"blob error RESP3", 3, func(c respOps) { c.WriteBlobError("boom") }, len("!4\r\nboom\r\n")},
		{"bulk RESP2", 2, func(c respOps) { c.WriteBulk([]byte{1, 2, 3, 4, 5}) }, len("$5\r\n\x01\x02\x03\x04\x05\r\n")},
		{"int RESP2", 2, func(c respOps) { c.WriteInt(1234) }, len(":1234\r\n")},
		{"array header RESP2", 2, func(c respOps) { c.WriteArray(3) }, len("*3\r\n")},
		{"string RESP2", 2, func(c respOps) { c.WriteString("hi") }, len("+hi\r\n")},
		{"verbatim RESP2 stays bulk-style sized", 2, func(c respOps) { c.WriteVerbatim("txt", "hello") }, len("=9\r\ntxt:hello\r\n")},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			f := &fakeConn{ver: tc.ver}
			wc, s := newCounting(f)
			wc.write(tc.w)
			if got := s.netOut.Load(); got != uint64(tc.got) {
				t.Fatalf("counted %d bytes, want %d", got, tc.got)
			}
		})
	}
}

func (c *countingConn) write(w func(respOps)) { w(c) }

func TestCountingMatchesWireForAll(t *testing.T) {
	// Cross-check every counted size against the fork's own encoder output:
	// the wrapper must agree byte-for-byte with what the writer actually emits.
	for _, ver := range []int{2, 3} {
		ops := map[string]func(respOps){
			"bulk":     func(c respOps) { c.WriteBulk([]byte{1, 2, 3}) },
			"string":   func(c respOps) { c.WriteString("ok") },
			"int":      func(c respOps) { c.WriteInt(9) },
			"null":     func(c respOps) { c.WriteNull() },
			"array2":   func(c respOps) { c.WriteArray(2) },
			"double":   func(c respOps) { c.WriteDouble(1.5) },
			"map":      func(c respOps) { c.WriteMap(3) },
			"verbatim": func(c respOps) { c.WriteVerbatim("txt", "x") },
			"bloberr":  func(c respOps) { c.WriteBlobError("e") },
		}
		for name, op := range ops {
			f := &fakeConn{ver: ver}
			wc, s := newCounting(f)
			wc.write(op)
			got := int64(s.netOut.Load())
			want := int64(len(respText(ver, op)))
			if got != want {
				t.Errorf("ver=%d %s: counted %d, fork emits %d", ver, name, got, want)
			}
		}
	}
}
