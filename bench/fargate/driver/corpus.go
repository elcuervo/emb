package driver

import (
	"crypto/sha256"
	"encoding/hex"
	"math/rand/v2"
	"strconv"
	"strings"
	"sync/atomic"
)

var corpusWords = strings.Fields(`the quick brown fox jumps over a lazy dog while embedders summarize photographs of mountains rivers cities and people in long descriptive captions that vary in length`)

// classWords is the word count of the short, typical and long text classes.
var classWords = [3]int{8, 40, 160}

// Cache phases.
const (
	CacheWarm = "warm"
	CacheCold = "cold"
)

// Corpus supplies the texts of a pass. The warm set is fixed by the seed and
// cycles short/typical/long by request index; cold texts are unique for the
// life of the process (and, through the nonce, across runs against a server
// that kept its cache), so a cold request can never hit the reply cache.
type Corpus struct {
	seed  int64
	nonce string
	cold  atomic.Int64
	warm  []string
}

// NewCorpus builds a corpus; warmSize is rounded up to a multiple of three so
// every class appears in the warm set in request order.
func NewCorpus(seed int64, warmSize int, nonce string) *Corpus {
	warmSize = max(3, (warmSize+2)/3*3)
	c := &Corpus{seed: seed, nonce: nonce}
	for i := range warmSize {
		c.warm = append(c.warm, classText(seed+int64(i), i%3))
	}
	return c
}

func classText(seed int64, class int) string {
	// #nosec G404 -- seeded, reproducible benchmark text; not security-sensitive.
	rng := rand.New(rand.NewPCG(uint64(seed), 0))
	var sb strings.Builder
	for i := range classWords[class] {
		if i > 0 {
			sb.WriteByte(' ')
		}
		sb.WriteString(corpusWords[rng.IntN(len(corpusWords))])
	}
	return sb.String()
}

// Warm is the set a warm pass answers from the cache; Prewarm sends it first.
func (c *Corpus) Warm() []string { return c.warm }

// Text returns the text of pair i. The class cycles with the request index so
// both legs of a pair, and every pair, see the same length mix.
func (c *Corpus) Text(cache string, i int) string {
	if cache == CacheWarm {
		return c.warm[i%len(c.warm)]
	}
	n := c.cold.Add(1)
	return classText(c.seed+1_000_000+n, i%3) + " " + c.nonce + "-" + strconv.FormatInt(n, 10)
}

// Digest is the corpus hash recorded in a bundle: the generator inputs and the
// warm set. Cold texts derive from the same generator and are covered by it.
func (c *Corpus) Digest() string {
	h := sha256.New()
	h.Write([]byte(strings.Join(corpusWords, " ")))
	for _, n := range classWords {
		h.Write([]byte(strconv.Itoa(n) + ","))
	}
	h.Write([]byte(strconv.FormatInt(c.seed, 10)))
	for _, t := range c.warm {
		h.Write([]byte("\n" + t))
	}
	return hex.EncodeToString(h.Sum(nil))
}
