package tokenizer

type Tokenizer interface {
	Encode(text string, maxLength int) (inputIDs, attnMask []int64, err error)
	Close() error
}
