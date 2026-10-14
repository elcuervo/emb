// Package embstress drives synthetic embedding load across a DNS-resolved
// fleet, parameterized by an emb server configuration.
//
// It is the load generator counterpart to embtop's observer: it speaks the same
// RESP wire client, opens no model and links no ONNX Runtime (build with
// CGO_ENABLED=0), and reports the client's own throughput and latency rather
// than any server counter.
package embstress

import (
	"bufio"
	"fmt"
	"math/rand/v2"
	"os"
	"strings"
)

// maxWords bounds a generated text's word count whatever the configuration
// says, so a bogus max_length cannot ask a node for a megabyte of text.
const maxWords = 4096

// vocabulary is the fixed word list the generator draws from. It is small on
// purpose: the run measures a server, not a vocabulary, and a fixed list keeps
// the request set reproducible from the seed alone.
var vocabulary = []string{
	"embedding", "server", "latency", "throughput", "token", "batch", "queue",
	"model", "vector", "dimension", "inference", "session", "worker", "cache",
	"memory", "budget", "admission", "throttle", "steal", "headroom", "cluster",
	"node", "fleet", "capacity", "operator", "request", "response", "socket",
	"protocol", "pipeline", "snapshot", "restore", "fingerprint", "quantized",
	"pooling", "truncated", "goroutine", "parallelism", "allowance", "reserve",
	"readme", "benchmark", "regression", "baseline", "calibration", "latency",
}

// Generator produces the request texts a run offers. Its sequence depends only
// on its seed, its worker index and the configured maximum length, so two runs
// with the same seed offer the same request set and neither needs a corpus.
type Generator struct {
	rng     *rand.Rand
	classes []int    // word counts, in the order they are offered
	next    int      // the next class to offer
	corpus  []string // when set, replaces generation entirely
	corpusI int
}

// NewGenerator returns a Generator for one worker of a run. maxLength is the
// configured model's maximum length; the length classes are fractions of it.
func NewGenerator(seed int64, worker, maxLength int) *Generator {
	// Fold the worker into the seed so two workers of one run offer different
	// texts while the whole run stays reproducible from -seed. PCG takes two
	// words, so the seed and the worker each occupy one.
	// #nosec G404 -- this drives the generated request texts, not anything
	// security-sensitive; a fixed seed is the whole point.
	return &Generator{
		rng:     rand.New(rand.NewPCG(uint64(seed), uint64(worker))),
		classes: lengthClasses(maxLength),
	}
}

// NewCorpusGenerator returns a Generator that offers the corpus's texts in
// order, cycling, instead of generating them.
func NewCorpusGenerator(corpus []string) *Generator {
	return &Generator{corpus: corpus}
}

// lengthClasses derives the word counts a run offers from a model's maximum
// length: an eighth, a quarter, a half and the whole of it, so a level mixes
// short and long requests. A client cannot count tokens (the tokenizer is
// in-process only), so the classes are word counts — a lower bound on the
// server's token count — and the server remains the authority on truncation.
func lengthClasses(maxLength int) []int {
	if maxLength <= 0 {
		maxLength = 512 // the server's own fallback when it cannot detect one
	}
	if maxLength > maxWords {
		maxLength = maxWords
	}
	seen := map[int]bool{}
	var out []int
	for _, f := range []float64{0.125, 0.25, 0.5, 1} {
		n := int(float64(maxLength) * f)
		if n < 1 {
			n = 1
		}
		if seen[n] {
			continue
		}
		seen[n] = true
		out = append(out, n)
	}
	return out
}

// Next returns the next text in the sequence. It never returns an empty string.
func (g *Generator) Next() string {
	if len(g.corpus) > 0 {
		t := g.corpus[g.corpusI%len(g.corpus)]
		g.corpusI++
		return t
	}
	words := g.classes[g.next%len(g.classes)]
	g.next++
	var b strings.Builder
	for i := range words {
		if i > 0 {
			b.WriteByte(' ')
		}
		b.WriteString(vocabulary[g.rng.IntN(len(vocabulary))])
	}
	return b.String()
}

// LoadCorpus reads a corpus file: one text per line, blank lines dropped. A
// corpus must be non-empty, or a run would offer nothing.
func LoadCorpus(path string) ([]string, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("corpus %s: %w", path, err)
	}
	defer func() { _ = f.Close() }()

	var out []string
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 0, 64*1024), 1<<20)
	for sc.Scan() {
		if line := strings.TrimSpace(sc.Text()); line != "" {
			out = append(out, line)
		}
	}
	if err := sc.Err(); err != nil {
		return nil, fmt.Errorf("corpus %s: %w", path, err)
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("corpus %s: no texts in it", path)
	}
	return out, nil
}
