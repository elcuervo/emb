// Command emb-verify compares the running server's embeddings against an
// independent Python (sentence-transformers) reference artifact.
//
// It embeds the reference's sentence set through the server and requires each
// embedding to reach a minimum cosine similarity against the stored reference.
package main

import (
	"flag"
	"fmt"
	"io"
	"os"
	"time"

	"github.com/elcuervo/emb/internal/embverify"
	"github.com/elcuervo/emb/internal/resp"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintf(os.Stderr, "emb-verify: %v\n", err)
		os.Exit(1)
	}
}

type config struct {
	addr    string
	model   string
	dim     int
	refPath string
	minCos  float64
	timeout time.Duration
}

func run() error {
	cfg := config{}
	flag.StringVar(&cfg.addr, "addr", "127.0.0.1:6379", "emb server address (host:port)")
	flag.StringVar(&cfg.model, "model", "minilm", "model to verify")
	flag.IntVar(&cfg.dim, "dim", 0, "expected embedding dimension (0 = take it from the reference)")
	flag.StringVar(&cfg.refPath, "reference", "reference-embeddings.json", "reference artifact to compare against")
	flag.Float64Var(&cfg.minCos, "min-cosine", 0.999, "minimum cosine similarity per sentence")
	flag.DurationVar(&cfg.timeout, "timeout", 10*time.Second, "per-request timeout")
	flag.Parse()
	return verify(cfg, os.Stdout)
}

// verify loads the reference, embeds its sentence set through the server, and
// reports each sentence's cosine. It returns an error when any sentence fails,
// so the process exits non-zero.
func verify(cfg config, out io.Writer) error {
	ref, err := embverify.Load(cfg.refPath)
	if err != nil {
		return err
	}
	if err := ref.CheckInputs(cfg.model, cfg.dim, nil); err != nil {
		return err
	}
	embverify.Printf(out, "reference %s: model=%s dim=%d sentences=%d", cfg.refPath, ref.Model, ref.Dim, len(ref.Sentences))
	if ref.Generator != "" {
		embverify.Printf(out, " generator=%s v%s", ref.Generator, ref.Version)
	}
	if v, ok := ref.Requires["sentence-transformers"]; ok {
		embverify.Printf(out, " sentence-transformers=%s", v)
	}
	embverify.Printf(out, "\n")

	c := resp.NewClient(cfg.addr, "", false)
	c.SetTimeout(cfg.timeout)
	if err := c.Dial(); err != nil {
		return err
	}
	defer func() { _ = c.Close() }()
	e := embverify.NewEmbedder(c)

	passed, failed := 0, 0
	for i, sentence := range ref.Sentences {
		vec, err := e.Embed(cfg.model, sentence)
		if err != nil {
			embverify.Printf(out, "  ✗ %d: %v\n", i+1, err)
			failed++
			continue
		}
		if len(vec) != ref.Dim {
			embverify.Printf(out, "  ✗ %d: got dim %d, want %d\n", i+1, len(vec), ref.Dim)
			failed++
			continue
		}
		cos := embverify.Cosine(vec, embverify.ToFloat32(ref.Embeddings[i]))
		if cos >= cfg.minCos {
			passed++
			embverify.Printf(out, "  ✓ %d: cosine=%.6f  (%s)\n", i+1, cos, sentence)
		} else {
			failed++
			embverify.Printf(out, "  ✗ %d: cosine=%.6f < %.4f  (%s)\n", i+1, cos, cfg.minCos, sentence)
		}
	}

	total := passed + failed
	embverify.Printf(out, "\n%d/%d passed at cosine ≥ %.4f, %d failed\n", passed, total, cfg.minCos, failed)
	if failed > 0 {
		return fmt.Errorf("%d of %d sentences fell below cosine %.4f", failed, total, cfg.minCos)
	}
	return nil
}
