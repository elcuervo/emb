package tokenizer

import "testing"

func TestEncodeOffsets(t *testing.T) {
	tok := refTokenizer(t)
	defer tok.Close()

	const text = "hello world"
	ids, mask, offsets, err := tok.EncodeOffsets(text, 0)
	if err != nil {
		t.Fatal(err)
	}
	// Plain encode with special tokens: [CLS] hello world [SEP] (4 tokens).
	if len(ids) != 4 || len(mask) != 4 {
		t.Fatalf("expected 4 tokens, got ids=%d mask=%d", len(ids), len(mask))
	}
	for i, m := range mask {
		if m != 1 {
			t.Fatalf("mask[%d] = %d", i, m)
		}
	}
	// Real tokens slice the original text.
	for i, off := range offsets {
		if off[0] == 0 && off[1] == 0 {
			continue // special tokens carry no span
		}
		slice := text[off[0]:off[1]]
		if slice == "" {
			t.Fatalf("token %d has empty slice at %v", i, off)
		}
	}
	// The "world" token spans chars 6..11 (index 2: CLS, hello, world, SEP).
	if !(offsets[2][0] == 6 && offsets[2][1] == 11) {
		t.Fatalf("expected world at [6,11), got %v", offsets[2])
	}
}

func TestEncodeOffsetsMatchesEncode(t *testing.T) {
	tok := refTokenizer(t)
	defer tok.Close()

	const text = "the quick brown fox"
	ids, _, _, err := tok.EncodeOffsets(text, 512)
	if err != nil {
		t.Fatal(err)
	}
	plainIDs, _, err := tok.Encode(text, 512)
	if err != nil {
		t.Fatal(err)
	}
	if len(ids) != len(plainIDs) {
		t.Fatalf("length mismatch: encodeOffsets=%d encode=%d", len(ids), len(plainIDs))
	}
	for i := range ids {
		if ids[i] != plainIDs[i] {
			t.Fatalf("id %d: encodeOffsets=%d encode=%d", i, ids[i], plainIDs[i])
		}
	}
}

func TestEncodeOffsetsMultibyte(t *testing.T) {
	tok := refTokenizer(t)
	defer tok.Close()

	// Byte offsets (not rune offsets): the é in café is 2 bytes, and the
	// token span must slice the original string back to "café".
	ids, _, offsets, err := tok.EncodeOffsets("café", 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(ids) == 0 {
		t.Fatal("no tokens")
	}
	joined := ""
	for _, off := range offsets {
		if off[0] == 0 && off[1] == 0 {
			continue // special tokens carry no span
		}
		joined += "café"[off[0]:off[1]]
	}
	if joined != "café" {
		t.Fatalf("byte offsets do not reconstruct the source: %q", joined)
	}
}

func TestEncodePairOffsets(t *testing.T) {
	tok := refTokenizer(t)
	defer tok.Close()

	first, second := "who founded Apple", "Apple was founded in 1976."
	ids, _, offsets, sep, err := tok.EncodePairOffsets(first, second, 512)
	if err != nil {
		t.Fatal(err)
	}

	// Template: [CLS] + encode(first) + [SEP] + second(no specials) + [SEP].
	encFirst, _, _ := tok.Encode(first, 512)
	encSecond, _, _ := tok.Encode(second, 512)
	// sep points at the inter-part separator: 1-based position len(encFirst).
	if sep != len(encFirst) {
		t.Fatalf("sep = %d, want %d", sep, len(encFirst))
	}
	if ids[0] != encFirst[0] {
		t.Fatalf("first id %d != CLS %d", ids[0], encFirst[0])
	}
	if ids[sep-1] != encFirst[len(encFirst)-1] {
		t.Fatalf("inter-part SEP = %d, want %d", ids[sep-1], encFirst[len(encFirst)-1])
	}
	secondPlain := encSecond[1 : len(encSecond)-1] // strip [CLS]/[SEP]
	for k, wantID := range secondPlain {
		if ids[sep+k] != wantID {
			t.Fatalf("second id %d = %d, want %d", sep+k, ids[sep+k], wantID)
		}
	}
	if ids[len(ids)-1] != encFirst[len(encFirst)-1] {
		t.Fatalf("trailing SEP = %d, want %d", ids[len(ids)-1], encFirst[len(encFirst)-1])
	}
	// Second-part tokens slice the second string.
	secondToks := offsets[len(encFirst)+1 : len(ids)-1]
	joined := ""
	for _, off := range secondToks {
		joined += second[off[0]:off[1]]
	}
	if len(joined) == 0 {
		t.Fatal("no second-part spans")
	}
	// "1976" appears in the joined second-part surface.
	found := false
	for _, off := range secondToks {
		if second[off[0]:off[1]] == "1976" {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("'1976' not found in second-part tokens: %q", joined)
	}
}

func TestEncodePairOffsetsTruncation(t *testing.T) {
	tok := refTokenizer(t)
	defer tok.Close()

	ids, _, offsets, sep, err := tok.EncodePairOffsets("question", "context", 8)
	if err != nil {
		t.Fatal(err)
	}
	if len(ids) > 8 {
		t.Fatalf("pair exceeds budget: %d", len(ids))
	}
	_ = offsets
	if sep < 2 || sep > len(ids) {
		t.Fatalf("sep %d out of range for %d ids", sep, len(ids))
	}
}
