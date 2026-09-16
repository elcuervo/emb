// Command repl is the sandbox bridge — see bridge.go for the contract.
package main

import (
	"context"
	//nolint:gosec // cache identity only, per Redis EVALSHA semantics
	"crypto/sha1"
	"encoding/hex"
	"errors"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/elcuervo/emb/internal/config"
)

func main() {
	listen := flag.String("listen", "127.0.0.1:8080", "HTTP listen address for the bridge")
	upstream := flag.String("upstream", "127.0.0.1:6379", "emb server address (loopback only)")
	configPath := flag.String("config", "", "emb server config, read for the preloaded preset digests")
	origins := flag.String("origins", "", "comma-separated CORS origin allowlist")
	trustProxy := flag.Bool("trust-proxy", false, "key the per-client rate limit on X-Forwarded-For (only when a trusted proxy sets it)")
	timeout := flag.Duration("timeout", 0, "per-command upstream deadline (0 = default)")
	embTop := flag.String("emb-top", "emb-top", "path to the emb-top binary that feeds the live dashboard")
	statsEvery := flag.Duration("stats-interval", statsInterval, "poll interval for the live dashboard")
	statsWindowPolls := flag.Int("stats-window", statsWindow, "history window in polls for the live dashboard")
	flag.Parse()

	p, err := loadPresets(*configPath)
	if err != nil {
		log.Fatalf("repl: presets: %v", err)
	}

	limits := DefaultLimits()
	if *timeout > 0 {
		limits.Timeout = *timeout
	}

	b := NewBridge(*upstream, p, limits, strings.Split(*origins, ","), *trustProxy)

	// The live dashboard is a child process, so a stop signal must reach it.
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	b.stats.start(ctx, *embTop, *upstream, *statsEvery, *statsWindowPolls)

	srv := &http.Server{
		Addr:              *listen,
		Handler:           b.Handler(),
		ReadHeaderTimeout: 10 * time.Second,
		// Bound the whole request read so a client cannot hold a connection
		// open by dribbling a body. WriteTimeout stays unset: /api/stats is an
		// unbounded SSE stream and a write deadline would cut it off.
		ReadTimeout: 30 * time.Second,
	}
	go func() {
		<-ctx.Done()
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = srv.Shutdown(shutdownCtx)
	}()
	log.Printf("repl bridge listening on %s, upstream %s, %d preset(s)", *listen, *upstream, countPresets(p))
	err = srv.ListenAndServe()
	stop()
	if err != nil && !errors.Is(err, http.ErrServerClosed) {
		log.Fatal(err)
	}
}

// loadPresets derives the digest manifest from the same files the server
// preloaded: it reads the server's own config and hashes each script's bytes,
// so the digest a client sends is the digest of the loaded source rather than
// a transcribed one.
func loadPresets(configPath string) (presets, error) {
	p := presets{}
	if configPath == "" {
		return p, nil
	}
	cfg, err := config.Load(configPath)
	if err != nil {
		return nil, err
	}
	for model, m := range cfg.Models {
		for _, path := range m.Scripts {
			src, err := os.ReadFile(path)
			if err != nil {
				return nil, fmt.Errorf("model %q: %w", model, err)
			}
			//nolint:gosec // cache identity only, per Redis EVALSHA semantics
			sum := sha1.Sum(src)
			if p[model] == nil {
				p[model] = map[string]string{}
			}
			name := strings.TrimSuffix(filepath.Base(path), filepath.Ext(path))
			p[model][hex.EncodeToString(sum[:])] = name
		}
	}
	return p, nil
}

func countPresets(p presets) int {
	n := 0
	for _, digests := range p {
		n += len(digests)
	}
	return n
}
