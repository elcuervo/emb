package main

import (
	"bufio"
	"encoding/binary"
	"flag"
	"fmt"
	"io"
	"math"
	"net"
	"os"
	"sort"
	"strconv"
	"strings"
)

// emb-verify-performance validates that embeddings from a fast-path model
// (reference model B) stay retrieval-correct relative to a reference model
// (reference model A): it embeds a fixed query/document corpus through both
// models and reports mean/min per-pair cosine and top-k ranking retention
// (nDCG@10 of B's ranking vs A's). Exits 1 when mean cosine < 0.99 or
// nDCG retention < 0.95 (the inference-performance / int8 gates).
//
// Usage: emb-verify-performance [addr] modelA modelB
//
// Both models must be served by the same running emb server (EMB.MULTI may be
// used for one payload, but plain EMB per text is used here for simplicity).

var corpus = []string{
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
}

var queries = []string{
	"how do embeddings enable similarity search",
	"what is l2 normalization in embedding pipelines",
	"does quantization affect retrieval quality",
	"how does batching improve embedding throughput",
	"why reindex after changing embedding precision",
}

func main() {
	flag.Parse()
	addr := "127.0.0.1:6379"
	if flag.NArg() >= 1 {
		addr = flag.Arg(0)
	}
	if flag.NArg() < 3 {
		fmt.Fprintln(os.Stderr, "usage: emb-verify-performance [addr] modelA modelB")
		os.Exit(2)
	}
	modelA, modelB := flag.Arg(1), flag.Arg(2)

	embA, err := embedAll(addr, modelA, append(append([]string{}, corpus...), queries...))
	if err != nil {
		fmt.Fprintf(os.Stderr, "embedding via %s: %v\n", modelA, err)
		os.Exit(1)
	}
	embB, err := embedAll(addr, modelB, append(append([]string{}, corpus...), queries...))
	if err != nil {
		fmt.Fprintf(os.Stderr, "embedding via %s: %v\n", modelB, err)
		os.Exit(1)
	}

	ndocs := len(corpus)
	docA, docB := embA[:ndocs], embB[:ndocs]
	qA, qB := embA[ndocs:], embB[ndocs:]

	var meanCos, minCos float64
	minCos = 2
	for i := range docA {
		c := cosine(qA[0], docA[i]) // corpus-level sanity cosine for reporting
		_ = c
		pc := pairCosine(docA[i], docB[i])
		meanCos += pc
		if pc < minCos {
			minCos = pc
		}
	}
	meanCos /= float64(ndocs)

	ndcg10 := meanNDCG10(qA, docA, qB, docB)

	fmt.Printf("model A: %s | model B: %s\n", modelA, modelB)
	fmt.Printf("docs=%d queries=%d\n", ndocs, len(queries))
	fmt.Printf("pairwise cosine mean=%.6f min=%.6f\n", meanCos, minCos)
	fmt.Printf("nDCG@10 retention (B ranking vs A ranking) = %.4f\n", ndcg10)

	fail := false
	if meanCos < 0.99 {
		fmt.Fprintf(os.Stderr, "FAIL: mean cosine %.6f < 0.99\n", meanCos)
		fail = true
	}
	if ndcg10 < 0.95 {
		fmt.Fprintf(os.Stderr, "FAIL: nDCG@10 retention %.4f < 0.95\n", ndcg10)
		fail = true
	}
	if fail {
		os.Exit(1)
	}
	fmt.Println("PASS")
}

// --- RESP client (minimal, mirrors cmd/emb-verify) ---

func embedAll(addr, model string, texts []string) ([][]float32, error) {
	conn, err := net.Dial("tcp", addr)
	if err != nil {
		return nil, err
	}
	defer conn.Close()
	out := make([][]float32, len(texts))
	for i, t := range texts {
		_, _ = fmt.Fprintf(conn, "*3\r\n$3\r\nEMB\r\n$%d\r\n%s\r\n$%d\r\n%s\r\n", len(model), model, len(t), t)
		buf := bufio.NewReader(conn)
		hdr, err := buf.ReadString('\n')
		if err != nil {
			return nil, fmt.Errorf("text %d: %w", i, err)
		}
		if !strings.HasPrefix(hdr, "$") {
			return nil, fmt.Errorf("text %d: unexpected header %q", i, hdr)
		}
		nBytes, err := strconv.Atoi(strings.TrimSpace(hdr[1:]))
		if err != nil {
			return nil, err
		}
		payload := make([]byte, nBytes)
		if _, err := io.ReadFull(buf, payload); err != nil {
			return nil, err
		}
		if _, err := io.ReadFull(buf, make([]byte, 2)); err != nil {
			return nil, err
		}
		out[i] = decodeFloats(payload)
	}
	return out, nil
}

func decodeFloats(b []byte) []float32 {
	v := make([]float32, len(b)/4)
	for i := range v {
		v[i] = math.Float32frombits(binary.LittleEndian.Uint32(b[i*4:]))
	}
	return v
}

func pairCosine(a, b []float32) float64 {
	var dot, na, nb float64
	for i := range a {
		dot += float64(a[i]) * float64(b[i])
		na += float64(a[i]) * float64(a[i])
		nb += float64(b[i]) * float64(b[i])
	}
	if na == 0 || nb == 0 {
		return 0
	}
	return dot / (math.Sqrt(na) * math.Sqrt(nb))
}

func cosine(q, d []float32) float64 { return pairCosine(q, d) }

// rankDocuments orders doc indices by descending cosine to q.
func rankDocuments(q []float32, docs [][]float32) []int {
	idx := make([]int, len(docs))
	for i := range idx {
		idx[i] = i
	}
	sort.SliceStable(idx, func(i, j int) bool {
		return pairCosine(q, docs[idx[i]]) > pairCosine(q, docs[idx[j]])
	})
	return idx
}

func dcg(ranking []int, relevant map[int]bool, k int) float64 {
	var sum float64
	for i, doc := range ranking {
		if i >= k {
			break
		}
		if relevant[doc] {
			sum += 1.0 / math.Log2(float64(i+2))
		}
	}
	return sum
}

// meanNDCG10Retention: for each query, rank docs by A's embeddings (reference),
// take the top-10 as the relevant set, then score B's ranking against it with
// nDCG@10 (B's ranking in A's relevance order). Averages over queries: 1.0
// means B ranks identically to A on A's top-10.
func meanNDCG10(qA [][]float32, docA [][]float32, qB [][]float32, docB [][]float32) float64 {
	var sum float64
	for qi := range qA {
		refRank := rankDocuments(qA[qi], docA)[:10]
		relevant := make(map[int]bool, 10)
		for _, d := range refRank {
			relevant[d] = true
		}
		ideal := dcg(refRank, relevant, 10)
		if ideal == 0 {
			continue
		}
		bRank := rankDocuments(qB[qi], docB)
		sum += dcg(bRank, relevant, 10) / ideal
	}
	return sum / float64(len(qA))
}
