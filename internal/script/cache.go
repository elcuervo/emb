package script

import (
	"crypto/sha1" //nolint:gosec // content-addressed cache key folding, not security
	"encoding/binary"
	"encoding/hex"
)

// CacheKey builds the content-addressed cache key for a scripted evaluation:
//
//	model:sha1(script):sha1(count|len|arg...):text
//
// The argument hash folds the argument count and each argument's length, so
// ARGV boundaries are unambiguous: nil, [""], and ["a","b"] vs ["a\x00b"] all
// hash differently. Distinct script versions, arg sets (label sets,
// thresholds, …) and texts always produce distinct keys, so schema changes are
// simply new cache entries. Determinism of the sandbox (no random/time) makes
// the key correct: identical inputs always produce identical replies.
func CacheKey(modelName, scriptSHA string, args []string, text string) string {
	//nolint:gosec // cache key only
	h := sha1.New()
	var size [8]byte
	binary.BigEndian.PutUint64(size[:], uint64(len(args)))
	_, _ = h.Write(size[:])
	for _, arg := range args {
		binary.BigEndian.PutUint64(size[:], uint64(len(arg)))
		_, _ = h.Write(size[:])
		_, _ = h.Write([]byte(arg))
	}
	return modelName + ":" + scriptSHA + ":" + hex.EncodeToString(h.Sum(nil)) + ":" + text
}
