// Command emb-verify-performance validates that a fast-path model (B) stays
// retrieval-correct relative to a reference model (A): it embeds a fixed
// query/document corpus through both and reports mean/min per-pair cosine and
// nDCG@10 ranking retention of B against A.
//
// Usage: emb-verify-performance [flags] (see -h)
package main

import (
	"flag"
	"fmt"
	"math"
	"os"
	"time"

	"github.com/elcuervo/emb/internal/embverify"
	"github.com/elcuervo/emb/internal/resp"
)

// defaultCorpus is the built-in corpus used when -corpus is not given: a small
// set of documents and queries about embeddings and retrieval.
func defaultCorpus() embverify.Corpus {
	return embverify.Corpus{
		Documents: []string{
			"the quick brown fox jumps over the lazy dog",
			"a vector database stores embeddings for similarity search",
			"opensearch powers full-text and vector search in one platform",
			"mean pooling averages the token embeddings of a sentence",
			"l2 normalization projects an embedding onto the unit sphere",
			"cosine similarity measures the angle between two vectors",
			"quantization reduces model size and speeds up inference",
			"the attention mechanism lets transformers weigh token relevance",
			"retrieval augmented generation grounds answers in documents",
			"embedding models map text to dense vector representations",
			"annoy and hnsw are popular approximate nearest neighbor indexes",
			"reindexing is required when embedding precision changes",
			"the batcher coalesces concurrent requests into shared onnx runs",
			"tokenizers split text into subword tokens with a vocabulary",
			"little endian float32 is the wire format for emb embeddings",
			"cache hits serve repeated text from memory without inference",
			"dynamic batching trades a small delay for higher throughput",
			"pre-pooled models ship the final embedding out of the graph",
			"large language models are trained on internet scale corpora",
			"benchmarking compares latency percentiles under concurrent load",
			"the router selects the nearest matching vector for a query",
			"clustering groups embeddings by their semantic proximity",
			"hybrid search merges lexical and vector results",
			"evaluation measures recall of retrieved documents",
			"serving embeddings at scale requires careful resource planning",
			"the onnx runtime executes the transformer graph on the cpu",
			"int8 quantization keeps retrieval quality within tolerance",
			"a query encoder and a document encoder share a text model",
			"the fan out pattern bounds concurrent inference goroutines",
			"idle flush serves a lone request immediately without waiting",
		},
		Queries: []string{
			"how do embeddings enable similarity search",
			"what is l2 normalization in embedding pipelines",
			"does quantization affect retrieval quality",
			"how does batching improve embedding throughput",
			"why reindex after changing embedding precision",
		},
	}
}

func main() {
	if err := run(); err != nil {
		fmt.Fprintf(os.Stderr, "emb-verify-performance: %v\n", err)
		os.Exit(1)
	}
}

func run() error {
	addr := flag.String("addr", "127.0.0.1:6379", "emb server address (host:port)")
	modelA := flag.String("model-a", "", "reference model (fp32 baseline)")
	modelB := flag.String("model-b", "", "fast-path model to validate")
	minCos := flag.Float64("min-cosine", 0.99, "minimum mean pairwise cosine")
	minNDCG := flag.Float64("min-ndcg", 0.95, "minimum nDCG@10 ranking retention")
	corpusPath := flag.String("corpus", "", "JSON file with {documents, queries} to verify (default: built-in corpus)")
	timeout := flag.Duration("timeout", 10*time.Second, "per-request timeout")
	flag.Parse()

	if *modelA == "" || *modelB == "" {
		return fmt.Errorf("both -model-a (reference) and -model-b (fast path) are required")
	}
	for _, t := range []struct {
		name string
		v    float64
	}{{"min-cosine", *minCos}, {"min-ndcg", *minNDCG}} {
		if math.IsNaN(t.v) || math.IsInf(t.v, 0) {
			return fmt.Errorf("-%s must be a finite number, got %v", t.name, t.v)
		}
	}

	corpus := defaultCorpus()
	if *corpusPath != "" {
		loaded, err := embverify.LoadCorpus(*corpusPath)
		if err != nil {
			return err
		}
		corpus = loaded
	}

	c := resp.NewClient(*addr, "", false)
	c.SetTimeout(*timeout)
	if err := c.Dial(); err != nil {
		return err
	}
	defer func() { _ = c.Close() }()
	e := embverify.NewEmbedder(c)

	all := append(append([]string{}, corpus.Documents...), corpus.Queries...)
	embA, err := e.EmbedAll(*modelA, all)
	if err != nil {
		return fmt.Errorf("embedding via %s: %w", *modelA, err)
	}
	embB, err := e.EmbedAll(*modelB, all)
	if err != nil {
		return fmt.Errorf("embedding via %s: %w", *modelB, err)
	}

	ndocs := len(corpus.Documents)
	docA, docB := embA[:ndocs], embB[:ndocs]
	qA, qB := embA[ndocs:], embB[ndocs:]

	meanCos, minPair := embverify.MeanCosine(docA, docB)
	ndcg := embverify.MeanNDCG10(qA, docA, qB, docB)

	fmt.Printf("model A: %s | model B: %s\n", *modelA, *modelB)
	fmt.Printf("docs=%d queries=%d\n", ndocs, len(corpus.Queries))
	fmt.Printf("pairwise cosine mean=%.6f min=%.6f\n", meanCos, minPair)
	fmt.Printf("nDCG@10 retention (B ranking vs A ranking) = %.4f\n", ndcg)

	// Fail closed: a NaN metric compares false against any threshold, which
	// would let a broken embedding pass the gate silently.
	failed := false
	if !finite(meanCos) || meanCos < *minCos {
		fmt.Fprintf(os.Stderr, "FAIL: mean cosine %.6f < %.4f\n", meanCos, *minCos)
		failed = true
	}
	if !finite(ndcg) || ndcg < *minNDCG {
		fmt.Fprintf(os.Stderr, "FAIL: nDCG@10 retention %.4f < %.4f\n", ndcg, *minNDCG)
		failed = true
	}
	if failed {
		return fmt.Errorf("retrieval quality below tolerance")
	}
	fmt.Println("PASS")
	return nil
}

// finite reports whether f is a real number, so the quality gates fail closed
// on NaN/Inf instead of passing a comparison that is always false.
func finite(f float64) bool { return !math.IsNaN(f) && !math.IsInf(f, 0) }
