package script

import (
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
)

// CacheKey builds the content-addressed cache key for a scripted evaluation:
//
//	model:sha1(script):sha256(count|len|arg...):text
//
// The argument hash folds the argument count and each argument's length, so
// ARGV boundaries are unambiguous: nil, [""], and ["a","b"] vs ["a\x00b"] all
// hash differently. ARGV is caller-controlled, so the digest uses SHA-256 (a
// SHA-1 collision could replay a cached reply for a different argument
// sequence); the model/script identity uses Redis EVALSHA's SHA-1 by protocol
// convention (scriptSHA). Distinct script versions, arg sets (label sets,
// thresholds, …) and texts always produce distinct keys, so schema changes are
// simply new cache entries. Determinism of the sandbox (no random/time) makes
// the key correct: identical inputs always produce identical replies.
func CacheKey(modelName, scriptSHA string, args []string, text string) string {
	h := sha256.New()
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
