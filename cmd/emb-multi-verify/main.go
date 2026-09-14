// Command emb-multi-verify checks that EMB.MULTI returns byte-identical
// embeddings to the same texts requested one at a time with EMB, for both
// cross-model pairs and several pairs of one model (the batcher path).
package main

import (
	"bytes"
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
		fmt.Fprintf(os.Stderr, "emb-multi-verify: %v\n", err)
		os.Exit(1)
	}
}

type config struct {
	addr    string
	modelA  string
	modelB  string
	dimA    int
	dimB    int
	timeout time.Duration
}

func run() error {
	cfg := config{}
	flag.StringVar(&cfg.addr, "addr", "127.0.0.1:6379", "emb server address (host:port)")
	flag.StringVar(&cfg.modelA, "model-a", "minilm", "first model for cross-model pairs")
	flag.StringVar(&cfg.modelB, "model-b", "siglip2", "second model for cross-model pairs")
	flag.IntVar(&cfg.dimA, "dim-a", 0, "expected dimension for model-a (0 = unchecked)")
	flag.IntVar(&cfg.dimB, "dim-b", 0, "expected dimension for model-b (0 = unchecked)")
	flag.DurationVar(&cfg.timeout, "timeout", 10*time.Second, "per-request timeout")
	flag.Parse()

	c := resp.NewClient(cfg.addr, "", false)
	c.SetTimeout(cfg.timeout)
	if err := c.Dial(); err != nil {
		return err
	}
	defer func() { _ = c.Close() }()
	e := embverify.NewEmbedder(c)

	dims := map[string]int{cfg.modelA: cfg.dimA, cfg.modelB: cfg.dimB}

	embverify.Printf(os.Stdout, "\nTest 1: cross-model EMB.MULTI vs sequential EMB\n")
	passed, failed, err := verifyGroup(e, []embverify.Pair{
		{Model: cfg.modelA, Text: "hello world"},
		{Model: cfg.modelB, Text: "query: test"},
	}, dims, os.Stdout)
	if err != nil {
		return err
	}

	embverify.Printf(os.Stdout, "\nTest 2: same-model EMB.MULTI (batcher) vs sequential EMB\n")
	p2, f2, err := verifyGroup(e, []embverify.Pair{
		{Model: cfg.modelA, Text: "a"},
		{Model: cfg.modelA, Text: "b"},
		{Model: cfg.modelA, Text: "c"},
	}, dims, os.Stdout)
	if err != nil {
		return err
	}

	passed += p2
	failed += f2
	total := passed + failed
	embverify.Printf(os.Stdout, "\n%d/%d passed, %d failed\n", passed, total, failed)
	if failed > 0 {
		return fmt.Errorf("%d of %d checks failed", failed, total)
	}
	return nil
}

// verifyGroup runs EMB.MULTI for pairs and compares each element, byte for
// byte, against a sequential EMB for the same model/text.
func verifyGroup(e *embverify.Embedder, pairs []embverify.Pair, dims map[string]int, out io.Writer) (passed, failed int, err error) {
	multiRaw, err := e.RawMultiEmbed(pairs)
	if err != nil {
		return 0, 0, fmt.Errorf("EMB.MULTI: %w", err)
	}
	for i, p := range pairs {
		label := fmt.Sprintf("%s/%q", p.Model, p.Text)
		if multiRaw[i] == nil {
			embverify.Printf(out, "  ✗ %s returned null\n", label)
			failed++
			continue
		}
		seq, err := e.RawEmbed(p.Model, p.Text)
		if err != nil {
			embverify.Printf(out, "  ✗ %s sequential EMB: %v\n", label, err)
			failed++
			continue
		}
		want := dims[p.Model]
		if !bytes.Equal(multiRaw[i], seq) || (want > 0 && len(seq) != want*4) {
			embverify.Printf(out, "  ✗ %s differs from sequential EMB\n", label)
			failed++
			continue
		}
		embverify.Printf(out, "  ✓ %s byte-identical to EMB\n", label)
		passed++
	}
	return passed, failed, nil
}
