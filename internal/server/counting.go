package server

import (
	"math"
	"strconv"

	"github.com/tidwall/redcon"
)

// countingConn wraps a redcon.Conn and accumulates the aggregate wire size of
// every reply written through it into the Server's netOut counter (INFO
// total_net_output_bytes). The fork's Write* methods return no byte counts, so
// the wire size is computed from the RESP encoding rules directly — exact,
// allocation-free (double texts are formatted into a stack scratch buffer), and
// centrally enforced (every reply flows through the wrapper; new handlers need
// no per-call bookkeeping). Encoding is protocol-aware: for connections that
// negotiated RESP3, nulls are `_\r\n` and aggregate headers use the RESP3 kinds.
type countingConn struct {
	redcon.Conn
	s *Server
}

func (s *Server) wrapConn(c redcon.Conn) redcon.Conn {
	return &countingConn{Conn: c, s: s}
}

// resp3 reports whether the wrapped connection negotiated protocol 3. The
// version is promoted from the embedded redcon.Conn (the fork's own Writer
// state), so the counting stays in lockstep with what the writer emits.
func (c *countingConn) resp3() bool {
	return c.ProtocolVersion() == 3
}

func (c *countingConn) WriteError(msg string) {
	c.s.netOut.Add(uint64(1 + len(msg) + 2)) // "-msg\r\n"
	c.Conn.WriteError(msg)
}

func (c *countingConn) WriteString(str string) {
	c.s.netOut.Add(uint64(1 + len(str) + 2)) // "+str\r\n"
	c.Conn.WriteString(str)
}

func (c *countingConn) WriteBulk(bulk []byte) {
	c.s.netOut.Add(uint64(sizeOfBulk(len(bulk))))
	c.Conn.WriteBulk(bulk)
}

func (c *countingConn) WriteBulkString(bulk string) {
	c.s.netOut.Add(uint64(sizeOfBulk(len(bulk))))
	c.Conn.WriteBulkString(bulk)
}

func (c *countingConn) WriteInt(num int) {
	c.s.netOut.Add(uint64(digitsInt64(int64(num)) + 3)) // ":n\r\n"
	c.Conn.WriteInt(num)
}

func (c *countingConn) WriteInt64(num int64) {
	c.s.netOut.Add(uint64(digitsInt64(num) + 3)) // ":n\r\n"
	c.Conn.WriteInt64(num)
}

func (c *countingConn) WriteUint64(num uint64) {
	c.s.netOut.Add(uint64(digitsUint64(num) + 3)) // ":n\r\n"
	c.Conn.WriteUint64(num)
}

func (c *countingConn) WriteArray(count int) {
	c.s.netOut.Add(uint64(digitsInt64(int64(count)) + 3)) // "*n\r\n"
	c.Conn.WriteArray(count)
}

func (c *countingConn) WriteNull() {
	if c.resp3() {
		c.s.netOut.Add(3) // "_\r\n"
	} else {
		c.s.netOut.Add(5) // "$-1\r\n"
	}
	c.Conn.WriteNull()
}

// doubleText returns the wire body of a RESP double (the text between the ','
// and CRLF) using the same shortest-round-trip encoding the fork's writer
// emits, formatted into a stack scratch buffer to stay allocation-free.
func doubleText(f float64, buf []byte) []byte {
	switch {
	case math.IsInf(f, 1):
		return append(buf, "inf"...)
	case math.IsInf(f, -1):
		return append(buf, "-inf"...)
	case math.IsNaN(f):
		return append(buf, "nan"...)
	}
	return strconv.AppendFloat(buf, f, 'g', -1, 64)
}

func (c *countingConn) WriteDouble(f float64) {
	var buf [32]byte
	n := len(doubleText(f, buf[:0]))
	c.s.netOut.Add(uint64(n + 3)) // ",<text>\r\n"
	c.Conn.WriteDouble(f)
}

func (c *countingConn) WriteBool(v bool) {
	c.s.netOut.Add(4) // "#t\r\n" or "#f\r\n"
	c.Conn.WriteBool(v)
}

func (c *countingConn) WriteBigNumber(s string) {
	c.s.netOut.Add(uint64(1 + len(s) + 2)) // "(<s>\r\n"
	c.Conn.WriteBigNumber(s)
}

func (c *countingConn) WriteVerbatim(format, s string) {
	c.s.netOut.Add(uint64(sizeOfLenPrefix(1 + len(format) + len(s))))
	c.Conn.WriteVerbatim(format, s)
}

func (c *countingConn) WriteBlobError(s string) {
	c.s.netOut.Add(uint64(sizeOfLenPrefix(len(s))))
	c.Conn.WriteBlobError(s)
}

func (c *countingConn) WriteMap(count int) {
	c.s.netOut.Add(uint64(sizeOfCountHeader('%', count)))
	c.Conn.WriteMap(count)
}

func (c *countingConn) WriteSet(count int) {
	c.s.netOut.Add(uint64(sizeOfCountHeader('~', count)))
	c.Conn.WriteSet(count)
}

func (c *countingConn) WritePush(count int) {
	c.s.netOut.Add(uint64(sizeOfCountHeader('>', count)))
	c.Conn.WritePush(count)
}

func (c *countingConn) WriteAttribute(count int) {
	c.s.netOut.Add(uint64(sizeOfCountHeader('|', count)))
	c.Conn.WriteAttribute(count)
}

func (c *countingConn) WriteRaw(data []byte) {
	c.s.netOut.Add(uint64(len(data)))
	c.Conn.WriteRaw(data)
}

// WriteAny sizes the reply by encoding it once with the fork's own encoder,
// matching the negotiated protocol (emb handlers never call WriteAny, so the
// transient allocation is fine).
func (c *countingConn) WriteAny(v interface{}) {
	if c.resp3() {
		c.s.netOut.Add(uint64(len(redcon.AppendAny3(nil, v))))
	} else {
		c.s.netOut.Add(uint64(len(redcon.AppendAny(nil, v))))
	}
	c.Conn.WriteAny(v)
}

// sizeOfLenPrefix is the wire size of a length-prefixed RESP3 body: the one-
// byte kind, "$<n>\r\n<len(payload)>\r\n" for n digits plus the CRLF.
func sizeOfLenPrefix(n int) int {
	return digitsUint64(uint64(n)) + 5 + n
}

// sizeOfCountHeader is the wire size of a typed aggregate header: the kind
// byte (%, ~, >, |), the count, and CRLF — e.g. "%5\r\n".
func sizeOfCountHeader(kind byte, count int) int {
	return 1 + digitsUint64(uint64(count)) + 2
}

// sizeOfBulk is the wire length of "$n\r\n<payload>\r\n".
func sizeOfBulk(n int) int {
	return digitsUint64(uint64(n)) + 5 + n
}

func digitsUint64(n uint64) int {
	if n <= 9 {
		return 1
	}
	d := 0
	for n > 0 {
		d++
		n /= 10
	}
	return d
}

func digitsInt64(n int64) int {
	if n < 0 {
		// -(n+1)+1 computes |MinInt64| without overflowing.
		return digitsUint64(uint64(-(n+1))+1) + 1 // +1 for the '-' sign
	}
	return digitsUint64(uint64(n))
}
