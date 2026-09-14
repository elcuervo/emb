// Package resp is a minimal RESP2 client shared by emb's operator tooling
// (emb-top and the embedding verifiers).
//
// It decodes only the reply shapes emb produces: bulk strings ('$'), status
// ('+'), integers (':'), arrays ('*'), errors ('-'), and nil (empty bulk or
// array). It imports nothing from internal/server, so tools built on it build
// without CGo/onnxruntime.
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

// Reply is one RESP2 reply.
type Reply struct {
	Type  byte    // '+', '-', ':', '$', '*'
	Str   string  // payload for + / - / $
	Int   int64   // payload for :
	Elems []Reply // payload for *
	Nil   bool    // nil bulk or array
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

// Client is a minimal RESP2 client for an emb node.
type Client struct {
	addr      string
	password  string
	useTLS    bool
	ioTimeout time.Duration

	conn net.Conn
	r    *bufio.Reader
	w    *bufio.Writer
}

// NewClient returns a Client for addr (host:port) with optional password
// (AUTH) and TLS transport.
func NewClient(addr, password string, useTLS bool) *Client {
	return &Client{addr: addr, password: password, useTLS: useTLS}
}

// Addr returns the configured address.
func (c *Client) Addr() string { return c.addr }

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

// WriteArgv appends one RESP2 command (array of bulk strings) to the write
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

// ReadReply decodes one RESP2 reply, closing the connection on a decode error
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
