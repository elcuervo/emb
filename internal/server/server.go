package server

import (
	"context"
	"crypto/tls"
	"encoding/binary"
	"errors"
	"fmt"
	"log"
	"math"
	"net"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/tidwall/redcon"

	"github.com/elcuervo/emb/internal/config"
	"github.com/elcuervo/emb/internal/registry"
	"github.com/elcuervo/emb/internal/script"
)

type serverState int64

const (
	stateLoading serverState = iota
	stateReady
	stateDraining
)

type connState struct {
	authenticated bool
}

type Server struct {
	reg          *registry.Registry
	srv          *redcon.Server
	ln           net.Listener
	active       sync.WaitGroup
	admissionMu  sync.Mutex
	shuttingDown atomic.Bool
	started      time.Time
	addr         string
	password     atomic.Value // string; live-updatable via CONFIG SET
	tlsConfig    *tls.Config
	tlsCert      string
	tlsKey       string
	cache        *Cache
	// cacheConfig retains the live cache size string (the boot value, then each
	// CONFIG SET value) so CONFIG GET echoes the current configuration. It is an
	// atomic.Value because CONFIG SET writes it while CONFIG GET reads it.
	cacheConfig atomic.Value // string
	// scripts is the per-model script cache for EMB.SCRIPT/EMB.EVAL/EMB.EVSHA.
	scripts *scriptCache
	// compiler caches compiled script prototypes per (model, sha) so repeat
	// EVSHA executions skip Lua parsing/compiling; FLUSH invalidates it.
	compiler *script.Compiler
	// cacheFile/cacheSave are runtime-editable snapshot parameters (consumed by
	// the cache-snapshot save loop; stored here even before that change lands).
	cacheFile           string
	cacheSave           string
	cacheLoad           bool
	cacheSaveOnShutdown bool
	cacheRestoreLimit   string
	cacheRestoreReserve string
	cacheSaveRateLimit  string
	persistenceMu       sync.RWMutex
	persistenceCfg      *PersistenceConfig
	snapshot            *snapshotCoordinator

	// quarantine holds restored snapshot entries for configured-but-unloaded
	// (lazy) models until their first request loads and validates them. It is
	// bounded by the same restore budget and is never served directly.
	quarantineMu    sync.Mutex
	quarantine      map[string]restoreQuarantine
	quarantineBytes int64
	// version is the injected build version ("dev" when unset), reported by INFO.
	version string
	state   atomic.Int64
	// conns counts accepted, unclosed connections (incremented in the redcon
	// accept callback, decremented in the closed callback — rejected conns are
	// never counted, keeping the two balanced).
	conns atomic.Int64
	// activeReqs counts EMB/EMB.MULTI commands currently being processed, used
	// both for EMB.STATS and for the max_concurrent_requests gate.
	activeReqs        atomic.Int64
	idleTimeout       time.Duration
	maxConns          int
	maxConcurrentReqs int
	// maxTexts bounds texts per EMB command (0 = unlimited; default 4096 via New).
	// Oversized commands are truncated: overflow texts are not processed and their
	// reply slots are null. Atomic because CONFIG SET mutates it while request
	// handlers read it.
	maxTexts atomic.Int64
	// maxPairs bounds pairs per EMB.MULTI command (0 = unlimited; default 4096).
	// Oversized commands are truncated: overflow pairs are not processed and their
	// reply slots are null. Atomic for the same reason as maxTexts.
	maxPairs       atomic.Int64
	truncatedTexts atomic.Int64
	truncatedPairs atomic.Int64
	// maxImages bounds images per EMB.IMG/EMB.IMGMULTI command (0 = unlimited;
	// default 4096). Overflow images are not decoded or inferred and their reply
	// slots are null. Atomic because CONFIG SET mutates it while request handlers
	// read it.
	maxImages atomic.Int64
	// maxImageBytes/maxImagePixels bound one image argument and its decoded pixel
	// count (0 = unlimited). Atomic because CONFIG SET mutates them while request
	// handlers read them.
	maxImageBytes  atomic.Int64
	maxImagePixels atomic.Int64
	// maxCommandBytes bounds the buffered bytes of a single command (0 = unlimited).
	// Atomic because CONFIG SET mutates it while the dispatch path reads it.
	maxCommandBytes atomic.Int64
	// imageRequests counts processed image requests (one per EMB.IMG command, one
	// per EMB.IMGMULTI pair); truncatedImages counts overflow images.
	imageRequests   atomic.Int64
	truncatedImages atomic.Int64
	// fanOut bounds concurrent EMB.MULTI pair processing for a single command, so a
	// request storm cannot spawn unbounded goroutines competing for inference cores.
	// 0 resolves to the machine's GOMAXPROCS. Overridable for tests.
	fanOut int
	// scriptDeadline bounds each EMB.EVAL/EMB.EVSHA evaluation's wall-clock
	// execution; 0 resolves to script.DefaultDeadline.
	scriptDeadline time.Duration
	// netIn/netOut are aggregate RESP bytes received from and sent to all
	// connections since process start, exposed as INFO's
	// total_net_input_bytes/total_net_output_bytes. RX is counted from the raw
	// command bytes at dispatch; TX is counted by countingConn around every reply.
	netIn  atomic.Uint64
	netOut atomic.Uint64
	// monitor records completed-request events for MONITOR (bounded ring).
	monitor *Monitor
	// scripted-evaluation counters (EMB.EVAL/EMB.EVSHA), reported by EMB.STATS
	// as script_requests/script_errors/script_avg_latency_us. Latency is
	// cumulative microseconds over completed evaluations.
	scriptRequests  atomic.Int64
	scriptErrors    atomic.Int64
	scriptLatencyUs atomic.Int64
}

// ErrShutdownTimeout reports that accepted work outlived the shutdown
// deadline. Callers must not destroy native model resources in this case.
var ErrShutdownTimeout = errors.New("server shutdown deadline exceeded")

func isInferenceCommand(name string) bool {
	switch name {
	case "emb", "emb.multi", "emb.img", "emb.imgmulti", "emb.eval", "emb.evsha":
		return true
	default:
		return false
	}
}

// admitInference atomically checks draining/capacity and registers accepted
// work with the shutdown waiter. The returned release must be called once.
func (s *Server) admitInference() (release func(), replyErr string) {
	s.admissionMu.Lock()
	defer s.admissionMu.Unlock()
	if s.shuttingDown.Load() {
		return nil, "ERR server shutting down"
	}
	if s.maxConcurrentReqs > 0 && s.activeReqs.Load() >= int64(s.maxConcurrentReqs) {
		return nil, fmt.Sprintf("ERR busy: max concurrent requests exceeded (%d)", s.maxConcurrentReqs)
	}
	s.activeReqs.Add(1)
	s.active.Add(1)
	var once sync.Once
	return func() {
		once.Do(func() {
			s.activeReqs.Add(-1)
			s.active.Done()
		})
	}, ""
}

func (s *Server) beginDraining() {
	s.admissionMu.Lock()
	s.shuttingDown.Store(true)
	s.state.Store(int64(stateDraining))
	s.admissionMu.Unlock()
}

// Option configures a Server.
type Option func(*Server)

// WithIdleTimeout closes connections that have not sent a command for the
// given duration. Zero (the default) never closes idle connections, matching
// Redis semantics.
func WithIdleTimeout(d time.Duration) Option {
	return func(s *Server) { s.idleTimeout = d }
}

// WithMaxConnections refuses and closes new connections beyond the cap.
// Zero (the default) is unlimited.
func WithMaxConnections(n int) Option {
	return func(s *Server) { s.maxConns = n }
}

// WithMaxConcurrentRequests answers EMB/EMB.MULTI commands with a busy error
// while this many are already in flight. Zero (the default) is unlimited.
// Control commands (PING, AUTH, EMB.READY, EMB.STATS, …) stay reachable.
func WithMaxConcurrentRequests(n int) Option {
	return func(s *Server) { s.maxConcurrentReqs = n }
}

// WithMaxTexts bounds the texts processed per EMB command; commands beyond the
// cap are truncated to the first maxTexts texts (overflow reply slots are null).
// Zero disables the cap (unlimited, pre-change behavior). The default when
// unset is 4096.
func WithMaxTexts(n int) Option {
	return func(s *Server) { s.maxTexts.Store(int64(n)) }
}

// WithScriptDeadline bounds each EMB.EVAL/EMB.EVSHA script evaluation's
// wall-clock execution. Zero (the default) uses script.DefaultDeadline.
func WithScriptDeadline(d time.Duration) Option {
	return func(s *Server) { s.scriptDeadline = d }
}

// WithMaxPairs bounds the pairs processed per EMB.MULTI command; commands beyond
// the cap are truncated to the first maxPairs pairs (overflow reply slots are
// null). Zero disables the cap (unlimited, pre-change behavior). The default
// when unset is 4096.
func WithMaxPairs(n int) Option {
	return func(s *Server) { s.maxPairs.Store(int64(n)) }
}

// WithMaxImages bounds the images processed per EMB.IMG/EMB.IMGMULTI command;
// overflow images are not decoded or inferred and their reply slots are null.
// Zero disables the cap (unlimited). The default when unset is 4096.
func WithMaxImages(n int) Option {
	return func(s *Server) { s.maxImages.Store(int64(n)) }
}

// WithMaxImageBytes bounds one image argument's byte size. Zero disables the cap.
func WithMaxImageBytes(n int64) Option {
	return func(s *Server) { s.maxImageBytes.Store(n) }
}

// WithMaxImagePixels bounds one image's decoded pixel count (checked from the
// header before the full decode). Zero disables the cap.
func WithMaxImagePixels(n int64) Option {
	return func(s *Server) { s.maxImagePixels.Store(n) }
}

// WithMaxCommandBytes bounds the buffered bytes of a single command. Zero
// disables the cap.
func WithMaxCommandBytes(n int64) Option {
	return func(s *Server) { s.maxCommandBytes.Store(n) }
}

func WithPersistence(cfg PersistenceConfig) Option {
	return func(s *Server) {
		s.persistenceCfg = &cfg
		s.cacheFile = cfg.File
		s.cacheLoad = cfg.Load
		s.cacheSaveOnShutdown = cfg.SaveOnShutdown
		s.cacheRestoreLimit = cfg.RestoreLimit
		s.cacheRestoreReserve = cfg.RestoreReserve
		s.cacheSaveRateLimit = cfg.SaveRateRaw
		if cfg.SaveInterval > 0 {
			s.cacheSave = cfg.SaveInterval.String()
		}
	}
}

func New(addr string, reg *registry.Registry, password string, cacheConfig string, tlsConfig *tls.Config, opts ...Option) *Server {
	cacheBytes, err := parseCacheConfig(cacheConfig)
	if err != nil {
		log.Fatalf("parsing cache config: %v", err)
	}
	var c *Cache
	if cacheBytes > 0 {
		c = NewCache(cacheBytes)
	}

	s := &Server{
		reg:                 reg,
		started:             time.Now(),
		addr:                addr,
		tlsConfig:           tlsConfig,
		cache:               c,
		scripts:             newScriptCache(0),
		compiler:            script.NewCompiler(),
		cacheLoad:           true,
		cacheSaveOnShutdown: true,
		version:             "dev",
		idleTimeout:         config.DefaultIdleTimeout,
		monitor:             NewMonitor(8192),
	}
	s.cacheConfig.Store(cacheConfig)
	s.maxTexts.Store(4096)
	s.maxPairs.Store(4096)
	s.maxImages.Store(4096)
	s.maxImageBytes.Store(config.DefaultMaxImageBytes)
	s.maxImagePixels.Store(config.DefaultMaxImagePixels)
	s.maxCommandBytes.Store(config.DefaultMaxCommandBytes)
	for _, o := range opts {
		o(s)
	}
	s.password.Store(password)
	if s.persistenceCfg != nil && s.persistenceCfg.File != "" && s.cache != nil {
		s.snapshot = newSnapshotCoordinator(s.cache, s.reg, *s.persistenceCfg)
		if s.persistenceCfg.Load {
			s.restoreSnapshot(*s.persistenceCfg)
		}
	}

	mux := redcon.NewServeMux()
	mux.HandleFunc("ping", s.handlePING)
	mux.HandleFunc("hello", s.handleHELLO)
	mux.HandleFunc("client", s.handleCLIENT)
	mux.HandleFunc("auth", s.handleAUTH)
	mux.HandleFunc("emb", s.handleEMB)
	mux.HandleFunc("emb.models", s.handleMODELS)
	mux.HandleFunc("emb.info", s.handleINFO)
	mux.HandleFunc("emb.stats", s.handleSTATS)
	mux.HandleFunc("monitor", s.handleMonitor)
	mux.HandleFunc("emb.help", s.handleHELP)
	mux.HandleFunc("emb.multi", s.handleEMBMULTI)
	mux.HandleFunc("emb.img", s.handleIMG)
	mux.HandleFunc("emb.imgmulti", s.handleIMGMULTI)
	mux.HandleFunc("emb.ready", s.handleREADY)
	mux.HandleFunc("emb.eval", s.handleEVAL)
	mux.HandleFunc("emb.evsha", s.handleEVSHA)
	mux.HandleFunc("emb.script", s.handleSCRIPT)
	mux.HandleFunc("info", s.handleInfo)
	mux.HandleFunc("config", s.handleConfig)
	mux.HandleFunc("emb.cache.flush", s.handleCACHEFLUSH)
	mux.HandleFunc("emb.save", s.handleSAVE)

	s.srv = redcon.NewServer(addr, func(conn redcon.Conn, cmd redcon.Command) {
		// Account the received command: cmd.Raw is the full RESP bytes including
		// framing. Counted before auth so ALL traffic is reflected in INFO.
		if len(cmd.Raw) > 0 {
			s.netIn.Add(uint64(len(cmd.Raw)))
		}
		// Wrap the connection so every reply written by any handler is counted
		// towards total_net_output_bytes.
		conn = s.wrapConn(conn)

		// Command-size guard: a command whose buffered bytes exceed the cap is
		// rejected without decode or inference. The per-bulk guard (SetMaxBulkSize
		// below) refuses an oversized declared bulk before its payload is read.
		if limit := s.maxCommandBytes.Load(); limit > 0 && int64(len(cmd.Raw)) > limit {
			conn.WriteError(fmt.Sprintf("ERR command of %d bytes exceeds max_command_bytes of %d", len(cmd.Raw), limit))
			return
		}

		if s.password.Load().(string) != "" && !isExempt(cmd) && !isAuthenticated(conn) {
			conn.WriteError("NOAUTH Authentication required.")
			return
		}
		// Bounded concurrency: only inference work goes through the gate; control
		// commands keep answering during saturation so operators can still
		// observe and probe. The counter increments for every EMB command so
		// EMB.STATS/INFO report live in-flight counts even when the cap is 0.
		if len(cmd.Args) > 0 {
			name := strings.ToLower(string(cmd.Args[0]))
			if isInferenceCommand(name) {
				release, replyErr := s.admitInference()
				if replyErr != "" {
					conn.WriteError(replyErr)
					return
				}
				defer release()
			}
		}
		mux.ServeRESP(conn, cmd)
	},
		func(conn redcon.Conn) bool {
			conn.SetContext(&connState{})
			s.admissionMu.Lock()
			defer s.admissionMu.Unlock()
			if s.shuttingDown.Load() {
				return false
			}
			if s.maxConns > 0 && s.conns.Load() >= int64(s.maxConns) {
				// redcon closes refused conns without firing the closed handler,
				// so refusing before counting keeps the accounting balanced.
				return false
			}
			s.conns.Add(1)
			return true
		},
		func(conn redcon.Conn, err error) {
			s.conns.Add(-1)
		},
	)

	// Reap idle connections when configured; redcon applies a read deadline per
	// command read (idleClose), and a reaped connection flows through the closed
	// handler so `connections` stays accurate.
	if s.idleTimeout > 0 {
		s.srv.SetIdleClose(s.idleTimeout)
	}

	// Bound both a single declared bulk and the cumulative RESP bytes of one
	// command in the reader, before any payload is buffered. max_command_bytes is
	// an aggregate cap: without SetMaxCommandSize a command made of many sub-cap
	// bulks would be buffered in full and only rejected at dispatch.
	if limit := s.maxCommandBytes.Load(); limit > 0 {
		s.srv.SetMaxBulkSize(limit)
		s.srv.SetMaxCommandSize(limit)
	}

	return s
}

func (s *Server) ListenAndServe() error {
	ln, err := s.listen()
	if err != nil {
		return err
	}
	return s.srv.Serve(ln)
}

// Start binds synchronously, then serves in a goroutine. Binding before the
// caller waits for signals removes the startup race where shutdown could run
// before the listener existed.
func (s *Server) Start() (<-chan error, error) {
	ln, err := s.listen()
	if err != nil {
		return nil, err
	}
	done := make(chan error, 1)
	go func() {
		done <- s.srv.Serve(ln)
	}()
	return done, nil
}

func (s *Server) listen() (net.Listener, error) {
	var ln net.Listener
	var err error
	if s.tlsConfig != nil {
		ln, err = tls.Listen("tcp", s.addr, s.tlsConfig)
	} else {
		ln, err = net.Listen("tcp", s.addr)
	}
	if err != nil {
		return nil, fmt.Errorf("listening on %s: %w", s.addr, err)
	}
	s.ln = ln
	if s.tlsConfig != nil {
		log.Printf("emb listening on %s (TLS)", s.addr)
	} else {
		log.Printf("emb listening on %s", s.addr)
	}
	return ln, nil
}

func (s *Server) Shutdown(ctx context.Context) error {
	// Keep redcon's accept loop alive while accepted requests drain. New TCP
	// connections reach the accept callback above and are rejected, while
	// existing sockets remain open long enough to receive completed replies.
	s.beginDraining()

	done := make(chan struct{})
	go func() {
		s.active.Wait()
		close(done)
	}()

	timedOut := false
	select {
	case <-done:
	case <-ctx.Done():
		log.Printf("shutdown timeout after %v", ctx.Err())
		timedOut = true
	}
	s.persistenceMu.RLock()
	coordinator := s.snapshot
	s.persistenceMu.RUnlock()
	if coordinator != nil {
		coordinator.Shutdown(ctx)
	}

	closeErr := s.srv.Close()
	if timedOut {
		return errors.Join(ErrShutdownTimeout, ctx.Err(), closeErr)
	}
	return closeErr
}

func (s *Server) restoreSnapshot(cfg PersistenceConfig) {
	cacheBytes := s.cache.Stats().MaxBytes
	limit, rss, headroom, err := effectiveRestoreLimit(cacheBytes, cfg.RestoreLimit, cfg.RestoreReserve)
	status := SnapshotStatus{Enabled: true, RestoreLimitBytes: limit, RestoreRSSBytes: rss, RestoreHeadroomBytes: headroom}
	if err == nil && limit <= 0 {
		status.RestoreError = fmt.Sprintf("effective restore limit is zero (host headroom exhausted with cache_restore_reserve %q); snapshot not restored", cfg.RestoreReserve)
		log.Printf("cache snapshot restore skipped: %s", status.RestoreError)
	}
	if err == nil && limit > 0 {
		models := s.reg.FingerprintState()
		var restored snapshotRestoreResult
		restored, err = readSnapshot(cfg.File, limit, models)
		if err == nil && restored.Found {
			s.cache.replaceStorageFrom(restored.Cache)
			s.snapshot.lastGen.Store(s.cache.Stats().Generation)
			s.snapshot.savedOnce.Store(true)
			status.RestoredEntries = restored.Restored
			status.SkippedUnknown = restored.SkippedUnknown
			status.SkippedFingerprint = restored.SkippedFingerprint
			status.SkippedMemory = restored.SkippedMemory
			status.QuarantinedEntries = restored.QuarantinedCount
			s.quarantineMu.Lock()
			s.quarantine = restored.Quarantine
			s.quarantineBytes = restored.QuarantineBytes
			s.quarantineMu.Unlock()
		}
	}
	if err != nil {
		status.RestoreError = err.Error()
		log.Printf("cache snapshot restore skipped: %v", err)
	}
	s.snapshot.statusMu.Lock()
	s.snapshot.status = status
	s.snapshot.statusMu.Unlock()
}

// admitQuarantine publishes a lazy model's restored snapshot entries once the
// model has loaded and its fingerprint matches the snapshot's stored
// fingerprint. Incompatible or over-budget records are discarded; the
// quarantine bucket is removed either way so admission runs at most once per
// model per process.
func (s *Server) admitQuarantine(model string, entry *registry.ModelEntry) {
	s.quarantineMu.Lock()
	q, ok := s.quarantine[model]
	if !ok {
		s.quarantineMu.Unlock()
		return
	}
	delete(s.quarantine, model)
	s.quarantineBytes -= q.bytes
	s.quarantineMu.Unlock()

	fp, err := entry.Fingerprint()
	if err != nil || fp != q.fingerprint {
		// Model files changed since the snapshot was written (or became
		// unreadable): the restored embeddings no longer match and are unsafe
		// to serve, so they are discarded.
		return
	}
	for _, e := range q.entries {
		s.cache.Set(e.Key, e.Value)
	}
}

func (s *Server) Close() error {
	s.beginDraining()
	s.persistenceMu.RLock()
	coordinator := s.snapshot
	s.persistenceMu.RUnlock()
	if coordinator != nil {
		coordinator.Close()
	}
	return s.srv.Close()
}

func (s *Server) SetReady() {
	s.state.Store(int64(stateReady))
}

func (s *Server) SetDraining() {
	s.state.Store(int64(stateDraining))
}

// SetVersion injects the build version (reported by INFO's redis_version /
// emb_version). The default is "dev", matching the -version flag default.
func (s *Server) SetVersion(v string) {
	s.version = v
}

// SetTLSConfigPaths records the raw TLS cert/key paths for CONFIG GET. The
// loaded tls.Config is boot-only; the paths are informational.
func (s *Server) SetTLSConfigPaths(cert, key string) {
	s.tlsCert = cert
	s.tlsKey = key
}

// PreloadScript validates, caches, and precompiles a script for a model.
// It is used at boot time from file paths declared in config. A bad script
// (unknown model, oversized, invalid Lua) returns an error so the caller can
// fail fatally. The returned string is the script's SHA1.
func (s *Server) PreloadScript(model, src string) (string, error) {
	if _, err := s.reg.Resolve(model); err != nil {
		return "", err
	}
	if len(src) > script.DefaultMaxScriptBytes {
		return "", script.ErrScriptTooLarge
	}
	if err := script.Compile(src); err != nil {
		return "", err
	}
	sha, _ := s.scripts.Load(model, src)
	if err := s.compiler.Precompile(model, src); err != nil {
		return "", err
	}
	return sha, nil
}

func (s *Server) handleREADY(conn redcon.Conn, cmd redcon.Command) {
	state := serverState(s.state.Load())
	if state == stateLoading && len(s.reg.List()) == 0 {
		conn.WriteError("no models")
		return
	}
	switch state {
	case stateReady:
		conn.WriteString("OK")
	case stateLoading:
		conn.WriteError("loading")
	case stateDraining:
		conn.WriteError("draining")
	}
}

func isExempt(cmd redcon.Command) bool {
	if len(cmd.Args) == 0 {
		return false
	}
	name := strings.ToLower(string(cmd.Args[0]))
	return name == "auth" || name == "ping" || name == "emb.ready" || name == "info"
}

func isAuthenticated(conn redcon.Conn) bool {
	state, ok := conn.Context().(*connState)
	return ok && state.authenticated
}

func (s *Server) handleAUTH(conn redcon.Conn, cmd redcon.Command) {
	if s.password.Load().(string) == "" {
		conn.WriteError("ERR Client sent AUTH, but no password is set")
		return
	}
	if len(cmd.Args) != 2 {
		conn.WriteError("ERR wrong number of arguments for 'AUTH' command")
		return
	}
	if string(cmd.Args[1]) != s.password.Load().(string) {
		conn.WriteError("ERR invalid password")
		return
	}
	conn.Context().(*connState).authenticated = true
	conn.WriteString("OK")
}

func (s *Server) handlePING(conn redcon.Conn, cmd redcon.Command) {
	conn.WriteString("PONG")
}

// handleCLIENT answers the small subset of CLIENT subcommands that RESP3 client
// handshakes send. SETINFO carries client library metadata and is acknowledged
// with OK; unknown subcommands get a NO such subcommand error. Without this,
// handshakes (e.g. redis-py's) error out right after HELLO 3.
func (s *Server) handleCLIENT(conn redcon.Conn, cmd redcon.Command) {
	if len(cmd.Args) < 2 {
		conn.WriteError("ERR wrong number of arguments for 'CLIENT' command")
		return
	}
	switch strings.ToLower(string(cmd.Args[1])) {
	case "setinfo":
		if len(cmd.Args) != 4 {
			conn.WriteError("ERR wrong number of arguments for 'CLIENT SETINFO' command")
			return
		}
		conn.WriteString("OK")
	default:
		conn.WriteError(fmt.Sprintf("ERR unknown subcommand '%s' for 'CLIENT' command", cmd.Args[1]))
	}
}

// handleHELLO negotiates the RESP protocol version for the connection. A bare
// HELLO reports the current version; HELLO 2|3 switches the connection. The
// reply carries server metadata in the standard Redis HELLO shape (a map under
// RESP3, a flat array under RESP2) via the fork's WriteHello helper. HELLO is
// deliberately NOT auth-exempt, so on a password-protected server the mux gate
// rejects it with NOAUTH before this handler runs (see the spec's "HELLO
// respects authentication" scenario).
func (s *Server) handleHELLO(conn redcon.Conn, cmd redcon.Command) {
	ver := conn.ProtocolVersion()
	if len(cmd.Args) > 2 {
		conn.WriteError("ERR wrong number of arguments for 'HELLO' command")
		return
	}
	if len(cmd.Args) == 2 {
		v, err := strconv.Atoi(string(cmd.Args[1]))
		if err != nil || (v != 2 && v != 3) {
			conn.WriteError(fmt.Sprintf("NOPROTO unsupported protocol version: %d", v))
			return
		}
		ver = v
	}
	conn.SetProtocolVersion(ver)
	redcon.WriteHello(conn,
		"server", "redis",
		"version", s.version,
		"proto", strconv.Itoa(ver),
		"mode", "standalone",
		"role", "master",
	)
}

func (s *Server) handleCACHEFLUSH(conn redcon.Conn, cmd redcon.Command) {
	if len(cmd.Args) > 2 {
		conn.WriteError("ERR wrong number of arguments for 'EMB.CACHE.FLUSH' command")
		return
	}
	s.quarantineMu.Lock()
	if len(cmd.Args) == 1 {
		s.quarantine = nil
		s.quarantineBytes = 0
	} else {
		model := string(cmd.Args[1])
		if q, ok := s.quarantine[model]; ok {
			s.quarantineBytes -= q.bytes
			delete(s.quarantine, model)
		}
	}
	s.quarantineMu.Unlock()
	if s.cache == nil {
		conn.WriteInt(0)
		return
	}
	if len(cmd.Args) == 1 {
		conn.WriteInt(s.cache.Flush())
		return
	}
	model := string(cmd.Args[1])
	if !s.reg.HasModel(model) {
		conn.WriteError(fmt.Sprintf("ERR model '%s' not found", model))
		return
	}
	conn.WriteInt(s.cache.FlushModel(model))
}

func (s *Server) handleSAVE(conn redcon.Conn, cmd redcon.Command) {
	if len(cmd.Args) != 1 {
		conn.WriteError("ERR wrong number of arguments for 'EMB.SAVE' command")
		return
	}
	if !s.persistenceControlAllowed() {
		conn.WriteError("ERR EMB.SAVE requires a configured password or a loopback-only listener")
		return
	}
	s.persistenceMu.RLock()
	coordinator := s.snapshot
	s.persistenceMu.RUnlock()
	if coordinator == nil {
		conn.WriteError("ERR cache persistence is disabled (cache_file is empty)")
		return
	}
	if err := coordinator.Save(context.Background()); err != nil {
		conn.WriteError("ERR " + err.Error())
		return
	}
	conn.WriteString("OK")
}

// replyFormat selects the reply representation of an embedding query, mirroring
// RedisAI's AI.TENSORGET <key> [BLOB|VALUES]. BLOB is the default and keeps the
// compact binary wire; VALUES returns a self-describing envelope.
type replyFormat int

const (
	formatBLOB replyFormat = iota
	formatVALUES
)

// parseFormatArg recognizes the leading reply-format keyword at a fixed
// position. A keyword is only recognized when at least one payload argument
// follows it; otherwise the position is an ordinary text (or model) argument
// and the format defaults to BLOB. The keyword is never an end-of-command
// sentinel, so trailing free text can never shadow it.
// isKeyword reports whether arg equals kw case-insensitively (ASCII) without
// allocating — so the reply-format check on the default BLOB path adds no
// per-command allocation.
func isKeyword(arg []byte, kw string) bool {
	if len(arg) != len(kw) {
		return false
	}
	for i := 0; i < len(arg); i++ {
		c := arg[i]
		if c >= 'A' && c <= 'Z' {
			c += 'a' - 'A'
		}
		if c != kw[i] {
			return false
		}
	}
	return true
}

func parseFormatArg(args [][]byte, keywordPos int) (replyFormat, bool) {
	if len(args) <= keywordPos+1 {
		return formatBLOB, false
	}
	switch {
	case isKeyword(args[keywordPos], "blob"):
		return formatBLOB, true
	case isKeyword(args[keywordPos], "values"):
		return formatVALUES, true
	default:
		return formatBLOB, false
	}
}

// embedTexts resolves embeddings for `texts` against `model` through the
// shared embedding path: cache lookups, one batched inference for the misses,
// and cache writes. It is the single entry point used by both the EMB command
// and the scripted `emb.embed` host, so embeddings produced by either path
// share cache entries and batcher admission. entry.Pool must already be
// resolved (see Registry.GetOrInit).
func (s *Server) embedTexts(entry *registry.ModelEntry, model string, texts []string) ([][]byte, error) {
	if s.cache == nil {
		resp, err := entry.Pool.Embed(texts)
		if err != nil {
			return nil, err
		}
		if resp.Err != nil {
			return nil, resp.Err
		}
		return resp.Embeddings, nil
	}

	results := make([][]byte, len(texts))
	// Admit any restored snapshot entries now that the model is loaded and its
	// fingerprint is verified, before the first cache lookup, so every embedding
	// path (EMB and scripted emb.embed) sees the recovered entries.
	s.admitQuarantine(model, entry)
	var missIdxs []int
	for i, text := range texts {
		if emb, ok := s.cache.Get(textCacheKey(model, text)); ok {
			results[i] = emb
		} else {
			missIdxs = append(missIdxs, i)
		}
	}
	if len(missIdxs) == 0 {
		return results, nil
	}

	missTexts := make([]string, len(missIdxs))
	for j, idx := range missIdxs {
		missTexts[j] = texts[idx]
	}
	resp, err := entry.Pool.Embed(missTexts)
	if err != nil {
		return nil, err
	}
	if resp.Err != nil {
		return nil, resp.Err
	}
	for j, idx := range missIdxs {
		results[idx] = resp.Embeddings[j]
		s.cache.Set(textCacheKey(model, texts[idx]), resp.Embeddings[j])
	}
	return results, nil
}

func (s *Server) handleEMB(conn redcon.Conn, cmd redcon.Command) {
	if len(cmd.Args) < 3 {
		conn.WriteError("ERR wrong number of arguments for 'EMB' command")
		return
	}

	// Start the MONITOR clock before argument parsing and conversion so the
	// recorded latency covers the whole request, not just inference.
	started := time.Now()

	modelName := string(cmd.Args[1])
	// Leading reply-format keyword (EMB <model> [BLOB|VALUES] <text>...):
	// recognized only at the fixed position right after the model, never in the
	// text tail.
	format, hasFormat := parseFormatArg(cmd.Args[1:], 1)
	textArgs := cmd.Args[2:]
	if hasFormat {
		textArgs = cmd.Args[3:]
	}
	texts := make([]string, len(textArgs))
	for i, arg := range textArgs {
		texts[i] = string(arg)
	}
	total := len(texts)

	// Record the request completion for MONITOR (bounded ring, no text
	// payloads). Latency spans argument parsing through reply writing.
	failed := false
	defer func() {
		s.monitor.Add(MonitorEvent{
			AtUs:      started.UnixMicro(),
			Model:     modelName,
			Texts:     total,
			LatencyUs: time.Since(started).Microseconds(),
			Err:       failed,
		})
	}()

	// Truncate oversized commands: process only the first maxTexts texts and
	// reply with null slots for the overflow. Truncation bounds the inference
	// work of a single command so the payload size cannot pin the task's cores.
	if limit := s.maxTexts.Load(); limit > 0 && int64(total) > limit {
		s.truncatedTexts.Add(int64(total) - limit)
		texts = texts[:int(limit)]
	}

	entry, err := s.reg.GetOrInit(modelName)
	if err != nil {
		failed = true
		conn.WriteError(fmt.Sprintf("ERR %v", err))
		return
	}

	results, err := s.embedTexts(entry, modelName, texts)
	if err != nil {
		failed = true
		conn.WriteError(fmt.Sprintf("ERR %v", err))
		return
	}

	// Single-text requests keep their single-bulk reply shape; multi-text
	// replies are arrays with null slots for truncated overflow texts.
	writeEmbResult(conn, format, results, total, entry.Dim)
}

// writeEmbResult routes an embedding reply to the requested format: the BLOB
// binary wire (unchanged shapes) or the RedisAI-style VALUES envelope.
func writeEmbResult(conn redcon.Conn, format replyFormat, results [][]byte, total, dim int) {
	if format == formatVALUES {
		writeValuesEmbReply(conn, results, dim)
		return
	}
	writeEmbReply(conn, results, total)
}

// writeEmbReply writes one reply slot per requested text: bulks for the results
// (the processed prefix) and nulls for truncated overflow texts after them. A
// single-text request keeps its single-bulk reply shape.
func writeEmbReply(conn redcon.Conn, results [][]byte, total int) {
	if total == 1 && len(results) == 1 {
		conn.WriteBulk(results[0])
		return
	}
	conn.WriteArray(total)
	for _, r := range results {
		conn.WriteBulk(r)
	}
	for i := len(results); i < total; i++ {
		conn.WriteNull()
	}
}

func (s *Server) handleMonitor(conn redcon.Conn, cmd redcon.Command) {
	after := uint64(0)
	limit := 500
	if len(cmd.Args) > 1 {
		if n, err := strconv.ParseUint(string(cmd.Args[1]), 10, 64); err == nil {
			after = n
		}
	}
	if len(cmd.Args) > 2 {
		if n, err := strconv.Atoi(string(cmd.Args[2])); err == nil {
			limit = n
		}
	}
	events := s.monitor.Since(after, limit)
	conn.WriteArray(len(events))
	for _, e := range events {
		if conn.ProtocolVersion() == 3 {
			// RESP3: a map per event, with the same field names as the
			// RESP2 flat array (see the resp3-protocol spec).
			conn.WriteMap(6)
			conn.WriteBulkString("seq")
			conn.WriteInt(int(e.Seq))
			conn.WriteBulkString("at_us")
			conn.WriteInt(int(e.AtUs))
			conn.WriteBulkString("model")
			conn.WriteBulkString(e.Model)
			conn.WriteBulkString("texts")
			conn.WriteInt(e.Texts)
			conn.WriteBulkString("latency_us")
			conn.WriteInt(int(e.LatencyUs))
			conn.WriteBulkString("err")
			if e.Err {
				conn.WriteInt(1)
			} else {
				conn.WriteInt(0)
			}
			continue
		}
		conn.WriteArray(6)
		conn.WriteInt(int(e.Seq))
		conn.WriteInt(int(e.AtUs))
		conn.WriteBulkString(e.Model)
		conn.WriteInt(e.Texts)
		conn.WriteInt(int(e.LatencyUs))
		if e.Err {
			conn.WriteInt(1)
		} else {
			conn.WriteInt(0)
		}

	}
}

// writeValuesEmbReply writes the RedisAI META+VALUES envelope for an EMB
// VALUES reply: dtype, shape [m, dim] (m = processed texts, so a truncated
// tail is reflected by the shape), and a flat row-major values array. Under
// RESP3 the values are typed doubles; under RESP2 they are decimal bulk
// strings, exactly like RedisAI's reply.
func writeValuesEmbReply(conn redcon.Conn, results [][]byte, dim int) {
	writePairs(conn, 3)
	conn.WriteBulkString("dtype")
	conn.WriteBulkString("FLOAT")
	conn.WriteBulkString("shape")
	conn.WriteArray(2)
	conn.WriteInt(len(results))
	conn.WriteInt(dim)
	conn.WriteBulkString("values")
	writeValuesArray(conn, results, dim)
}

// writeValuesArray writes the flat values array for the given embedding blobs.
// Each float32 dimension is widened to float64 and serialized with the standard
// Redis double representation (typed RESP3 doubles, decimal bulk strings under
// RESP2) — the same widening RedisAI applies via RAI_TensorGetValueAsDouble.
func writeValuesArray(conn redcon.Conn, results [][]byte, dim int) {
	conn.WriteArray(len(results) * dim)
	for _, emb := range results {
		for i := 0; i < dim; i++ {
			v := float64(math.Float32frombits(binary.LittleEndian.Uint32(emb[i*4 : i*4+4])))
			if conn.ProtocolVersion() == 3 {
				conn.WriteDouble(v)
			} else {
				conn.WriteBulkString(strconv.FormatFloat(v, 'g', -1, 64))
			}
		}
	}
}

// writePairs opens a flat key/value reply: a RESP3 map header when the
// connection negotiated protocol 3, and the RESP2 flat array header (2n
// elements) otherwise. The caller then writes the pairs with the usual
// Write* calls — the reply body is identical in both encodings.
func writePairs(conn redcon.Conn, n int) {
	if conn.ProtocolVersion() == 3 {
		conn.WriteMap(n)
	} else {
		conn.WriteArray(n * 2)

	}
}

func (s *Server) handleMODELS(conn redcon.Conn, cmd redcon.Command) {
	models := s.reg.List()
	if len(models) == 0 {
		if conn.ProtocolVersion() == 3 {
			conn.WriteMap(0)
		} else {
			conn.WriteArray(0)
		}
		return
	}
	if conn.ProtocolVersion() == 3 {
		// RESP3: a map keyed by model name whose values carry dim and status.
		conn.WriteMap(len(models))
		for _, m := range models {
			conn.WriteBulkString(m.Name)
			conn.WriteMap(2)
			conn.WriteBulkString("dim")
			conn.WriteInt(m.Dim)
			conn.WriteBulkString("status")
			conn.WriteBulkString("ready")
		}
		return
	}
	conn.WriteArray(len(models))
	for _, m := range models {
		conn.WriteArray(3)
		conn.WriteBulkString(m.Name)
		conn.WriteInt(m.Dim)
		conn.WriteBulkString("ready")
	}
}

func (s *Server) handleINFO(conn redcon.Conn, cmd redcon.Command) {
	if len(cmd.Args) < 2 {
		conn.WriteError("ERR wrong number of arguments for 'EMB.INFO' command")
		return
	}

	modelName := string(cmd.Args[1])
	entry, err := s.reg.GetOrInit(modelName)
	if err != nil {
		conn.WriteError(fmt.Sprintf("ERR %v", err))
		return
	}

	stats := entry.Pool.Stats()

	if s.cache != nil {
		writePairs(conn, 27)
	} else {
		writePairs(conn, 20)
	}
	conn.WriteBulkString("dim")
	conn.WriteInt(entry.Dim)
	conn.WriteBulkString("max_length")
	conn.WriteInt(stats.MaxLen)
	conn.WriteBulkString("workers")
	conn.WriteInt(stats.NumWorkers)
	conn.WriteBulkString("requests")
	conn.WriteInt(int(stats.Requests))
	conn.WriteBulkString("avg_latency_us")
	conn.WriteInt(int(stats.AvgLatency))
	conn.WriteBulkString("tokens")
	conn.WriteInt(int(stats.Tokens))
	conn.WriteBulkString("errors")
	conn.WriteInt(int(stats.Errors))
	conn.WriteBulkString("pooling")
	conn.WriteBulkString(stats.Pooling)
	conn.WriteBulkString("normalize")
	if stats.Normalize {
		conn.WriteBulkString("true")
	} else {
		conn.WriteBulkString("false")
	}
	conn.WriteBulkString("batching_timeout_ms")
	conn.WriteInt(stats.BatchingTimeout)
	conn.WriteBulkString("batching_max_batch")
	conn.WriteInt(stats.BatchingMaxBatch)
	conn.WriteBulkString("batching_max_tokens")
	conn.WriteInt(stats.BatchingMaxTokens)
	conn.WriteBulkString("padding_efficiency")
	conn.WriteBulkString(fmt.Sprintf("%.4f", stats.PaddingEfficiency))
	conn.WriteBulkString("quantization")
	conn.WriteBulkString(entry.Quantization)
	conn.WriteBulkString("model_bytes")
	conn.WriteInt(int(entry.ModelSize))
	// Scripted-evaluation counters and resource footprint, distinct from the
	// embedding pool's requests/errors above.
	scriptReqs, scriptErrs := entry.ScriptStats()
	scriptSessions, scriptTokenizer := entry.ScriptFootprint()
	conn.WriteBulkString("script_requests")
	conn.WriteInt(int(scriptReqs))
	conn.WriteBulkString("script_errors")
	conn.WriteInt(int(scriptErrs))
	conn.WriteBulkString("script_sessions")
	conn.WriteInt(int(scriptSessions))
	conn.WriteBulkString("script_tokenizer")
	conn.WriteInt(boolInt(scriptTokenizer))
	conn.WriteBulkString("image_sessions")
	conn.WriteInt(int(entry.ImageFootprint()))
	if s.cache != nil {
		cs := s.cache.Stats()
		hitRate := 0.0
		total := cs.Hits + cs.Misses
		if total > 0 {
			hitRate = float64(cs.Hits) / float64(total) * 100
		}
		conn.WriteBulkString("cache_hits")
		conn.WriteInt(int(cs.Hits))
		conn.WriteBulkString("cache_misses")
		conn.WriteInt(int(cs.Misses))
		conn.WriteBulkString("cache_hit_rate")
		conn.WriteBulkString(fmt.Sprintf("%.1f%%", hitRate))
		conn.WriteBulkString("cache_evictions")
		conn.WriteInt(int(cs.Evictions))
		conn.WriteBulkString("cache_entries")
		conn.WriteInt(cs.Entries)
		conn.WriteBulkString("cache_max_bytes")
		conn.WriteInt(int(cs.MaxBytes))
		conn.WriteBulkString("cache_memory_bytes")
		conn.WriteInt(int(cs.CurBytes))
	}
}

func (s *Server) handleSTATS(conn redcon.Conn, cmd redcon.Command) {
	models := s.reg.List()
	uptime := int(time.Since(s.started).Seconds())
	totalReqs := int64(0)
	totalToks := int64(0)

	perModel := make([]string, 0, len(models))
	perModelScripts := make([]string, 0, len(models))
	for _, m := range models {
		sreq, serr := m.ScriptStats()
		sessions, tokenizer := m.ScriptFootprint()
		if sreq > 0 {
			perModelScripts = append(perModelScripts, fmt.Sprintf("%s: req=%d err=%d sessions=%d tokenizer=%t",
				m.Name, sreq, serr, sessions, tokenizer))
		}
		if pool := m.LoadedPool(); pool != nil {
			st := pool.Stats()
			totalReqs += st.Requests
			totalToks += st.Tokens
			batchInfo := ""
			if st.BatchingTimeout > 0 {
				batchInfo = fmt.Sprintf(" batch=%d/%d budget=%d eff=%.3f",
					st.BatchingTimeout, st.BatchingMaxBatch, st.BatchingMaxTokens, st.PaddingEfficiency)
			}
			perModel = append(perModel, fmt.Sprintf("%s: req=%d avg=%dus tok=%d err=%d pool=%s norm=%t%s",
				m.Name, st.Requests, int(st.AvgLatency), st.Tokens, st.Errors, st.Pooling, st.Normalize, batchInfo))
		}
	}

	totalErrors := s.reg.TotalErrors()

	totalCacheHits := int64(0)
	totalCacheMisses := int64(0)
	totalCacheEvictions := int64(0)
	cacheStats := CacheStats{}
	if s.cache != nil {
		cacheStats = s.cache.Stats()
		totalCacheHits = cacheStats.Hits
		totalCacheMisses = cacheStats.Misses
		totalCacheEvictions = cacheStats.Evictions
	}
	snapshotStatus := SnapshotStatus{}
	s.persistenceMu.RLock()
	coordinator := s.snapshot
	saveOnShutdown := s.cacheSaveOnShutdown
	s.persistenceMu.RUnlock()
	if coordinator != nil {
		snapshotStatus = coordinator.Status()
	}

	writePairs(conn, 50)
	conn.WriteBulkString("uptime_secs")
	conn.WriteInt(uptime)
	conn.WriteBulkString("total_requests")
	conn.WriteInt(int(totalReqs))
	conn.WriteBulkString("active_requests")
	conn.WriteInt(int(s.activeReqs.Load()))
	conn.WriteBulkString("truncated_texts")
	conn.WriteInt(int(s.truncatedTexts.Load()))
	conn.WriteBulkString("truncated_pairs")
	conn.WriteInt(int(s.truncatedPairs.Load()))
	conn.WriteBulkString("image_requests")
	conn.WriteInt(int(s.imageRequests.Load()))
	conn.WriteBulkString("truncated_images")
	conn.WriteInt(int(s.truncatedImages.Load()))
	conn.WriteBulkString("total_tokens")
	conn.WriteInt(int(totalToks))
	conn.WriteBulkString("total_errors")
	conn.WriteInt(int(totalErrors))
	conn.WriteBulkString("models_loaded")
	conn.WriteInt(len(models))
	conn.WriteBulkString("per_model")
	conn.WriteBulkString(strings.Join(perModel, " | "))
	conn.WriteBulkString("script_requests")
	conn.WriteInt(int(s.scriptRequests.Load()))
	conn.WriteBulkString("script_errors")
	conn.WriteInt(int(s.scriptErrors.Load()))
	conn.WriteBulkString("script_avg_latency_us")
	conn.WriteInt(avgLatencyUs(s.scriptLatencyUs.Load(), s.scriptRequests.Load()))
	conn.WriteBulkString("per_model_scripts")
	conn.WriteBulkString(strings.Join(perModelScripts, " | "))
	conn.WriteBulkString("connections")
	conn.WriteInt(int(s.conns.Load()))
	conn.WriteBulkString("idle_timeout_ms")
	conn.WriteInt(int(s.idleTimeout.Milliseconds()))
	conn.WriteBulkString("max_connections")
	conn.WriteInt(s.maxConns)
	conn.WriteBulkString("max_concurrent_requests")
	conn.WriteInt(s.maxConcurrentReqs)
	// Process resource usage: mem is process RSS in MB (heap fallback on
	// platforms without RSS), and CPU/goroutines are live runtime values.
	res := s.resourceStats()
	conn.WriteBulkString("mem")
	conn.WriteInt(int(res.rssBytes / (1024 * 1024)))
	conn.WriteBulkString("cpu_user_usec")
	conn.WriteInt(int(res.cpuUserUsec))
	conn.WriteBulkString("cpu_sys_usec")
	conn.WriteInt(int(res.cpuSysUsec))
	conn.WriteBulkString("goroutines")
	conn.WriteInt(int(res.goroutines))
	conn.WriteBulkString("cache_hits")
	conn.WriteInt(int(totalCacheHits))
	conn.WriteBulkString("cache_misses")
	conn.WriteInt(int(totalCacheMisses))
	conn.WriteBulkString("cache_evictions")
	conn.WriteInt(int(totalCacheEvictions))
	conn.WriteBulkString("cache_flushes")
	conn.WriteInt(int(cacheStats.Flushes))
	conn.WriteBulkString("cache_flushed_entries")
	conn.WriteInt(int(cacheStats.FlushedEntries))
	conn.WriteBulkString("cache_last_flush_duration_usec")
	conn.WriteInt(int(cacheStats.LastFlushDuration.Microseconds()))
	conn.WriteBulkString("cache_snapshot_enabled")
	conn.WriteInt(boolInt(snapshotStatus.Enabled))
	conn.WriteBulkString("cache_load")
	conn.WriteInt(boolInt(s.cacheLoad))
	conn.WriteBulkString("cache_save_on_shutdown")
	conn.WriteInt(boolInt(saveOnShutdown))
	conn.WriteBulkString("cache_snapshot_in_progress")
	conn.WriteInt(boolInt(snapshotStatus.InProgress))
	conn.WriteBulkString("cache_snapshot_successes")
	conn.WriteInt(int(snapshotStatus.Successes))
	conn.WriteBulkString("cache_snapshot_failures")
	conn.WriteInt(int(snapshotStatus.Failures))
	conn.WriteBulkString("cache_snapshot_skipped")
	conn.WriteInt(int(snapshotStatus.Skipped))
	conn.WriteBulkString("cache_snapshot_last_success_unix")
	conn.WriteInt(int(snapshotStatus.LastSuccessUnix))
	conn.WriteBulkString("cache_snapshot_last_duration_usec")
	conn.WriteInt(int(snapshotStatus.LastDuration.Microseconds()))
	conn.WriteBulkString("cache_snapshot_last_entries")
	conn.WriteInt(int(snapshotStatus.LastEntries))
	conn.WriteBulkString("cache_snapshot_last_bytes")
	conn.WriteInt(int(snapshotStatus.LastBytes))
	conn.WriteBulkString("cache_snapshot_capture_duration_usec")
	conn.WriteInt(int(snapshotStatus.LastCaptureDuration.Microseconds()))
	conn.WriteBulkString("cache_restore_limit_bytes")
	conn.WriteInt(int(snapshotStatus.RestoreLimitBytes))
	conn.WriteBulkString("cache_restore_rss_bytes")
	conn.WriteInt(int(snapshotStatus.RestoreRSSBytes))
	conn.WriteBulkString("cache_restore_headroom_bytes")
	conn.WriteInt(int(snapshotStatus.RestoreHeadroomBytes))
	conn.WriteBulkString("cache_restore_entries")
	conn.WriteInt(int(snapshotStatus.RestoredEntries))
	conn.WriteBulkString("cache_restore_skipped_unknown")
	conn.WriteInt(int(snapshotStatus.SkippedUnknown))
	conn.WriteBulkString("cache_restore_skipped_fingerprint")
	conn.WriteInt(int(snapshotStatus.SkippedFingerprint))
	conn.WriteBulkString("cache_restore_skipped_memory")
	conn.WriteInt(int(snapshotStatus.SkippedMemory))
	conn.WriteBulkString("cache_restore_quarantined")
	conn.WriteInt(int(snapshotStatus.QuarantinedEntries))
	conn.WriteBulkString("cache_restore_error")
	conn.WriteBulkString(snapshotStatus.RestoreError)
}

func boolInt(v bool) int {
	if v {
		return 1
	}
	return 0
}

// avgLatencyUs returns the mean latency in microseconds, or 0 when no request
// has completed (avoids a divide-by-zero on a freshly started server).
func avgLatencyUs(totalUs, count int64) int {
	if count <= 0 {
		return 0
	}
	return int(totalUs / count)
}

func (s *Server) handleEMBMULTI(conn redcon.Conn, cmd redcon.Command) {
	pairs := cmd.Args[1:]
	// Leading reply-format keyword (EMB.MULTI [BLOB|VALUES] <model> <text>...):
	// recognized only at the fixed first position, never among the pairs.
	format, hasFormat := parseFormatArg(cmd.Args[1:], 0)
	if hasFormat {
		pairs = cmd.Args[2:]
	}
	if len(pairs) < 2 || len(pairs)%2 != 0 {
		conn.WriteError("ERR wrong number of arguments for 'EMB.MULTI' command")
		return
	}

	// Truncate oversized commands: process only the first maxPairs pairs and
	// reply with null slots for the overflow. Truncation bounds the inference
	// work of a single command so the payload size cannot pin the task's cores.
	total := len(pairs) / 2
	n := total
	if limit := s.maxPairs.Load(); limit > 0 && int64(n) > limit {
		s.truncatedPairs.Add(int64(n) - limit)
		n = int(limit)
		pairs = pairs[:n*2]
	}
	results := make([][]byte, n)

	// Bound pair fan-out: at most `fanOut` goroutines process pairs concurrently,
	// each pulling the next pair from jobs — so N pairs spawn O(fanOut) goroutines,
	// not O(N), preserving MGET semantics and result ordering (results written by
	// index).
	fanOut := s.multiPairFanOut(n)
	jobs := make(chan int)
	var wg sync.WaitGroup
	for w := 0; w < fanOut; w++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for idx := range jobs {
				s.processMultiPair(pairs, results, idx)
			}
		}()
	}
	for i := range n {
		jobs <- i
	}
	close(jobs)
	wg.Wait()

	writeMultiResult(conn, s, format, pairs, results, n, total)
}

// writeMultiResult writes the EMB.MULTI reply: an ordered array with one slot
// per requested pair — bulk/null slots under BLOB, per-pair VALUE envelopes
// (with a model key, since dims are ragged across models) or nulls under
// VALUES. Truncated overflow pairs stay null in both formats.
func writeMultiResult(conn redcon.Conn, s *Server, format replyFormat, pairs [][]byte, results [][]byte, n, total int) {
	conn.WriteArray(total)
	if format == formatVALUES {
		for i, r := range results {
			if r == nil {
				conn.WriteNull()
				continue
			}
			model := string(pairs[i*2])
			entry, err := s.reg.GetOrInit(model)
			if err != nil {
				conn.WriteNull()
				continue
			}
			writePairs(conn, 4)
			conn.WriteBulkString("model")
			conn.WriteBulkString(model)
			conn.WriteBulkString("dtype")
			conn.WriteBulkString("FLOAT")
			conn.WriteBulkString("shape")
			conn.WriteArray(2)
			conn.WriteInt(1)
			conn.WriteInt(entry.Dim)
			conn.WriteBulkString("values")
			writeValuesArray(conn, [][]byte{r}, entry.Dim)
		}
		for i := n; i < total; i++ {
			conn.WriteNull()
		}
		return
	}
	for _, r := range results {
		if r == nil {
			conn.WriteNull()
		} else {
			conn.WriteBulk(r)
		}
	}
	for i := n; i < total; i++ {
		conn.WriteNull()
	}
}

// multiPairFanOut returns how many goroutines may process pairs of one command
// concurrently: the configured fanOut, or the machine's GOMAXPROCS, capped at the
// number of pairs.
func (s *Server) multiPairFanOut(n int) int {
	cap := s.fanOut
	if cap <= 0 {
		cap = runtime.GOMAXPROCS(0)
	}
	if cap < 1 {
		cap = 1
	}
	if cap > n {
		cap = n
	}
	return cap
}

func (s *Server) processMultiPair(pairs [][]byte, results [][]byte, idx int) {
	model := string(pairs[idx*2])
	text := string(pairs[idx*2+1])

	started := time.Now()
	failed := false
	defer func() {
		s.monitor.Add(MonitorEvent{
			AtUs:      started.UnixMicro(),
			Model:     model,
			Texts:     1,
			LatencyUs: time.Since(started).Microseconds(),
			Err:       failed,
		})
	}()

	if s.cache != nil {
		key := textCacheKey(model, text)
		if emb, ok := s.cache.Get(key); ok {
			results[idx] = emb
			return
		}
	}

	entry, err := s.reg.GetOrInit(model)
	if err != nil {
		failed = true
		return
	}
	s.admitQuarantine(model, entry)
	// Admission may have published a restored entry for this exact text, so
	// re-check before paying for inference.
	if s.cache != nil {
		key := textCacheKey(model, text)
		if emb, ok := s.cache.Get(key); ok {
			results[idx] = emb
			return
		}
	}

	resp, err := entry.Pool.Embed([]string{text})
	if err != nil || resp.Err != nil {
		failed = true
		return
	}

	if s.cache != nil {
		s.cache.Set(textCacheKey(model, text), resp.Embeddings[0])
	}

	results[idx] = resp.Embeddings[0]
}

func (s *Server) handleHELP(conn redcon.Conn, cmd redcon.Command) {
	help := strings.Join([]string{
		"EMB <model> [BLOB|VALUES] <text> [text...] - Generate embeddings (default BLOB: float32 binary; VALUES: dtype/shape/values envelope with decimal values)",
		"EMB.IMG <model> [BLOB|VALUES] <image-bytes> [<image-bytes>...] - Embed one or more raw images (JPEG/PNG/GIF/WebP bytes; no URLs — the client fetches)",
		"EMB.IMGMULTI [BLOB|VALUES] <model> <image-bytes> [<model> <image-bytes>...] - Multi-model image embedding with MGET-style per-pair nulls",
		"EMB.MODELS - List available models and their dimensions",
		"EMB.INFO <model> - Show model details and statistics (includes cache stats)",
		"EMB.MULTI [BLOB|VALUES] <model> <text> [<model> <text>...] - Multi-model embedding with MGET-style partial failures",
		"EMB.STATS - Show server statistics (requests, connections, mem/cpu, goroutines)",
		"MONITOR [seq] [limit] - Recent completed request events (model, latency, errors; no text)",
		"EMB.READY - Check server readiness (OK/loading/draining)",
		"EMB.EVAL <model> <script> <numtexts> <text...> <arg...> - Evaluate a Lua script against a model (KEYS=texts, ARGV=args)",
		"EMB.EVSHA <model> <sha> <numtexts> <text...> <arg...> - Evaluate a cached script by SHA (see EMB.SCRIPT LOAD)",
		"EMB.SCRIPT LOAD <model> <script> - Compile, cache and return the script SHA1 (scripts can also be preloaded from config at boot)",
		"EMB.SCRIPT EXISTS <model> <sha...> - Check which scripts are cached (1/0 per sha)",
		"EMB.SCRIPT FLUSH [<model>] - Clear cached scripts (all models when omitted)",
		"EMB.HELP - Show this help message",
		"EMB.CACHE.FLUSH [model] - Invalidate all cached embeddings or one model",
		"EMB.SAVE - Asynchronously save the embedding cache snapshot",
		"INFO [section ...] - Redis-style server info (version, stats, memory, cpu, cache hit ratios)",
		"CONFIG GET [pattern] - List runtime configuration parameters",
		"CONFIG SET <param> <value> - Change a runtime configuration parameter",
		"AUTH <password> - Authenticate with the server",
		"PING - Redis compatibility",
		"Script replies: string→bulk, list→array, string-keyed table→hash (flat field/value pairs), nil→null, {err=...}→error",
		"Script input specs: {shape, data|fill|bytes, dtype} - fill builds a constant tensor host-side (no Lua data table); bytes packs little-endian elements; data/fill/bytes are mutually exclusive",
		"Script blocks: emb.run / emb.run_batch(named tensors) emb.embed / emb.image.{embed,preprocess,info} emb.tokenize.{encode,encode_pair,words,pretokenized} emb.similarity / emb.distance emb.math.{sigmoid,softmax,argmax,float32_bytes,dot,cosine,l2,norm,mean_pool,cls,topk,gather,slice,scale,add} json.{encode,decode,null} emb.API_VERSION",
	}, "\n")
	conn.WriteBulkString(help)
}
