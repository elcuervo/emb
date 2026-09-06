package tokenizer

import (
	"reflect"
	"testing"
)

func TestSplitWordsBasic(t *testing.T) {
	words, starts, ends := SplitWords("Apple CEO Tim Cook")
	want := []string{"Apple", "CEO", "Tim", "Cook"}
	if !reflect.DeepEqual(words, want) {
		t.Fatalf("words: got %v want %v", words, want)
	}
	if !reflect.DeepEqual(starts, []int{1, 7, 11, 15}) {
		t.Fatalf("starts: got %v", starts)
	}
	if !reflect.DeepEqual(ends, []int{6, 10, 14, 19}) {
		t.Fatalf("ends: got %v", ends)
	}
}

func TestSplitWordsPunctuation(t *testing.T) {
	words, starts, ends := SplitWords("Hello, world!")
	want := []string{"Hello", ",", "world", "!"}
	if !reflect.DeepEqual(words, want) {
		t.Fatalf("words: got %v want %v", words, want)
	}
	if !reflect.DeepEqual(starts, []int{1, 6, 8, 13}) {
		t.Fatalf("starts: got %v", starts)
	}
	if !reflect.DeepEqual(ends, []int{6, 7, 13, 14}) {
		t.Fatalf("ends: got %v", ends)
	}
}

func TestSplitWordsWhitespaceRuns(t *testing.T) {
	words, _, _ := SplitWords("  a\t\tb\n\nc  ")
	if !reflect.DeepEqual(words, []string{"a", "b", "c"}) {
		t.Fatalf("words: got %v", words)
	}
}

func TestSplitWordsMultibyte(t *testing.T) {
	// café and wörld contain multi-byte runes; splits must respect rune
	// boundaries and return byte offsets.
	words, starts, ends := SplitWords("café wörld")
	if !reflect.DeepEqual(words, []string{"café", "wörld"}) {
		t.Fatalf("words: got %v", words)
	}
	if !reflect.DeepEqual(starts, []int{1, 7}) {
		t.Fatalf("starts: got %v", starts)
	}
	// len("café") = 5 bytes → word ends at 6; "wörld" ends at 13 (exclusive).
	if !reflect.DeepEqual(ends, []int{6, 13}) {
		t.Fatalf("ends: got %v", ends)
	}
}

func TestSplitWordsSliceRoundTrip(t *testing.T) {
	// Reassembling words from [starts, ends) must reproduce the text.
	text := "Apple CEO Tim Cook announced iPhone 15."
	words, starts, ends := SplitWords(text)
	var b []byte
	for i, w := range words {
		got := text[starts[i]-1 : ends[i]-1]
		if got != w {
			t.Fatalf("word %d: slice %q != %q", i, got, w)
		}
		b = append(b, got...)
	}
}

func TestSplitWordsCJKPunctuation(t *testing.T) {
	// CJK period U+3002 (0xE3 0x80 0x82) and ideographic comma U+3001 are
	// punctuation, split as their own words; CJK text accumulates.
	words, _, _ := SplitWords("東京、大阪。")
	if !reflect.DeepEqual(words, []string{"東京", "、", "大阪", "。"}) {
		t.Fatalf("words: got %v", words)
	}
}

func TestSplitWordsNoLowercase(t *testing.T) {
	// The block stays raw: models decide their own casing rules.
	words, _, _ := SplitWords("Tim Cook")
	if !reflect.DeepEqual(words, []string{"Tim", "Cook"}) {
		t.Fatalf("words must not be lowercased: %v", words)
	}
}
