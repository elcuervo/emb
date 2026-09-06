package tokenizer

type Tokenizer interface {
	Encode(text string, maxLength int) (inputIDs, attnMask []int64, err error)
	Close() error
}

// PretokenizedTokenizer is the optional word-aligned encoding capability used
// by the scripted-model path. Tokenizers used by scripts that need per-word
// alignment (e.g. GLiNER's schema-baked sequence construction) implement it;
// the embedding pipeline only ever needs the plain Encode.
type PretokenizedTokenizer interface {
	// EncodePretokenized encodes an already-split sequence of words with
	// special tokens disabled, returning per-subword input IDs and, for each
	// subword, the index of the word it belongs to in the input list.
	EncodePretokenized(words []string, maxLength int) (inputIDs, wordIDs []int64, err error)
}
