package main

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"log"
	"os"
	"os/signal"
	"sort"
	"syscall"
	"time"

	"github.com/elcuervo/emb/internal/config"
	"github.com/elcuervo/emb/internal/onnx"
	"github.com/elcuervo/emb/internal/registry"
	"github.com/elcuervo/emb/internal/server"
)

var version = "dev"

func main() {
	if err := run(); err != nil {
		log.Fatalf("%v", err)
	}
}

func run() error {
	fc, err := config.ParseFlags(os.Args[1:])
	if err != nil {
		if err.Error() == "__version__" {
			fmt.Println(version)
			return nil
		}
		return fmt.Errorf("parsing flags: %w", err)
	}

	if err := onnx.InitEnvironment(fc.OrtLib); err != nil {
		return fmt.Errorf("initializing ONNX Runtime: %w", err)
	}
	destroyEnvironment := true
	defer func() {
		if destroyEnvironment {
			_ = onnx.DestroyEnvironment()
		}
	}()

	reg := registry.New()
	closeRegistry := true
	defer func() {
		if closeRegistry {
			_ = reg.Close()
		}
	}()

	var modelCount int
	for name, modelCfg := range fc.Models {
		log.Printf("registering model %q", name)
		entry, err := registry.LoadModel(modelCfg, name)
		if err != nil {
			return fmt.Errorf("loading model %q: %w", name, err)
		}
		reg.Add(name, entry)
		modelCount++
	}
	reg.SetModelCount(modelCount)

	var tlsConfig *tls.Config
	if fc.TLSCert != "" && fc.TLSKey != "" {
		cert, err := tls.LoadX509KeyPair(fc.TLSCert, fc.TLSKey)
		if err != nil {
			return fmt.Errorf("loading TLS cert/key: %w", err)
		}
		tlsConfig = &tls.Config{Certificates: []tls.Certificate{cert}}
	}

	// idle_timeout defaults to a 15m TTL; an explicit 0 disables reaping.
	idleTimeout := config.DefaultIdleTimeout
	if fc.IdleTimeout != nil {
		idleTimeout = time.Duration(*fc.IdleTimeout)
	}
	var cacheSaveInterval time.Duration
	if fc.CacheSave != "" {
		cacheSaveInterval, err = time.ParseDuration(fc.CacheSave)
		if err != nil {
			return fmt.Errorf("parsing cache_save: %w", err)
		}
	}
	cacheSaveRate, err := fc.CacheSaveRateBytes()
	if err != nil {
		return fmt.Errorf("parsing cache_save_rate_limit: %w", err)
	}
	telemetryCfg, err := fc.ResolveTelemetry(os.Getenv)
	if err != nil {
		return fmt.Errorf("resolving telemetry config: %w", err)
	}
	if telemetryCfg.ServiceVersion == "" {
		telemetryCfg.ServiceVersion = version
	}
	srv := server.New(fc.Listen, reg, fc.Password, fc.Cache, tlsConfig,
		server.WithIdleTimeout(idleTimeout),
		server.WithMaxConnections(fc.MaxConnections),
		server.WithMaxConcurrentRequests(fc.MaxConcurrentRequests),
		server.WithMaxTexts(intPtrOrDefault(fc.MaxTexts, 4096)),
		server.WithMaxPairs(intPtrOrDefault(fc.MaxPairs, 4096)),
		server.WithMaxImages(intPtrOrDefault(fc.MaxImages, 4096)),
		server.WithMaxImageBytes(fc.EffectiveMaxImageBytes()),
		server.WithMaxImagePixels(fc.EffectiveMaxImagePixels()),
		server.WithMaxCommandBytes(fc.EffectiveMaxCommandBytes()),
		server.WithPersistence(server.PersistenceConfig{
			File:           fc.CacheFile,
			Load:           fc.CacheLoadEnabled(),
			SaveInterval:   cacheSaveInterval,
			SaveOnShutdown: fc.CacheShutdownSaveEnabled(),
			RestoreLimit:   fc.CacheRestoreLimit,
			RestoreReserve: fc.CacheRestoreReserve,
			SaveRateBytes:  cacheSaveRate,
			SaveRateRaw:    fc.CacheSaveRateLimit,
		}),
		server.WithTelemetry(telemetryCfg),
	)
	srv.SetVersion(version)
	srv.SetTLSConfigPaths(fc.TLSCert, fc.TLSKey)

	var warmups []server.ScriptWarmup
	for name, modelCfg := range fc.Models {
		for _, entry := range modelCfg.Scripts {
			src, err := os.ReadFile(entry.Path)
			if err != nil {
				return fmt.Errorf("reading script %q for model %q: %w", entry.Path, name, err)
			}
			sha, err := srv.PreloadScriptConfig(name, string(src), entry.Config)
			if err != nil {
				return fmt.Errorf("preloading script %q for model %q: %w", entry.Path, name, err)
			}
			log.Printf("preloaded script %s for model %q", entry.Path, name)
			if len(entry.Warm) > 0 {
				var payloads []server.WarmPayload
				for _, group := range entry.Warm {
					for _, text := range group.Texts {
						payloads = append(payloads, server.WarmPayload{Text: text, Args: group.Args})
					}
				}
				warmups = append(warmups, server.ScriptWarmup{Model: name, SHA: sha, Payloads: payloads})
			}
		}
	}
	// Model iteration is a map; fix the warm order so the declared payload order
	// is what runs, not Go's iteration order.
	sort.Slice(warmups, func(i, j int) bool {
		if warmups[i].Model != warmups[j].Model {
			return warmups[i].Model < warmups[j].Model
		}
		return warmups[i].SHA < warmups[j].SHA
	})

	if modelCount > 0 {
		srv.SetReady()
	}
	// The warm runs after readiness, in the background: a declared payload is an
	// optimization, and the platform's health check must not wait on it. It
	// yields to a client request between payloads (see Server.WarmScripts).
	if len(warmups) > 0 {
		srv.WarmScripts(warmups)
	}

	sig := make(chan os.Signal, 1)
	signal.Notify(sig, syscall.SIGINT, syscall.SIGTERM)
	defer signal.Stop(sig)

	serveDone, err := srv.Start()
	if err != nil {
		return fmt.Errorf("server error: %w", err)
	}

	select {
	case serveErr := <-serveDone:
		if serveErr != nil {
			return fmt.Errorf("server error: %w", serveErr)
		}
	case received := <-sig:
		log.Printf("shutting down (signal: %v)...", received)
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		shutdownErr := srv.Shutdown(ctx)
		cancel()
		if errors.Is(shutdownErr, server.ErrShutdownTimeout) {
			// Returning reaches main's fatal exit. Skip native cleanup because an
			// inference call may still be using those resources; the OS reclaims
			// them when the process exits.
			closeRegistry = false
			destroyEnvironment = false
			return shutdownErr
		}
		if shutdownErr != nil {
			return fmt.Errorf("shutting down server: %w", shutdownErr)
		}
		if serveErr := <-serveDone; serveErr != nil {
			return fmt.Errorf("server shutdown: %w", serveErr)
		}
	}

	if err := reg.Close(); err != nil {
		return fmt.Errorf("closing model registry: %w", err)
	}
	closeRegistry = false

	log.Print("server stopped")
	return nil
}

// intPtrOrDefault resolves a possibly-nil *int config value to def, so unset
// config keys take the documented default while an explicit 0 stays meaningful
// (0 = unlimited for max_texts/max_pairs).
func intPtrOrDefault(v *int, def int) int {
	if v == nil {
		return def
	}
	return *v
}
