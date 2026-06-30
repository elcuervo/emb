package tokenizer

import (
	"fmt"

	"github.com/daulet/tokenizers"
)

type RefTokenizer struct {
	tk *tokenizers.Tokenizer
}

func NewTokenizer(path string) (*RefTokenizer, error) {
	tk, err := tokenizers.FromFile(path)
	if err != nil {
		return nil, fmt.Errorf("loading tokenizer from %q: %w", path, err)
	}
	return &RefTokenizer{tk: tk}, nil
}

func (t *RefTokenizer) Encode(text string, maxLength int) ([]int64, []int64, error) {
	enc := t.tk.EncodeWithOptions(text, true, tokenizers.WithReturnAttentionMask())
	ids := enc.IDs
	mask := enc.AttentionMask

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
