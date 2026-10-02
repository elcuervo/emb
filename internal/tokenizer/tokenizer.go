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

// OffsetTokenizer is the optional token-offset capability used by the
// scripted-model path: plain and pair encodes with per-token byte offsets
// into the source string(s), so scripts can slice surface text without a
// token-decode block.
type OffsetTokenizer interface {
	// EncodeOffsets encodes text through the model tokenizer's own pipeline
	// (special tokens included), returning ids, mask, and each token's byte
	// span [start, end) into text.
	EncodeOffsets(text string, maxLength int) (ids, mask []int64, offsets [][2]int, err error)

	// EncodePairOffsets composes the BERT-family pair template
	// [CLS] first [SEP] second [SEP]: ids and mask for the full sequence,
	// the 1-based position sep of the inter-part [SEP] token, and byte
	// offsets per token into its own part (first-part tokens slice `first`,
	// second-part tokens slice `second`).
	EncodePairOffsets(first, second string, maxLength int) (ids, mask []int64, offsets [][2]int, sep int, err error)
}

// SpecialTokenIDs is the set of special-token IDs a sequence builder needs to
// reproduce a model's templates: the [MASK], [CLS], [SEP] and [PAD] tokens.
// MaskToken is the resolved mask token's text (e.g. "[MASK]" or "<mask>"),
// which reference sequence construction replaces with a space in the
// instructions, options and state.
type SpecialTokenIDs struct {
	Mask      int64
	CLS       int64
	SEP       int64
	PAD       int64
	MaskToken string
}

// PlainTokenizer is the optional no-special-token encoding capability used by
// sequence-building scripts (e.g. Laya's build_sequence): it encodes text the
// way `encode(text, add_special_tokens=False)` does, returning only the plain
// token IDs, without special tokens and without padding.
type PlainTokenizer interface {
	// EncodePlain encodes a single text without special tokens. No padding is
	// applied: the returned slice mirrors the real token count (front-truncated
	// to maxLength when positive), matching EncodePretokenized's contract.
	EncodePlain(text string, maxLength int) ([]int64, error)
}

// SpecialTokenIDsProvider reports a model's special-token IDs so scripts do
// not have to probe single-token encodes. Discovery follows the reference
// implementation's semantics: tokenizer-config special-token names first,
// then the tokenizer.json added-token entries, then a single-token encode.
type SpecialTokenIDsProvider interface {
	SpecialTokenIDs() (SpecialTokenIDs, error)
}
