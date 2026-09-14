package server

import (
	"crypto/sha256"
	"encoding/hex"
	"strings"
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

// TestModelOfHandlesColonsInModelNames verifies per-model attribution when a
// model name itself contains a colon. The keys are parsed positionally (a
// fixed-width digest suffix), not up to the first colon, so such models keep
// their own stats bucket and FlushModel scope.
func TestModelOfHandlesColonsInModelNames(t *testing.T) {
	const model = "team:model"
	cases := map[string]string{
		textCacheKey(model, "some text"):            "txt key",
		imageCacheKey(model, []byte("image bytes")): "img key",
		model + ":" + strings.Repeat("a", sha1HexLen) + ":" + strings.Repeat("b", sha256HexLen) + ":some text": "script key",
	}
	for key, kind := range cases {
		if got := modelOf(key); got != model {
			t.Errorf("modelOf(%s key) = %q, want %q", kind, got, model)
		}
	}
	// Legacy unprefixed keys without a digest suffix still attribute to the
	// first colon.
	if got := modelOf("plain:entry"); got != "plain" {
		t.Errorf("modelOf(legacy key) = %q, want plain", got)
	}
}
