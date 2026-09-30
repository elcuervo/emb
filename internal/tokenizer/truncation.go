package tokenizer

// truncatePreservingSpecialTokens truncates ids to maxLength while keeping
// every special token (as declared by the parallel specialMask, 1 = special)
// in place and keeping content in order, matching the standard
// truncation-with-special-tokens semantics used by the reference stacks.
// It is a no-op when len(ids) <= maxLength; callers guarantee specialMask is
// parallel to ids.
func truncatePreservingSpecialTokens(ids []int64, specialMask []uint32, maxLength int) []int64 {
	keep := specialKeepIndexes(len(ids), specialMask, maxLength)
	if keep == nil {
		return ids
	}
	out := make([]int64, len(keep))
	for j, i := range keep {
		out[j] = ids[i]
	}
	return out
}

// specialKeepIndexes returns the indexes into a token sequence to keep when
// truncating to maxLength while preserving special tokens: every special
// token is kept in place and content is kept in order until the remaining
// budget is spent. nil means nothing is dropped (len <= maxLength).
func specialKeepIndexes(n int, specialMask []uint32, maxLength int) []int {
	if n <= maxLength {
		return nil
	}
	var totalSpecials int
	for _, s := range specialMask {
		if s == 1 {
			totalSpecials++
		}
	}
	budget := maxLength - totalSpecials
	if budget < 0 {
		budget = 0
	}
	keep := make([]int, 0, maxLength)
	kept := 0
	for i := 0; i < n; i++ {
		if specialMask[i] == 1 {
			keep = append(keep, i)
			continue
		}
		if kept < budget {
			keep = append(keep, i)
			kept++
		}
	}
	return keep
}

// compactSpecials returns the first n entries of a special-tokens mask,
// zero-padding if the mask is shorter than n. After right-padding is trimmed
// the real tokens occupy the leading window (the same one trimByMask uses),
// so this keeps the mask parallel to the compacted token sequence.
func compactSpecials(mask []uint32, n int) []uint32 {
	spec := make([]uint32, n)
	for i := 0; i < n && i < len(mask); i++ {
		spec[i] = mask[i]
	}
	return spec
}