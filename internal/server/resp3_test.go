package server

import (
	"strings"
	"testing"
)

// By default connections speak RESP2: PING replies as a simple string and no
// protocol negotiation has happened.
func TestRESP3DefaultIsRESP2(t *testing.T) {
	addr := serveTest(t)
	c := dial(t, addr)
	defer c.Close()

	c.Write([]byte("*1\r\n$4\r\nPING\r\n"))
	if resp := readRESP(t, c); resp != "+PONG\r\n" {
		t.Fatalf("expected PONG, got %q", resp)
	}

	c.Write([]byte("*2\r\n$5\r\nHELLO\r\n$1\r\n2\r\n"))
	resp := readRESP(t, c)
	if !strings.HasPrefix(resp, "*") {
		t.Fatalf("expected flat array HELLO reply under RESP2, got %q", resp)
	}
	if !strings.Contains(resp, "$5\r\nproto\r\n$1\r\n2\r\n") {
		t.Fatalf("expected proto 2 in HELLO reply, got %q", resp)
	}
}

func TestRESP3HELLOUpgrade(t *testing.T) {
	addr := serveTest(t)
	c := dial(t, addr)
	defer c.Close()

	// Bare HELLO reports the current (RESP2) version without switching.
	c.Write([]byte("*1\r\n$5\r\nHELLO\r\n"))
	resp := readRESP(t, c)
	if !strings.Contains(resp, "$5\r\nproto\r\n$1\r\n2\r\n") {
		t.Fatalf("bare HELLO should report proto 2, got %q", resp)
	}

	// Upgrade to RESP3: reply is a map with proto 3.
	c.Write([]byte("*2\r\n$5\r\nHELLO\r\n$1\r\n3\r\n"))
	resp = readRESP(t, c)
	if !strings.HasPrefix(resp, "%") {
		t.Fatalf("expected map HELLO reply under RESP3, got %q", resp)
	}
	if !strings.Contains(resp, "$5\r\nproto\r\n$1\r\n3\r\n") {
		t.Fatalf("expected proto 3 in HELLO reply, got %q", resp)
	}

	// RESP3 null encoding: a failing EMB.MULTI pair is `_`, not `$-1`.
	c.Write([]byte("*3\r\n$9\r\nEMB.MULTI\r\n$11\r\nnonexistent\r\n$1\r\nx\r\n"))
	resp = readRESP(t, c)
	if resp != "*1\r\n_\r\n" {
		t.Fatalf("expected RESP3 null (_), got %q", resp)
	}

	// Switch back to RESP2: nulls revert to $-1.
	c.Write([]byte("*2\r\n$5\r\nHELLO\r\n$1\r\n2\r\n"))
	resp = readRESP(t, c)
	if !strings.Contains(resp, "$5\r\nproto\r\n$1\r\n2\r\n") {
		t.Fatalf("downgrade should report proto 2, got %q", resp)
	}
	c.Write([]byte("*3\r\n$9\r\nEMB.MULTI\r\n$11\r\nnonexistent\r\n$1\r\nx\r\n"))
	resp = readRESP(t, c)
	if resp != "*1\r\n$-1\r\n" {
		t.Fatalf("expected RESP2 null ($-1), got %q", resp)
	}
}

func TestRESP3HELLOInvalidVersion(t *testing.T) {
	addr := serveTest(t)
	c := dial(t, addr)
	defer c.Close()

	c.Write([]byte("*2\r\n$5\r\nHELLO\r\n$1\r\n4\r\n"))
	resp := readRESP(t, c)
	if !strings.HasPrefix(resp, "-NOPROTO") {
		t.Fatalf("expected NOPROTO error, got %q", resp)
	}

	// Connection stays RESP2: nulls are still $-1.
	c.Write([]byte("*3\r\n$9\r\nEMB.MULTI\r\n$11\r\nnonexistent\r\n$1\r\nx\r\n"))
	resp = readRESP(t, c)
	if resp != "*1\r\n$-1\r\n" {
		t.Fatalf("connection should remain RESP2 after invalid HELLO, got %q", resp)
	}
}

func TestRESP3HELLORespectsAuth(t *testing.T) {
	addr := serveTestWithAuth(t, "sekret")
	c := dial(t, addr)
	defer c.Close()

	// Unauthenticated HELLO is rejected (HELLO is not auth-exempt).
	c.Write([]byte("*2\r\n$5\r\nHELLO\r\n$1\r\n3\r\n"))
	if resp := readRESP(t, c); !strings.HasPrefix(resp, "-NOAUTH") {
		t.Fatalf("expected NOAUTH, got %q", resp)
	}

	// After AUTH, HELLO negotiates RESP3.
	c.Write([]byte("*2\r\n$4\r\nAUTH\r\n$6\r\nsekret\r\n"))
	if resp := readRESP(t, c); resp != "+OK\r\n" {
		t.Fatalf("expected OK, got %q", resp)
	}
	c.Write([]byte("*2\r\n$5\r\nHELLO\r\n$1\r\n3\r\n"))
	resp := readRESP(t, c)
	if !strings.Contains(resp, "$5\r\nproto\r\n$1\r\n3\r\n") {
		t.Fatalf("expected proto 3 after auth, got %q", resp)
	}
}
func TestCLIENTSETINFO(t *testing.T) {
	addr := serveTest(t)
	c := dial(t, addr)
	defer c.Close()

	c.Write([]byte("*4\r\n$6\r\nCLIENT\r\n$7\r\nSETINFO\r\n$8\r\nlib-name\r\n$8\r\nredis-py\r\n"))
	if resp := readRESP(t, c); resp != "+OK\r\n" {
		t.Fatalf("expected OK, got %q", resp)
	}
}

func TestRESP3IntrospectionMaps(t *testing.T) {
	addr := serveTest(t)
	c := dial(t, addr)
	defer c.Close()

	c.Write(respCommand("HELLO", "3"))
	readRESP(t, c)

	// EMB.MODELS becomes a map keyed by model name; values carry dim/status.
	c.Write(respCommand("EMB.MODELS"))
	if resp := readRESP(t, c); resp != "%1\r\n$4\r\ntest\r\n%2\r\n$3\r\ndim\r\n:4\r\n$6\r\nstatus\r\n$5\r\nready\r\n" {
		t.Fatalf("expected model map under RESP3, got %q", resp)
	}

	// EMB.INFO is a 15-pair map (no cache), same field order as RESP2.
	c.Write(respCommand("EMB.INFO", "test"))
	resp := readRESP(t, c)
	if !strings.HasPrefix(resp, "%15\r\n$3\r\ndim\r\n:4\r\n") {
		t.Fatalf("expected EMB.INFO map under RESP3, got %q", resp)
	}

	// EMB.STATS is a 44-pair map.
	c.Write(respCommand("EMB.STATS"))
	resp = readRESP(t, c)
	if !strings.HasPrefix(resp, "%44\r\n$11\r\nuptime_secs\r\n") {
		t.Fatalf("expected EMB.STATS map under RESP3, got %q", resp)
	}

	// CONFIG GET is a map too.
	c.Write(respCommand("CONFIG", "GET", "*"))
	resp = readRESP(t, c)
	if !strings.HasPrefix(resp, "%") {
		t.Fatalf("expected CONFIG GET map under RESP3, got %q", resp)
	}

	// INFO stays a bulk string under RESP3, like real Redis.
	c.Write(respCommand("INFO"))
	resp = readRESP(t, c)
	if !strings.HasPrefix(resp, "$") {
		t.Fatalf("expected INFO bulk string under RESP3, got %q", resp)
	}
}
