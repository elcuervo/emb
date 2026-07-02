package main

import (
	"context"
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
	fc, err := config.ParseFlags(os.Args[1:])
	if err != nil {
		if err.Error() == "__version__" {
			fmt.Println(version)
			os.Exit(0)
		}
		log.Fatalf("parsing flags: %v", err)
	}

	if err := onnx.InitEnvironment(fc.OrtLib); err != nil {
		log.Fatalf("initializing ONNX Runtime: %v", err)
	}
	defer onnx.DestroyEnvironment()

	reg := registry.New()

	for name, modelCfg := range fc.Models {
		log.Printf("registering model %q", name)
		entry, err := registry.LoadModel(modelCfg, name)
		if err != nil {
			log.Fatalf("loading model %q: %v", name, err)
		}
		reg.Add(name, entry)
	}

	srv := server.New(fc.Listen, reg, fc.Password)

	sig := make(chan os.Signal, 1)
	signal.Notify(sig, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		s := <-sig
		log.Printf("shutting down (signal: %v)...", s)
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		srv.Shutdown(ctx)
		reg.Close()
	}()

	if err := srv.ListenAndServe(); err != nil {
		log.Fatalf("server error: %v", err)
	}

	log.Print("server stopped")
}
