package hfhub

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

// TestExtraModelFilesIncludesPreprocessorConfig guards the image-embedding
// autoconfiguration contract: DownloadModel must fetch preprocessor_config.json
// so the registry can read rescale/mean/std/crop/resample/size from it.
func TestExtraModelFilesIncludesPreprocessorConfig(t *testing.T) {
	want := map[string]bool{
		"tokenizer.json":           false,
		"config.json":              false,
		"tokenizer_config.json":    false,
		"special_tokens_map.json":  false,
		"preprocessor_config.json": false,
	}
	for _, f := range ExtraModelFiles {
		if _, ok := want[f]; ok {
			want[f] = true
		}
	}
	for name, found := range want {
		if !found {
			t.Fatalf("ExtraModelFiles is missing %q", name)
		}
	}
}

// newTestClient serves the HuggingFace API and resolve endpoints from the given
// sibling list and file map.
func newTestClient(t *testing.T, siblings []string, files map[string]string) *Client {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/models/test/model" {
			infos := make([]FileInfo, 0, len(siblings))
			for _, s := range siblings {
				infos = append(infos, FileInfo{Path: s})
			}
			_ = json.NewEncoder(w).Encode(modelAPIResponse{Siblings: infos})
			return
		}
		if body, ok := files[filepath.Base(r.URL.Path)]; ok {
			_, _ = w.Write([]byte(body))
			return
		}
		http.NotFound(w, r)
	}))
	t.Cleanup(srv.Close)
	return &Client{HTTPClient: srv.Client(), BaseURL: srv.URL}
}

func TestListFiles(t *testing.T) {
	c := newTestClient(t, []string{"model.onnx", "tokenizer.json"}, nil)
	files, err := c.ListFiles("test/model")
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(files) != 2 || files[0].Path != "model.onnx" {
		t.Fatalf("files = %+v", files)
	}
}

func TestListFilesHTTPError(t *testing.T) {
	c := newTestClient(t, nil, nil) // /api/models/test/model returns the API, but repo "other" 404s
	if _, err := c.ListFiles("other/repo"); err == nil {
		t.Fatal("expected an HTTP error")
	}
}

func TestFindONNXPriority(t *testing.T) {
	c := New()
	files := []FileInfo{{Path: "onnx/model.onnx"}, {Path: "model.onnx"}, {Path: "zzz.onnx"}}
	if got := c.FindONNX(files); got == nil || got.Path != "model.onnx" {
		t.Fatalf("FindONNX = %+v, want model.onnx first", got)
	}
	if got := c.FindONNX([]FileInfo{{Path: "b.onnx"}, {Path: "a.onnx"}}); got == nil || got.Path != "a.onnx" {
		t.Fatalf("FindONNX fallback = %+v, want a.onnx", got)
	}
	if got := c.FindONNX([]FileInfo{{Path: "README.md"}}); got != nil {
		t.Fatalf("FindONNX with no onnx = %+v, want nil", got)
	}
}

func TestFindQuantizedONNXPriority(t *testing.T) {
	c := New()
	files := []FileInfo{{Path: "model_quantized.onnx"}, {Path: "onnx/model_quantized.onnx"}}
	if got := c.FindQuantizedONNX(files); got == nil || got.Path != "onnx/model_quantized.onnx" {
		t.Fatalf("FindQuantizedONNX = %+v, want onnx/model_quantized.onnx", got)
	}
	if got := c.FindQuantizedONNX([]FileInfo{{Path: "model.onnx"}}); got != nil {
		t.Fatalf("FindQuantizedONNX = %+v, want nil", got)
	}
}

func TestDownloadWritesAndShortCircuits(t *testing.T) {
	c := newTestClient(t, nil, map[string]string{"model.onnx": "weights"})
	dest := t.TempDir()

	path, err := c.Download("test/model", "onnx/model.onnx", dest)
	if err != nil {
		t.Fatalf("download: %v", err)
	}
	if path != filepath.Join(dest, "model.onnx") {
		t.Fatalf("dest = %q", path)
	}
	if b, err := os.ReadFile(path); err != nil || string(b) != "weights" {
		t.Fatalf("contents = %q err=%v", b, err)
	}

	// A second call must not re-fetch (the file already exists), so it must
	// succeed even if the payload would differ.
	if _, err := c.Download("test/model", "onnx/model.onnx", dest); err != nil {
		t.Fatalf("second download: %v", err)
	}
}

func TestDownloadModelFetchesWeightsAndExtras(t *testing.T) {
	c := newTestClient(t,
		[]string{"onnx/model.onnx", "tokenizer.json", "config.json", "preprocessor_config.json", "README.md"},
		map[string]string{
			"model.onnx":               "onnx-bytes",
			"tokenizer.json":           "{}",
			"config.json":              "{}",
			"preprocessor_config.json": "{}",
		},
	)
	dest := t.TempDir()
	if err := c.DownloadModel("test/model", dest, false); err != nil {
		t.Fatalf("download model: %v", err)
	}
	for _, f := range []string{"model.onnx", "tokenizer.json", "config.json", "preprocessor_config.json"} {
		if _, err := os.Stat(filepath.Join(dest, f)); err != nil {
			t.Fatalf("%s not written: %v", f, err)
		}
	}
}

func TestDownloadModelPrefersQuantized(t *testing.T) {
	c := newTestClient(t,
		[]string{"model.onnx", "onnx/model_quantized.onnx", "tokenizer.json"},
		map[string]string{"model_quantized.onnx": "int8", "model.onnx": "fp32"},
	)
	dest := t.TempDir()
	if err := c.DownloadModel("test/model", dest, true); err != nil {
		t.Fatalf("download model: %v", err)
	}
	// Download writes to filepath.Base, so the quantized artifact lands as
	// model_quantized.onnx.
	got, err := os.ReadFile(filepath.Join(dest, "model_quantized.onnx"))
	if err != nil || string(got) != "int8" {
		t.Fatalf("quantized weights = %q err=%v", got, err)
	}
}

func TestDownloadModelNoONNX(t *testing.T) {
	c := newTestClient(t, []string{"README.md", "config.json"}, nil)
	if err := c.DownloadModel("test/model", t.TempDir(), false); err == nil {
		t.Fatal("expected an error when the repo has no ONNX files")
	}
}

func TestDownloadMissingFile(t *testing.T) {
	c := newTestClient(t, nil, nil)
	if _, err := c.Download("test/model", "nope.onnx", t.TempDir()); err == nil {
		t.Fatal("expected an HTTP error for a missing file")
	}
}
