package server

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"syscall"
	"testing"
	"time"
)

// TestRedisShutdownParity stops a real redis-server and emb with the same client
// script and asserts the client-visible results are identical: the SHUTDOWN
// replies, the error class a connection sees after a shutdown, a refused dial
// after exit, and the exit status of a double SIGINT.
//
// Redis waits only for lagging replicas, so a standalone redis-server has no
// grace: the grace, ABORT during it, and blocked-waiter replies are emb-only and
// are covered by shutdown_test.go.
//
// Enabled by EMB_REDIS_PARITY=1 (`EMB_REDIS_PARITY=1 go test ./internal/server/
// -run Parity`); it skips when redis-server is not on PATH or bin/emb is not
// built (`just build`).
func TestRedisShutdownParity(t *testing.T) {
	if os.Getenv("EMB_REDIS_PARITY") != "1" {
		t.Skip("set EMB_REDIS_PARITY=1 to run the Redis shutdown parity check")
	}
	redisBin, err := exec.LookPath("redis-server")
	if err != nil {
		t.Skip("redis-server is not on PATH")
	}
	embBin, err := filepath.Abs("../../bin/emb")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(embBin); err != nil {
		t.Skipf("bin/emb is missing; run `just build` (%v)", err)
	}

	t.Run("replies", func(t *testing.T) {
		redis := startRedis(t, redisBin)
		emb := startEmb(t, embBin, 0)
		script := []struct {
			args []string
			want string
		}{
			{[]string{"SHUTDOWN", "SAVE", "NOSAVE"}, "-ERR syntax error\r\n"},
			{[]string{"SHUTDOWN", "ABORT", "NOW"}, "-ERR syntax error\r\n"},
			{[]string{"SHUTDOWN", "LATER"}, "-ERR syntax error\r\n"},
			{[]string{"SHUTDOWN", "ABORT"}, "-ERR No shutdown in progress.\r\n"},
			{[]string{"PING"}, "+PONG\r\n"},
		}
		got, want := runParityScript(t, emb.addr, script), runParityScript(t, redis.addr, script)
		if !reflect.DeepEqual(got, want) {
			t.Fatalf("replies differ\nemb:   %q\nredis: %q", got, want)
		}
		for i, c := range script {
			if got[i] != c.want {
				t.Errorf("reply %d (%v) = %q, want %q", i, c.args, got[i], c.want)
			}
		}
	})

	t.Run("idle connection error class", func(t *testing.T) {
		redis := startRedis(t, redisBin)
		emb := startEmb(t, embBin, 0)
		for _, server := range []*parityServer{emb, redis} {
			idle := dialParity(t, server.addr)
			shutdown := dialParity(t, server.addr)
			shutdown.Write(respCommand("SHUTDOWN", "NOW", "NOSAVE"))
			_ = shutdown.SetReadDeadline(time.Now().Add(3 * time.Second))
			_ = idle.SetReadDeadline(time.Now().Add(3 * time.Second))
			_, _ = idle.Write(respCommand("PING"))
			reply, err := bufio.NewReader(idle).ReadString('\n')
			if err == nil && strings.HasPrefix(reply, "-") {
				t.Fatalf("%s answered an idle connection with a RESP error: %q", server.name, reply)
			}
			_ = idle.Close()
			_ = shutdown.Close()
			server.wait(t)
		}
	})

	t.Run("refused dial after exit", func(t *testing.T) {
		redis := startRedis(t, redisBin)
		emb := startEmb(t, embBin, 0)
		for _, server := range []*parityServer{emb, redis} {
			c := dialParity(t, server.addr)
			c.Write(respCommand("SHUTDOWN", "NOW", "NOSAVE"))
			_ = c.Close()
			if code := server.wait(t); code != 0 {
				t.Fatalf("%s exited with %d, want 0", server.name, code)
			}
			if _, err := net.DialTimeout("tcp", server.addr, 200*time.Millisecond); err == nil {
				t.Fatalf("%s accepted a connection after exit", server.name)
			}
		}
	})

	t.Run("second SIGINT exits 1", func(t *testing.T) {
		// Standalone redis-server finishes a SIGINT shutdown before a second
		// signal can be delivered (it has no lagging replica to wait for), so
		// only emb is measured here; 1 is the status Redis documents for
		// insisting. The grace keeps emb's first SIGINT pending.
		emb := startEmb(t, embBin, 30)
		_ = dialParity(t, emb.addr)
		if err := emb.proc.Process.Signal(syscall.SIGINT); err != nil {
			t.Fatalf("SIGINT: %v", err)
		}
		time.Sleep(20 * time.Millisecond)
		if err := emb.proc.Process.Signal(syscall.SIGINT); err != nil {
			t.Fatalf("second SIGINT: %v", err)
		}
		if code := emb.wait(t); code != 1 {
			t.Fatalf("emb exited with %d after a double SIGINT, want 1", code)
		}
	})
}

// parityServer is one running server under comparison.
type parityServer struct {
	name    string
	addr    string
	proc    *exec.Cmd
	waitErr error
}

// wait waits for the process and returns its exit code.
func (p *parityServer) wait(t *testing.T) int {
	t.Helper()
	err := p.proc.Wait()
	if err == nil {
		return 0
	}
	var exitErr *exec.ExitError
	if !errors.As(err, &exitErr) {
		t.Fatalf("%s: waiting for exit: %v", p.name, err)
	}
	return exitErr.ExitCode()
}

func freeParityAddr(t *testing.T) string {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	addr := ln.Addr().String()
	ln.Close()
	return addr
}

func startRedis(t *testing.T, bin string) *parityServer {
	t.Helper()
	addr := freeParityAddr(t)
	port := addr[strings.LastIndex(addr, ":")+1:]
	proc := exec.Command(bin, "--port", port, "--bind", "127.0.0.1", "--save", "", "--appendonly", "no", "--dir", t.TempDir())
	if err := proc.Start(); err != nil {
		t.Fatalf("starting redis-server: %v", err)
	}
	server := &parityServer{name: "redis-server", addr: addr, proc: proc}
	t.Cleanup(func() {
		if proc.ProcessState == nil {
			_ = proc.Process.Kill()
			_ = proc.Wait()
		}
	})
	waitParityReady(t, addr)
	return server
}

func startEmb(t *testing.T, bin string, shutdownTimeout int) *parityServer {
	t.Helper()
	addr := freeParityAddr(t)
	dir, err := filepath.Abs("../../testdata")
	if err != nil {
		t.Fatal(err)
	}
	cfg := filepath.Join(t.TempDir(), "emb.yaml")
	body := fmt.Sprintf(`listen: %q
shutdown_timeout: %d
models:
  plain:
    onnx: %q
    tokenizer: %q
    pooling: mean
    dim: %d
    max_length: %d
`, addr, shutdownTimeout,
		filepath.Join(dir, "plain-emb", "model.onnx"),
		filepath.Join(dir, "laya", "tokenizer", "tokenizer.json"),
		embFixtureDim, embFixtureMaxLength)
	if err := os.WriteFile(cfg, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	proc := exec.Command(bin, "-config", cfg, "-listen", addr)
	proc.Stdout = io.Discard
	proc.Stderr = io.Discard
	if err := proc.Start(); err != nil {
		t.Fatalf("starting emb: %v", err)
	}
	server := &parityServer{name: "emb", addr: addr, proc: proc}
	t.Cleanup(func() {
		if proc.ProcessState == nil {
			_ = proc.Process.Kill()
			_ = proc.Wait()
		}
	})
	waitParityReady(t, addr)
	return server
}

func waitParityReady(t *testing.T, addr string) {
	t.Helper()
	deadline := time.Now().Add(30 * time.Second)
	for time.Now().Before(deadline) {
		c, err := net.DialTimeout("tcp", addr, 200*time.Millisecond)
		if err != nil {
			time.Sleep(50 * time.Millisecond)
			continue
		}
		_ = c.SetDeadline(time.Now().Add(time.Second))
		_, _ = c.Write(respCommand("PING"))
		line, err := bufio.NewReader(c).ReadString('\n')
		_ = c.Close()
		if err == nil && strings.HasPrefix(line, "+PONG") {
			return
		}
		time.Sleep(50 * time.Millisecond)
	}
	t.Fatalf("%s did not become ready", addr)
}

func dialParity(t *testing.T, addr string) net.Conn {
	t.Helper()
	c, err := net.DialTimeout("tcp", addr, time.Second)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = c.Close() })
	return c
}

// runParityScript sends each command on one connection and returns the raw
// replies.
func runParityScript(t *testing.T, addr string, script []struct {
	args []string
	want string
}) []string {
	t.Helper()
	c := dialParity(t, addr)
	reader := bufio.NewReader(c)
	replies := make([]string, 0, len(script))
	for _, step := range script {
		_ = c.SetDeadline(time.Now().Add(3 * time.Second))
		if _, err := c.Write(respCommand(step.args...)); err != nil {
			t.Fatalf("writing %v: %v", step.args, err)
		}
		reply, err := reader.ReadString('\n')
		if err != nil {
			t.Fatalf("reading the reply to %v: %v", step.args, err)
		}
		replies = append(replies, reply)
	}
	return replies
}
