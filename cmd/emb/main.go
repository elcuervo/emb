package main

import (
	"crypto/tls"
	"fmt"
	"log"
	"os"
	"os/signal"
	"sort"
	"sync/atomic"
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

	// Shutdown requests are collected from the first moment: a signal while
	// models load must unwind startup cleanly instead of killing the process
	// with Go's default action.
	requests := make(chan server.ShutdownRequest, 4)
	var pending atomic.Bool
	signals := make(chan os.Signal, 2)
	signal.Notify(signals, syscall.SIGINT, syscall.SIGTERM)
	defer signal.Stop(signals)
	go watchSignals(signals, &pending, requests, os.Exit)

	if err := onnx.InitEnvironment(fc.OrtLib); err != nil {
		return fmt.Errorf("initializing ONNX Runtime: %w", err)
	}
	destroyEnvironment := true
	defer func() {
		if destroyEnvironment {
			_ = onnx.DestroyEnvironment()
		}
	}()

	reg, err := registry.NewWithConfig(&fc.Config)
	if err != nil {
		return fmt.Errorf("resolving CPU budget: %w", err)
	}
	closeRegistry := true
	defer func() {
		if closeRegistry {
			_ = reg.Close()
		}
	}()

	var modelCount int
	for name, modelCfg := range fc.Models {
		if startupAborted(requests) {
			return nil
		}
		log.Printf("registering model %q", name)
		entry, err := registry.LoadModelDeferred(modelCfg, name)
		if err != nil {
			return fmt.Errorf("loading model %q: %w", name, err)
		}
		reg.Add(name, entry)
		// The embedding layout is decided at startup for a model configured for
		// preload — never inside a request. A lazy model is laid out in the
		// background and answers not-ready until it is.
		if modelCfg.Preload {
			if err := reg.Initialize(name); err != nil {
				return fmt.Errorf("initializing model %q: %w", name, err)
			}
		}
		if err := entry.Preload(); err != nil {
			return fmt.Errorf("preloading model %q: %w", name, err)
		}
		modelCount++
	}
	reg.SetModelCount(modelCount)
	if fc.CPUBudget == "off" {
		registry.WarnThreadBudget(reg.List(), registry.EffectiveNumCPU())
	}

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
	if startupAborted(requests) {
		return nil
	}
	srv := server.New(fc.Listen, reg, fc.Password, fc.Cache, tlsConfig,
		server.WithIdleTimeout(idleTimeout),
		server.WithMaxConnections(fc.MaxConnections),
		server.WithMaxConcurrentRequests(fc.MaxConcurrentRequests),
		server.WithAdmissionTimeout(fc.AdmissionTimeoutOrDefault()),
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
		server.WithShutdownRequests(requests, &pending),
		server.WithShutdownConfig(int(fc.ShutdownTimeout), fc.ShutdownOnSigterm, fc.ShutdownOnSigint),
	)
	srv.SetVersion(version)
	srv.SetTLSConfigPaths(fc.TLSCert, fc.TLSKey)
	if startupAborted(requests) {
		// A signal during the snapshot restore: unwind before listening.
		_ = srv.Close()
		return nil
	}

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

	if ok, blocker := reg.LayoutsDecided(); ok {
		srv.SetReady()
	} else {
		// A model configured for preload has no decided layout: report it and
		// stay not-ready rather than advertising a server that cannot serve it.
		log.Printf("not ready: %s", blocker) // #nosec G706 -- blocker is a server-owned model name/reason, not request input
	}
	// The warm runs after readiness, in the background: a declared payload is an
	// optimization, and the platform's health check must not wait on it. It
	// yields to a client request between payloads (see Server.WarmScripts).
	if len(warmups) > 0 {
		srv.WarmScripts(warmups)
	}

	serveDone, err := srv.Start()
	if err != nil {
		return fmt.Errorf("server error: %w", err)
	}

	outcome, err := awaitShutdown(requests, serveDone, &pending, srv.SignalShutdownFlags, srv.Shutdown, srv.ShutdownInProgress)
	if err != nil {
		if outcome == outcomeTimeout {
			// Returning reaches main's fatal exit. Skip native cleanup because an
			// inference call may still be using those resources; the OS reclaims
			// them when the process exits.
			closeRegistry = false
			destroyEnvironment = false
		}
		return err
	}

	if err := reg.Close(); err != nil {
		return fmt.Errorf("closing model registry: %w", err)
	}
	closeRegistry = false

	if outcome == outcomeShutdown {
		log.Print("emb is now ready to exit, bye bye...")
	}
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
