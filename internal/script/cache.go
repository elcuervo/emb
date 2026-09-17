package script

import (
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
)

// CacheKeyInlineLimit is the maximum KEYS element inlined verbatim into a
// reply-cache key; larger payloads (image bytes) become a SHA-256 digest so one
// key cannot retain megabytes.
const CacheKeyInlineLimit = 256

// CacheKey builds the content-addressed cache key for a scripted evaluation:
//
//	model:sha1(script):sha256(apiVersion|numTexts|count|len|arg...):text
//
// numTexts is folded into the digest because the server replies with the
// script's whole value for one text but one element per text for several; the
// count keeps those namespaces from colliding. apiVersion is folded in so a
// host-semantics change never serves a reply produced under the old surface.
// Each arg's length precedes it, so ARGV boundaries are unambiguous (nil, [""],
// and ["a","b"] vs ["a\x00b"] all hash differently). The hash is SHA-256
// because ARGV is caller-controlled (a SHA-1 collision could replay another
// request's reply); model/script identity stays Redis EVALSHA's SHA-1. The
// trailing `text` is inlined when short and digested past CacheKeyInlineLimit.
func CacheKey(modelName, scriptSHA string, args []string, numTexts int, text string) string {
	return cacheKey(APIVersion, modelName, scriptSHA, args, numTexts, text)
}

// cacheKey is CacheKey with the host API version supplied explicitly, so the
// version-folding behavior is directly testable.
func cacheKey(apiVersion, modelName, scriptSHA string, args []string, numTexts int, text string) string {
	h := sha256.New()
	var size [8]byte
	binary.BigEndian.PutUint64(size[:], uint64(len(apiVersion)))
	_, _ = h.Write(size[:])
	_, _ = h.Write([]byte(apiVersion))
	binary.BigEndian.PutUint64(size[:], uint64(numTexts))
	_, _ = h.Write(size[:])
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
