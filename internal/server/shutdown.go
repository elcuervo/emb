package server

import (
	"context"
	"errors"
	"log"
	"net"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/tidwall/redcon"

	"github.com/elcuervo/emb/internal/config"
)

// ErrShutdownAborted reports that SHUTDOWN ABORT cancelled a running sequence.
// The entrypoint goes back to waiting for the next request when it sees this.
var ErrShutdownAborted = errors.New("shutdown aborted")

// DefaultShutdownDrainTimeout bounds the drain, snapshot, and telemetry flush
// after the grace ends. It starts after the grace, so the grace never eats
// into it.
const DefaultShutdownDrainTimeout = 30 * time.Second

const (
	// errShutdownAborted is Redis's reply to callers blocked on a SHUTDOWN that
	// was cancelled.
	errShutdownAbortedText = "ERR Errors trying to SHUTDOWN. Check logs."
	// shutdownCronInterval is Redis's default cron period: how often the grace
	// re-checks the deadline and the connected clients.
	shutdownCronInterval = 100 * time.Millisecond
)

// ShutdownRequest is one request for the shutdown sequence: a signal pass from
// the entrypoint's signal watcher, or the SHUTDOWN command.
type ShutdownRequest struct {
	// Source names the requester in logs: "SIGTERM", "SIGINT", or "SHUTDOWN".
	Source string
	// Flags are the command's flags. Signals carry the configured ones, and the
	// entrypoint resolves them when the sequence starts.
	Flags config.ShutdownFlags
	// Preset marks a command that already registered its sequence (and its
	// waiter). If an ABORT resolved it before the loop picked it up, the loop
	// sees ShutdownInProgress false and skips it.
	Preset bool
}

// ShutdownInProgress reports whether a shutdown sequence is pending or running.
func (s *Server) ShutdownInProgress() bool {
	s.shutdownMu.Lock()
	defer s.shutdownMu.Unlock()
	return s.shutdownRunning
}

// WithShutdownRequests wires the entrypoint's shutdown-request loop: the
// SHUTDOWN command sends requests here, and pending is shared with the signal
// watcher so a second SIGINT can tell a shutdown is already scheduled.
func WithShutdownRequests(ch chan<- ShutdownRequest, pending *atomic.Bool) Option {
	return func(s *Server) {
		s.shutdownReqs = ch
		s.shutdownPending = pending
	}
}

// WithShutdownDrainTimeout bounds the drain, snapshot, and telemetry flush that
// follow the grace. Zero keeps DefaultShutdownDrainTimeout.
func WithShutdownDrainTimeout(d time.Duration) Option {
	return func(s *Server) {
		if d > 0 {
			s.drainTimeout = d
		}
	}
}

// WithShutdownConfig sets the grace period in seconds and the flags each signal
// runs the shutdown with, as ParseShutdownFlags strings.
func WithShutdownConfig(timeoutSeconds int, sigterm, sigint string) Option {
	return func(s *Server) {
		s.shutdownTimeout.Store(int64(timeoutSeconds))
		term, err := config.ParseShutdownFlags(sigterm)
		if err != nil {
			// Validation happens in config; fall back to the default rather than
			// letting a bad value panic at boot.
			term = config.ShutdownFlags{}
		}
		intr, err := config.ParseShutdownFlags(sigint)
		if err != nil {
			intr = config.ShutdownFlags{}
		}
		s.shutdownOnSigterm.Store(term.String())
		s.shutdownOnSigint.Store(intr.String())
	}
}

// SignalShutdownFlags resolves the flags a signal runs the shutdown with, from
// the live shutdown_on_sigterm/shutdown_on_sigint values (CONFIG SET applies to
// the next shutdown).
func (s *Server) SignalShutdownFlags(source string) config.ShutdownFlags {
	raw := s.shutdownOnSigterm.Load().(string)
	if source == "SIGINT" {
		raw = s.shutdownOnSigint.Load().(string)
	}
	flags, err := config.ParseShutdownFlags(raw)
	if err != nil {
		// Values are validated on every set, so this is unreachable.
		return config.ShutdownFlags{}
	}
	return flags
}

// Shutdown runs one shutdown sequence: the grace, then client stop, drain,
// snapshot, telemetry flush, and close. The drain budget (drainTimeout) starts
// after the grace; ctx only bounds it further. It returns ErrShutdownAborted
// when SHUTDOWN ABORT cancels the grace; the caller then keeps serving.
func (s *Server) Shutdown(ctx context.Context, flags config.ShutdownFlags) error {
	log.Printf("User requested shutdown...")
	// A signal reaches Shutdown directly; a command already marked the sequence
	// running. Either way ABORT must see it as in progress.
	s.shutdownMu.Lock()
	if !s.shutdownRunning {
		s.shutdownRunning = true
		s.shutdownCancel = make(chan struct{})
		if s.shutdownPending != nil {
			s.shutdownPending.Store(true)
		}
	}
	s.shutdownMu.Unlock()

	if err := s.grace(flags); err != nil {
		return err
	}
	// An ABORT that landed on the grace's deadline boundary wins: refuse new
	// abort attempts from here on, and treat a cleared sequence as cancelled.
	s.shutdownMu.Lock()
	aborted := s.shutdownCancel == nil
	if !aborted {
		s.stopping.Store(true)
	}
	s.shutdownMu.Unlock()
	if aborted {
		return ErrShutdownAborted
	}
	s.stopClients()

	drainCtx, cancel := context.WithTimeout(ctx, s.drainTimeout)
	defer cancel()

	done := make(chan struct{})
	go func() {
		s.active.Wait()
		close(done)
	}()

	timedOut := false
	select {
	case <-done:
	case <-drainCtx.Done():
		log.Printf("shutdown timeout after %v", drainCtx.Err())
		timedOut = true
	}

	s.persistenceMu.RLock()
	coordinator := s.snapshot
	saveOnShutdown := s.cacheSaveOnShutdown
	hasCacheFile := s.cacheFile != ""
	s.persistenceMu.RUnlock()
	if flags.Save && !hasCacheFile {
		log.Printf("SAVE requested but no cache_file is configured; skipping the final snapshot")
	}
	if coordinator != nil {
		coordinator.Shutdown(drainCtx, flags.SaveEnabled(saveOnShutdown))
	}
	if s.telemetryExport != nil {
		s.telemetryExport.Shutdown(drainCtx)
	}
	if gate := s.gate.Load(); gate != nil {
		gate.Release()
	}
	closeErr := s.srv.Close()
	s.finishShutdown()
	if timedOut {
		return errors.Join(ErrShutdownTimeout, drainCtx.Err(), closeErr)
	}
	return closeErr
}

// grace waits, at most shutdown_timeout, while clients other than blocked
// SHUTDOWN callers are connected. It reports ErrShutdownAborted when SHUTDOWN
// ABORT cancels it.
func (s *Server) grace(flags config.ShutdownFlags) error {
	timeout := time.Duration(s.shutdownTimeout.Load()) * time.Second
	if flags.Now || timeout <= 0 {
		return nil
	}

	s.shutdownMu.Lock()
	// An ABORT that landed between Shutdown marking the sequence and here has
	// already cleared the cancel channel: do not start a stale grace, which
	// would report draining and ignore further aborts until its deadline.
	cancel := s.shutdownCancel
	if cancel == nil {
		s.shutdownMu.Unlock()
		return ErrShutdownAborted
	}
	if s.graceClients() == 0 {
		s.shutdownMu.Unlock()
		return nil
	}
	deadline := time.Now().Add(timeout)
	s.graceDeadline.Store(deadline.UnixNano())
	// EMB.READY reports draining during the grace; admission and accept stay
	// open, so every command still runs.
	s.state.Store(int64(stateDraining))
	s.shutdownMu.Unlock()

	defer s.graceDeadline.Store(0)

	log.Printf("Waiting for clients before shutting down.")
	ticker := time.NewTicker(shutdownCronInterval)
	defer ticker.Stop()
	for {
		select {
		case <-cancel:
			s.state.Store(int64(stateReady))
			return ErrShutdownAborted
		case now := <-ticker.C:
			if !now.Before(deadline) || s.graceClients() == 0 {
				return nil
			}
		}
	}
}

// graceClients counts connected clients that are not blocked SHUTDOWN callers.
func (s *Server) graceClients() int {
	s.clientMu.Lock()
	defer s.clientMu.Unlock()
	n := 0
	for c := range s.clients {
		if st, ok := c.Context().(*connState); ok && st != nil && st.blocked.Load() {
			continue
		}
		n++
	}
	return n
}

// stopClients refuses new connections and stops executing commands: idle
// connections close at once, and busy ones close right after their reply.
func (s *Server) stopClients() {
	s.admissionMu.Lock()
	s.shuttingDown.Store(true)
	s.state.Store(int64(stateDraining))
	s.admissionMu.Unlock()

	if gate := s.gate.Load(); gate != nil {
		_ = gate.Close()
	}

	s.clientMu.Lock()
	conns := make([]redcon.Conn, 0, len(s.clients))
	for c := range s.clients {
		conns = append(conns, c)
	}
	s.clientMu.Unlock()

	for _, c := range conns {
		st, _ := c.Context().(*connState)
		if st != nil && st.inProgress.Load() > 0 {
			// A running command keeps its connection: the dispatch closes it
			// from its own goroutine right after the reply is flushed.
			st.closeAfter.Store(true)
			continue
		}
		// An idle connection is closed at the socket, never through redcon's
		// Close: its writer is flushed by the connection's own goroutine, so a
		// flush from here would race with it.
		// shortcut: a reply the handler buffered just before this close may be
		// flushed after it and lost (the client retries); upgrade when redcon
		// exposes a flush on Conn.
		_ = c.NetConn().Close()
	}
}

// requestShutdown registers a waiter and reports whether this caller starts the
// sequence. A caller arriving while one runs only joins the waiter list, and
// its flags are ignored, as in Redis.
func (s *Server) requestShutdown() (wait chan string, first bool) {
	s.shutdownMu.Lock()
	defer s.shutdownMu.Unlock()
	wait = make(chan string, 1)
	s.shutdownWaiters = append(s.shutdownWaiters, wait)
	if s.shutdownRunning {
		return wait, false
	}
	s.shutdownRunning = true
	s.shutdownCancel = make(chan struct{})
	if s.shutdownPending != nil {
		s.shutdownPending.Store(true)
	}
	return wait, true
}

// abortShutdown cancels a pending or running shutdown and releases every
// blocked SHUTDOWN caller.
func (s *Server) abortShutdown() bool {
	s.shutdownMu.Lock()
	cancel := s.shutdownCancel
	if !s.shutdownRunning || cancel == nil || s.stopping.Load() {
		s.shutdownMu.Unlock()
		return false
	}
	s.shutdownCancel = nil
	s.shutdownRunning = false
	s.graceDeadline.Store(0)
	if s.shutdownPending != nil {
		s.shutdownPending.Store(false)
	}
	waiters := s.shutdownWaiters
	s.shutdownWaiters = nil
	s.shutdownMu.Unlock()

	close(cancel)
	s.state.Store(int64(stateReady))
	for _, w := range waiters {
		w <- errShutdownAbortedText
	}
	log.Printf("Shutdown manually aborted.")
	return true
}

// finishShutdown releases blocked SHUTDOWN callers with no reply: their
// connections close as the process exits.
func (s *Server) finishShutdown() {
	s.shutdownMu.Lock()
	s.shutdownRunning = false
	s.stopping.Store(false)
	s.shutdownCancel = nil
	waiters := s.shutdownWaiters
	s.shutdownWaiters = nil
	s.shutdownMu.Unlock()
	for _, w := range waiters {
		w <- ""
	}
}

// handleSHUTDOWN implements SHUTDOWN [NOSAVE|SAVE] [NOW] [FORCE] [ABORT] with
// Redis's parsing: case-insensitive flags, a syntax error for an unknown flag,
// save with nosave, and abort combined with anything else.
func (s *Server) handleSHUTDOWN(conn redcon.Conn, cmd redcon.Command) {
	if !s.shutdownAllowed(conn) {
		conn.WriteError("DENIED SHUTDOWN is only accepted from loopback clients when no password is configured")
		return
	}

	var flags config.ShutdownFlags
	abort := false
	for _, arg := range cmd.Args[1:] {
		switch strings.ToLower(string(arg)) {
		case "nosave":
			flags.NoSave = true
		case "save":
			flags.Save = true
		case "now":
			flags.Now = true
		case "force":
			flags.Force = true
		case "abort":
			abort = true
		default:
			conn.WriteError("ERR syntax error")
			return
		}
	}
	if flags.Save && flags.NoSave || abort && flags != (config.ShutdownFlags{}) {
		conn.WriteError("ERR syntax error")
		return
	}

	if abort {
		if s.abortShutdown() {
			conn.WriteString("OK")
		} else {
			conn.WriteError("ERR No shutdown in progress.")
		}
		return
	}

	if st, ok := conn.Context().(*connState); ok && st != nil {
		// A blocked caller is excluded from the grace count and is never closed
		// as an idle connection.
		st.blocked.Store(true)
		defer st.blocked.Store(false)
	}
	wait, first := s.requestShutdown()
	if first {
		if s.shutdownReqs != nil {
			s.shutdownReqs <- ShutdownRequest{Source: "SHUTDOWN", Flags: flags, Preset: true}
		} else {
			go func() { _ = s.Shutdown(context.Background(), flags) }()
		}
	}
	// The caller blocks: on success it gets no reply, because its connection
	// closes with the process; on abort it gets Redis's error.
	if errText := <-wait; errText != "" {
		conn.WriteError(errText)
	}
}

// shutdownAllowed applies Redis protected mode to SHUTDOWN: with a password the
// dispatch auth gate has already run, and without one only loopback clients may
// stop the server.
func (s *Server) shutdownAllowed(conn redcon.Conn) bool {
	if s.password.Load().(string) != "" {
		return true
	}
	host, _, err := net.SplitHostPort(conn.RemoteAddr())
	if err != nil {
		return false
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}

// gateListener wraps the real listener so redcon's accept loop can be held
// while the socket is closed. redcon closes every connection when Accept
// returns an error, so the gate closes the real socket (new dials are refused)
// yet keeps Accept blocked until Release.
type gateListener struct {
	net.Listener
	mu      sync.Mutex
	closed  bool
	release chan struct{}
}

func newGateListener(ln net.Listener) *gateListener {
	return &gateListener{Listener: ln, release: make(chan struct{})}
}

func (g *gateListener) Close() error {
	g.mu.Lock()
	defer g.mu.Unlock()
	if g.closed {
		return nil
	}
	g.closed = true
	return g.Listener.Close()
}

func (g *gateListener) Accept() (net.Conn, error) {
	c, err := g.Listener.Accept()
	if err == nil {
		return c, nil
	}
	g.mu.Lock()
	closed := g.closed
	g.mu.Unlock()
	if !closed {
		return nil, err
	}
	<-g.release
	return nil, net.ErrClosed
}

// Release lets a held Accept return net.ErrClosed, which ends redcon's serve
// loop and its close-all only after every connection has finished.
func (g *gateListener) Release() {
	g.mu.Lock()
	defer g.mu.Unlock()
	select {
	case <-g.release:
	default:
		close(g.release)
	}
}
