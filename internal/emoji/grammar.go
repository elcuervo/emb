package emoji

import (
	"errors"
	"fmt"
	"strings"
)

// Transport limits, in bytes. A DNS label cannot exceed 63 bytes and a whole
// name cannot exceed 253, and a query that would not fit is refused rather than
// truncated: a truncated name is a different question.
const (
	MaxLabelBytes = 63
	MaxNameBytes  = 253
)

// Errors a query can fail with. The service maps them to refusals; nothing here
// is recoverable, and none of them is a ranking.
var (
	// ErrEmpty is a name with no content at all.
	ErrEmpty = errors.New("emoji: empty query")
	// ErrNoTerms is a name made only of operators, or one that opens with a
	// subtraction and so has nothing to subtract from.
	ErrNoTerms = errors.New("emoji: query has no terms")
	// ErrTooLong is a name or label the transport cannot carry.
	ErrTooLong = errors.New("emoji: query too long")
	// ErrMixedOperators is a name that both composes arithmetic terms and asks
	// a conjunction: each operator means one thing, and there is no precedence
	// to decide between them.
	ErrMixedOperators = errors.New("emoji: mixed composition operators")
)

// operators separate the terms of a query. Every other character belongs to the
// term being read, which is why a vocabulary slug never contains one: see
// dns/tools/build-emoji-vocab.py. `+` and `-` compose a term arithmetically and
// `*` conjoins one: an operator belongs to the term that follows it.
const operators = "+-*"

// Term is one operand of a query. Text is what the model embeds for it, and
// Subtract and Conjoin are the operator that introduced it: `-` subtracts a
// term and `*` conjoins it. At most one of the two is set, and neither is set
// for the first term, which no operator introduced.
type Term struct {
	Subtract bool
	Conjoin  bool
	Text     string
}

// Query is a parsed name: one or more terms, in the order they were written,
// evaluated left to right with no precedence.
type Query struct {
	Terms []Term
	// Sentence marks a query that wrote one term of more than one word, as
	// opposed to naming a single thing or composing several. It is decided from
	// the term as the query wrote it, before any vocabulary word is spelled out,
	// so naming an entry is one word however long its description runs.
	Sentence bool
	// Conjunction marks a query whose terms are joined by `*`, which asks for
	// the entries closest to all of them at once rather than for a composition
	// of their vectors. A single term is not a conjunction: there is nothing to
	// conjoin it with.
	Conjunction bool
}

// Parse reads a query name, given as the labels the transport carried. Labels
// are the words of one sentence (a `.` between them is a word break), and `+`,
// `-`, or `*` inside a label ends the term being read and starts the next one.
// A name may compose arithmetically or conjoin, never both: an operator means
// one thing and no precedence decides between them.
//
// Parse is pure: it knows nothing about the vocabulary, so a glyph or a slug
// travels as written. Vocab.Spell is what turns those into their descriptions.
func Parse(labels []string) (Query, error) {
	if len(labels) == 0 {
		return Query{}, ErrEmpty
	}
	total := 0
	for _, label := range labels {
		if len(label) > MaxLabelBytes {
			return Query{}, fmt.Errorf("%w: label of %d bytes", ErrTooLong, len(label))
		}
		total += len(label) + 1
	}
	if total > MaxNameBytes {
		return Query{}, fmt.Errorf("%w: name of %d bytes", ErrTooLong, total)
	}

	// Labels join with a space: a dot in a name is the word break, and the
	// label boundary has already carried it.
	text := strings.ToLower(strings.Join(labels, " "))
	if strings.TrimSpace(text) == "" {
		return Query{}, ErrEmpty
	}

	var (
		terms    []Term
		current  strings.Builder
		subtract bool
		conjoin  bool
	)
	flush := func() {
		if word := strings.Join(strings.Fields(current.String()), " "); word != "" {
			terms = append(terms, Term{Subtract: subtract, Conjoin: conjoin, Text: word})
		}
		current.Reset()
	}
	for _, r := range text {
		if strings.ContainsRune(operators, r) {
			flush()
			subtract = r == '-'
			conjoin = r == '*'
			continue
		}
		current.WriteRune(r)
	}
	flush()

	if len(terms) == 0 {
		return Query{}, ErrNoTerms
	}
	if terms[0].Subtract {
		return Query{}, fmt.Errorf("%w: the query opens with a subtraction", ErrNoTerms)
	}

	// A name composes or conjoins, never both: only one of the two readings can
	// answer, and a precedence rule would make it a third thing nobody wrote.
	var conjoined, arithmetic bool
	for i, term := range terms {
		switch {
		case term.Conjoin:
			conjoined = true
		case i > 0:
			arithmetic = true
		}
	}
	if conjoined && arithmetic {
		return Query{}, fmt.Errorf("%w: `*` cannot be mixed with `+` or `-`", ErrMixedOperators)
	}
	return Query{
		Terms:       terms,
		Sentence:    len(terms) == 1 && len(strings.Fields(terms[0].Text)) > 1,
		Conjunction: conjoined && len(terms) > 1,
	}, nil
}

// Spell replaces every word of the query that names a vocabulary entry with
// that entry's description, so a glyph and the entry's slug reach the model as
// the same text. A word that names nothing is left as written: the vocabulary
// bounds the answers, never the question.
func (v Vocab) Spell(q Query) Query {
	spelled := make([]Term, len(q.Terms))
	for i, term := range q.Terms {
		words := strings.Fields(term.Text)
		for j, word := range words {
			if entry, ok := v.Lookup(word); ok {
				words[j] = entry.Description
			}
		}
		spelled[i] = Term{Subtract: term.Subtract, Conjoin: term.Conjoin, Text: strings.Join(words, " ")}
	}
	return Query{Terms: spelled, Sentence: q.Sentence, Conjunction: q.Conjunction}
}

// Query parses labels and spells them against the vocabulary, which is what a
// caller that intends to embed the result wants in one step.
func (v Vocab) Query(labels []string) (Query, error) {
	parsed, err := Parse(labels)
	if err != nil {
		return Query{}, err
	}
	return v.Spell(parsed), nil
}
