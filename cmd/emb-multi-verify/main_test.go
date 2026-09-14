package main

import (
	"bufio"
	"bytes"
	"encoding/binary"
	"fmt"
	"math"
	"net"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/elcuervo/emb/internal/embverify"
	"github.com/elcuervo/emb/internal/resp"
)

// serveRESP starts a loopback server answering each parsed command from respond.
func serveRESP(t *testing.T, respond func(cmd []string) []byte) string {
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
				for {
					cmd, err := readCommand(r)
					if err != nil {
						return
					}
					if _, err := c.Write(respond(cmd)); err != nil {
						return
					}
				}
			}(conn)
		}
	}()
	return ln.Addr().String()
}

func encodeBulk(v []float32) []byte {
	body := make([]byte, len(v)*4)
	for i, x := range v {
		binary.LittleEndian.PutUint32(body[i*4:], math.Float32bits(x))
	}
	return append([]byte(fmt.Sprintf("$%d\r\n", len(body))), append(body, '\r', '\n')...)
}

func encodeArray(replies ...[]byte) []byte {
	out := []byte(fmt.Sprintf("*%d\r\n", len(replies)))
	for _, r := range replies {
		out = append(out, r...)
	}
	return out
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

func connected(t *testing.T, addr string) *embverify.Embedder {
	t.Helper()
	c := resp.NewClient(addr, "", false)
	c.SetTimeout(time.Second)
	if err := c.Dial(); err != nil {
		t.Fatalf("dial: %v", err)
	}
	t.Cleanup(func() { _ = c.Close() })
	return embverify.NewEmbedder(c)
}

func TestVerifyGroupPassesAndFails(t *testing.T) {
	pairs := []embverify.Pair{{Model: "a", Text: "x"}, {Model: "b", Text: "y"}}

	matching := connected(t, serveRESP(t, func(cmd []string) []byte {
		if cmd[0] == "EMB.MULTI" {
			return encodeArray(encodeBulk([]float32{1}), encodeBulk([]float32{1}))
		}
		return encodeBulk([]float32{1})
	}))
	passed, failed, err := verifyGroup(matching, pairs, nil, &bytes.Buffer{})
	if err != nil || passed != 2 || failed != 0 {
		t.Fatalf("matching: passed=%d failed=%d err=%v", passed, failed, err)
	}

	// EMB.MULTI's second element differs from the sequential EMB reply.
	differs := connected(t, serveRESP(t, func(cmd []string) []byte {
		if cmd[0] == "EMB.MULTI" {
			return encodeArray(encodeBulk([]float32{1}), encodeBulk([]float32{9}))
		}
		return encodeBulk([]float32{1})
	}))
	passed, failed, err = verifyGroup(differs, pairs, nil, &bytes.Buffer{})
	if err != nil || passed != 1 || failed != 1 {
		t.Fatalf("differs: passed=%d failed=%d err=%v", passed, failed, err)
	}

	// A null slot is a failed pair, not a call failure.
	nullable := connected(t, serveRESP(t, func(cmd []string) []byte {
		if cmd[0] == "EMB.MULTI" {
			return encodeArray(encodeBulk([]float32{1}), []byte("$-1\r\n"))
		}
		return encodeBulk([]float32{1})
	}))
	passed, failed, err = verifyGroup(nullable, pairs, nil, &bytes.Buffer{})
	if err != nil || passed != 1 || failed != 1 {
		t.Fatalf("nullable: passed=%d failed=%d err=%v", passed, failed, err)
	}
}

func TestVerifyGroupDimCheck(t *testing.T) {
	pairs := []embverify.Pair{{Model: "a", Text: "x"}}
	e := connected(t, serveRESP(t, func(cmd []string) []byte {
		if cmd[0] == "EMB.MULTI" {
			return encodeArray(encodeBulk([]float32{1, 2}))
		}
		return encodeBulk([]float32{1, 2})
	}))
	// Declaring dim 3 makes the 2-element reply a failure.
	_, failed, err := verifyGroup(e, pairs, map[string]int{"a": 3}, &bytes.Buffer{})
	if err != nil || failed != 1 {
		t.Fatalf("dim check: failed=%d err=%v", failed, err)
	}
}

func TestVerifyGroupTransportError(t *testing.T) {
	e := connected(t, serveRESP(t, func(cmd []string) []byte { return []byte("-ERR nope\r\n") }))
	if _, _, err := verifyGroup(e, []embverify.Pair{{Model: "a", Text: "x"}}, nil, &bytes.Buffer{}); err == nil {
		t.Fatal("expected the EMB.MULTI server error to fail the group")
	}
}
