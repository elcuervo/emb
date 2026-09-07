package tokenizer

import (
	"testing"
)

func refTokenizer(t *testing.T) *RefTokenizer {
	t.Helper()
	tok, err := NewTokenizer("../../models/minilm/tokenizer.json", false)
	if err != nil {
		t.Skipf("test tokenizer not present: %v (run: just download-model)", err)
	}
	return tok
}

// assertWordIDs checks every subword maps to a word index in range, word
// indices never go backwards, and the given (start, end) sub-ranges map to the
// expected word indices.
func assertWordIDs(t *testing.T, wordIDs []int64, checks map[int]int64) {
	t.Helper()
	for i, w := range wordIDs {
		if w < 0 {
			t.Fatalf("subword %d has no word (got %d)", i, w)
		}
	}
	for i := 1; i < len(wordIDs); i++ {
		if wordIDs[i] < wordIDs[i-1] {
			t.Fatalf("wordIDs not monotonic at %d: %d -> %d", i, wordIDs[i-1], wordIDs[i])
		}
	}
	for idx, want := range checks {
		if idx >= len(wordIDs) || wordIDs[idx] != want {
			t.Fatalf("subword %d: want word %d, got %d (all: %v)", idx, want, wordIDs[idx], wordIDs)
		}
	}
}

func TestEncodePretokenizedBasicWords(t *testing.T) {
	tok := refTokenizer(t)
	defer tok.Close()

	ids, wordIDs, err := tok.EncodePretokenized([]string{"hello", "world"}, 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(ids) != 2 || len(wordIDs) != 2 {
		t.Fatalf("expected 2 subwords for hello/world, got ids=%d wordIDs=%d", len(ids), len(wordIDs))
	}
	assertWordIDs(t, wordIDs, map[int]int64{0: 0, 1: 1})
}

func TestEncodePretokenizedMultiSubword(t *testing.T) {
	tok := refTokenizer(t)
	defer tok.Close()

	// "tokenization" splits into ["token", "##ization"]; both subwords belong
	// to word 0, and a following word must map to 1.
	ids, wordIDs, err := tok.EncodePretokenized([]string{"tokenization", "world"}, 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(ids) < 3 {
		t.Fatalf("expected >=3 subwords, got %d", len(ids))
	}
	// Find the transition: all before index 1+len(first word's subwords) are word 0.
	assertWordIDs(t, wordIDs, map[int]int64{0: 0, len(ids) - 1: 1})
}

func TestEncodePretokenizedMultibyte(t *testing.T) {
	tok := refTokenizer(t)
	defer tok.Close()

	// café and naïve contain multi-byte runes: if the tokenizer's offsets were
	// byte-based (rather than rune-based), attribution would drift and this
	// test fails at the last word.
	ids, wordIDs, err := tok.EncodePretokenized([]string{"café", "naïve", "wörld"}, 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(ids) == 0 {
		t.Fatal("no subwords produced")
	}
	assertWordIDs(t, wordIDs, map[int]int64{len(ids) - 1: 2})
}

func TestEncodePretokenizedTruncation(t *testing.T) {
	tok := refTokenizer(t)
	defer tok.Close()

	full, _, err := tok.EncodePretokenized([]string{"apple", "banana", "cherry", "date"}, 0)
	if err != nil {
		t.Fatal(err)
	}
	const maxLen = 4
	ids, wordIDs, err := tok.EncodePretokenized([]string{"apple", "banana", "cherry", "date"}, maxLen)
	if err != nil {
		t.Fatal(err)
	}
	if len(ids) != maxLen {
		t.Fatalf("expected %d subwords after truncation, got %d", maxLen, len(ids))
	}
	if len(wordIDs) != maxLen {
		t.Fatalf("wordIDs must truncate with ids: len=%d", len(wordIDs))
	}
	for i := 0; i < maxLen; i++ {
		if ids[i] != full[i] {
			t.Fatalf("truncated ids mismatch at %d", i)
		}
	}
	assertWordIDs(t, wordIDs, nil)
}

func TestEncodePretokenizedNoSpecialTokens(t *testing.T) {
	tok := refTokenizer(t)
	defer tok.Close()

	// Words that naturally split into subwords must keep per-word alignment
	// end-to-end with no hidden special tokens injected (add_special_tokens
	// is disabled, so no [CLS]/[SEP] anywhere in the ids).
	words := []string{"apple", "banana", "cherry", "date", "elderberry"}
	ids, wordIDs, err := tok.EncodePretokenized(words, 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(ids) == 0 {
		t.Fatal("no subwords produced")
	}
	assertWordIDs(t, wordIDs, map[int]int64{0: 0, len(ids) - 1: int64(len(words) - 1)})
	// All five words must be represented.
	seen := map[int64]bool{}
	for _, w := range wordIDs {
		seen[w] = true
	}
	for i := range words {
		if !seen[int64(i)] {
			t.Fatalf("word %d never appears in wordIDs %v", i, wordIDs)
		}
	}
}
