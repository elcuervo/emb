package script

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/elcuervo/emb/internal/onnx"
)

// TestAPIVersionReadable verifies emb.API_VERSION is exposed and stable.
func TestAPIVersionReadable(t *testing.T) {
	v, err := EvalWithHosts(`return emb.API_VERSION`, nil, nil, Hosts{}, EvalOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if v.String() != APIVersion || APIVersion == "" {
		t.Fatalf("emb.API_VERSION = %q, want %q", v.String(), APIVersion)
	}
	if !strings.HasPrefix(APIVersion, "1.") {
		t.Fatalf("unexpected API version %q", APIVersion)
	}
}

// TestExampleScriptsCompile is the compatibility gate: every shipped example
// must still compile against the current host surface (parse + wrap), so an
// API change cannot silently break a published script.
func TestExampleScriptsCompile(t *testing.T) {
	files, err := filepath.Glob("../../examples/scripts/*/*.lua")
	if err != nil {
		t.Fatal(err)
	}
	if len(files) == 0 {
		t.Fatal("no example scripts found")
	}
	for _, f := range files {
		src, err := os.ReadFile(f)
		if err != nil {
			t.Fatal(err)
		}
		if err := Compile(string(src)); err != nil {
			t.Errorf("%s no longer compiles: %v", filepath.Base(f), err)
		}
	}
}

// TestReplyCacheKeyUnchanged pins the content-addressed reply-cache key format
// (with the host API version folded into the digest) so the cache identity of
// existing scripts cannot drift silently. The digest changed once when the
// version was folded in (design decision 8); it is pinned again here.
func TestReplyCacheKeyUnchanged(t *testing.T) {
	got := CacheKey("minilm", "0123456789abcdef0123456789abcdef01234567", []string{"PERSON", "ORG"}, "hello world")
	const want = "minilm:0123456789abcdef0123456789abcdef01234567:9a249ece555117246fdca83f58b01aeccdd383e81eaf40d48e4ce2d63a221117:hello world"
	if got != want {
		t.Fatalf("reply cache key changed:\n got %q\nwant %q", got, want)
	}
}

// TestHostSurfaceIsComplete asserts every function the API reference advertises
// is actually registered, so the reference cannot drift from the code.
func TestHostSurfaceIsComplete(t *testing.T) {
	hosts := Hosts{
		Run:                func([]onnx.NamedTensor) (map[string]onnx.NamedTensor, error) { return nil, nil },
		Embed:              func([]string) ([][]byte, error) { return nil, nil },
		EncodePlain:        func(string, int) ([]int64, []int64, [][2]int, error) { return nil, nil, nil, nil },
		EncodePair:         func(string, string, int) ([]int64, []int64, [][2]int, int, error) { return nil, nil, nil, 0, nil },
		EncodePretokenized: func([]string, int) ([]int64, []int64, error) { return nil, nil, nil },
		Image: &ImageHost{
			Preprocess: func([]byte) ([]float32, error) { return nil, nil },
			Embed:      func([][]byte) ([][]byte, error) { return nil, nil },
		},
	}
	want := []string{
		"emb.run", "emb.run_batch", "emb.embed", "emb.similarity", "emb.distance",
		"emb.tokenize.pretokenized", "emb.tokenize.words", "emb.tokenize.encode",
		"emb.tokenize.encode_pair",
		"emb.math.sigmoid", "emb.math.softmax", "emb.math.argmax", "emb.math.float32_bytes",
		"emb.math.dot", "emb.math.cosine", "emb.math.l2", "emb.math.norm",
		"emb.math.mean_pool", "emb.math.cls", "emb.math.topk", "emb.math.gather",
		"emb.math.slice", "emb.math.scale", "emb.math.add",
		"emb.image.preprocess", "emb.image.info", "emb.image.embed",
		"json.encode", "json.decode", "json.null", "emb.API_VERSION",
	}
	for _, path := range want {
		v, err := EvalWithHosts("return type("+path+")", nil, nil, hosts, EvalOptions{})
		if err != nil {
			t.Errorf("%s: %v", path, err)
			continue
		}
		if v.String() == "nil" {
			t.Errorf("%s is not registered", path)
		}
	}
}
