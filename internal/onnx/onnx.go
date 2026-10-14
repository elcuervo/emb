package onnx

import "context"

type Session interface {
	Run(inputIDs, attnMask []int64, batchSize, seqLen, dim int) ([]float32, error)
	Close() error
}

// ContextSession is implemented by sessions that can abandon a run whose caller
// has gone away (a cancelled request or an expired admission deadline). A
// session that does not implement it is run as before, so fakes and
// non-governed sessions are unaffected.
type ContextSession interface {
	RunContext(ctx context.Context, inputIDs, attnMask []int64, batchSize, seqLen, dim int) ([]float32, error)
}

// RunContext runs one inference, refusing to start when ctx is already done and
// using the session's own context support when it has any.
func RunContext(ctx context.Context, sess Session, inputIDs, attnMask []int64, batchSize, seqLen, dim int) ([]float32, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if cs, ok := sess.(ContextSession); ok {
		return cs.RunContext(ctx, inputIDs, attnMask, batchSize, seqLen, dim)
	}
	return sess.Run(inputIDs, attnMask, batchSize, seqLen, dim)
}

type OutputInfo struct {
	Name string
	Rank int
	Dim  int64
}

// InputInfo describes one graph input tensor. Dimensions may contain -1 for a
// dynamic axis.
type InputInfo struct {
	Name       string
	Rank       int
	Dimensions []int64
}
