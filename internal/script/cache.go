package script

import (
	"crypto/sha1" //nolint:gosec // content-addressed cache key folding, not security
	"encoding/hex"
	"strings"
)

// CacheKey builds the content-addressed cache key for a scripted evaluation:
//
//	model:sha1(script):sha1(args joined by NUL):text
//
// Distinct script versions, arg sets (label sets, thresholds, …) and texts
// always produce distinct keys, so schema changes are simply new cache
// entries. Determinism of the sandbox (no random/time) makes the key correct:
// identical inputs always produce identical replies.
func CacheKey(modelName, scriptSHA string, args []string, text string) string {
	//nolint:gosec // cache key only
	h := sha1.Sum([]byte(strings.Join(args, "\x00")))
	return modelName + ":" + scriptSHA + ":" + hex.EncodeToString(h[:]) + ":" + text
}
