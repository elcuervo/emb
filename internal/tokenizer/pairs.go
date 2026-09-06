package tokenizer

import (
	"fmt"

	"github.com/daulet/tokenizers"
)

// encodeOffsets runs the tokenizer's own pipeline on a single text and trims
// any built-in padding by the attention mask (like Encode), returning the
// kept ids, a ones mask, and each token's byte span into text. Offsets of
// added/special tokens are typically [0, 0); scripts use the real-token spans.
func (t *RefTokenizer) EncodeOffsets(text string, maxLength int) ([]int64, []int64, [][2]int, error) {
	enc := t.tk.EncodeWithOptions(text, true, tokenizers.WithReturnOffsets(), tokenizers.WithReturnAttentionMask())
	return slicesFromEncoding(&enc, maxLength)
}

// EncodePairOffsets composes the BERT-family pair template [CLS] first [SEP]
// second [SEP]. Each part is encoded alone (first with special tokens, second
// without), so offsets are relative to each part and slice it directly. The
// combined sequence is capped at maxLength by truncating the second part
// first, then the first; sep is the 1-based position of the inter-part [SEP].
func (t *RefTokenizer) EncodePairOffsets(first, second string, maxLength int) ([]int64, []int64, [][2]int, int, error) {
	encA := t.tk.EncodeWithOptions(first, true, tokenizers.WithReturnOffsets(), tokenizers.WithReturnAttentionMask())
	encB := t.tk.EncodeWithOptions(second, false, tokenizers.WithReturnOffsets(), tokenizers.WithReturnAttentionMask())

	idsA, offA := trimByMask(&encA)
	idsB, offB := trimByMask(&encB)

	if maxLength <= 0 {
		maxLength = DefaultMaxLength
	}

	// The template is [CLS] A [SEP] B [SEP]. A's encode already ends with the
	// inter-part [SEP]; keep that structure when A is truncated (prefix +
	// separator), so the separator position stays exact.
	sepID := idsA[len(idsA)-1]
	prefixA := idsA[:len(idsA)-1]
	offPrefixA := offA[:len(offA)-1]

	// Budget the whole sequence including the trailing [SEP]: truncate B
	// (usually the long context) first, then A's prefix.
	budgetB := maxLength - len(idsA) - 1
	if budgetB < 0 {
		budgetB = 0
	}
	if len(idsB) > budgetB {
		idsB, offB = truncatePairPart(idsB, offB, budgetB)
	}

	budgetPrefix := maxLength - len(idsB) - 2 // prefix + A's sep + trailing sep
	if budgetPrefix < 1 {
		budgetPrefix = 1
	}
	if len(prefixA) > budgetPrefix {
		prefixA, offPrefixA = truncatePairPart(prefixA, offPrefixA, budgetPrefix)
	}

	sep := len(prefixA) + 1 // 1-based position of the inter-part [SEP]
	if sep < 2 {
		return nil, nil, nil, 0, fmt.Errorf("pair encode: first part too short")
	}

	ids := make([]int64, 0, len(prefixA)+1+len(idsB)+1)
	ids = append(ids, prefixA...)
	ids = append(ids, sepID) // inter-part [SEP]
	ids = append(ids, idsB...)
	ids = append(ids, sepID) // trailing [SEP]

	mask := make([]int64, len(ids))
	for i := range mask {
		mask[i] = 1
	}
	offsets := make([][2]int, 0, len(offPrefixA)+1+len(offB)+1)
	offsets = append(offsets, offPrefixA...)
	offsets = append(offsets, [2]int{0, 0}) // inter-part [SEP]: no source span
	offsets = append(offsets, offB...)
	offsets = append(offsets, [2]int{0, 0}) // trailing [SEP]: no source span

	return ids, mask, offsets, sep, nil
}

// DefaultMaxLength is the fallback sequence budget for pair encodes when the
// caller passes 0 (the export-style default).
const DefaultMaxLength = 512

func slicesFromEncoding(enc *tokenizers.Encoding, maxLength int) ([]int64, []int64, [][2]int, error) {
	ids, off := trimByMask(enc)
	if len(ids) == 0 {
		return nil, nil, nil, fmt.Errorf("encode produced no tokens")
	}
	if maxLength > 0 && len(ids) > maxLength {
		ids, off = truncatePairPart(ids, off, maxLength)
	}
	mask := make([]int64, len(ids))
	for i := range mask {
		mask[i] = 1
	}
	return ids, mask, off, nil
}

// trimByMask drops built-in padding (tokenizer.json may pad to a fixed
// length) using the attention mask, returning parallel ids and offsets.
func trimByMask(enc *tokenizers.Encoding) ([]int64, [][2]int) {
	n := 0
	for _, m := range enc.AttentionMask {
		if m == 1 {
			n++
		}
	}
	ids := make([]int64, n)
	off := make([][2]int, n)
	for i := 0; i < n; i++ {
		ids[i] = int64(enc.IDs[i])
		off[i] = [2]int{int(enc.Offsets[i][0]), int(enc.Offsets[i][1])}
	}
	return ids, off
}

func truncatePairPart(ids []int64, off [][2]int, n int) ([]int64, [][2]int) {
	if n <= 0 {
		n = 1
	}
	if n > len(ids) {
		n = len(ids)
	}
	return ids[:n], off[:n]
}
