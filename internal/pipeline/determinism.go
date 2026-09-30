package pipeline

import (
	"bytes"
	"errors"
	"fmt"

	"github.com/elcuervo/emb/internal/onnx"
	"github.com/elcuervo/emb/internal/tokenizer"
)

// ErrBatchDependence is returned by ProbeBatchDeterminism when a graph's
// output depends on which texts share its inference batch. Graphs with
// dynamic activation quantization (e.g. DynamicQuantizeLinear scaling over
// the whole [batch, seq, dim] tensor) fail; fp32 and static-quantized (QDQ)
// graphs are batch-invariant and pass.
var ErrBatchDependence = errors.New("model output depends on batch composition")

// probeTexts are the canned probe inputs, chosen to move activation extrema
// (a short phrase and a long, dense one) so a dynamic quantization grid is
// likely to shift between a 1-row run and a 2-row run.
var probeTexts = [2]string{
	"a photo of a cat sitting on a windowsill",
	"quantum entanglement and the nature of spacetime curvature across many nested scales of physical description",
}

// ProbeBatchDeterminism verifies that a model's output bytes are independent
// of batch composition: it embeds two probe texts alone (1-row runs) and
// co-batched (1 2-row run) on a freshly opened session, padding every run to
// the same sequence length so the comparison isolates batch composition from
// padding, then byte-compares each text's alone vs co-batched output (exact,
// no tolerance — same process/build, so any difference is batch dependence).
//
// It returns nil for batch-invariant graphs and ErrBatchDependence otherwise.
// Cost: one session + three small runs, intended to run once per model at load.
func ProbeBatchDeterminism(sessionFactory func() (onnx.Session, error), tok tokenizer.Tokenizer, dim, maxLen int, normalize bool, pooling string) error {
	if dim <= 0 || maxLen <= 0 {
		return fmt.Errorf("probe requires dim and max_length > 0 (got dim=%d max_length=%d)", dim, maxLen)
	}

	sess, err := sessionFactory()
	if err != nil {
		return fmt.Errorf("opening probe session: %w", err)
	}
	defer func() { _ = sess.Close() }()

	// Tokenize both probe texts and pad to a common sequence length: native
	// padding (PadEncodings) re-pads per run, which would make the solo and
	// co-batched runs differ in width for batch-invariant graphs too (mean
	// pooling over a longer padded sequence changes bytes deterministically).
	encs := make([][]Encoding, len(probeTexts))
	seqLen := 0
	for i, text := range probeTexts {
		ids, mask, err := tok.Encode(text, maxLen)
		if err != nil {
			return fmt.Errorf("probe tokenization: %w", err)
		}
		if len(ids) > seqLen {
			seqLen = len(ids)
		}
		encs[i] = []Encoding{{InputIDs: ids, AttentionMask: mask}}
	}
	if seqLen == 0 {
		return errors.New("probe tokenization produced empty encodings")
	}
	for i := range encs {
		id, mask := encs[i][0].InputIDs, encs[i][0].AttentionMask
		for len(id) < seqLen {
			id = append(id, 0)
			mask = append(mask, 0)
		}
		encs[i][0].InputIDs, encs[i][0].AttentionMask = id, mask
	}

	aloneA, _, err := runEncodings(sess, encs[0], dim, normalize, pooling)
	if err != nil {
		return fmt.Errorf("probe solo run A: %w", err)
	}
	aloneB, _, err := runEncodings(sess, encs[1], dim, normalize, pooling)
	if err != nil {
		return fmt.Errorf("probe solo run B: %w", err)
	}
	co, _, err := runEncodings(sess, []Encoding{encs[0][0], encs[1][0]}, dim, normalize, pooling)
	if err != nil {
		return fmt.Errorf("probe co-batched run: %w", err)
	}

	if !bytes.Equal(aloneA[0], co[0]) || !bytes.Equal(aloneB[0], co[1]) {
		return ErrBatchDependence
	}
	return nil
}
