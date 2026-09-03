package server

import (
	"github.com/tidwall/redcon"
)

// countingConn wraps a redcon.Conn and accumulates the aggregate RESP2 wire
// size of every reply written through it into the Server's netOut counter
// (INFO total_net_output_bytes). redcon v1.6.2's Write* methods return no
// byte counts, so the wire size is computed from the RESP2 encoding rules
// directly — exact, allocation-free, and centrally enforced (every reply
// flows through the wrapper; new handlers need no per-call bookkeeping).
type countingConn struct {
	redcon.Conn
	s *Server
}

func (s *Server) wrapConn(c redcon.Conn) redcon.Conn {
	return &countingConn{Conn: c, s: s}
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
	c.s.netOut.Add(5) // "$-1\r\n"
	c.Conn.WriteNull()
}

func (c *countingConn) WriteRaw(data []byte) {
	c.s.netOut.Add(uint64(len(data)))
	c.Conn.WriteRaw(data)
}

// WriteAny sizes the reply by encoding it once with redcon's own encoder
// (emb handlers never call WriteAny, so the transient allocation is fine).
func (c *countingConn) WriteAny(v interface{}) {
	c.s.netOut.Add(uint64(len(redcon.AppendAny(nil, v))))
	c.Conn.WriteAny(v)
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
