// Package resp is a minimal RESP client shared by emb's operator tooling
// (emb-top, the embedding verifiers, and the sandbox bridge).
//
// It decodes the reply shapes emb produces: bulk strings ('$'), status ('+'),
// integers (':'), arrays ('*'), errors ('-'), nil (empty bulk or array), and the
// RESP3 kinds the server emits — maps ('%'), doubles (','), and nulls ('_').
// It imports nothing from internal/server, so tools built on it build without
// CGo/onnxruntime.
package resp

import (
	"bufio"
	"crypto/tls"
	"errors"
	"fmt"
	"net"
	"strconv"
	"strings"
	"time"
)

// Reply is one RESP reply.
type Reply struct {
	Type  byte    // '+', '-', ':', '$', '*', '%', ',', '_'
	Str   string  // payload for + / - / $
	Int   int64   // payload for :
	Float float64 // payload for ,
	Elems []Reply // payload for * (elements) and % (alternating key/value)
	Nil   bool    // nil bulk, array, or null (_, $-1, *-1)
}

// MapPairs returns a map reply's entries as key/value pairs. Elems holds them
// flattened in wire order, so ordering is preserved; the count is always even.
func (r Reply) MapPairs() [][2]Reply {
	pairs := make([][2]Reply, 0, len(r.Elems)/2)
	for i := 0; i+1 < len(r.Elems); i += 2 {
		pairs = append(pairs, [2]Reply{r.Elems[i], r.Elems[i+1]})
	}
	return pairs
}

// Err returns the error for an error reply, else nil.
func (r Reply) Err() error {
	if r.Type == '-' {
		return errors.New(r.Str)
	}
	return nil
}

// String returns the string payload (bulk/status) or "" for other types.
func (r Reply) String() string { return r.Str }

// Client is a minimal RESP client for an emb node.
type Client struct {
	addr      string
	password  string
	useTLS    bool
	ioTimeout time.Duration

	conn net.Conn
	r    *bufio.Reader
	w    *bufio.Writer

	// proto is the RESP version negotiated on this connection: 0 (fresh, i.e.
	// RESP2 by default) or 3 after a successful HELLO 3. Dial resets it.
	proto int
}

// NewClient returns a Client for addr (host:port) with optional password
// (AUTH) and TLS transport.
func NewClient(addr, password string, useTLS bool) *Client {
	return &Client{addr: addr, password: password, useTLS: useTLS}
}

// Addr returns the configured address.
func (c *Client) Addr() string { return c.addr }

// Proto returns the version negotiated on the current connection (2 when none
// has been negotiated, which is RESP2's default).
func (c *Client) Proto() int {
	if c.proto == 3 {
		return 3
	}
	return 2
}

// Hello negotiates the RESP version on the connection by sending HELLO 2 or
// HELLO 3 and reading its reply, which itself arrives as an array under RESP2
// and a map under RESP3. It is a no-op when the connection already speaks the
// requested version. The version travels on the connection, so a caller gets
// the server's own encoding of subsequent commands rather than a synthesized
// form.
func (c *Client) Hello(version int) error {
	if version != 2 && version != 3 {
		return fmt.Errorf("resp: protocol version must be 2 or 3, got %d", version)
	}
	if c.conn == nil {
		return errors.New("resp: not connected")
	}
	if err := c.conn.SetDeadline(time.Now().Add(c.Timeout())); err != nil {
		return err
	}
	if c.Proto() == version {
		return nil
	}
	if err := c.WriteArgv("HELLO", strconv.Itoa(version)); err != nil {
		return err
	}
	if err := c.Flush(); err != nil {
		return err
	}
	rep, err := c.ReadReply()
	if err != nil {
		return err
	}
	if err := rep.Err(); err != nil {
		return fmt.Errorf("hello %d: %w", version, err)
	}
	if version == 3 {
		c.proto = 3
	} else {
		c.proto = 0
	}
	return nil
}

// DefaultTimeout bounds the TLS handshake, each round trip's write/read, and
// the AUTH exchange, so a stalled peer surfaces as an error instead of hanging
// forever.
const DefaultTimeout = 10 * time.Second

// Dial connects (plain TCP or TLS), authenticates if a password is set, and
// waits for the AUTH reply when applicable.
func (c *Client) Dial() error {
	if c.conn != nil {
		_ = c.Close()
	}
	timeout := c.Timeout()
	nc, err := net.DialTimeout("tcp", c.addr, 5*time.Second)
	if err != nil {
		return fmt.Errorf("connect %s: %w", c.addr, err)
	}
	// Bound the handshake and AUTH exchange; callers refresh the deadline.
	_ = nc.SetDeadline(time.Now().Add(timeout))
	if c.useTLS {
		host := c.addr
		if h, _, err := net.SplitHostPort(c.addr); err == nil {
			host = h
		}
		tc := tls.Client(nc, &tls.Config{
			ServerName: host,
			MinVersion: tls.VersionTLS12,
		})
		if err := tc.Handshake(); err != nil {
			_ = nc.Close()
			return fmt.Errorf("tls handshake %s: %w", c.addr, err)
		}
		nc = tc
	}
	c.conn = nc
	c.r = bufio.NewReader(nc)
	c.w = bufio.NewWriter(nc)
	c.proto = 0 // a fresh connection defaults to RESP2

	if c.password != "" {
		if err := c.WriteArgv("AUTH", c.password); err != nil {
			_ = c.Close()
			return err
		}
		if err := c.Flush(); err != nil {
			_ = c.Close()
			return err
		}
		rep, err := c.ReadReply()
		if err != nil {
			_ = c.Close()
			return err
		}
		if err := rep.Err(); err != nil {
			_ = c.Close()
			return fmt.Errorf("auth: %w", err)
		}
	}
	return nil
}

// EnsureConn dials the node when not currently connected, reporting whether a
// new connection was established (so callers can rebase sequence cursors).
func (c *Client) EnsureConn() (dialed bool, err error) {
	if c.conn == nil {
		if err := c.Dial(); err != nil {
			return false, err
		}
		return true, nil
	}
	return false, nil
}

// Timeout returns the effective per-operation deadline.
func (c *Client) Timeout() time.Duration {
	if c.ioTimeout > 0 {
		return c.ioTimeout
	}
	return DefaultTimeout
}

// SetTimeout overrides the per-operation deadline (tests use short values).
func (c *Client) SetTimeout(d time.Duration) { c.ioTimeout = d }

// SetDeadline sets the connection's absolute read/write deadline, bounding a
// pipelined round trip so a stalled peer cannot block the caller.
func (c *Client) SetDeadline(t time.Time) error {
	if c.conn == nil {
		return errors.New("resp: not connected")
	}
	return c.conn.SetDeadline(t)
}

// Close closes the underlying connection.
func (c *Client) Close() error {
	if c.conn == nil {
		return nil
	}
	err := c.conn.Close()
	c.conn = nil
	return err
}

// WriteArgv appends one RESP command (array of bulk strings) to the write
// buffer. Call Flush to send. Multiple WriteArgv calls batch into a single
// round trip. It returns an error when the client is not connected.
func (c *Client) WriteArgv(args ...string) error {
	if c.conn == nil {
		return errors.New("resp: not connected")
	}
	w := c.w
	if _, err := w.WriteString("*" + strconv.Itoa(len(args)) + "\r\n"); err != nil {
		return err
	}
	for _, a := range args {
		if _, err := w.WriteString("$" + strconv.Itoa(len(a)) + "\r\n" + a + "\r\n"); err != nil {
			return err
		}
	}
	return nil
}

// Flush sends the buffered commands, closing the connection on a write error
// so the next EnsureConn re-establishes it. It returns an error when the
// client is not connected.
func (c *Client) Flush() error {
	if c.conn == nil {
		return errors.New("resp: not connected")
	}
	err := c.w.Flush()
	if err != nil {
		_ = c.Close()
	}
	return err
}

// Parser bounds: the peer controls bulk lengths, array counts and nesting, so
// cap them before allocating or recursing (a hostile or broken server must not
// be able to exhaust a client's memory or stack).
const (
	// MaxBulkBytes caps one bulk string's payload.
	MaxBulkBytes = 16 << 20 // 16 MiB
	// MaxArrayLen caps one array's element count.
	MaxArrayLen = 1 << 20 // 1M elements
	// MaxDepth caps reply nesting.
	MaxDepth = 32
	// MaxLineBytes caps one status/error/integer line. The peer controls the
	// line length, so bound it the same way as bulks and arrays.
	MaxLineBytes = 64 << 10 // 64 KiB
)

// ReadReply decodes one RESP reply, closing the connection on a decode error
// so a misaligned stream cannot be reused.
func (c *Client) ReadReply() (Reply, error) {
	rep, err := c.readReply(0)
	if err != nil {
		_ = c.Close()
	}
	return rep, err
}

func (c *Client) readReply(depth int) (Reply, error) {
	if depth > MaxDepth {
		return Reply{}, fmt.Errorf("resp: reply nested deeper than %d", MaxDepth)
	}
	prefix, err := c.r.ReadByte()
	if err != nil {
		return Reply{}, err
	}
	switch prefix {
	case '+', '-':
		line, err := c.readLine()
		return Reply{Type: prefix, Str: line}, err
	case ':':
		line, err := c.readLine()
		if err != nil {
			return Reply{}, err
		}
		n, err := strconv.ParseInt(line, 10, 64)
		return Reply{Type: ':', Int: n}, err
	case '$':
		line, err := c.readLine()
		if err != nil {
			return Reply{}, err
		}
		n, err := strconv.Atoi(line)
		if err != nil {
			return Reply{}, err
		}
		if n < 0 {
			return Reply{Type: '$', Nil: true}, nil
		}
		if n > MaxBulkBytes {
			return Reply{}, fmt.Errorf("resp: bulk length %d exceeds %d", n, MaxBulkBytes)
		}
		payload := make([]byte, n)
		if _, err := ioReadFull(c.r, payload); err != nil {
			return Reply{}, err
		}
		if _, err := c.r.Discard(2); err != nil { // trailing CRLF
			return Reply{}, err
		}
		return Reply{Type: '$', Str: string(payload)}, nil
	case '*':
		line, err := c.readLine()
		if err != nil {
			return Reply{}, err
		}
		n, err := strconv.Atoi(line)
		if err != nil {
			return Reply{}, err
		}
		if n < 0 {
			return Reply{Type: '*', Nil: true}, nil
		}
		if n > MaxArrayLen {
			return Reply{}, fmt.Errorf("resp: array length %d exceeds %d", n, MaxArrayLen)
		}
		rep := Reply{Type: '*', Elems: make([]Reply, 0, n)}
		for i := 0; i < n; i++ {
			el, err := c.readReply(depth + 1)
			if err != nil {
				return Reply{}, err
			}
			rep.Elems = append(rep.Elems, el)
		}
		return rep, nil
	case '%':
		line, err := c.readLine()
		if err != nil {
			return Reply{}, err
		}
		n, err := strconv.Atoi(line)
		if err != nil {
			return Reply{}, err
		}
		if n < 0 {
			return Reply{}, fmt.Errorf("resp: negative map length %d", n)
		}
		if n*2 > MaxArrayLen {
			return Reply{}, fmt.Errorf("resp: map length %d exceeds %d entries", n, MaxArrayLen/2)
		}
		rep := Reply{Type: '%', Elems: make([]Reply, 0, n*2)}
		for i := 0; i < n*2; i++ {
			el, err := c.readReply(depth + 1)
			if err != nil {
				return Reply{}, err
			}
			rep.Elems = append(rep.Elems, el)
		}
		return rep, nil
	case ',':
		line, err := c.readLine()
		if err != nil {
			return Reply{}, err
		}
		f, err := strconv.ParseFloat(line, 64)
		if err != nil {
			return Reply{}, fmt.Errorf("resp: invalid double %q: %w", line, err)
		}
		return Reply{Type: ',', Float: f}, nil
	case '_':
		line, err := c.readLine()
		if err != nil {
			return Reply{}, err
		}
		if line != "" {
			return Reply{}, fmt.Errorf("resp: null payload %q is not empty", line)
		}
		return Reply{Type: '_', Nil: true}, nil
	default:
		return Reply{}, fmt.Errorf("resp: unexpected prefix %q", prefix)
	}
}

// readLine reads one CRLF-terminated line, accumulating in bounded chunks so a
// peer that never sends '\n' cannot make the client grow a buffer without
// limit. Reads longer than MaxLineBytes are rejected.
func (c *Client) readLine() (string, error) {
	var line []byte
	for {
		chunk, err := c.r.ReadSlice('\n')
		line = append(line, chunk...)
		if len(line) > MaxLineBytes {
			return "", fmt.Errorf("resp: line length exceeds %d", MaxLineBytes)
		}
		if err == nil {
			return strings.TrimRight(string(line), "\r\n"), nil
		}
		if !errors.Is(err, bufio.ErrBufferFull) {
			return "", err
		}
	}
}

// ioReadFull mirrors io.ReadFull using the bufio.Reader.
func ioReadFull(r *bufio.Reader, buf []byte) (int, error) {
	total := 0
	for total < len(buf) {
		n, err := r.Read(buf[total:])
		total += n
		if err != nil {
			return total, err
		}
	}
	return total, nil
}
