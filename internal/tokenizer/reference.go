package tokenizer

import (
	"fmt"
	"strings"

	"github.com/daulet/tokenizers"
)

type RefTokenizer struct {
	tk        *tokenizers.Tokenizer
	padOutput bool
}

func NewTokenizer(path string, padOutput bool) (*RefTokenizer, error) {
	tk, err := tokenizers.FromFile(path)
	if err != nil {
		return nil, fmt.Errorf("loading tokenizer from %q: %w", path, err)
	}
	return &RefTokenizer{tk: tk, padOutput: padOutput}, nil
}

func (t *RefTokenizer) Encode(text string, maxLength int) ([]int64, []int64, error) {
	enc := t.tk.EncodeWithOptions(text, true, tokenizers.WithReturnAttentionMask())
	ids := enc.IDs
	mask := enc.AttentionMask

	if t.padOutput {
		realLen := 0
		for _, m := range mask {
			if m == 1 {
				realLen++
			}
		}
		if realLen > maxLength {
			realLen = maxLength
		}
		inputIDs := make([]int64, maxLength)
		attnMask := make([]int64, maxLength)
		for i := 0; i < realLen; i++ {
			inputIDs[i] = int64(ids[i])
			attnMask[i] = 1
		}
		return inputIDs, attnMask, nil
	}

	realLen := 0
	for _, m := range mask {
		if m == 1 {
			realLen++
		}
	}

	ids = ids[:realLen]
	if len(ids) > maxLength {
		ids = ids[:maxLength]
	}

	inputIDs := make([]int64, len(ids))
	attnMask := make([]int64, len(ids))
	for i, id := range ids {
		inputIDs[i] = int64(id)
		attnMask[i] = 1
	}
	return inputIDs, attnMask, nil
}

func (t *RefTokenizer) Close() error {
	return t.tk.Close()
}

// EncodePretokenized encodes a sequence of already-split words as a single
// string (words joined by a single space) with special tokens disabled, then
// attributes every subword to its word via the tokenizer's char offsets. This
// reproduces the behavior of is_split_into_words-style word-level encoding:
// each element of `words` maps to exactly one word index (its position in the
// list), and byte-level boundary markers (leading-space prefixes on non-first
// words) are produced naturally by encoding the joined string in one pass.
//
// Unlike Encode, no padding is applied: the returned lengths mirror the real
// token count (front-truncated to maxLength when positive), which is what
// scripted input construction expects.
func (t *RefTokenizer) EncodePretokenized(words []string, maxLength int) ([]int64, []int64, error) {
	joined, wStart, wEnd := joinWords(words)
	enc := t.tk.EncodeWithOptions(joined, false, tokenizers.WithReturnOffsets(), tokenizers.WithReturnAttentionMask())
	if len(enc.IDs) != len(enc.Offsets) {
		return nil, nil, fmt.Errorf("tokenizer returned %d ids and %d offsets", len(enc.IDs), len(enc.Offsets))
	}
	// Trim built-in padding (tokenizer.json may declare padding to a fixed
	// length) using the attention mask, mirroring Encode's real-length logic.
	realLen := 0
	for _, m := range enc.AttentionMask {
		if m == 1 {
			realLen++
		}
	}
	n := realLen
	if maxLength > 0 && n > maxLength {
		n = maxLength
	}
	ids := make([]int64, n)
	wordIDs := make([]int64, n)
	for i := 0; i < n; i++ {
		ids[i] = int64(enc.IDs[i])
		wordIDs[i] = int64(wordForOffset(int(enc.Offsets[i][0]), int(enc.Offsets[i][1]), wStart, wEnd))
	}
	return ids, wordIDs, nil
}

// joinWords joins words with single spaces and returns each word's byte-offset
// span in the joined string. The embedded tokenizers report byte offsets, so
// spans are byte-based (len(w) is the byte length of a Go string).
func joinWords(words []string) (joined string, start, end []int) {
	start = make([]int, len(words))
	end = make([]int, len(words))
	var b strings.Builder
	nbytes := 0
	for i, w := range words {
		start[i] = nbytes
		if i > 0 {
			b.WriteByte(' ')
			nbytes++
		}
		b.WriteString(w)
		nbytes += len(w)
		end[i] = nbytes
	}
	return b.String(), start, end
}

// wordForOffset attributes a token's char span [s, e) to the first word whose
// span ends at or after e. This is robust to byte-level tokenizers whose
// subword offsets include the separating space of the previous position: such
// a token still ends inside its own word's span.
func wordForOffset(s, e int, wStart, wEnd []int) int {
	for k := range wStart {
		if e <= wEnd[k] {
			return k
		}
	}
	return -1
}
