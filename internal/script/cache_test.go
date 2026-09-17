package script

import (
	"strings"
	"testing"
)

func TestCacheKeyBoundedForLargePayloads(t *testing.T) {
	// Short text payloads stay inline (not digested), so the key ends with the
	// text itself.
	short := CacheKey("m", "s", []string{"a"}, 1, "hello")
	if !strings.HasSuffix(short, ":hello") {
		t.Fatalf("short-text key changed format: %q", short)
	}

	big := strings.Repeat("x", 1<<20)
	key := CacheKey("m", "s", nil, 1, big)
	if len(key) > 512 {
		t.Fatalf("large payload key size = %d, want bounded (<=512)", len(key))
	}
	// Distinct large payloads remain distinct entries.
	if key == CacheKey("m", "s", nil, 1, big+"y") {
		t.Fatal("distinct large payloads collided")
	}
	// Deterministic.
	if key != CacheKey("m", "s", nil, 1, big) {
		t.Fatal("large-payload key is not deterministic")
	}

	// The threshold is inclusive: at the limit the text stays inline, above it
	// is digested.
	atLimit := strings.Repeat("z", CacheKeyInlineLimit)
	if !strings.HasSuffix(CacheKey("m", "s", nil, 1, atLimit), ":"+atLimit) {
		t.Fatal("at-limit KEYS element should stay inline")
	}
	overLimit := strings.Repeat("z", CacheKeyInlineLimit+1)
	if strings.HasSuffix(CacheKey("m", "s", nil, 1, overLimit), ":"+overLimit) {
		t.Fatal("over-limit KEYS element must be digested")
	}
}

func TestCacheKeyDistinct(t *testing.T) {
	a := CacheKey("gliner2", "sha1", []string{"PERSON", "ORG"}, 1, "apple text")
	b := CacheKey("gliner2", "sha1", []string{"PERSON", "ORG"}, 1, "other text")
	c := CacheKey("gliner2", "sha1", []string{"PERSON"}, 1, "apple text")
	d := CacheKey("gliner2", "sha2", []string{"PERSON", "ORG"}, 1, "apple text")
	e := CacheKey("other", "sha1", []string{"PERSON", "ORG"}, 1, "apple text")
	// The KEYS count separates entries: single- vs multi-text evaluations of
	// the same text must never share a key (the arity-collision regression).
	f := CacheKey("m", "s", nil, 2, "apple text")
	g := CacheKey("m", "s", nil, 3, "apple text")

	seen := map[string]bool{}
	for _, k := range []string{a, b, c, d, e, f, g} {
		if k == "" {
			t.Fatal("empty cache key")
		}
		if seen[k] {
			t.Fatalf("duplicate cache key %q", k)
		}
		seen[k] = true
	}
}

func TestCacheKeyDeterministic(t *testing.T) {
	a := CacheKey("m", "s", []string{"x", "y"}, 2, "t")
	b := CacheKey("m", "s", []string{"x", "y"}, 2, "t")
	if a != b {
		t.Fatalf("cache key not deterministic: %q vs %q", a, b)
	}
	// Arg order matters (positional ARGV semantics).
	reversed := CacheKey("m", "s", []string{"y", "x"}, 2, "t")
	if a == reversed {
		t.Fatal("arg order must change the cache key")
	}
}

func TestCacheKeyArgBoundaries(t *testing.T) {
	// nil vs [""] must differ (a NUL join would flatten both to the same string).
	if CacheKey("m", "s", nil, 1, "t") == CacheKey("m", "s", []string{""}, 1, "t") {
		t.Fatal("nil and [\"\"] args must not collide")
	}
	// ["a","b"] vs ["a\x00b"] must differ (NUL is a valid ARGV byte).
	if CacheKey("m", "s", []string{"a", "b"}, 1, "t") == CacheKey("m", "s", []string{"a\x00b"}, 1, "t") {
		t.Fatal("args with embedded NUL must not collide")
	}
	// ["a","b"] vs ["ab"] must differ (count boundaries matter).
	if CacheKey("m", "s", []string{"a", "b"}, 1, "t") == CacheKey("m", "s", []string{"ab"}, 1, "t") {
		t.Fatal("different arg splits must not collide")
	}
}

func TestCacheKeyFoldsAPIVersion(t *testing.T) {
	old := cacheKey("1.1.0", "m", "s", []string{"a"}, 1, "t")
	newer := cacheKey("1.2.0", "m", "s", []string{"a"}, 1, "t")
	if old == newer {
		t.Fatal("distinct API versions must produce distinct cache keys")
	}
	// A fixed version is deterministic, and CacheKey folds the live one.
	if old != cacheKey("1.1.0", "m", "s", []string{"a"}, 1, "t") {
		t.Fatal("cache key is not deterministic for a fixed version")
	}
	if CacheKey("m", "s", []string{"a"}, 1, "t") != cacheKey(APIVersion, "m", "s", []string{"a"}, 1, "t") {
		t.Fatal("CacheKey must fold the current APIVersion")
	}
}

func TestEncodeReplyMatchesConvert(t *testing.T) {
	v, err := Eval(`return {PERSON = {"Tim Cook"}, ORG = {"Apple"}}`, nil, nil, EvalOptions{})
	if err != nil {
		t.Fatal(err)
	}
	encoded, err := EncodeReply(v)
	if err != nil {
		t.Fatal(err)
	}
	buf := &respBuffer{}
	if err := Convert(buf, v); err != nil {
		t.Fatal(err)
	}
	if string(encoded) != buf.b.String() {
		t.Fatalf("EncodeReply and Convert diverge:\n%q\n%q", encoded, buf.b.String())
	}
}
