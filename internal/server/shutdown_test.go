package server

import (
	"context"
	"errors"
	"log"
	"net"
	"os"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/tidwall/redcon"

	"github.com/elcuervo/emb/internal/config"
	"github.com/elcuervo/emb/internal/onnx"
)

// serveWithShutdownLoop starts a test server wired like the entrypoint: SHUTDOWN
// requests are executed by a loop goroutine.
func serveWithShutdownLoop(t *testing.T, opts ...Option) (string, *Server) {
	t.Helper()
	requests := make(chan ShutdownRequest, 8)
	var pending atomic.Bool
	addr, srv := serveTestWithOptions(t, append(opts, WithShutdownRequests(requests, &pending))...)
	done := make(chan struct{})
	t.Cleanup(func() { close(done) })
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		for {
			select {
			case <-done:
				return
			case req := <-requests:
				_ = srv.Shutdown(ctx, req.Flags)
			}
		}
	}()
	return addr, srv
}

// recordConn is a redcon.Conn that records the replies a handler writes and
// reports a settable remote address.
type recordConn struct {
	redcon.Conn
	addr   string
	state  *connState
	errs   []string
	status []string
	closed bool
}

func (c *recordConn) RemoteAddr() string           { return c.addr }
func (c *recordConn) Context() interface{}         { return c.state }
func (c *recordConn) SetContext(v interface{})     { c.state = v.(*connState) }
func (c *recordConn) Close() error                 { c.closed = true; return nil }
func (c *recordConn) WriteError(msg string)        { c.errs = append(c.errs, msg) }
func (c *recordConn) WriteString(msg string)       { c.status = append(c.status, msg) }
func (c *recordConn) WriteBulkString(string)       {}
func (c *recordConn) WriteBulk([]byte)             {}
func (c *recordConn) WriteInt(int)                 {}
func (c *recordConn) WriteArray(int)               {}
func (c *recordConn) WriteNull()                   {}
func (c *recordConn) WriteRaw([]byte)              {}
func (c *recordConn) WriteDouble(float64)          {}
func (c *recordConn) WriteBool(bool)               {}
func (c *recordConn) WriteBigNumber(string)        {}
func (c *recordConn) WriteVerbatim(string, string) {}
func (c *recordConn) WriteBlobError(string)        {}
func (c *recordConn) WriteMap(int)                 {}
func (c *recordConn) WriteSet(int)                 {}
func (c *recordConn) WritePush(int)                {}
func (c *recordConn) WriteAttribute(int)           {}
func (c *recordConn) WriteAny(interface{})         {}
func (c *recordConn) SetProtocolVersion(int)       {}
func (c *recordConn) ProtocolVersion() int         { return 2 }
func (c *recordConn) WriteInt64(int64)             {}
func (c *recordConn) WriteUint64(uint64)           {}

func shutdownCmd(args ...string) redcon.Command {
	all := append([]string{"SHUTDOWN"}, args...)
	parsed := make([][]byte, len(all))
	for i, a := range all {
		parsed[i] = []byte(a)
	}
	return redcon.Command{Raw: respCommand(all...), Args: parsed}
}

func statusOf(t *testing.T, tok respToken) string {
	t.Helper()
	if tok.kind != "status" {
		t.Fatalf("expected status, got %s", tok.kind)
	}
	return tok.val.(string)
}

func TestGateListenerHoldsAcceptUntilRelease(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	gate := newGateListener(ln)
	accepted := make(chan net.Conn, 1)
	go func() {
		c, err := gate.Accept()
		if err == nil {
			accepted <- c
		}
	}()
	first, err := net.Dial("tcp", ln.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	defer first.Close()
	var live net.Conn
	select {
	case live = <-accepted:
	case <-time.After(time.Second):
		t.Fatal("accept did not return a connection")
	}
	defer live.Close()

	if err := gate.Close(); err != nil {
		t.Fatalf("closing the gate: %v", err)
	}
	if _, err := net.DialTimeout("tcp", ln.Addr().String(), 200*time.Millisecond); err == nil {
		t.Fatal("dial succeeded after the gate closed")
	}

	// The already accepted connection is untouched by Close.
	if _, err := live.Write([]byte("x")); err != nil {
		t.Fatalf("live connection broke on gate close: %v", err)
	}

	returned := make(chan error, 1)
	go func() {
		_, err := gate.Accept()
		returned <- err
	}()
	select {
	case err := <-returned:
		t.Fatalf("held Accept returned before Release: %v", err)
	case <-time.After(50 * time.Millisecond):
	}
	gate.Release()
	select {
	case err := <-returned:
		if !errors.Is(err, net.ErrClosed) {
			t.Fatalf("Accept after Release = %v, want net.ErrClosed", err)
		}
	case <-time.After(time.Second):
		t.Fatal("Release did not unblock Accept")
	}
}

func TestConnectionRegistryTracksConnectCommandDisconnect(t *testing.T) {
	addr, srv := serveTestWithOptions(t)
	c := dial(t, addr)
	waitFor(t, func() bool { return srv.graceClients() == 1 })

	c.Write(respCommand("PING"))
	if got := readRESP(t, c); got != "+PONG\r\n" {
		t.Fatalf("PING = %q", got)
	}

	srv.clientMu.Lock()
	var st *connState
	for conn := range srv.clients {
		if state, ok := conn.Context().(*connState); ok {
			st = state
		}
	}
	srv.clientMu.Unlock()
	if st == nil {
		t.Fatal("accepted connection has no connState")
	}
	waitFor(t, func() bool { return st.inProgress.Load() == 0 })

	c.Close()
	waitFor(t, func() bool { return srv.graceClients() == 0 })
}

func TestShutdownStopsIdleClientWithoutReply(t *testing.T) {
	addr, srv := serveTestWithOptions(t)
	srv.SetReady()
	client := dial(t, addr)
	waitFor(t, func() bool { return srv.graceClients() == 1 })

	if err := srv.Shutdown(context.Background(), config.ShutdownFlags{Now: true}); err != nil {
		t.Fatalf("Shutdown: %v", err)
	}

	_ = client.SetReadDeadline(time.Now().Add(time.Second))
	_, _ = client.Write(respCommand("PING"))
	buf := make([]byte, 16)
	if n, err := client.Read(buf); err == nil {
		t.Fatalf("idle client got a reply after clients stopped: %q", buf[:n])
	}
	if _, err := net.DialTimeout("tcp", addr, time.Second); err == nil {
		t.Fatal("stopped server accepted a new connection")
	}
}

func TestShutdownDeliversInFlightReplyThenCloses(t *testing.T) {
	gate := make(chan struct{})
	addr, srv := serveWithPool(t,
		func() (onnx.Session, error) { return &blockingSession{gate: gate}, nil },
		1, 0, 32,
	)
	client := dial(t, addr)
	client.Write(respCommand("EMB", "test", "hello"))
	waitFor(t, func() bool { return srv.activeReqs.Load() == 1 })

	done := make(chan error, 1)
	go func() { done <- srv.Shutdown(context.Background(), config.ShutdownFlags{Now: true}) }()
	waitFor(t, func() bool {
		_, err := net.DialTimeout("tcp", addr, 100*time.Millisecond)
		return err != nil
	})

	close(gate)
	if reply := readRESP(t, client); !strings.HasPrefix(reply, "$") {
		t.Fatalf("in-flight request did not receive its reply: %q", reply)
	}
	if err := <-done; err != nil {
		t.Fatalf("Shutdown: %v", err)
	}

	_ = client.SetReadDeadline(time.Now().Add(time.Second))
	buf := make([]byte, 8)
	if n, err := client.Read(buf); err == nil {
		t.Fatalf("connection stayed open after its reply: %q", buf[:n])
	}
}

func TestShutdownDropsPipelinedCommandBehindRunning(t *testing.T) {
	gate := make(chan struct{})
	addr, srv := serveWithPool(t,
		func() (onnx.Session, error) { return &blockingSession{gate: gate}, nil },
		1, 0, 32,
	)
	client := dial(t, addr)
	client.Write(append(respCommand("EMB", "test", "hello"), respCommand("PING")...))
	waitFor(t, func() bool { return srv.activeReqs.Load() == 1 })

	done := make(chan error, 1)
	go func() { done <- srv.Shutdown(context.Background(), config.ShutdownFlags{Now: true}) }()
	waitFor(t, srv.shuttingDown.Load)

	close(gate)
	if reply := readRESP(t, client); !strings.HasPrefix(reply, "$") {
		t.Fatalf("first pipelined command did not receive its reply: %q", reply)
	}
	if err := <-done; err != nil {
		t.Fatalf("Shutdown: %v", err)
	}

	_ = client.SetReadDeadline(time.Now().Add(time.Second))
	buf := make([]byte, 64)
	if n, err := client.Read(buf); err == nil {
		t.Fatalf("pipelined command got a reply: %q", buf[:n])
	}
}

func TestGraceServesCommandsAndReportsDraining(t *testing.T) {
	addr, srv := serveTestWithOptions(t, WithShutdownConfig(30, "default", "default"))
	srv.SetReady()
	client := dial(t, addr)
	waitFor(t, func() bool { return srv.graceClients() == 1 })

	done := make(chan error, 1)
	go func() { done <- srv.Shutdown(context.Background(), config.ShutdownFlags{}) }()
	waitFor(t, func() bool { return srv.graceDeadline.Load() != 0 })

	client.Write(respCommand("EMB", "test", "hello"))
	if reply := readRESP(t, client); !strings.HasPrefix(reply, "$") {
		t.Fatalf("EMB during grace = %q", reply)
	}
	client.Write(respCommand("EMB.READY"))
	if reply := readRESP(t, client); !strings.Contains(reply, "draining") {
		t.Fatalf("EMB.READY during grace = %q", reply)
	}
	client.Write(respCommand("INFO", "server"))
	info := readRESP(t, client)
	if !strings.Contains(info, "shutdown_in_milliseconds:") {
		t.Fatalf("INFO server during grace missing shutdown_in_milliseconds:\n%s", info)
	}

	client.Write(respCommand("SHUTDOWN", "ABORT"))
	if reply := readRESP(t, client); reply != "+OK\r\n" {
		t.Fatalf("SHUTDOWN ABORT = %q", reply)
	}
	if err := <-done; !errors.Is(err, ErrShutdownAborted) {
		t.Fatalf("aborted Shutdown = %v, want ErrShutdownAborted", err)
	}
	client.Write(respCommand("EMB.READY"))
	if reply := readRESP(t, client); reply != "+OK\r\n" {
		t.Fatalf("EMB.READY after abort = %q", reply)
	}
	client.Write(respCommand("INFO", "server"))
	if info := readRESP(t, client); strings.Contains(info, "shutdown_in_milliseconds") {
		t.Fatalf("INFO server after abort still reports the countdown:\n%s", info)
	}
}

func TestNoGraceWithoutClientsWithNowOrZeroTimeout(t *testing.T) {
	cases := []struct {
		name  string
		opts  []Option
		flags config.ShutdownFlags
	}{
		{name: "no clients", opts: []Option{WithShutdownConfig(30, "default", "default")}},
		{name: "now", opts: []Option{WithShutdownConfig(30, "default", "default")}, flags: config.ShutdownFlags{Now: true}},
		{name: "zero timeout", opts: []Option{WithShutdownConfig(0, "default", "default")}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			_, srv := serveTestWithOptions(t, c.opts...)
			if c.name != "no clients" {
				// A connected client would otherwise hold the grace open.
				addr := srv.addr
				dial(t, addr)
				waitFor(t, func() bool { return srv.graceClients() == 1 })
			}
			start := time.Now()
			if err := srv.Shutdown(context.Background(), c.flags); err != nil {
				t.Fatalf("Shutdown: %v", err)
			}
			if elapsed := time.Since(start); elapsed > 500*time.Millisecond {
				t.Fatalf("shutdown took %v; no grace should run", elapsed)
			}
		})
	}
}

func TestGraceEndsWhenClientsLeave(t *testing.T) {
	addr, srv := serveTestWithOptions(t, WithShutdownConfig(30, "default", "default"))
	client := dial(t, addr)
	waitFor(t, func() bool { return srv.graceClients() == 1 })

	done := make(chan error, 1)
	go func() { done <- srv.Shutdown(context.Background(), config.ShutdownFlags{}) }()
	waitFor(t, func() bool { return srv.graceDeadline.Load() != 0 })

	start := time.Now()
	client.Close()
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("Shutdown: %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("grace did not end after the last client left")
	}
	// The grace re-checks every 100ms (Redis's cron).
	if elapsed := time.Since(start); elapsed > time.Second {
		t.Fatalf("grace ended %v after the last disconnect", elapsed)
	}
}

func TestGraceIsBounded(t *testing.T) {
	addr, srv := serveTestWithOptions(t, WithShutdownConfig(1, "default", "default"))
	dial(t, addr)
	waitFor(t, func() bool { return srv.graceClients() == 1 })

	start := time.Now()
	if err := srv.Shutdown(context.Background(), config.ShutdownFlags{}); err != nil {
		t.Fatalf("Shutdown: %v", err)
	}
	elapsed := time.Since(start)
	if elapsed < 900*time.Millisecond || elapsed > 3*time.Second {
		t.Fatalf("grace lasted %v, want about 1s", elapsed)
	}
}

// TestGraceDoesNotConsumeDrainDeadline gives the drain a 1s budget and the
// grace 2s: the drain budget must start after the grace, so in-flight work
// still gets its full second.
func TestGraceDoesNotConsumeDrainDeadline(t *testing.T) {
	gate := make(chan struct{})
	addr, srv := serveWithPool(t,
		func() (onnx.Session, error) { return &blockingSession{gate: gate}, nil },
		1, 0, 32,
		WithShutdownConfig(2, "default", "default"),
		WithShutdownDrainTimeout(time.Second),
	)
	client := dial(t, addr)
	client.Write(respCommand("EMB", "test", "hello"))
	waitFor(t, func() bool { return srv.activeReqs.Load() == 1 })

	done := make(chan error, 1)
	go func() { done <- srv.Shutdown(context.Background(), config.ShutdownFlags{}) }()
	waitFor(t, func() bool { return srv.graceDeadline.Load() != 0 })

	// Let the grace expire, then finish the in-flight request inside the drain
	// budget the grace must not have consumed.
	time.Sleep(2100 * time.Millisecond)
	close(gate)
	if reply := readRESP(t, client); !strings.HasPrefix(reply, "$") {
		t.Fatalf("in-flight request lost its reply: %q", reply)
	}
	if err := <-done; err != nil {
		t.Fatalf("Shutdown after the grace consumed the drain budget: %v", err)
	}
}

func TestShutdownSaveFlagsOverrideCacheSetting(t *testing.T) {
	dirty := func(t *testing.T, dir string, saveOnShutdown bool) (*Server, string) {
		t.Helper()
		path := dir + "/cache.embcache"
		s := New("", lifecycleRegistry(), "", "1MB", nil, WithPersistence(PersistenceConfig{
			File: path, Load: false, SaveOnShutdown: saveOnShutdown,
		}))
		s.cache.Set("test:dirty", make([]byte, 16))
		return s, path
	}
	saved := func(t *testing.T, path string) bool {
		t.Helper()
		_, err := os.Stat(path)
		return err == nil
	}

	t.Run("save overrides disabled", func(t *testing.T) {
		dir := t.TempDir()
		s, path := dirty(t, dir, false)
		_ = s.Shutdown(context.Background(), config.ShutdownFlags{Save: true})
		if !saved(t, path) {
			t.Fatal("SAVE did not write the final snapshot")
		}
	})

	t.Run("nosave overrides enabled", func(t *testing.T) {
		dir := t.TempDir()
		s, path := dirty(t, dir, true)
		_ = s.Shutdown(context.Background(), config.ShutdownFlags{NoSave: true})
		if saved(t, path) {
			t.Fatal("NOSAVE wrote the final snapshot")
		}
	})

	t.Run("save without cache_file logs and continues", func(t *testing.T) {
		var logs strings.Builder
		log.SetOutput(&logs)
		defer log.SetOutput(os.Stderr)
		s := New("", lifecycleRegistry(), "", "1MB", nil)
		if err := s.Shutdown(context.Background(), config.ShutdownFlags{Save: true}); err != nil && !errors.Is(err, net.ErrClosed) && err.Error() != "not serving" {
			t.Fatalf("SAVE without cache_file: %v", err)
		}
		if !strings.Contains(logs.String(), "no cache_file is configured") {
			t.Fatalf("SAVE without cache_file did not log: %q", logs.String())
		}
	})
}

func TestShutdownSyntaxErrors(t *testing.T) {
	addr := serveTest(t)
	for _, args := range [][]string{
		{"SAVE", "NOSAVE"},
		{"ABORT", "NOW"},
		{"LATER"},
	} {
		reply := redisCmd(t, addr, append([]string{"SHUTDOWN"}, args...)...)
		if got := errorOf(t, reply); got != "ERR syntax error" {
			t.Fatalf("SHUTDOWN %v = %q", args, got)
		}
		if got := statusOf(t, redisCmd(t, addr, "PING")); got != "PONG" {
			t.Fatalf("server stopped on a syntax error: %q", got)
		}
	}
}

func TestShutdownAbortWithNothingRunning(t *testing.T) {
	addr := serveTest(t)
	if got := errorOf(t, redisCmd(t, addr, "SHUTDOWN", "ABORT")); got != "ERR No shutdown in progress." {
		t.Fatalf("SHUTDOWN ABORT = %q", got)
	}
}

func TestShutdownBlocksAndAbortReleasesCallers(t *testing.T) {
	addr, srv := serveWithShutdownLoop(t, WithShutdownConfig(30, "default", "default"))
	srv.SetReady()

	// The grace needs a client other than the blocked SHUTDOWN caller.
	dial(t, addr)
	waitFor(t, func() bool { return srv.graceClients() == 1 })

	caller := dial(t, addr)
	caller.Write(respCommand("SHUTDOWN"))
	waitFor(t, func() bool { return srv.graceDeadline.Load() != 0 })
	// The blocked caller gets no reply while the grace runs.
	_ = caller.SetReadDeadline(time.Now().Add(100 * time.Millisecond))
	buf := make([]byte, 8)
	if n, err := caller.Read(buf); err == nil {
		t.Fatalf("SHUTDOWN caller got a reply during the grace: %q", buf[:n])
	}
	_ = caller.SetReadDeadline(time.Time{})

	// A second caller blocks too, and its flags are ignored.
	second := dial(t, addr)
	second.Write(respCommand("SHUTDOWN", "NOSAVE"))
	_ = second.SetReadDeadline(time.Now().Add(100 * time.Millisecond))
	if n, err := second.Read(buf); err == nil {
		t.Fatalf("second SHUTDOWN caller got a reply during the grace: %q", buf[:n])
	}
	_ = second.SetReadDeadline(time.Time{})

	aborter := dial(t, addr)
	aborter.Write(respCommand("SHUTDOWN", "ABORT"))
	if reply := readRESP(t, aborter); reply != "+OK\r\n" {
		t.Fatalf("SHUTDOWN ABORT = %q", reply)
	}
	if reply := readRESP(t, caller); !strings.Contains(reply, "Errors trying to SHUTDOWN. Check logs.") {
		t.Fatalf("blocked caller reply = %q", reply)
	}
	if reply := readRESP(t, second); !strings.Contains(reply, "Errors trying to SHUTDOWN. Check logs.") {
		t.Fatalf("second caller reply = %q", reply)
	}

	ready := dial(t, addr)
	ready.Write(respCommand("EMB.READY"))
	if reply := readRESP(t, ready); reply != "+OK\r\n" {
		t.Fatalf("EMB.READY after abort = %q", reply)
	}

	// A later shutdown runs the whole sequence again.
	final := dial(t, addr)
	final.Write(respCommand("SHUTDOWN", "NOSAVE", "NOW"))
	waitFor(t, srv.shuttingDown.Load)
	_ = final.SetReadDeadline(time.Now().Add(time.Second))
	_, _ = final.Write(respCommand("PING"))
	if n, err := final.Read(buf); err == nil {
		t.Fatalf("SHUTDOWN NOW caller got a reply: %q", buf[:n])
	}
}

func TestShutdownAccessControl(t *testing.T) {
	abort := shutdownCmd("ABORT")
	t.Run("remote client without password is denied", func(t *testing.T) {
		s := New("", nil, "", "", nil)
		conn := &recordConn{addr: "10.0.0.5:5555", state: &connState{}}
		s.handleSHUTDOWN(conn, abort)
		if len(conn.errs) != 1 || !strings.HasPrefix(conn.errs[0], "DENIED") {
			t.Fatalf("remote SHUTDOWN errors = %v", conn.errs)
		}
	})

	t.Run("loopback client without password is allowed", func(t *testing.T) {
		s := New("", nil, "", "", nil)
		conn := &recordConn{addr: "127.0.0.1:5555", state: &connState{}}
		s.handleSHUTDOWN(conn, abort)
		if len(conn.errs) != 1 || conn.errs[0] != "ERR No shutdown in progress." {
			t.Fatalf("loopback SHUTDOWN errors = %v", conn.errs)
		}
	})

	t.Run("a password replaces the loopback rule", func(t *testing.T) {
		s := New("", nil, "secret", "", nil)
		conn := &recordConn{addr: "10.0.0.5:5555", state: &connState{}}
		s.handleSHUTDOWN(conn, abort)
		if len(conn.errs) != 1 || conn.errs[0] != "ERR No shutdown in progress." {
			t.Fatalf("authenticated remote SHUTDOWN errors = %v", conn.errs)
		}
	})
}

func TestConfigShutdownParameters(t *testing.T) {
	addr := serveTest(t)

	if got := statusOf(t, redisCmd(t, addr, "CONFIG", "SET", "shutdown-timeout", "10")); got != "OK" {
		t.Fatalf("CONFIG SET shutdown-timeout = %q", got)
	}
	pairs := arrayOf(t, redisCmd(t, addr, "CONFIG", "GET", "shutdown-timeout"))
	if len(pairs) != 2 || bulkOf(t, pairs[0]) != "shutdown_timeout" || bulkOf(t, pairs[1]) != "10" {
		t.Fatalf("CONFIG GET shutdown-timeout = %v", pairs)
	}

	// Existing parameters answer to the Redis spelling too.
	pairs = arrayOf(t, redisCmd(t, addr, "CONFIG", "GET", "cache-save-on-shutdown"))
	if len(pairs) != 2 || bulkOf(t, pairs[0]) != "cache_save_on_shutdown" {
		t.Fatalf("CONFIG GET cache-save-on-shutdown = %v", pairs)
	}

	// A rejected value keeps the previous one.
	if got := errorOf(t, redisCmd(t, addr, "CONFIG", "SET", "shutdown_timeout", "-1")); got == "" {
		t.Fatal("negative shutdown_timeout accepted")
	}
	if got := errorOf(t, redisCmd(t, addr, "CONFIG", "SET", "shutdown_timeout", "1.5")); got == "" {
		t.Fatal("fractional shutdown_timeout accepted")
	}
	pairs = arrayOf(t, redisCmd(t, addr, "CONFIG", "GET", "shutdown_timeout"))
	if bulkOf(t, pairs[1]) != "10" {
		t.Fatalf("shutdown_timeout after rejected values = %q", bulkOf(t, pairs[1]))
	}

	if got := statusOf(t, redisCmd(t, addr, "CONFIG", "SET", "shutdown-on-sigterm", "nosave now")); got != "OK" {
		t.Fatalf("CONFIG SET shutdown-on-sigterm = %q", got)
	}
	pairs = arrayOf(t, redisCmd(t, addr, "CONFIG", "GET", "shutdown_on_sigterm"))
	if bulkOf(t, pairs[1]) != "nosave now" {
		t.Fatalf("shutdown_on_sigterm = %q", bulkOf(t, pairs[1]))
	}
	if got := errorOf(t, redisCmd(t, addr, "CONFIG", "SET", "shutdown_on_sigterm", "save nosave")); got == "" {
		t.Fatal("save nosave accepted for shutdown_on_sigterm")
	}
	pairs = arrayOf(t, redisCmd(t, addr, "CONFIG", "GET", "shutdown_on_sigterm"))
	if bulkOf(t, pairs[1]) != "nosave now" {
		t.Fatalf("shutdown_on_sigterm after rejected value = %q", bulkOf(t, pairs[1]))
	}
}

func TestInfoOmitsShutdownCountdownOutsideAGrace(t *testing.T) {
	addr := serveTest(t)
	if body := bulkOf(t, redisCmd(t, addr, "INFO", "server")); strings.Contains(body, "shutdown_in_milliseconds") {
		t.Fatalf("INFO server reports a countdown without a grace:\n%s", body)
	}
}

func TestHelpListsSHUTDOWN(t *testing.T) {
	addr := serveTest(t)
	body := bulkOf(t, redisCmd(t, addr, "EMB.HELP"))
	if !strings.Contains(body, "SHUTDOWN [NOSAVE|SAVE] [NOW] [FORCE] [ABORT]") {
		t.Fatalf("EMB.HELP missing SHUTDOWN:\n%s", body)
	}
}

func TestAbortRefusedOnceClientsAreStopping(t *testing.T) {
	s := New("", nil, "", "", nil)
	wait, first := s.requestShutdown()
	if !first {
		t.Fatal("the first request must start the sequence")
	}
	s.shutdownMu.Lock()
	cancel := s.shutdownCancel
	s.shutdownMu.Unlock()

	// The sequence has committed to stopping clients; ABORT is too late.
	s.stopping.Store(true)
	if s.abortShutdown() {
		t.Fatal("ABORT succeeded after clients were being stopped")
	}
	select {
	case text := <-wait:
		t.Fatalf("refused ABORT released a waiter with %q", text)
	default:
	}
	select {
	case <-cancel:
		t.Fatal("refused ABORT cancelled the sequence")
	default:
	}
	if !s.ShutdownInProgress() {
		t.Fatal("refused ABORT cleared the running sequence")
	}
	s.finishShutdown()
}

// TestGraceRejectsAnAbortThatLandedBeforeItStarts pins the window between
// Shutdown marking the sequence and grace taking the lock: an ABORT there must
// not leave a stale grace that reports draining and ignores further aborts.
func TestGraceRejectsAnAbortThatLandedBeforeItStarts(t *testing.T) {
	s := New("", nil, "", "", nil)
	s.shutdownTimeout.Store(30)
	if _, first := s.requestShutdown(); !first {
		t.Fatal("the first request must start the sequence")
	}
	if !s.abortShutdown() {
		t.Fatal("the pending shutdown was not abortable")
	}

	start := time.Now()
	err := s.grace(config.ShutdownFlags{})
	if !errors.Is(err, ErrShutdownAborted) {
		t.Fatalf("grace after an abort = %v, want ErrShutdownAborted", err)
	}
	if elapsed := time.Since(start); elapsed > time.Second {
		t.Fatalf("grace waited %v after the abort", elapsed)
	}
	if got := serverState(s.state.Load()); got != stateReady {
		t.Fatalf("state after the aborted grace = %v, want ready", got)
	}
	if s.graceDeadline.Load() != 0 {
		t.Fatal("grace left a deadline after the abort")
	}
}
