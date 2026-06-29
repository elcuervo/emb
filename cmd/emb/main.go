package main

import (
	"flag"
	"log"
	"os"
	"os/signal"

	"github.com/elcuervo/emb/internal/config"
	"github.com/elcuervo/emb/internal/onnx"
	"github.com/elcuervo/emb/internal/registry"
	"github.com/elcuervo/emb/internal/server"
)

func main() {
	configPath := flag.String("config", "config.yaml", "path to config file")
	ortLib := flag.String("ort-lib", "", "path to ONNX Runtime shared library")
	flag.Parse()

	if err := onnx.InitEnvironment(*ortLib); err != nil {
		log.Fatalf("initializing ONNX Runtime: %v", err)
	}
	defer onnx.DestroyEnvironment()

	cfg, err := config.Load(*configPath)
	if err != nil {
		log.Fatalf("loading config: %v", err)
	}

	reg := registry.New()

	for name, modelCfg := range cfg.Models {
		log.Printf("registering model %q", name)
		entry, err := registry.LoadModel(modelCfg, name)
		if err != nil {
			log.Fatalf("loading model %q: %v", name, err)
		}
		reg.Add(name, entry)
	}

	srv := server.New(cfg.Listen, reg)

	sig := make(chan os.Signal, 1)
	signal.Notify(sig, os.Interrupt)

	go func() {
		<-sig
		log.Print("shutting down...")
		srv.Close()
		reg.Close()
		os.Exit(0)
	}()

	if err := srv.ListenAndServe(); err != nil {
		log.Fatalf("server error: %v", err)
	}
}
