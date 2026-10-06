// Package emoji turns a query name into a ranked list of emoji.
//
// The vocabulary is a committed asset (dns/emoji-vocab.json): one entry per
// emoji, carrying its glyph, a slug, its CLDR name, and the description the
// model embeds. A query name is a small expression over those words — its
// labels read as a sentence, and `+` and `-` compose terms — and the answer is
// the vocabulary ranked by cosine similarity to the query's vector, or, for a
// conjunction, by the product of its similarity to each of the query's terms.
//
// Nothing here touches a model. The package owns the vocabulary, the grammar,
// and the ranking, so it is testable with CGO_ENABLED=0, no ONNX Runtime, and
// no server: the vectors arrive from whoever embedded the terms.
package emoji

import (
	"encoding/json"
	"fmt"
	"math"
	"os"
	"sort"
	"strings"
)

// Entry is one emoji in the vocabulary. Description is what the model embeds;
// Name and Glyph are what a surface shows.
type Entry struct {
	Glyph       string `json:"glyph"`
	Slug        string `json:"slug"`
	Name        string `json:"name"`
	Description string `json:"description"`
}

// Vocab is a loaded vocabulary. The zero value is empty and usable.
type Vocab struct {
	// Source and License are the asset's provenance, carried so a surface can
	// attribute the vocabulary without reading the file itself.
	Source  string
	License string

	Entries []Entry

	bySlug  map[string]int
	byGlyph map[string]int
}

type document struct {
	Source       string  `json:"source"`
	License      string  `json:"license"`
	SourceSHA256 string  `json:"source_sha256"`
	GeneratedBy  string  `json:"generated_by"`
	Entries      []Entry `json:"entries"`
}

// Load reads a vocabulary asset from path.
func Load(path string) (Vocab, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return Vocab{}, fmt.Errorf("emoji: read vocabulary: %w", err)
	}
	return LoadBytes(raw)
}

// LoadBytes parses a vocabulary asset and validates its invariants: every entry
// needs a glyph, a slug, and a description, and no two entries may share a slug
// or a glyph. A vocabulary that breaks one of these would answer with an
// ambiguous result, so it is refused at load rather than at query time.
func LoadBytes(raw []byte) (Vocab, error) {
	var doc document
	if err := json.Unmarshal(raw, &doc); err != nil {
		return Vocab{}, fmt.Errorf("emoji: parse vocabulary: %w", err)
	}
	v := Vocab{
		Source:  doc.Source,
		License: doc.License,
		Entries: doc.Entries,
		bySlug:  make(map[string]int, len(doc.Entries)),
		byGlyph: make(map[string]int, len(doc.Entries)),
	}
	for i, e := range v.Entries {
		switch {
		case e.Glyph == "":
			return Vocab{}, fmt.Errorf("emoji: entry %d (%s) has no glyph", i, e.Slug)
		case e.Slug == "":
			return Vocab{}, fmt.Errorf("emoji: entry %d (%s) has no slug", i, e.Glyph)
		case strings.TrimSpace(e.Description) == "":
			return Vocab{}, fmt.Errorf("emoji: entry %d (%s) has no description", i, e.Slug)
		}
		if prev, ok := v.bySlug[e.Slug]; ok {
			return Vocab{}, fmt.Errorf("emoji: %q is the slug of both %s and %s", e.Slug, v.Entries[prev].Glyph, e.Glyph)
		}
		if prev, ok := v.byGlyph[e.Glyph]; ok {
			return Vocab{}, fmt.Errorf("emoji: %s is the glyph of both %q and %q", e.Glyph, v.Entries[prev].Slug, e.Slug)
		}
		v.bySlug[e.Slug] = i
		v.byGlyph[e.Glyph] = i
	}
	return v, nil
}

// Len is the number of entries.
func (v Vocab) Len() int { return len(v.Entries) }

// Lookup finds the entry a word names, by slug or by glyph. It is how a query
// term spelled as `shark` and one spelled as `🦈` reach the same vector.
func (v Vocab) Lookup(word string) (Entry, bool) {
	if i, ok := v.bySlug[word]; ok {
		return v.Entries[i], true
	}
	if i, ok := v.byGlyph[word]; ok {
		return v.Entries[i], true
	}
	return Entry{}, false
}

// Index is the vocabulary with the vectors the model produced for its
// descriptions. Rows are normalized once, at construction, so a ranking costs
// one dot product per entry.
type Index struct {
	vocab  Vocab
	dim    int
	matrix []float32
}

// NewIndex takes ownership of matrix, which is row-major with one row per
// vocabulary entry, in vocabulary order. Every row is scaled to unit length, so
// a row the model returned unnormalized still ranks by direction.
func NewIndex(v Vocab, dim int, matrix []float32) (*Index, error) {
	if dim <= 0 {
		return nil, fmt.Errorf("emoji: index dimension must be positive, got %d", dim)
	}
	if want := dim * v.Len(); len(matrix) != want {
		return nil, fmt.Errorf("emoji: index holds %d values, want %d (%d entries × %d dimensions)", len(matrix), want, v.Len(), dim)
	}
	for row := range v.Len() {
		normalize(matrix[row*dim : (row+1)*dim])
	}
	return &Index{vocab: v, dim: dim, matrix: matrix}, nil
}

// Dim is the vector dimension the index was built with.
func (ix *Index) Dim() int { return ix.dim }

// Result is one ranked emoji and the similarity that placed it.
type Result struct {
	Entry Entry
	Score float64
	// Legs is the entry's similarity to each term of a conjunction, in the
	// query's term order. It is nil for a query ranked against a single vector:
	// a composition has no per-term working to report.
	Legs []float64
}

// Rank returns the k entries closest to the query vector, ordered by cosine
// similarity descending. Equally scoring entries keep vocabulary order, so a
// reply is reproducible across restarts and a cached answer cannot disagree
// with a fresh query. A non-positive k returns no results.
func (ix *Index) Rank(query []float32, k int) ([]Result, error) {
	if ix == nil || len(ix.matrix) == 0 || k <= 0 {
		return nil, nil
	}
	if len(query) != ix.dim {
		return nil, fmt.Errorf("emoji: query has %d dimensions, index has %d", len(query), ix.dim)
	}
	q := make([]float32, len(query))
	copy(q, query)
	normalize(q)

	results := make([]Result, ix.vocab.Len())
	for i := range results {
		row := ix.matrix[i*ix.dim : (i+1)*ix.dim]
		var dot float64
		for d, value := range row {
			dot += float64(value) * float64(q[d])
		}
		results[i] = Result{Entry: ix.vocab.Entries[i], Score: dot}
	}
	sort.SliceStable(results, func(a, b int) bool { return results[a].Score > results[b].Score })
	if k > len(results) {
		k = len(results)
	}
	return results[:k], nil
}

// RankJoint ranks the vocabulary against a conjunction: every entry is scored
// by the product of its similarity to each of the terms, so an entry close to
// all of them outranks an entry that matches only the nearest. The entries the
// query named are excluded, because a conjunction answers what its terms have
// in common rather than one of the terms itself. Equally scoring entries keep
// vocabulary order, as Rank does, and each result carries its similarity to
// every term in the order the terms were given.
func (ix *Index) RankJoint(terms [][]float32, exclude map[int]struct{}, k int) ([]Result, error) {
	if ix == nil || len(ix.matrix) == 0 || k <= 0 {
		return nil, nil
	}
	if len(terms) == 0 {
		return nil, ErrNoTerms
	}
	unit := make([][]float32, len(terms))
	for i, term := range terms {
		if len(term) != ix.dim {
			return nil, fmt.Errorf("emoji: term %d has %d dimensions, index has %d", i, len(term), ix.dim)
		}
		unit[i] = make([]float32, len(term))
		copy(unit[i], term)
		normalize(unit[i])
	}

	results := make([]Result, 0, ix.vocab.Len())
	for i := range ix.vocab.Entries {
		if _, skip := exclude[i]; skip {
			continue
		}
		row := ix.matrix[i*ix.dim : (i+1)*ix.dim]
		legs := make([]float64, len(unit))
		score := 1.0
		for t, term := range unit {
			var dot float64
			for d, value := range row {
				dot += float64(value) * float64(term[d])
			}
			legs[t] = dot
			score *= dot
		}
		results = append(results, Result{Entry: ix.vocab.Entries[i], Score: score, Legs: legs})
	}
	sort.SliceStable(results, func(a, b int) bool { return results[a].Score > results[b].Score })
	if k > len(results) {
		k = len(results)
	}
	return results[:k], nil
}

// Indices is the set of entries a query's terms name by slug or glyph, which is
// what a conjunction excludes from its answer. A term that names nothing is not
// an entry and excludes nothing.
func (v Vocab) Indices(q Query) map[int]struct{} {
	var named map[int]struct{}
	for _, term := range q.Terms {
		for _, word := range strings.Fields(term.Text) {
			if i, ok := v.bySlug[word]; ok {
				named = add(named, i)
				continue
			}
			if i, ok := v.byGlyph[word]; ok {
				named = add(named, i)
			}
		}
	}
	return named
}

func add(set map[int]struct{}, i int) map[int]struct{} {
	if set == nil {
		set = make(map[int]struct{}, 1)
	}
	set[i] = struct{}{}
	return set
}

// normalize scales v to unit length in place; a zero vector is left alone,
// which scores every entry equally rather than producing NaNs.
func normalize(v []float32) {
	var sum float64
	for _, value := range v {
		sum += float64(value) * float64(value)
	}
	if sum == 0 {
		return
	}
	scale := float32(1 / math.Sqrt(sum))
	for i := range v {
		v[i] *= scale
	}
}
