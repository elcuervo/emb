package script

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/elcuervo/emb/internal/onnx"
)

// TestAPIVersionReadable verifies emb.API_VERSION is exposed and reports the
// build version the server injected, falling back to DefaultVersion when no
// ldflag set one.
func TestAPIVersionReadable(t *testing.T) {
	v, err := EvalWithHosts(`return emb.API_VERSION`, nil, nil, Hosts{}, EvalOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if v.String() != APIVersion || APIVersion == "" {
		t.Fatalf("emb.API_VERSION = %q, want %q", v.String(), APIVersion)
	}
	if APIVersion != DefaultVersion {
		t.Fatalf("unset build version = %q, want %q", APIVersion, DefaultVersion)
	}
	// SetVersion is the one write, and it moves the reported value with it.
	SetVersion("9.9.9")
	defer SetVersion("")
	if got, err := EvalWithHosts(`return emb.API_VERSION`, nil, nil, Hosts{}, EvalOptions{}); err != nil || got.String() != "9.9.9" {
		t.Fatalf("emb.API_VERSION after SetVersion = %q (err %v), want 9.9.9", got, err)
	}
}

// TestShippedScriptsCompile is the compatibility gate: every shipped example
// and every preset a deployment loads at boot must still compile against the
// current host surface (parse + wrap), so an API change cannot silently break a
// published or preloaded script. The deliberately broken fixture is the one
// file this skips.
func TestShippedScriptsCompile(t *testing.T) {
	patterns := []string{
		"../../examples/scripts/*/*.lua",
		"../../scripts/*.lua", // the presets the sandbox and the DNS zone preload
	}
	var files []string
	for _, pattern := range patterns {
		matches, err := filepath.Glob(pattern)
		if err != nil {
			t.Fatal(err)
		}
		if len(matches) == 0 {
			t.Fatalf("no scripts matched %s", pattern)
		}
		files = append(files, matches...)
	}
	for _, f := range files {
		if filepath.Base(f) == "invalid.lua" {
			continue
		}
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
// (with the server version and the KEYS count folded into the digest) so the
// cache identity of existing scripts cannot drift silently. The digest changed
// when the version was folded in, again when the KEYS count was folded in (the
// arity-collision fix), again at host API 1.3.0 (bool tensors, encode_plain,
// special_ids, decode_ordered), again with the v2 key domain and literal/hash
// discriminator, and once more when the script API version became the server
// version. It is pinned against a fixed version here so the format is tested,
// not the build.
func TestReplyCacheKeyUnchanged(t *testing.T) {
	SetVersion("0.4.3.pre3")
	defer SetVersion("")
	got := CacheKey("minilm", "0123456789abcdef0123456789abcdef01234567", []string{"PERSON", "ORG"}, 1, "hello world")
	const want = "minilm:0123456789abcdef0123456789abcdef01234567:297bb1bfc91255f5a26dfce04dfebe683cadb3f5f275cb9b24f69e4491fc7d0a:hello world"
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
		"emb.tokenize.encode_pair", "emb.tokenize.encode_plain", "emb.tokenize.special_ids",
		"emb.math.sigmoid", "emb.math.softmax", "emb.math.argmax", "emb.math.float32_bytes",
		"emb.math.dot", "emb.math.cosine", "emb.math.l2", "emb.math.norm",
		"emb.math.mean_pool", "emb.math.cls", "emb.math.topk", "emb.math.gather",
		"emb.math.slice", "emb.math.scale", "emb.math.add",
		"emb.image.preprocess", "emb.image.info", "emb.image.embed",
		"json.encode", "json.decode", "json.decode_ordered", "json.null", "emb.API_VERSION",
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
