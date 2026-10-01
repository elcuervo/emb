package tokenizer

import (
	"reflect"
	"strings"
	"testing"
)

func TestTruncatePreservingSpecialTokens(t *testing.T) {
	cases := []struct {
		name      string
		ids       []int64
		specials  []uint32
		maxLength int
		want      []int64
	}{
		{"no truncation needed", []int64{1, 2, 3}, []uint32{0, 0, 0}, 4, []int64{1, 2, 3}},
		{"exact fit", []int64{1, 2, 3}, []uint32{0, 0, 0}, 3, []int64{1, 2, 3}},
		{"bert style keeps cls and sep", []int64{101, 10, 11, 12, 13, 14, 102}, []uint32{1, 0, 0, 0, 0, 0, 1}, 4, []int64{101, 10, 11, 102}},
		{"trailing eos kept (siglip2 style)", []int64{10, 11, 12, 13, 14, 15, 1}, []uint32{0, 0, 0, 0, 0, 0, 1}, 6, []int64{10, 11, 12, 13, 14, 1}},
		{"content budget exhausted before specials", []int64{101, 10, 11, 102}, []uint32{1, 0, 0, 1}, 2, []int64{101, 102}},
		{"maxLength smaller than specials keeps specials", []int64{101, 10, 102}, []uint32{1, 0, 1}, 1, []int64{101, 102}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := truncatePreservingSpecialTokens(c.ids, c.specials, c.maxLength)
			if !reflect.DeepEqual(got, c.want) {
				t.Fatalf("got %v, want %v", got, c.want)
			}
		})
	}
}

func TestSpecialKeepIndexesNoOp(t *testing.T) {
	if keep := specialKeepIndexes(3, []uint32{0, 0, 0}, 4); keep != nil {
		t.Fatalf("expected nil keep (no truncation), got %v", keep)
	}
	if keep := specialKeepIndexes(3, []uint32{1, 0, 0}, 3); keep != nil {
		t.Fatalf("expected nil keep (exact fit), got %v", keep)
	}
}

// refTokenizer is defined in reference_test.go; these tests follow its
// skip-if-absent pattern so CI without downloaded models stays green.

func TestEncodeTruncationPreservesSpecials(t *testing.T) {
	tok := refTokenizer(t) // BERT-style (CLS/SEP)
	defer tok.Close()

	short, _, err := tok.Encode("hello world", 64)
	if err != nil {
		t.Fatal(err)
	}
	if len(short) < 4 {
		t.Fatalf("short encode too small: %v", short)
	}
	clsID, sepID := short[0], short[len(short)-1]

	words := strings.Repeat("one two three four five six seven eight nine ten eleven twelve ", 4)
	ids, _, err := tok.Encode(words, 8)
	if err != nil {
		t.Fatal(err)
	}
	if len(ids) != 8 {
		t.Fatalf("expected 8 ids, got %d", len(ids))
	}
	if ids[0] != clsID {
		t.Fatalf("expected CLS at position 0, got %d (ids %v)", ids[0], ids)
	}
	if ids[7] != sepID {
		t.Fatalf("expected SEP at position 7, got %d (ids %v)", ids[7], ids)
	}
}

func siglip2Tokenizer(t *testing.T) *RefTokenizer {
	t.Helper()
	tok, err := NewTokenizer("../../models/siglip2/tokenizer.json", false)
	if err != nil {
		t.Skipf("siglip2 tokenizer not present: %v", err)
	}
	return tok
}

func TestSiglip2TruncationKeepsEos(t *testing.T) {
	tok := siglip2Tokenizer(t)
	defer tok.Close()

	// 127 sevens tokenize to > 64 tokens; the post-processor appends <eos>
	// (id 1). Truncation must keep <eos> at the final position (63), so the
	// prepooled siglip2 export pools the EOS token like the reference stacks.
	ids, mask, err := tok.Encode(strings.Repeat("7", 127), 64)
	if err != nil {
		t.Fatal(err)
	}
	if len(ids) != 64 {
		t.Fatalf("expected 64 padded ids, got %d", len(ids))
	}
	if ids[63] != 1 {
		t.Fatalf("expected <eos> (id 1) at position 63, got %d", ids[63])
	}
	seven, _, err := tok.Encode("7", 64)
	if err != nil {
		t.Fatal(err)
	}
	if len(seven) < 2 {
		t.Fatalf("single digit encode too small: %v", seven)
	}
	if ids[0] != seven[0] || ids[62] != seven[0] {
		t.Fatalf("expected digit token %d in content positions, got %v", seven[0], ids[:5])
	}
	var maskSum int64
	for _, m := range mask {
		maskSum += m
	}
	if maskSum != 64 {
		t.Fatalf("expected full ones mask, got %d ones", maskSum)
	}
}

func TestEncodeOffsetsTruncationMatchesEncode(t *testing.T) {
	tok := refTokenizer(t)
	defer tok.Close()

	words := strings.Repeat("one two three four five six seven eight nine ten eleven twelve ", 4)
	offIDs, offMask, offsets, err := tok.EncodeOffsets(words, 8)
	if err != nil {
		t.Fatal(err)
	}
	encIDs, encMask, err := tok.Encode(words, 8)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(offIDs, encIDs) {
		t.Fatalf("offsets ids %v != encode ids %v", offIDs, encIDs)
	}
	if !reflect.DeepEqual(offMask, encMask) {
		t.Fatalf("offsets mask %v != encode mask %v", offMask, encMask)
	}
	if len(offsets) != len(offIDs) {
		t.Fatalf("expected one offset span per id, got %d spans for %d ids", len(offsets), len(offIDs))
	}
}
