package resp

import (
	"bytes"
	"errors"
	"net"
	"strings"
	"testing"
	"time"
)

// serveOnce starts a one-shot TCP server that writes payload (when non-nil)
// and then holds the connection open for hold before closing it.
func serveOnce(t *testing.T, payload []byte, hold time.Duration) string {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	t.Cleanup(func() { _ = ln.Close() })
	go func() {
		conn, err := ln.Accept()
		if err != nil {
			return
		}
		defer conn.Close()
		if len(payload) > 0 {
			_, _ = conn.Write(payload)
		}
		if hold > 0 {
			time.Sleep(hold)
		}
	}()
	return ln.Addr().String()
}

func dial(t *testing.T, addr string) *Client {
	t.Helper()
	c := NewClient(addr, "", false)
	if err := c.Dial(); err != nil {
		t.Fatalf("dial %s: %v", addr, err)
	}
	return c
}

func TestReadReplyShapes(t *testing.T) {
	payload := []byte(
		"+OK\r\n" +
			"$3\r\nfoo\r\n" +
			":42\r\n" +
			"$-1\r\n" +
			"*-1\r\n" +
			"*2\r\n$1\r\na\r\n$-1\r\n" +
			"-ERR nope\r\n",
	)
	c := dial(t, serveOnce(t, payload, time.Second))
	defer c.Close()

	for i, want := range []struct {
		typ  byte
		str  string
		n    int64
		nilp bool
	}{
		{typ: '+', str: "OK"},
		{typ: '$', str: "foo"},
		{typ: ':', n: 42},
		{typ: '$', nilp: true},
		{typ: '*', nilp: true},
		{typ: '*'},
		{typ: '-', str: "ERR nope"},
	} {
		rep, err := c.ReadReply()
		if err != nil {
			t.Fatalf("reply %d: %v", i, err)
		}
		if rep.Type != want.typ || rep.Str != want.str || rep.Int != want.n || rep.Nil != want.nilp {
			t.Fatalf("reply %d = %+v, want type=%q str=%q int=%d nil=%v", i, rep, want.typ, want.str, want.n, want.nilp)
		}
	}
}

func TestReadReplyArrayElements(t *testing.T) {
	c := dial(t, serveOnce(t, []byte("*2\r\n$1\r\na\r\n$-1\r\n"), time.Second))
	defer c.Close()

	rep, err := c.ReadReply()
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if rep.Type != '*' || len(rep.Elems) != 2 {
		t.Fatalf("got %+v, want array of 2", rep)
	}
	if rep.Elems[0].Str != "a" || !rep.Elems[1].Nil {
		t.Fatalf("elements = %+v, want [a nil]", rep.Elems)
	}
}

func TestErrorReplyReturnsError(t *testing.T) {
	c := dial(t, serveOnce(t, []byte("-ERR unknown model\r\n"), time.Second))
	defer c.Close()

	rep, err := c.ReadReply()
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if got := rep.Err(); got == nil || !strings.Contains(got.Error(), "unknown model") {
		t.Fatalf("Err() = %v, want message containing 'unknown model'", got)
	}
}

func TestReadReplyTimeout(t *testing.T) {
	// The server accepts and then stays silent; a short timeout must surface.
	c := NewClient(serveOnce(t, nil, 2*time.Second), "", false)
	c.SetTimeout(150 * time.Millisecond)
	if err := c.Dial(); err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer c.Close()

	start := time.Now()
	_, err := c.ReadReply()
	if err == nil {
		t.Fatal("expected timeout, got a reply")
	}
	if elapsed := time.Since(start); elapsed > time.Second {
		t.Fatalf("timeout took %v, want < 1s", elapsed)
	}
	var ne net.Error
	if !errors.As(err, &ne) || !ne.Timeout() {
		if !strings.Contains(err.Error(), "timeout") && !strings.Contains(err.Error(), "deadline") {
			t.Fatalf("error = %v, want a timeout/deadline error", err)
		}
	}
}

func TestDialConnectionRefused(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	addr := ln.Addr().String()
	_ = ln.Close()

	c := NewClient(addr, "", false)
	err = c.Dial()
	if err == nil {
		t.Fatal("expected dial error on a closed port")
	}
	if !strings.Contains(err.Error(), addr) {
		t.Fatalf("error %q does not name the address %q", err, addr)
	}
}

func TestSetDeadlineWhenNotConnected(t *testing.T) {
	c := NewClient("127.0.0.1:1", "", false)
	if err := c.SetDeadline(time.Now()); err == nil {
		t.Fatal("expected SetDeadline to fail while not connected")
	}
}

func TestWriteWhenNotConnected(t *testing.T) {
	c := NewClient("127.0.0.1:1", "", false)
	if err := c.WriteArgv("PING"); err == nil || !strings.Contains(err.Error(), "not connected") {
		t.Fatalf("WriteArgv err = %v, want not connected", err)
	}
	if err := c.Flush(); err == nil || !strings.Contains(err.Error(), "not connected") {
		t.Fatalf("Flush err = %v, want not connected", err)
	}
}

func TestReadLineIsBounded(t *testing.T) {
	// A status line with no terminator, longer than MaxLineBytes, must be
	// rejected rather than accumulated without limit.
	payload := append([]byte("+"), bytes.Repeat([]byte("a"), MaxLineBytes+1)...)
	c := dial(t, serveOnce(t, payload, time.Second))
	defer c.Close()

	_, err := c.ReadReply()
	if err == nil || !strings.Contains(err.Error(), "line length") {
		t.Fatalf("err = %v, want a line-length error", err)
	}
}
