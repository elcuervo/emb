package main

import (
	"context"
	"crypto/tls"
	"fmt"
	"log"
	"os"
	"os/signal"
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
	defer onnx.DestroyEnvironment()

	reg := registry.New()

	for name, modelCfg := range fc.Models {
		log.Printf("registering model %q", name)
		entry, err := registry.LoadModel(modelCfg, name)
		if err != nil {
			onnx.DestroyEnvironment()
			return fmt.Errorf("loading model %q: %w", name, err)
		}
		reg.Add(name, entry)
	}

	var tlsConfig *tls.Config
	if fc.TLSCert != "" && fc.TLSKey != "" {
		cert, err := tls.LoadX509KeyPair(fc.TLSCert, fc.TLSKey)
		if err != nil {
			return fmt.Errorf("loading TLS cert/key: %w", err)
		}
		tlsConfig = &tls.Config{Certificates: []tls.Certificate{cert}}
	}

	srv := server.New(fc.Listen, reg, fc.Password, fc.Cache, tlsConfig)

	sig := make(chan os.Signal, 1)
	signal.Notify(sig, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		s := <-sig
		log.Printf("shutting down (signal: %v)...", s)
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		_ = srv.Shutdown(ctx)
		_ = reg.Close()
	}()

	if err := srv.ListenAndServe(); err != nil {
		return fmt.Errorf("server error: %w", err)
	}

	log.Print("server stopped")
	return nil
}
