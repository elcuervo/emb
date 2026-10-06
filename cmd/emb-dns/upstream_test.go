package main

import (
	"bufio"
	"encoding/binary"
	"fmt"
	"math"
	"net"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"testing"

	"github.com/elcuervo/emb/internal/emoji"
)

// fakeEmb is the smallest emb that can answer the zone: it speaks RESP, returns
// one fixed float32 vector for EMB and EMB.EVSHA, and can drop its connections
// the way emb drops one that has been idle for `idle_timeout`.
type fakeEmb struct {
	ln     net.Listener
	vector []float32
	// replyError makes it answer a command with a server error instead of a
	// vector, which is a reply the connection survived.
	replyError bool

	mu    sync.Mutex
	conns []net.Conn
	dials int
}

func newFakeEmb(t *testing.T, vector []float32) *fakeEmb {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	fake := &fakeEmb{ln: ln, vector: vector}
	go fake.serve()
	t.Cleanup(func() {
		_ = ln.Close()
		fake.dropConnections()
	})
	return fake
}

func (f *fakeEmb) serve() {
	for {
		conn, err := f.ln.Accept()
		if err != nil {
			return
		}
		f.mu.Lock()
		f.conns = append(f.conns, conn)
		f.dials++
		f.mu.Unlock()
		go f.handle(conn)
	}
}

// dropConnections closes every live connection, which is what emb's idle
// timeout does to a zone that has not asked anything for fifteen minutes.
func (f *fakeEmb) dropConnections() {
	f.mu.Lock()
	conns := f.conns
	f.conns = nil
	f.mu.Unlock()
	for _, conn := range conns {
		_ = conn.Close()
	}
}

// connections is how many connections the fake has accepted, which is how a
// test sees a redial.
func (f *fakeEmb) connections() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.dials
}

func (f *fakeEmb) handle(conn net.Conn) {
	reader := bufio.NewReader(conn)
	for {
		argv, err := readArgv(reader)
		if err != nil {
			return
		}
		if f.replyError && strings.EqualFold(argv[0], "EMB.EVSHA") {
			// A server error, which the connection survives: it is not a reason
			// to redial.
			_, _ = conn.Write([]byte("-ERR the model is unhappy\r\n"))
			continue
		}
		switch strings.ToUpper(argv[0]) {
		case "EMB.SCRIPT":
			// A digest, as the server returns for EMB.SCRIPT LOAD.
			_, _ = conn.Write([]byte("+" + strings.Repeat("a", 40) + "\r\n"))
		case "EMB", "EMB.EVSHA":
			payload := make([]byte, 4*len(f.vector))
			for i, value := range f.vector {
				binary.LittleEndian.PutUint32(payload[i*4:], math.Float32bits(value))
			}
			_, _ = fmt.Fprintf(conn, "$%d\r\n", len(payload))
			_, _ = conn.Write(payload)
			_, _ = conn.Write([]byte("\r\n"))
		default:
			_, _ = conn.Write([]byte("-ERR unknown command\r\n"))
		}
	}
}

// readArgv reads one RESP array of bulk strings, which is every command the
// zone sends.
func readArgv(reader *bufio.Reader) ([]string, error) {
	line, err := reader.ReadString('\n')
	if err != nil {
		return nil, err
	}
	line = strings.TrimRight(line, "\r\n")
	if !strings.HasPrefix(line, "*") {
		return nil, fmt.Errorf("fakeEmb: expected an array, got %q", line)
	}
	count, err := strconv.Atoi(line[1:])
	if err != nil {
		return nil, err
	}
	argv := make([]string, 0, count)
	for range count {
		length, err := reader.ReadString('\n')
		if err != nil {
			return nil, err
		}
		length = strings.TrimRight(length, "\r\n")
		if !strings.HasPrefix(length, "$") {
			return nil, fmt.Errorf("fakeEmb: expected a bulk string, got %q", length)
		}
		size, err := strconv.Atoi(length[1:])
		if err != nil {
			return nil, err
		}
		payload := make([]byte, size+2)
		if _, err := reader.Read(payload); err != nil {
			return nil, err
		}
		argv = append(argv, string(payload[:size]))
	}
	return argv, nil
}

func writeScript(t *testing.T) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "emoji.lua")
	if err := os.WriteFile(path, []byte("return emb.embed(KEYS[1])\n"), 0o600); err != nil {
		t.Fatalf("write script: %v", err)
	}
	return path
}

func upstreamFor(t *testing.T, fake *fakeEmb) *Upstream {
	t.Helper()
	cfg := testConfig()
	cfg.Upstream = fake.ln.Addr().String()
	cfg.Script = writeScript(t)
	u, err := Dial(cfg)
	if err != nil {
		t.Fatalf("Dial: %v", err)
	}
	t.Cleanup(func() { _ = u.Close() })
	return u
}

func oneTerm() emoji.Query {
	return emoji.Query{Terms: []emoji.Term{{Text: "shark"}}}
}

func TestComposeRedialsWhenTheServerDropsTheConnection(t *testing.T) {
	fake := newFakeEmb(t, []float32{1, 0, 0})
	u := upstreamFor(t, fake)

	if _, err := u.Compose(oneTerm()); err != nil {
		t.Fatalf("Compose: %v", err)
	}
	if got := fake.connections(); got != 1 {
		t.Fatalf("the first query opened %d connections, want 1", got)
	}

	// emb closes a connection idle for its timeout; the zone's next query must
	// not be the end of the zone.
	fake.dropConnections()
	vector, err := u.Compose(oneTerm())
	if err != nil {
		t.Fatalf("a dropped connection was not recovered: %v", err)
	}
	if len(vector) != 3 || vector[0] != 1 {
		t.Fatalf("the retried query answered %v, want the fixture vector", vector)
	}
	if got := fake.connections(); got != 2 {
		t.Fatalf("the recovered query used %d connections, want a redial (2)", got)
	}

	// And it stays recovered: a later query reuses the new connection.
	if _, err := u.Compose(oneTerm()); err != nil {
		t.Fatalf("Compose after recovery: %v", err)
	}
	if got := fake.connections(); got != 2 {
		t.Fatalf("a healthy query opened a new connection (%d, want 2)", got)
	}
}

func TestEmbedRedialsWhenTheServerDropsTheConnection(t *testing.T) {
	fake := newFakeEmb(t, []float32{0, 1, 0})
	u := upstreamFor(t, fake)

	if _, err := u.Embed([]string{"a", "b"}); err != nil {
		t.Fatalf("Embed: %v", err)
	}
	fake.dropConnections()
	vectors, err := u.Embed([]string{"a", "b"})
	if err != nil {
		t.Fatalf("a dropped connection was not recovered while embedding: %v", err)
	}
	if len(vectors) != 2 {
		t.Fatalf("got %d vectors, want 2", len(vectors))
	}
	if got := fake.connections(); got != 2 {
		t.Fatalf("the recovered build used %d connections, want a redial (2)", got)
	}
}

func TestAServerErrorIsNotRedialed(t *testing.T) {
	fake := newFakeEmb(t, []float32{1, 0, 0})
	fake.replyError = true
	u := upstreamFor(t, fake)

	if _, err := u.Compose(oneTerm()); err == nil {
		t.Fatal("Compose swallowed a server error")
	}
	if got := fake.connections(); got != 1 {
		t.Fatalf("a reply error caused %d connections, want no redial (1)", got)
	}
}

func TestConcurrentComposeShareOneConnection(t *testing.T) {
	fake := newFakeEmb(t, []float32{1, 0, 0})
	u := upstreamFor(t, fake)

	var wg sync.WaitGroup
	errs := make([]error, 8)
	for i := range errs {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, errs[i] = u.Compose(oneTerm())
		}()
	}
	wg.Wait()
	for i, err := range errs {
		if err != nil {
			t.Fatalf("concurrent query %d failed: %v", i, err)
		}
	}
	if got := fake.connections(); got != 1 {
		t.Fatalf("concurrent queries opened %d connections, want the one the zone holds", got)
	}
}
