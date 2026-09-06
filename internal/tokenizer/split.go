package tokenizer

import (
	"strings"
	"unicode"
	"unicode/utf8"
)

// bertPunct reports whether r is punctuation under the BertPreTokenizer
// rules (ASCII punctuation plus the CJK/fullwidth ranges the Rust tokenizers
// crate recognizes). Whitespace is handled separately.
func bertPunct(r rune) bool {
	if 33 <= r && r <= 47 {
		return true
	}
	if 58 <= r && r <= 64 {
		return true
	}
	if 91 <= r && r <= 96 {
		return true
	}
	if 123 <= r && r <= 126 {
		return true
	}
	switch {
	case 0x3000 <= r && r <= 0x303F: // CJK symbols & punctuation (incl. U+3000)
	case 0x2018 <= r && r <= 0x201F: // quotes / dashes
	case 0x3014 <= r && r <= 0x301F: // CJK brackets
	case 0xFE30 <= r && r <= 0xFE4F: // CJK compatibility forms
	case 0xFF01 <= r && r <= 0xFF3F: // fullwidth forms (!..? and letters)
	case 0xFF5B <= r && r <= 0xFF65: // fullwidth brackets and halfwidth punct
	default:
		return false
	}
	return true
}

// SplitWords splits text into words with byte offsets, following the
// BertPreTokenizer rules used by word-aligned models (GLiNER-style scripts):
// whitespace separates words and is dropped, each punctuation character is
// its own word, everything else accumulates into a word. It returns the words
// as-is (no lowercasing — models differ) plus each word's byte span as
// 1-based [starts[i], ends[i]) offsets (Lua-friendly).
//
// The offsets are byte-based (matching the tokenizers binding's offsets), so
// scripts can slice the original text with string.sub(text, start, end-1).
func SplitWords(text string) (words []string, starts, ends []int) {
	for i := 0; i < len(text); {
		r, size := utf8.DecodeRuneInString(text[i:])
		next := i + size
		if isBertWhitespace(r) {
			i = next
			continue
		}
		if bertPunct(r) {
			words = append(words, string(r))
			starts = append(starts, i+1)
			ends = append(ends, next+1)
			i = next
			continue
		}
		wordStart := i
		var b strings.Builder
		for i < len(text) {
			r, size = utf8.DecodeRuneInString(text[i:])
			if isBertWhitespace(r) || bertPunct(r) {
				break
			}
			b.WriteRune(r)
			i += size
		}
		if w := b.String(); w != "" {
			words = append(words, w)
			starts = append(starts, wordStart+1)
			ends = append(ends, i+1)
		}
	}
	return words, starts, ends
}

// isBertWhitespace mirrors Rust's char::is_whitespace for the characters the
// pretokenizer must treat as separators (U+3000 ideographic space is in the
// punctuation range per the tokenizers crate, so it is handled by bertPunct;
// here we only classify the remaining Unicode whitespace).
func isBertWhitespace(r rune) bool {
	return unicode.IsSpace(r)
}
