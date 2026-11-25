package main

import (
	"context"
	"embed"
	"errors"
	"flag"
	"fmt"
	"io"
	"io/fs"
	"net"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/elcuervo/emb/internal/embtop"
)

//go:embed assets/*
var assets embed.FS

var version = "dev"

type nodeFlags []string

func (n *nodeFlags) String() string     { return strings.Join(*n, ",") }
func (n *nodeFlags) Set(s string) error { *n = append(*n, s); return nil }

type options struct {
	listen   string
	nodes    nodeFlags
	nodesCSV string
	useTLS   bool
	tlsName  string
	version  bool
	specs    []embtop.NodeSpec
}

func parseFlags(args []string) (options, error) {
	var o options
	f := flag.NewFlagSet("emb-sidekick", flag.ContinueOnError)
	f.StringVar(&o.listen, "listen", "127.0.0.1:8080", "HTTP listen address (explicitly choose a private interface for remote viewers)")
	f.Var(&o.nodes, "node", "EMB host, host:port or DNS name; repeatable")
	f.StringVar(&o.nodesCSV, "nodes", "", "comma-separated EMB nodes")
	f.BoolVar(&o.useTLS, "tls", false, "verify upstream EMB TLS certificate")
	f.StringVar(&o.tlsName, "tls-server-name", "", "upstream TLS name override for IP literals")
	f.BoolVar(&o.version, "version", false, "print version and exit")
	if err := f.Parse(args); err != nil {
		return o, err
	}
	if len(f.Args()) > 0 {
		return o, fmt.Errorf("unexpected argument %q", f.Args()[0])
	}
	if o.version {
		return o, nil
	}
	if len(o.nodes) == 0 && strings.TrimSpace(o.nodesCSV) == "" {
		return o, errors.New("missing target: supply -node or -nodes")
	}
	for _, raw := range o.nodes {
		spec, err := embtop.ParseNodeSpec(raw)
		if err != nil {
			return o, err
		}
		o.specs = append(o.specs, spec)
	}
	if o.nodesCSV != "" {
		for _, raw := range strings.Split(o.nodesCSV, ",") {
			if strings.TrimSpace(raw) == "" {
				return o, errors.New("empty -nodes entry")
			}
			spec, err := embtop.ParseNodeSpec(raw)
			if err != nil {
				return o, err
			}
			o.specs = append(o.specs, spec)
		}
	}
	if len(o.specs) > maxNodes {
		return o, fmt.Errorf("at most %d node targets", maxNodes)
	}
	return o, nil
}
func run(args []string, getenv func(string) string, stdout, stderr io.Writer) int {
	o, err := parseFlags(args)
	if err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return 0
		}
		_, _ = fmt.Fprintln(stderr, "emb-sidekick:", err)
		return 2
	}
	if o.version {
		_, _ = fmt.Fprintln(stdout, version)
		return 0
	}
	password := getenv("EMB_SIDEKICK_PASSWORD")
	if password != "" && !o.useTLS && !embtop.AllLoopback(o.specs) {
		_, _ = fmt.Fprintln(stderr, "emb-sidekick: warning: upstream AUTH over non-loopback without TLS")
	}
	m := newMonitor(o.specs, password, o.useTLS, o.tlsName)
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	server := &http.Server{Addr: o.listen, Handler: m.handler(), ReadHeaderTimeout: 5 * time.Second, ReadTimeout: 10 * time.Second, MaxHeaderBytes: 8192, IdleTimeout: 30 * time.Second}
	listener, err := net.Listen("tcp", o.listen)
	if err != nil {
		_, _ = fmt.Fprintln(stderr, "emb-sidekick: listen:", err)
		return 1
	}
	defer listener.Close()
	go m.run(ctx)
	go func() {
		<-ctx.Done()
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = server.Shutdown(shutdownCtx)
	}()
	_, _ = fmt.Fprintln(stdout, "emb-sidekick listening on", listener.Addr())
	if err := server.Serve(listener); err != nil && !errors.Is(err, http.ErrServerClosed) {
		_, _ = fmt.Fprintln(stderr, "emb-sidekick: serve:", err)
		return 1
	}
	return 0
}
func main() { os.Exit(run(os.Args[1:], os.Getenv, os.Stdout, os.Stderr)) }

func (m *monitor) handler() http.Handler {
	assetFS, err := fs.Sub(assets, "assets")
	if err != nil {
		panic(err)
	}
	page, err := assets.ReadFile("assets/index.html")
	if err != nil {
		panic(err)
	}
	files := http.FileServer(http.FS(assetFS))
	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/" {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = w.Write(page)
	})
	mux.Handle("/assets/", http.StripPrefix("/assets/", files))
	mux.HandleFunc("/api/snapshot", func(w http.ResponseWriter, r *http.Request) {
		m.mu.Lock()
		data := m.latest
		m.mu.Unlock()
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		w.Header().Set("Cache-Control", "no-store")
		_, _ = w.Write(data)
	})
	mux.HandleFunc("/api/stream", m.stream)
	mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		_, _ = io.WriteString(w, "ok\n")
	})
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("Referrer-Policy", "no-referrer")
		w.Header().Set("X-Frame-Options", "DENY")
		w.Header().Set("Content-Security-Policy", "default-src 'none'; script-src 'self'; style-src 'self'; font-src 'self'; img-src 'self' data:; connect-src 'self'; base-uri 'none'; form-action 'none'; frame-ancestors 'none'")
		if r.Method != http.MethodGet {
			w.Header().Set("Allow", http.MethodGet)
			http.Error(w, "GET only", http.StatusMethodNotAllowed)
			return
		}
		if r.URL.RawQuery != "" {
			http.Error(w, "query parameters unsupported", http.StatusBadRequest)
			return
		}
		if r.ContentLength > 0 || len(r.TransferEncoding) > 0 {
			http.Error(w, "request body unsupported", http.StatusRequestEntityTooLarge)
			return
		}
		mux.ServeHTTP(w, r)
	})
}
