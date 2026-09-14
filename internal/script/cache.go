package script

import (
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
)

// CacheKeyInlineLimit is the maximum size of a KEYS element inlined verbatim
// into a script reply-cache key. Larger payloads (for example image bytes) are
// represented by a SHA-256 digest instead, so a single image-sized KEYS element
// does not retain megabytes per cache entry. Short text elements stay
// byte-identical to the pre-change key format, so existing cached entries
// remain reachable.
const CacheKeyInlineLimit = 256

// CacheKey builds the content-addressed cache key for a scripted evaluation:
//
//	model:sha1(script):sha256(apiVersion|count|len|arg...):text
//
// where the trailing `text` is the KEYS element itself when it is small, or
// "#<sha256(text)>" when it exceeds CacheKeyInlineLimit. The host API version is
// folded into the digest (design decision 8), so a host-function semantics
// change never serves a reply produced under the previous surface. The
// argument hash folds the argument count and each argument's length, so ARGV
// boundaries are unambiguous: nil, [""], and ["a","b"] vs ["a\x00b"] all hash
// differently. ARGV is caller-controlled, so the digest uses SHA-256 (a SHA-1
// collision could replay a cached reply for a different argument sequence); the
// model/script identity uses Redis EVALSHA's SHA-1 by protocol convention
// (scriptSHA). Distinct script versions, arg sets (label sets, thresholds, …)
// and texts always produce distinct keys, so schema changes are simply new
// cache entries. Determinism of the sandbox (no random/time) makes the key
// correct: identical inputs always produce identical replies.
func CacheKey(modelName, scriptSHA string, args []string, text string) string {
	return cacheKey(APIVersion, modelName, scriptSHA, args, text)
}

// cacheKey is CacheKey with the host API version supplied explicitly, so the
// version-folding behavior is directly testable.
func cacheKey(apiVersion, modelName, scriptSHA string, args []string, text string) string {
	h := sha256.New()
	var size [8]byte
	binary.BigEndian.PutUint64(size[:], uint64(len(apiVersion)))
	_, _ = h.Write(size[:])
	_, _ = h.Write([]byte(apiVersion))
	binary.BigEndian.PutUint64(size[:], uint64(len(args)))
	_, _ = h.Write(size[:])
	for _, arg := range args {
		binary.BigEndian.PutUint64(size[:], uint64(len(arg)))
		_, _ = h.Write(size[:])
		_, _ = h.Write([]byte(arg))
	}
	tail := text
	if len(text) > CacheKeyInlineLimit {
		sum := sha256.Sum256([]byte(text))
		tail = "#" + hex.EncodeToString(sum[:])
	}
	return modelName + ":" + scriptSHA + ":" + hex.EncodeToString(h.Sum(nil)) + ":" + tail
}
