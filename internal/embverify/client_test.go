package embverify

import (
	"bufio"
	"encoding/binary"
	"fmt"
	"math"
	"net"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/elcuervo/emb/internal/resp"
)

// startFake serves RESP commands on a loopback listener, answering each with
// responder(cmd, n). A nil reply closes the connection (used to simulate a
// stalled peer via the client's timeout).
func startFake(t *testing.T, responder func(cmd []string, n int) []byte) string {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	t.Cleanup(func() { _ = ln.Close() })
	go func() {
		for {
			conn, err := ln.Accept()
			if err != nil {
				return
			}
			go func(c net.Conn) {
				defer c.Close()
				r := bufio.NewReader(c)
				for n := 0; ; n++ {
					cmd, err := readCommand(r)
					if err != nil {
						return
					}
					reply := responder(cmd, n)
					if reply == nil {
						time.Sleep(2 * time.Second)
						return
					}
					if _, err := c.Write(reply); err != nil {
						return
					}
				}
			}(conn)
		}
	}()
	return ln.Addr().String()
}

func readCommand(r *bufio.Reader) ([]string, error) {
	line, err := r.ReadString('\n')
	if err != nil {
		return nil, err
	}
	if !strings.HasPrefix(line, "*") {
		return nil, fmt.Errorf("expected array, got %q", line)
	}
	n, err := strconv.Atoi(strings.TrimSpace(line[1:]))
	if err != nil {
		return nil, err
	}
	args := make([]string, 0, n)
	for i := 0; i < n; i++ {
		hdr, err := r.ReadString('\n')
		if err != nil {
			return nil, err
		}
		if !strings.HasPrefix(hdr, "$") {
			return nil, fmt.Errorf("expected bulk, got %q", hdr)
		}
		l, err := strconv.Atoi(strings.TrimSpace(hdr[1:]))
		if err != nil {
			return nil, err
		}
		buf := make([]byte, l)
		if _, err := readFull(r, buf); err != nil {
			return nil, err
		}
		if _, err := r.Discard(2); err != nil {
			return nil, err
		}
		args = append(args, string(buf))
	}
	return args, nil
}

func readFull(r *bufio.Reader, buf []byte) (int, error) {
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

func encodeBulkFloats(vals ...float32) []byte {
	body := make([]byte, len(vals)*4)
	for i, v := range vals {
		binary.LittleEndian.PutUint32(body[i*4:], math.Float32bits(v))
	}
	return append([]byte(fmt.Sprintf("$%d\r\n", len(body))), append(body, '\r', '\n')...)
}

func connectedEmbedder(t *testing.T, addr string) (*Embedder, *resp.Client) {
	t.Helper()
	c := resp.NewClient(addr, "", false)
	c.SetTimeout(500 * time.Millisecond)
	if err := c.Dial(); err != nil {
		t.Fatalf("dial: %v", err)
	}
	t.Cleanup(func() { _ = c.Close() })
	return NewEmbedder(c), c
}

func TestVectorFromReply(t *testing.T) {
	decode := func(rep resp.Reply) ([]float32, error) {
		raw, err := RawVectorFromReply(rep)
		if err != nil {
			return nil, err
		}
		return DecodeFloat32(raw)
	}
	ok, err := decode(resp.Reply{Type: '$', Str: string(encodeFloat32(1, 2))})
	if err != nil || len(ok) != 2 {
		t.Fatalf("bulk: %v %v", ok, err)
	}
	arr, err := decode(resp.Reply{Type: '*', Elems: []resp.Reply{{Type: '$', Str: string(encodeFloat32(3))}}})
	if err != nil || arr[0] != 3 {
		t.Fatalf("array-of-one: %v %v", arr, err)
	}
	if _, err := decode(resp.Reply{Type: '$', Nil: true}); err == nil {
		t.Fatal("expected nil bulk to error")
	}
	if _, err := decode(resp.Reply{Type: '*', Elems: []resp.Reply{{Type: '$'}, {Type: '$'}}}); err == nil {
		t.Fatal("expected array-of-two to error")
	}
	if _, err := decode(resp.Reply{Type: ':'}); err == nil {
		t.Fatal("expected unexpected type to error")
	}
	if _, err := decode(resp.Reply{Type: '$', Str: "abc"}); err == nil {
		t.Fatal("expected a bad length to error")
	}
}

func TestEmbedderEmbed(t *testing.T) {
	addr := startFake(t, func(cmd []string, _ int) []byte {
		if cmd[0] != "EMB" || cmd[1] != "minilm" {
			return []byte("-ERR unknown model\r\n")
		}
		return encodeBulkFloats(1, 2, 3)
	})
	e, _ := connectedEmbedder(t, addr)

	vec, err := e.Embed("minilm", "hello")
	if err != nil {
		t.Fatalf("embed: %v", err)
	}
	if len(vec) != 3 || vec[2] != 3 {
		t.Fatalf("vector = %v", vec)
	}

	if _, err := e.Embed("nope", "hello"); err == nil || !strings.Contains(err.Error(), "unknown model") {
		t.Fatalf("expected the server error, got %v", err)
	}
}

func TestEmbedderEmbedAllNamesFailingText(t *testing.T) {
	addr := startFake(t, func(cmd []string, n int) []byte {
		if cmd[2] == "bad" {
			return []byte("-ERR bad text\r\n")
		}
		return encodeBulkFloats(1)
	})
	e, _ := connectedEmbedder(t, addr)

	if _, err := e.EmbedAll("minilm", []string{"ok", "bad"}); err == nil || !strings.Contains(err.Error(), "text 1") {
		t.Fatalf("expected failure naming text 1, got %v", err)
	}
}

func TestEmbedderTimeout(t *testing.T) {
	addr := startFake(t, func(cmd []string, _ int) []byte { return nil })
	e, _ := connectedEmbedder(t, addr)

	start := time.Now()
	if _, err := e.Embed("minilm", "hello"); err == nil {
		t.Fatal("expected a timeout")
	}
	if elapsed := time.Since(start); elapsed > time.Second {
		t.Fatalf("timeout took %v", elapsed)
	}
}

func TestRawEmbedIsByteStable(t *testing.T) {
	payload := encodeBulkFloats(1.5, -2.5)
	addr := startFake(t, func(cmd []string, _ int) []byte { return payload })
	e, _ := connectedEmbedder(t, addr)

	raw, err := e.RawEmbed("minilm", "hello")
	if err != nil {
		t.Fatalf("raw embed: %v", err)
	}
	if want := encodeFloat32(1.5, -2.5); string(raw) != string(want) {
		t.Fatalf("raw payload = %q, want %q", raw, want)
	}
}

func TestRawMultiEmbedPreservesOrderAndNulls(t *testing.T) {
	addr := startFake(t, func(cmd []string, _ int) []byte {
		if cmd[0] != "EMB.MULTI" {
			return []byte("-ERR wrong command\r\n")
		}
		// Two pairs: first succeeds, second is a null slot.
		return append(append([]byte("*2\r\n"), encodeBulkFloats(7, 8)...), []byte("$-1\r\n")...)
	})
	e, _ := connectedEmbedder(t, addr)

	out, err := e.RawMultiEmbed([]Pair{{Model: "a", Text: "x"}, {Model: "b", Text: "y"}})
	if err != nil {
		t.Fatalf("multi embed: %v", err)
	}
	if len(out) != 2 || string(out[0]) != string(encodeFloat32(7, 8)) {
		t.Fatalf("first pair = %v", out[0])
	}
	if out[1] != nil {
		t.Fatalf("second pair = %v, want nil", out[1])
	}
}

func TestRawMultiEmbedTransportError(t *testing.T) {
	addr := startFake(t, func(cmd []string, _ int) []byte { return []byte("-ERR nope\r\n") })
	e, _ := connectedEmbedder(t, addr)
	if _, err := e.RawMultiEmbed([]Pair{{Model: "a", Text: "x"}}); err == nil {
		t.Fatal("expected the server error to fail the call")
	}
}
