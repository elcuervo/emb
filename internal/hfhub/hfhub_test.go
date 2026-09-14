package hfhub

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

type failingCloseTemp struct {
	name string
	w    io.WriteCloser
}

func (f *failingCloseTemp) Write(p []byte) (int, error) { return f.w.Write(p) }
func (f *failingCloseTemp) Close() error {
	_ = f.w.Close()
	return errors.New("injected close failure")
}
func (f *failingCloseTemp) Name() string { return f.name }

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

func TestInterruptedDownloadIsNotPublishedAndCanRetry(t *testing.T) {
	requests := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		requests++
		if requests == 1 {
			w.Header().Set("Content-Length", "100")
			_, _ = w.Write([]byte("partial"))
			return
		}
		_, _ = w.Write([]byte("complete"))
	}))
	t.Cleanup(srv.Close)
	c := &Client{HTTPClient: srv.Client(), BaseURL: srv.URL}
	dest := t.TempDir()
	if _, err := c.Download("test/model", "model.onnx", dest); err == nil {
		t.Fatal("expected interrupted transfer error")
	}
	if _, err := os.Stat(filepath.Join(dest, "model.onnx")); !os.IsNotExist(err) {
		t.Fatalf("partial final file exists: %v", err)
	}
	path, err := c.Download("test/model", "model.onnx", dest)
	if err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "complete" || requests != 2 {
		t.Fatalf("retry returned %q after %d HTTP requests", data, requests)
	}
	assertNoDownloadTemps(t, dest)
}

func TestDownloadCleansUpAfterCloseFailure(t *testing.T) {
	original := createDownloadTemp
	t.Cleanup(func() { createDownloadTemp = original })
	dest := t.TempDir()
	createDownloadTemp = func(dir, pattern string) (downloadTempFile, error) {
		f, err := os.CreateTemp(dir, pattern)
		if err != nil {
			return nil, err
		}
		return &failingCloseTemp{name: f.Name(), w: f}, nil
	}
	c := newTestClient(t, nil, map[string]string{"model.onnx": "weights"})
	if _, err := c.Download("test/model", "model.onnx", dest); err == nil {
		t.Fatal("expected close error")
	}
	if _, err := os.Stat(filepath.Join(dest, "model.onnx")); !os.IsNotExist(err) {
		t.Fatalf("final file exists after close failure: %v", err)
	}
	assertNoDownloadTemps(t, dest)
}

func TestDownloadCleansUpAfterPublishFailure(t *testing.T) {
	original := publishDownload
	t.Cleanup(func() { publishDownload = original })
	publishDownload = func(_, _ string) error { return errors.New("injected rename failure") }
	dest := t.TempDir()
	c := newTestClient(t, nil, map[string]string{"model.onnx": "weights"})
	if _, err := c.Download("test/model", "model.onnx", dest); err == nil {
		t.Fatal("expected publish error")
	}
	assertNoDownloadTemps(t, dest)
}

func TestConcurrentDownloadsPublishOneCompleteArtifact(t *testing.T) {
	c := newTestClient(t, nil, map[string]string{"model.onnx": "complete-weights"})
	dest := t.TempDir()
	start := make(chan struct{})
	errs := make(chan error, 8)
	for range 8 {
		go func() {
			<-start
			_, err := c.Download("test/model", "model.onnx", dest)
			errs <- err
		}()
	}
	close(start)
	for range 8 {
		if err := <-errs; err != nil {
			t.Fatal(err)
		}
	}
	data, err := os.ReadFile(filepath.Join(dest, "model.onnx"))
	if err != nil || string(data) != "complete-weights" {
		t.Fatalf("published artifact = %q, err=%v", data, err)
	}
	assertNoDownloadTemps(t, dest)
}

func assertNoDownloadTemps(t *testing.T, dir string) {
	t.Helper()
	matches, err := filepath.Glob(filepath.Join(dir, ".*.download-*"))
	if err != nil {
		t.Fatal(err)
	}
	if len(matches) != 0 {
		t.Fatalf("temporary downloads remain: %v", matches)
	}
}
