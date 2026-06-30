package server

import (
	"context"
	"fmt"
	"log"
	"net"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/tidwall/redcon"

	"github.com/elcuervo/emb/internal/registry"
)

type Server struct {
	reg          *registry.Registry
	srv          *redcon.Server
	ln           net.Listener
	active       sync.WaitGroup
	shuttingDown atomic.Bool
	started      time.Time
	addr         string
}

func New(addr string, reg *registry.Registry) *Server {
	s := &Server{
		reg:     reg,
		started: time.Now(),
		addr:    addr,
	}

	mux := redcon.NewServeMux()
	mux.HandleFunc("ping", s.handlePING)
	mux.HandleFunc("emb", s.handleEMB)
	mux.HandleFunc("emb.models", s.handleMODELS)
	mux.HandleFunc("emb.info", s.handleINFO)
	mux.HandleFunc("emb.stats", s.handleSTATS)
	mux.HandleFunc("emb.help", s.handleHELP)
	mux.HandleFunc("emb.multi", s.handleEMBMULTI)

	s.srv = redcon.NewServer(addr, mux.ServeRESP,
		func(conn redcon.Conn) bool { return true },
		func(conn redcon.Conn, err error) {},
	)

	return s
}

func (s *Server) ListenAndServe() error {
	ln, err := net.Listen("tcp", s.addr)
	if err != nil {
		return fmt.Errorf("listening on %s: %w", s.addr, err)
	}
	s.ln = ln
	log.Printf("emb listening on %s", s.addr)
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

func (s *Server) handlePING(conn redcon.Conn, cmd redcon.Command) {
	conn.WriteString("PONG")
}

func (s *Server) handleEMB(conn redcon.Conn, cmd redcon.Command) {
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

	conn.WriteArray(22)
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
				batchInfo = fmt.Sprintf(" batch=%d/%d", st.BatchingTimeout, st.BatchingMaxBatch)
			}
			perModel = append(perModel, fmt.Sprintf("%s: req=%d avg=%dus tok=%d err=%d mem=%dmb pool=%s norm=%t%s",
				m.Name, st.Requests, int(st.AvgLatency), st.Tokens, st.Errors, st.MemoryMB, st.Pooling, st.Normalize, batchInfo))
		}
	}

	totalErrors := s.reg.TotalErrors()

	conn.WriteArray(14)
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
}

func (s *Server) handleEMBMULTI(conn redcon.Conn, cmd redcon.Command) {
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
	var wg sync.WaitGroup
	for i := range n {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			model := string(pairs[idx*2])
			text := string(pairs[idx*2+1])

			entry, err := s.reg.GetOrInit(model)
			if err != nil {
				return
			}

			resp, err := entry.Pool.Embed([]string{text})
			if err != nil || resp.Err != nil {
				return
			}

			results[idx] = resp.Embeddings[0]
		}(i)
	}
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

func (s *Server) handleHELP(conn redcon.Conn, cmd redcon.Command) {
	help := strings.Join([]string{
		"EMB <model> <text> [text...] - Generate embeddings for one or more texts",
		"EMB.MODELS - List available models and their dimensions",
		"EMB.INFO <model> - Show model details and statistics",
		"EMB.MULTI <model> <text> [<model> <text>...] - Multi-model embedding with MGET-style partial failures",
		"EMB.STATS - Show server statistics",
		"EMB.HELP - Show this help message",
		"PING - Redis compatibility",
	}, "\n")
	conn.WriteBulkString(help)
}
