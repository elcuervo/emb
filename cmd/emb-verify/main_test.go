package main

import (
	"bufio"
	"bytes"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"math"
	"net"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/elcuervo/emb/internal/embverify"
)

// serveEmbed starts a loopback server that answers every EMB command with one
// bulk float32 vector.
func serveEmbed(t *testing.T, vector []float32) string {
	t.Helper()
	payload := encodeBulk(vector)
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
					if _, err := readCommand(r); err != nil {
						return
					}
					if _, err := c.Write(payload); err != nil {
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

func writeReference(t *testing.T, ref *embverify.Reference) string {
	t.Helper()
	ref.Checksum = ref.ComputeChecksum()
	data, err := json.Marshal(ref)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	path := filepath.Join(t.TempDir(), "reference.json")
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatalf("write: %v", err)
	}
	return path
}

func TestVerifyPassesAndFails(t *testing.T) {
	refPath := writeReference(t, &embverify.Reference{
		Model:      "minilm",
		Dim:        2,
		Sentences:  []string{"a", "b"},
		Embeddings: [][]float64{{1, 0}, {1, 0}},
	})

	passing := config{addr: serveEmbed(t, []float32{1, 0}), model: "minilm", refPath: refPath, minCos: 0.999, timeout: time.Second}
	var out bytes.Buffer
	if err := verify(passing, &out); err != nil {
		t.Fatalf("expected pass, got %v\n%s", err, out.String())
	}
	if !strings.Contains(out.String(), "2/2 passed") {
		t.Fatalf("unexpected report:\n%s", out.String())
	}

	failing := passing
	failing.addr = serveEmbed(t, []float32{0, 1})
	out.Reset()
	if err := verify(failing, &out); err == nil {
		t.Fatalf("expected failure for orthogonal vectors\n%s", out.String())
	}
}

func TestVerifyRejectsBadInputs(t *testing.T) {
	refPath := writeReference(t, &embverify.Reference{
		Model:      "minilm",
		Dim:        2,
		Sentences:  []string{"a"},
		Embeddings: [][]float64{{1, 0}},
	})

	// Model mismatch against the artifact.
	if err := verify(config{addr: serveEmbed(t, []float32{1, 0}), model: "bge", refPath: refPath, minCos: 0.999, timeout: time.Second}, &bytes.Buffer{}); err == nil {
		t.Fatal("expected a model mismatch error")
	}
	// Missing reference artifact.
	if err := verify(config{addr: serveEmbed(t, []float32{1, 0}), model: "minilm", refPath: filepath.Join(t.TempDir(), "nope.json"), timeout: time.Second}, &bytes.Buffer{}); err == nil {
		t.Fatal("expected a missing-reference error")
	}
	// Connection refused.
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	addr := ln.Addr().String()
	_ = ln.Close()
	if err := verify(config{addr: addr, model: "minilm", refPath: refPath, timeout: time.Second}, &bytes.Buffer{}); err == nil {
		t.Fatal("expected a connection error")
	}
}

// TestVerifyRejectsTamperedReferenceBeforeConnecting guards the ordering: a bad
// artifact must fail on the checksum, not reach the server.
func TestVerifyRejectsTamperedReferenceBeforeConnecting(t *testing.T) {
	refPath := writeReference(t, &embverify.Reference{
		Model:      "minilm",
		Dim:        2,
		Sentences:  []string{"a"},
		Embeddings: [][]float64{{1, 0}},
	})
	var ref embverify.Reference
	data, err := os.ReadFile(refPath)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if err := json.Unmarshal(data, &ref); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	ref.Embeddings[0][0] = 9 // mutate without recomputing the checksum
	tampered, err := json.Marshal(ref)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	path := filepath.Join(t.TempDir(), "tampered.json")
	if err := os.WriteFile(path, tampered, 0o600); err != nil {
		t.Fatalf("write: %v", err)
	}

	// Point at a closed port: a checksum rejection must not depend on dialing.
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	addr := ln.Addr().String()
	_ = ln.Close()

	err = verify(config{addr: addr, model: "minilm", refPath: path, timeout: time.Second}, &bytes.Buffer{})
	if err == nil || !strings.Contains(err.Error(), "checksum mismatch") {
		t.Fatalf("expected a checksum error before connecting, got %v", err)
	}
}
