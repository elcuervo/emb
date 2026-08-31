package server

import (
	"context"
	"crypto/tls"
	"fmt"
	"log"
	"net"
	"runtime"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/tidwall/redcon"

	"github.com/elcuervo/emb/internal/registry"
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
	shuttingDown atomic.Bool
	started      time.Time
	addr         string
	password     atomic.Value // string; live-updatable via CONFIG SET
	tlsConfig    *tls.Config
	tlsCert      string
	tlsKey       string
	cache        *Cache
	// cacheConfig retains the raw boot cache string so CONFIG GET echoes it.
	cacheConfig string
	// cacheFile/cacheSave are runtime-editable snapshot parameters (consumed by
	// the cache-snapshot save loop; stored here even before that change lands).
	cacheFile string
	cacheSave string
	// version is the injected build version ("dev" when unset), reported by INFO.
	version   string
	activeReq atomic.Int64
	state     atomic.Int64
	// fanOut bounds concurrent EMB.MULTI pair processing for a single command, so a
	// request storm cannot spawn unbounded goroutines competing for inference cores.
	// 0 resolves to the machine's GOMAXPROCS. Overridable for tests.
	fanOut int
}

func New(addr string, reg *registry.Registry, password string, cacheConfig string, tlsConfig *tls.Config) *Server {
	cacheBytes, err := parseCacheConfig(cacheConfig)
	if err != nil {
		log.Fatalf("parsing cache config: %v", err)
	}
	var c *Cache
	if cacheBytes > 0 {
		c = NewCache(cacheBytes)
	}

	s := &Server{
		reg:         reg,
		started:     time.Now(),
		addr:        addr,
		tlsConfig:   tlsConfig,
		cache:       c,
		cacheConfig: cacheConfig,
		version:     "dev",
	}
	s.password.Store(password)

	mux := redcon.NewServeMux()
	mux.HandleFunc("ping", s.handlePING)
	mux.HandleFunc("auth", s.handleAUTH)
	mux.HandleFunc("emb", s.handleEMB)
	mux.HandleFunc("emb.models", s.handleMODELS)
	mux.HandleFunc("emb.info", s.handleINFO)
	mux.HandleFunc("emb.stats", s.handleSTATS)
	mux.HandleFunc("emb.help", s.handleHELP)
	mux.HandleFunc("emb.multi", s.handleEMBMULTI)
	mux.HandleFunc("emb.ready", s.handleREADY)
	mux.HandleFunc("info", s.handleInfo)
	mux.HandleFunc("config", s.handleConfig)

	s.srv = redcon.NewServer(addr, func(conn redcon.Conn, cmd redcon.Command) {
		if s.password.Load().(string) != "" && !isExempt(cmd) && !isAuthenticated(conn) {
			conn.WriteError("NOAUTH Authentication required.")
			return
		}
		mux.ServeRESP(conn, cmd)
	},
		func(conn redcon.Conn) bool {
			conn.SetContext(&connState{})
			return true
		},
		func(conn redcon.Conn, err error) {},
	)

	return s
}

func (s *Server) ListenAndServe() error {
	var ln net.Listener
	var err error
	if s.tlsConfig != nil {
		ln, err = tls.Listen("tcp", s.addr, s.tlsConfig)
	} else {
		ln, err = net.Listen("tcp", s.addr)
	}
	if err != nil {
		return fmt.Errorf("listening on %s: %w", s.addr, err)
	}
	s.ln = ln
	if s.tlsConfig != nil {
		log.Printf("emb listening on %s (TLS)", s.addr)
	} else {
		log.Printf("emb listening on %s", s.addr)
	}
	return s.srv.Serve(ln)
}

func (s *Server) Shutdown(ctx context.Context) error {
	s.shuttingDown.Store(true)

	if s.ln != nil {
		s.ln.Close()
	}

	done := make(chan struct{})
	go func() {
		s.active.Wait()
		close(done)
	}()

	select {
	case <-done:
	case <-ctx.Done():
		log.Printf("shutdown timeout after %v", ctx.Err())
	}

	return s.srv.Close()
}

func (s *Server) Close() error {
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

func (s *Server) handleEMB(conn redcon.Conn, cmd redcon.Command) {
	s.activeReq.Add(1)
	defer s.activeReq.Add(-1)
	if s.shuttingDown.Load() {
		conn.WriteError("ERR server shutting down")
		return
	}

	if len(cmd.Args) < 3 {
		conn.WriteError("ERR wrong number of arguments for 'EMB' command")
		return
	}

	s.active.Add(1)
	defer s.active.Done()

	modelName := string(cmd.Args[1])
	texts := make([]string, len(cmd.Args)-2)
	for i, arg := range cmd.Args[2:] {
		texts[i] = string(arg)
	}

	if s.cache != nil {
		results := make([][]byte, len(texts))
		var missIdxs []int
		for i, text := range texts {
			key := modelName + ":" + text
			if emb, ok := s.cache.Get(key); ok {
				results[i] = emb
			} else {
				missIdxs = append(missIdxs, i)
			}
		}
		if len(missIdxs) == 0 {
			if len(results) == 1 {
				conn.WriteBulk(results[0])
			} else {
				conn.WriteArray(len(results))
				for _, emb := range results {
					conn.WriteBulk(emb)
				}
			}
			return
		}

		entry, err := s.reg.GetOrInit(modelName)
		if err != nil {
			conn.WriteError(fmt.Sprintf("ERR %v", err))
			return
		}

		missTexts := make([]string, len(missIdxs))
		for j, idx := range missIdxs {
			missTexts[j] = texts[idx]
		}

		resp, err := entry.Pool.Embed(missTexts)
		if err != nil {
			conn.WriteError(fmt.Sprintf("ERR %v", err))
			return
		}
		if resp.Err != nil {
			conn.WriteError(fmt.Sprintf("ERR %v", resp.Err))
			return
		}

		for j, idx := range missIdxs {
			results[idx] = resp.Embeddings[j]
			s.cache.Set(modelName+":"+texts[idx], resp.Embeddings[j])
		}

		if len(results) == 1 {
			conn.WriteBulk(results[0])
		} else {
			conn.WriteArray(len(results))
			for _, emb := range results {
				conn.WriteBulk(emb)
			}
		}
		return
	}

	entry, err := s.reg.GetOrInit(modelName)
	if err != nil {
		conn.WriteError(fmt.Sprintf("ERR %v", err))
		return
	}

	resp, err := entry.Pool.Embed(texts)
	if err != nil {
		conn.WriteError(fmt.Sprintf("ERR %v", err))
		return
	}

	if resp.Err != nil {
		conn.WriteError(fmt.Sprintf("ERR %v", resp.Err))
		return
	}

	if len(resp.Embeddings) == 1 {
		conn.WriteBulk(resp.Embeddings[0])
	} else {
		conn.WriteArray(len(resp.Embeddings))
		for _, emb := range resp.Embeddings {
			conn.WriteBulk(emb)
		}
	}
}

func (s *Server) handleMODELS(conn redcon.Conn, cmd redcon.Command) {
	models := s.reg.List()
	if len(models) == 0 {
		conn.WriteArray(0)
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
		conn.WriteArray(44)
	} else {
		conn.WriteArray(30)
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
	for _, m := range models {
		if m.Pool != nil {
			st := m.Pool.Stats()
			totalReqs += st.Requests
			totalToks += st.Tokens
			batchInfo := ""
			if st.BatchingTimeout > 0 {
				batchInfo = fmt.Sprintf(" batch=%d/%d budget=%d eff=%.3f",
					st.BatchingTimeout, st.BatchingMaxBatch, st.BatchingMaxTokens, st.PaddingEfficiency)
			}
			perModel = append(perModel, fmt.Sprintf("%s: req=%d avg=%dus tok=%d err=%d mem=%dmb pool=%s norm=%t%s",
				m.Name, st.Requests, int(st.AvgLatency), st.Tokens, st.Errors, st.MemoryMB, st.Pooling, st.Normalize, batchInfo))
		}
	}

	totalErrors := s.reg.TotalErrors()

	totalCacheHits := int64(0)
	totalCacheMisses := int64(0)
	totalCacheEvictions := int64(0)
	if s.cache != nil {
		cs := s.cache.Stats()
		totalCacheHits = cs.Hits
		totalCacheMisses = cs.Misses
		totalCacheEvictions = cs.Evictions
	}

	conn.WriteArray(20)
	conn.WriteBulkString("uptime_secs")
	conn.WriteInt(uptime)
	conn.WriteBulkString("total_requests")
	conn.WriteInt(int(totalReqs))
	conn.WriteBulkString("active_requests")
	conn.WriteBulkString("0") // TODO: track active via atomic counter
	conn.WriteBulkString("total_tokens")
	conn.WriteInt(int(totalToks))
	conn.WriteBulkString("total_errors")
	conn.WriteInt(int(totalErrors))
	conn.WriteBulkString("models_loaded")
	conn.WriteInt(len(models))
	conn.WriteBulkString("per_model")
	conn.WriteBulkString(strings.Join(perModel, " | "))
	conn.WriteBulkString("cache_hits")
	conn.WriteInt(int(totalCacheHits))
	conn.WriteBulkString("cache_misses")
	conn.WriteInt(int(totalCacheMisses))
	conn.WriteBulkString("cache_evictions")
	conn.WriteInt(int(totalCacheEvictions))
}

func (s *Server) handleEMBMULTI(conn redcon.Conn, cmd redcon.Command) {
	s.activeReq.Add(1)
	defer s.activeReq.Add(-1)
	if s.shuttingDown.Load() {
		conn.WriteError("ERR server shutting down")
		return
	}

	pairs := cmd.Args[1:]
	if len(pairs) < 2 || len(pairs)%2 != 0 {
		conn.WriteError("ERR wrong number of arguments for 'EMB.MULTI' command")
		return
	}

	s.active.Add(1)
	defer s.active.Done()

	n := len(pairs) / 2
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

	conn.WriteArray(n)
	for _, r := range results {
		if r == nil {
			conn.WriteNull()
		} else {
			conn.WriteBulk(r)
		}
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

	if s.cache != nil {
		key := model + ":" + text
		if emb, ok := s.cache.Get(key); ok {
			results[idx] = emb
			return
		}
	}

	entry, err := s.reg.GetOrInit(model)
	if err != nil {
		return
	}

	resp, err := entry.Pool.Embed([]string{text})
	if err != nil || resp.Err != nil {
		return
	}

	if s.cache != nil {
		s.cache.Set(model+":"+text, resp.Embeddings[0])
	}

	results[idx] = resp.Embeddings[0]
}

func (s *Server) handleHELP(conn redcon.Conn, cmd redcon.Command) {
	help := strings.Join([]string{
		"EMB <model> <text> [text...] - Generate embeddings for one or more texts (cached)",
		"EMB.MODELS - List available models and their dimensions",
		"EMB.INFO <model> - Show model details and statistics (includes cache stats)",
		"EMB.MULTI <model> <text> [<model> <text>...] - Multi-model embedding with MGET-style partial failures",
		"EMB.STATS - Show server statistics",
		"EMB.READY - Check server readiness (OK/loading/draining)",
		"EMB.HELP - Show this help message",
		"INFO [section ...] - Redis-style server info (version, stats, cache hit ratios)",
		"CONFIG GET [pattern] - List runtime configuration parameters",
		"CONFIG SET <param> <value> - Change a runtime configuration parameter",
		"AUTH <password> - Authenticate with the server",
		"PING - Redis compatibility",
	}, "\n")
	conn.WriteBulkString(help)
}
