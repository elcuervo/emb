package main

import (
	"bufio"
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"io"
	"net"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/elcuervo/emb/internal/embtop"
)

func testCommand(r *bufio.Reader) (string, error) {
	line, err := r.ReadString('\n')
	if err != nil {
		return "", err
	}
	n, err := strconv.Atoi(strings.TrimSpace(strings.TrimPrefix(line, "*")))
	if err != nil {
		return "", err
	}
	var name string
	for i := range n {
		sizeLine, err := r.ReadString('\n')
		if err != nil {
			return "", err
		}
		size, err := strconv.Atoi(strings.TrimSpace(strings.TrimPrefix(sizeLine, "$")))
		if err != nil {
			return "", err
		}
		arg := make([]byte, size+2)
		if _, err = io.ReadFull(r, arg); err != nil {
			return "", err
		}
		if i == 0 {
			name = string(arg[:size])
		}
	}
	return name, nil
}
func testUpstream(t *testing.T, respond func(string) string) (string, *atomic.Int32) {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = ln.Close() })
	var count atomic.Int32
	go func() {
		for {
			conn, err := ln.Accept()
			if err != nil {
				return
			}
			go func() {
				defer conn.Close()
				r := bufio.NewReader(conn)
				for {
					command, err := testCommand(r)
					if err != nil {
						return
					}
					count.Add(1)
					if _, err := io.WriteString(conn, respond(command)); err != nil {
						return
					}
				}
			}()
		}
	}()
	return ln.Addr().String(), &count
}

func TestOnePollPerNodeIsolatedFromStalledPeer(t *testing.T) {
	good, commands := testUpstream(t, func(command string) string {
		switch command {
		case "EMB.MODELS", "MONITOR":
			return "*0\r\n"
		case "EMB.STATS":
			return "*4\r\n$14\r\ntotal_requests\r\n:1\r\n$11\r\nuptime_secs\r\n:1\r\n"
		case "INFO":
			return "$0\r\n\r\n"
		default:
			return "-ERR unexpected command\r\n"
		}
	})
	stall, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer stall.Close()
	go func() {
		conn, err := stall.Accept()
		if err == nil {
			defer conn.Close()
			<-time.After(2 * time.Second)
		}
	}()
	specGood, _ := embtop.ParseNodeSpec(good)
	specSlow, _ := embtop.ParseNodeSpec(stall.Addr().String())
	m := newMonitor([]embtop.NodeSpec{specGood, specSlow}, "", false, "")
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go m.run(ctx)
	deadline := time.After(time.Second)
	for {
		select {
		case <-deadline:
			t.Fatal("stalled peer held up healthy node")
		case <-time.After(10 * time.Millisecond):
			m.mu.Lock()
			var s snapshot
			_ = json.Unmarshal(m.latest, &s)
			m.mu.Unlock()
			if len(s.Nodes) == 2 && s.Nodes[0].LastOK != nil {
				if commands.Load() != 4 {
					t.Fatalf("extra upstream polls from viewers or stalled peer: %d", commands.Load())
				}
				return
			}
		}
	}
}

func TestRejectedPasswordAndCertificateName(t *testing.T) {
	addr, _ := testUpstream(t, func(command string) string {
		if command == "AUTH" {
			return "-ERR invalid credentials\r\n"
		}
		return "-NOAUTH authenticate first\r\n"
	})
	client := embtop.NewClient(addr, "SHOULD-NOT-LEAK", false)
	if _, err := client.EnsureConn(); err == nil || !strings.Contains(err.Error(), "auth:") {
		t.Fatalf("rejected password not reported: %v", err)
	}
	_ = client.Close()
	server := httptest.NewTLSServer(nil)
	defer server.Close()
	roots := x509.NewCertPool()
	roots.AddCert(server.Certificate())
	tlsAddr := strings.TrimPrefix(server.URL, "https://")
	secured := embtop.NewClient(tlsAddr, "", true)
	secured.SetTLSConfig(&tls.Config{MinVersion: tls.VersionTLS12, RootCAs: roots})
	secured.SetTLSServerName("mismatch.invalid")
	if _, err := secured.EnsureConn(); err == nil || !strings.Contains(err.Error(), "tls handshake") {
		t.Fatalf("certificate name mismatch accepted: %v", err)
	}
	_ = secured.Close()
	m := newMonitor(nil, "SHOULD-NOT-LEAK", true, "")
	m.newClient = func(address, password string, useTLS bool) *embtop.Client {
		c := embtop.NewClient(address, password, useTLS)
		c.SetTLSConfig(&tls.Config{MinVersion: tls.VersionTLS12, RootCAs: roots})
		return c
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	m.applyResolution(ctx, resolveResult{nodes: []embtop.ResolvedNode{{Key: "secure", Spec: "secure", TLSName: "mismatch.invalid", Addr: tlsAddr}}})
	defer close(m.nodes[0].stop)
	select {
	case result := <-m.results:
		m.applyPoll(result)
	case <-time.After(2 * time.Second):
		t.Fatal("TLS mismatch did not complete")
	}
	if s := m.buildSnapshot(); s.Nodes[0].Reason != "TLS verification failed" || strings.Contains(string(m.latest), "SHOULD-NOT-LEAK") {
		t.Fatalf("unsafe TLS status: %+v", s.Nodes[0])
	}
}
