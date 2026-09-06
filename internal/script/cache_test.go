package script

import "testing"

func TestCacheKeyDistinct(t *testing.T) {
	a := CacheKey("gliner2", "sha1", []string{"PERSON", "ORG"}, "apple text")
	b := CacheKey("gliner2", "sha1", []string{"PERSON", "ORG"}, "other text")
	c := CacheKey("gliner2", "sha1", []string{"PERSON"}, "apple text")
	d := CacheKey("gliner2", "sha2", []string{"PERSON", "ORG"}, "apple text")
	e := CacheKey("other", "sha1", []string{"PERSON", "ORG"}, "apple text")

	seen := map[string]bool{}
	for _, k := range []string{a, b, c, d, e} {
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
	a := CacheKey("m", "s", []string{"x", "y"}, "t")
	b := CacheKey("m", "s", []string{"x", "y"}, "t")
	if a != b {
		t.Fatalf("cache key not deterministic: %q vs %q", a, b)
	}
	// Arg order matters (positional ARGV semantics).
	reversed := CacheKey("m", "s", []string{"y", "x"}, "t")
	if a == reversed {
		t.Fatal("arg order must change the cache key")
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
