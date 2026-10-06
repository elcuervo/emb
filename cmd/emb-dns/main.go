// Command emb-dns serves an emoji query zone.
//
// Every name under the zone is a sentence, a glyph, or a small composition of
// them; the answer is the emoji whose description the model places nearest to
// the query, one TXT record per result. The model itself lives in an emb server
// on loopback: this process holds no ONNX Runtime, no tokenizer, and no CGo.
package main

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/miekg/dns"

	"github.com/elcuervo/emb/internal/emoji"
)

// version is injected at build time (`-ldflags "-X main.version=..."`).
var version = "dev"

func main() {
	log.SetFlags(log.Ltime)
	if err := run(); err != nil {
		log.Fatalf("emb-dns: %v", err)
	}
}

func run() error {
	cfg, showVersion, err := Load(os.Args[1:])
	if err != nil {
		return err
	}
	if showVersion {
		fmt.Println("emb-dns", version)
		return nil
	}
	vocab, err := emoji.Load(cfg.Vocabulary)
	if err != nil {
		return err
	}
	// #nosec G706 -- the values are the operator's own configuration, read once at boot.
	log.Printf("emb-dns %s: zone %s, %d vocabulary entries from %s", version, cfg.FQDN(), vocab.Len(), cfg.Vocabulary)

	upstream, err := Dial(cfg)
	if err != nil {
		return err
	}
	defer func() { _ = upstream.Close() }()

	service := NewService(cfg, vocab, upstream)

	// Bind before the index is built, so a port that is already taken fails
	// here rather than an hour after a deploy. Nothing is answered until the
	// index exists: a query that arrives meanwhile is a server failure.
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	servers, err := bindDNS(cfg, NewDNSHandler(service))
	if err != nil {
		return err
	}
	httpListener, err := bindHTTP(cfg)
	if err != nil {
		closeServers(context.Background(), servers)
		return err
	}

	// A resolver gives up in seconds, so a zone that answers from a half-built
	// index is worse than one that refuses until it is ready.
	buildCtx, cancelBuild := context.WithTimeout(ctx, 5*time.Minute)
	defer cancelBuild()
	if err := service.BuildIndex(buildCtx); err != nil {
		closeServers(context.Background(), servers)
		_ = httpListener.Close()
		return err
	}

	failures := make(chan error, len(servers)+1)
	for _, server := range servers {
		go func() {
			log.Printf("emb-dns: DNS on %s/%s", server.Net, listenerAddr(server))
			if err := server.ActivateAndServe(); err != nil && ctx.Err() == nil {
				failures <- fmt.Errorf("dns %s: %w", server.Net, err)
			}
		}()
	}
	httpServer := &http.Server{
		Handler:           NewHTTPHandler(service),
		ReadHeaderTimeout: 5 * time.Second,
	}
	go func() {
		log.Printf("emb-dns: HTTP on %s", httpListener.Addr())
		if err := httpServer.Serve(httpListener); err != nil && !errors.Is(err, http.ErrServerClosed) {
			failures <- fmt.Errorf("http: %w", err)
		}
	}()

	select {
	case err := <-failures:
		closeServers(context.Background(), servers)
		_ = httpServer.Close()
		return err
	case <-ctx.Done():
	}

	log.Printf("emb-dns: shutting down")
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := httpServer.Shutdown(shutdownCtx); err != nil {
		log.Printf("emb-dns: http shutdown: %v", err)
	}
	closeServers(shutdownCtx, servers)
	return nil
}

// bindDNS opens every configured socket up front. On Fly the UDP socket binds
// the platform's own address rather than the wildcard, because a reply sent
// from any other local address leaves the machine with the wrong source.
func bindDNS(cfg Config, handler dns.Handler) ([]*dns.Server, error) {
	var servers []*dns.Server
	for _, listener := range []struct{ network, addr string }{
		{"udp", cfg.ListenUDP},
		{"tcp", cfg.ListenTCP},
	} {
		if listener.addr == "" {
			continue
		}
		switch listener.network {
		case "udp":
			conn, err := net.ListenPacket("udp", listener.addr)
			if err != nil {
				closeServers(context.Background(), servers)
				return nil, fmt.Errorf("dns udp on %s: %w", listener.addr, err)
			}
			servers = append(servers, &dns.Server{PacketConn: conn, Handler: handler})
		default:
			ln, err := net.Listen("tcp", listener.addr)
			if err != nil {
				closeServers(context.Background(), servers)
				return nil, fmt.Errorf("dns tcp on %s: %w", listener.addr, err)
			}
			servers = append(servers, &dns.Server{Listener: ln, Handler: handler})
		}
	}
	return servers, nil
}

func bindHTTP(cfg Config) (net.Listener, error) {
	ln, err := net.Listen("tcp", cfg.ListenHTTP)
	if err != nil {
		return nil, fmt.Errorf("http on %s: %w", cfg.ListenHTTP, err)
	}
	return ln, nil
}

func closeServers(ctx context.Context, servers []*dns.Server) {
	for _, server := range servers {
		if err := server.ShutdownContext(ctx); err != nil {
			log.Printf("emb-dns: dns shutdown: %v", err)
		}
	}
}

func listenerAddr(server *dns.Server) string {
	if server.PacketConn != nil {
		return server.PacketConn.LocalAddr().String()
	}
	if server.Listener != nil {
		return server.Listener.Addr().String()
	}
	return server.Addr
}
