package tokenizer

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/daulet/tokenizers"
)

type RefTokenizer struct {
	tk        *tokenizers.Tokenizer
	path      string
	padOutput bool
}

func NewTokenizer(path string, padOutput bool) (*RefTokenizer, error) {
	tk, err := tokenizers.FromFile(path)
	if err != nil {
		return nil, fmt.Errorf("loading tokenizer from %q: %w", path, err)
	}
	return &RefTokenizer{tk: tk, path: path, padOutput: padOutput}, nil
}

func (t *RefTokenizer) Encode(text string, maxLength int) ([]int64, []int64, error) {
	enc := t.tk.EncodeWithOptions(text, true,
		tokenizers.WithReturnAttentionMask(),
		tokenizers.WithReturnSpecialTokensMask())
	ids := enc.IDs
	mask := enc.AttentionMask

	realLen := 0
	for _, m := range mask {
		if m == 1 {
			realLen++
		}
	}

	// Real tokens occupy the leading window (right-padding trimmed); keep
	// special tokens (e.g. the trailing <eos> a post-processor appends) in
	// place when truncating, matching the reference truncation semantics.
	real := make([]int64, realLen)
	for i := 0; i < realLen; i++ {
		real[i] = int64(ids[i])
	}
	real = truncatePreservingSpecialTokens(real, compactSpecials(enc.SpecialTokensMask, realLen), maxLength)

	if t.padOutput {
		// Truncation preserves special tokens even when they alone exceed
		// maxLength, but the padded output is allocated to exactly maxLength:
		// reject the impossible request instead of indexing past the buffer.
		if len(real) > maxLength {
			return nil, nil, fmt.Errorf("tokenizer produced %d tokens (including special tokens) for max_length %d", len(real), maxLength)
		}
		inputIDs := make([]int64, maxLength)
		attnMask := make([]int64, maxLength)
		for i, id := range real {
			inputIDs[i] = id
			attnMask[i] = 1
		}
		return inputIDs, attnMask, nil
	}

	attnMask := make([]int64, len(real))
	for i := range real {
		attnMask[i] = 1
	}
	return real, attnMask, nil
}

func (t *RefTokenizer) Close() error {
	return t.tk.Close()
}

// EncodePlain encodes a single text without special tokens (the tokenizers
// `encode(text, add_special_tokens=False)` form), returning real-length ids
// with built-in padding trimmed via the attention mask and front-truncated to
// maxLength when positive. This is the primitive Laya-style sequence builders
// compose; it deliberately does not pad.
func (t *RefTokenizer) EncodePlain(text string, maxLength int) ([]int64, error) {
	enc := t.tk.EncodeWithOptions(text, false, tokenizers.WithReturnAttentionMask())
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
	for i := 0; i < n; i++ {
		ids[i] = int64(enc.IDs[i])
	}
	return ids, nil
}

// SpecialTokenIDs resolves mask/cls/sep/pad ids by the reference discovery
// order: the tokenizer's own special-token names (tokenizer_config.json /
// special_tokens_map.json siblings, falling back to the conventional
// bracket/angle candidates), then the tokenizer.json added_tokens entries,
// then a single-token encode probe. This mirrors the Ruby gem's
// `Tokenizers::Tokenizer#token_to_id` resolution without a token_to_id
// binding in the CGo API.
func (t *RefTokenizer) SpecialTokenIDs() (SpecialTokenIDs, error) {
	var out SpecialTokenIDs
	dir := filepath.Dir(t.path)
	byContent := addedTokensByContent(t.path)
	resolve := func(kind string, candidates []string) (int64, string, error) {
		names := specialNames(dir, kind)
		if len(names) > 0 {
			candidates = append(names, candidates...)
		}
		for _, name := range candidates {
			if id, ok := byContent[name]; ok {
				return int64(id), name, nil
			}
		}
		for _, name := range candidates {
			ids, _ := t.tk.Encode(name, false)
			if len(ids) == 1 {
				return int64(ids[0]), name, nil
			}
		}
		return 0, "", fmt.Errorf("tokenizer has no %s token (tried %v)", kind, candidates)
	}
	var err error
	if out.Mask, out.MaskToken, err = resolve("mask", []string{"[MASK]", "<mask>"}); err != nil {
		return out, err
	}
	if out.CLS, _, err = resolve("cls", []string{"[CLS]", "<s>", "<cls>", "<bos>"}); err != nil {
		return out, err
	}
	if out.SEP, _, err = resolve("sep", []string{"[SEP]", "</s>", "<sep>", "<eos>"}); err != nil {
		return out, err
	}
	if out.PAD, _, err = resolve("pad", []string{"[PAD]", "<pad>"}); err != nil {
		return out, err
	}
	return out, nil
}

// addedTokensByContent parses a tokenizer.json file's added_tokens entries
// into a content → id map. The daulet CGo surface has no token_to_id, and the
// added_tokens array is the exact source the reference uses for the specials
// (e.g. `[PAD] 0/[UNK] 1/[CLS] 2/[SEP] 3/[MASK] 4` in the Laya fixture).
func addedTokensByContent(path string) map[string]int {
	out := make(map[string]int)
	data, err := os.ReadFile(path)
	if err != nil {
		return out
	}
	var doc struct {
		AddedTokens []struct {
			Content string `json:"content"`
			ID      int    `json:"id"`
		} `json:"added_tokens"`
	}
	if err := json.Unmarshal(data, &doc); err != nil {
		return out
	}
	for _, at := range doc.AddedTokens {
		out[at.Content] = at.ID
	}
	return out
}

// specialNames reads the tokenizer-config sibling files for a kind's declared
// special-token name (e.g. mask_token; a Hash value's "content" is used). It
// mirrors the reference's file order — special_tokens_map.json then
// tokenizer_config.json, the later file winning — and returns at most one
// name per kind.
func specialNames(dir, kind string) []string {
	var found string
	for _, name := range []string{"special_tokens_map.json", "tokenizer_config.json"} {
		data, err := os.ReadFile(filepath.Join(dir, name))
		if err != nil {
			continue
		}
		var cfg map[string]json.RawMessage
		if err := json.Unmarshal(data, &cfg); err != nil {
			continue
		}
		raw, ok := cfg[kind+"_token"]
		if !ok {
			continue
		}
		var s string
		if err := json.Unmarshal(raw, &s); err == nil && s != "" {
			found = s
			continue
		}
		var obj struct {
			Content string `json:"content"`
		}
		if err := json.Unmarshal(raw, &obj); err == nil && obj.Content != "" {
			found = obj.Content
		}
	}
	if found == "" {
		return nil
	}
	return []string{found}
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
