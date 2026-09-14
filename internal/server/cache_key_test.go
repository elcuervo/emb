package server

import (
	"crypto/sha256"
	"encoding/hex"
	"testing"
)

// TestTextCacheKeyNamespaces verifies text embeddings live in their own cache
// namespace. Text, image, and script reply-cache entries share one cache map,
// so an unprefixed "model:text" key could collide with an image entry
// ("img:<model>:<sha256>") for a text literally shaped like one.
func TestTextCacheKeyNamespaces(t *testing.T) {
	data := []byte("some cached image bytes")
	sum := sha256.Sum256(data)
	text := "vision:" + hex.EncodeToString(sum[:])

	textKey := textCacheKey("img", text)
	imgKey := imageCacheKey("vision", data)
	if textKey == imgKey {
		t.Fatalf("text and image cache keys collide: %q", textKey)
	}
	if got := modelOf(textKey); got != "img" {
		t.Fatalf("modelOf(text key) = %q, want img", got)
	}
	if got := modelOf(imgKey); got != "vision" {
		t.Fatalf("modelOf(image key) = %q, want vision", got)
	}
	// Two different texts under the same model stay distinct.
	if textCacheKey("m", "a") == textCacheKey("m", "b") {
		t.Fatal("distinct texts produced the same cache key")
	}
}
