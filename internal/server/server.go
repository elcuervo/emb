package server

import (
	"fmt"
	"log"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/tidwall/redcon"

	"github.com/elcuervo/emb/internal/registry"
)

type Server struct {
	reg     *registry.Registry
	srv     *redcon.Server
	started time.Time
	total   atomic.Int64
	mu      sync.Mutex
	addr    string
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

	s.srv = redcon.NewServer(addr, mux.ServeRESP,
		func(conn redcon.Conn) bool { return true },
		func(conn redcon.Conn, err error) {},
	)

	return s
}

func (s *Server) ListenAndServe() error {
	log.Printf("emb listening on %s", s.addr)
	return s.srv.ListenAndServe()
}

func (s *Server) Close() error {
	return s.srv.Close()
}

func (s *Server) handlePING(conn redcon.Conn, cmd redcon.Command) {
	conn.WriteString("PONG")
}

func (s *Server) handleEMB(conn redcon.Conn, cmd redcon.Command) {
	if len(cmd.Args) < 3 {
		conn.WriteError("ERR wrong number of arguments for 'EMB' command")
		return
	}

	modelName := string(cmd.Args[1])
	texts := make([]string, len(cmd.Args)-2)
	for i, arg := range cmd.Args[2:] {
		texts[i] = string(arg)
	}

	entry, ok := s.reg.Get(modelName)
	if !ok {
		conn.WriteError(fmt.Sprintf("ERR model '%s' not found", modelName))
		return
	}

	s.total.Add(1)

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
	entry, ok := s.reg.Get(modelName)
	if !ok {
		conn.WriteError(fmt.Sprintf("ERR model '%s' not found", modelName))
		return
	}

	pool := entry.Pool

	conn.WriteArray(6)
	conn.WriteBulkString("dim")
	conn.WriteInt(entry.Dim)
	conn.WriteBulkString("workers")
	conn.WriteInt(pool.NumWorkers())
	conn.WriteBulkString("requests")
	conn.WriteInt(0)
}

func (s *Server) handleSTATS(conn redcon.Conn, cmd redcon.Command) {
	models := s.reg.List()
	uptime := int(time.Since(s.started).Seconds())

	conn.WriteArray(6)
	conn.WriteBulkString("uptime_secs")
	conn.WriteInt(uptime)
	conn.WriteBulkString("total_requests")
	conn.WriteInt(int(s.total.Load()))
	conn.WriteBulkString("models_loaded")
	conn.WriteInt(len(models))
}

func (s *Server) handleHELP(conn redcon.Conn, cmd redcon.Command) {
	help := strings.Join([]string{
		"EMB <model> <text> [text...] - Generate embeddings for one or more texts",
		"EMB.MODELS - List available models and their dimensions",
		"EMB.INFO <model> - Show model details and statistics",
		"EMB.STATS - Show server statistics",
		"EMB.HELP - Show this help message",
		"PING - Redis compatibility",
	}, "\n")
	conn.WriteBulkString(help)
}
