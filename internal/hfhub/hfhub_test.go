package hfhub

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
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
	cases := []struct {
		name  string
		files []string
		want  string // "" means nil
	}{
		{"model_quantized family beats int8", []string{"model_int8.onnx", "model_quantized.onnx"}, "model_quantized.onnx"},
		{"nested quantized beats nested int8", []string{"model_quantized.onnx", "onnx/model_quantized.onnx", "onnx/model_int8.onnx"}, "onnx/model_quantized.onnx"},
		{"quantized layout", []string{"onnx/quantized/model.onnx"}, "onnx/quantized/model.onnx"},
		{"int8 named at root", []string{"model.onnx", "model_int8.onnx"}, "model_int8.onnx"},
		{"int8 named nested", []string{"model.onnx", "onnx/model_int8.onnx"}, "onnx/model_int8.onnx"},
		{"int8 named outranks sibling fp32", []string{"model_fp32.onnx", "model_int8.onnx"}, "model_int8.onnx"},
		{"fp32 only is nil", []string{"model.onnx", "tokenizer.json"}, ""},
		{"laya-onnx shaped repo is nil", []string{"typed-decisions/model.onnx", "typed-decisions/tokenizer/tokenizer.json"}, ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			files := make([]FileInfo, 0, len(tc.files))
			for _, p := range tc.files {
				files = append(files, FileInfo{Path: p})
			}
			got := c.FindQuantizedONNX(files)
			if tc.want == "" {
				if got != nil {
					t.Fatalf("FindQuantizedONNX = %+v, want nil", got)
				}
				return
			}
			if got == nil || got.Path != tc.want {
				t.Fatalf("FindQuantizedONNX = %+v, want %s", got, tc.want)
			}
		})
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

func TestDownloadCreatesMissingDestDir(t *testing.T) {
	c := newTestClient(t, nil, map[string]string{"model.onnx": "weights"})
	// Nested under a temp dir that exists, so only Download can create the leaf.
	dest := filepath.Join(t.TempDir(), "int8", "clap")

	path, err := c.Download("test/model", "onnx/model.onnx", dest)
	if err != nil {
		t.Fatalf("download: %v", err)
	}
	if b, err := os.ReadFile(path); err != nil || string(b) != "weights" {
		t.Fatalf("contents = %q err=%v", b, err)
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
	if err := c.DownloadModel("test/model", "", dest, false); err != nil {
		t.Fatalf("download model: %v", err)
	}
	for _, f := range []string{"model.onnx", "tokenizer.json", "config.json", "preprocessor_config.json"} {
		if _, err := os.Stat(filepath.Join(dest, f)); err != nil {
			t.Fatalf("%s not written: %v", f, err)
		}
	}
}

func TestDownloadModelFetchesSplitExportWhole(t *testing.T) {
	c := newTestClient(t,
		[]string{"onnx/model.onnx", "onnx/model.onnx_data", "tokenizer.json"},
		map[string]string{"model.onnx": "graph-bytes", "model.onnx_data": "weight-bytes", "tokenizer.json": "{}"},
	)
	dest := t.TempDir()
	if err := c.DownloadModel("test/model", "", dest, false); err != nil {
		t.Fatalf("download model: %v", err)
	}
	for name, want := range map[string]string{"model.onnx": "graph-bytes", "model.onnx_data": "weight-bytes"} {
		b, err := os.ReadFile(filepath.Join(dest, name))
		if err != nil {
			t.Fatalf("%s not written: %v", name, err)
		}
		if string(b) != want {
			t.Fatalf("%s = %q, want %q", name, b, want)
		}
	}
}

func TestDownloadModelFetchesSubfolderSidecar(t *testing.T) {
	c := newTestClient(t,
		[]string{"typed-decisions/model.onnx", "typed-decisions/model.onnx_data", "typed-decisions/tokenizer.json"},
		map[string]string{"model.onnx": "g", "model.onnx_data": "w", "tokenizer.json": "{}"},
	)
	dest := t.TempDir()
	if err := c.DownloadModel("test/model", "typed-decisions", dest, false); err != nil {
		t.Fatalf("download model: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dest, "model.onnx_data")); err != nil {
		t.Fatalf("subfolder sidecar not written beside the graph: %v", err)
	}
}

func TestDownloadModelSelfContainedGraphFetchesNoSidecar(t *testing.T) {
	c := newTestClient(t,
		[]string{"model.onnx", "tokenizer.json"},
		map[string]string{"model.onnx": "g", "tokenizer.json": "{}"},
	)
	dest := t.TempDir()
	if err := c.DownloadModel("test/model", "", dest, false); err != nil {
		t.Fatalf("download model: %v", err)
	}
	entries, err := os.ReadDir(dest)
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range entries {
		if strings.HasSuffix(e.Name(), "_data") {
			t.Fatalf("unexpected sidecar %s for a self-contained graph", e.Name())
		}
	}
}

func TestDownloadModelSidecarFailureRemovesFreshGraph(t *testing.T) {
	// The sidecar is listed in the repository but 404s: the export must not be
	// left half-published, or the cached graph short-circuits every later
	// download and fails to load.
	c := newTestClient(t,
		[]string{"onnx/model.onnx", "onnx/model.onnx_data", "tokenizer.json"},
		map[string]string{"model.onnx": "graph-bytes", "tokenizer.json": "{}"},
	)
	dest := t.TempDir()
	err := c.DownloadModel("test/model", "", dest, false)
	if err == nil {
		t.Fatal("expected a sidecar failure")
	}
	if !strings.Contains(err.Error(), "model.onnx_data") {
		t.Fatalf("error should name the sidecar, got: %v", err)
	}
	if _, statErr := os.Stat(filepath.Join(dest, "model.onnx")); statErr == nil {
		t.Fatal("graph left behind after a sidecar failure")
	}
	entries, err := os.ReadDir(dest)
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range entries {
		if strings.Contains(e.Name(), "download-") {
			t.Fatalf("temp file left behind: %s", e.Name())
		}
	}
}

func TestDownloadExternalDataRepairsCachedGraph(t *testing.T) {
	c := newTestClient(t,
		[]string{"onnx/model.onnx", "onnx/model.onnx_data"},
		map[string]string{"model.onnx": "g", "model.onnx_data": "w"},
	)
	dest := t.TempDir()
	// A cache that holds the graph but not its weights, as an earlier version or
	// an interrupted transfer would leave it.
	if err := os.WriteFile(filepath.Join(dest, "model.onnx"), []byte("g"), 0o600); err != nil {
		t.Fatal(err)
	}
	found, err := c.DownloadExternalData("test/model", "", "model.onnx", dest)
	if err != nil {
		t.Fatalf("external data: %v", err)
	}
	if !found {
		t.Fatal("expected the repository's sidecar to be found")
	}
	if b, err := os.ReadFile(filepath.Join(dest, "model.onnx_data")); err != nil || string(b) != "w" {
		t.Fatalf("sidecar contents = %q err=%v", b, err)
	}
}

func TestDownloadExternalDataReportsInlineWeights(t *testing.T) {
	c := newTestClient(t, []string{"model.onnx"}, map[string]string{"model.onnx": "g"})
	found, err := c.DownloadExternalData("test/model", "", "model.onnx", t.TempDir())
	if err != nil {
		t.Fatalf("external data: %v", err)
	}
	if found {
		t.Fatal("a graph with inline weights must not report a sidecar")
	}
}

// TestLayaOnnxShapeResolvesTheSiteCheckpoint pins the artifact the preview site's
// `laya-real` mount resolves. codenamev/laya-onnx publishes one `model.onnx` per
// subfolder — no quantized member and no external data — so `quantize: auto`
// must not substitute another export, and the download must be
// typed-decisions/model.onnx. The second run proves the pin bites: add an
// int8 member and it is preferred, as it should be.
func TestLayaOnnxShapeResolvesTheSiteCheckpoint(t *testing.T) {
	siblings := []string{
		"english/model.onnx", "english/onnx_config.json", "english/rl_agent_config.json",
		"english/tokenizer/tokenizer.json", "english/tokenizer/tokenizer_config.json",
		"multilingual/model.onnx", "multilingual/onnx_config.json", "multilingual/rl_agent_config.json",
		"multilingual/tokenizer/tokenizer.json", "multilingual/tokenizer/tokenizer_config.json",
		"typed-decisions/model.onnx", "typed-decisions/onnx_config.json", "typed-decisions/rl_agent_config.json",
		"typed-decisions/tokenizer/tokenizer.json", "typed-decisions/tokenizer/tokenizer_config.json",
	}

	download := func(t *testing.T, files []string) []string {
		t.Helper()
		var requested []string
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Path == "/api/models/test/model" {
				infos := make([]FileInfo, 0, len(files))
				for _, f := range files {
					infos = append(infos, FileInfo{Path: f})
				}
				_ = json.NewEncoder(w).Encode(modelAPIResponse{Siblings: infos})
				return
			}
			requested = append(requested, r.URL.Path)
			_, _ = w.Write([]byte("bytes"))
		}))
		t.Cleanup(srv.Close)
		c := &Client{HTTPClient: srv.Client(), BaseURL: srv.URL}
		if err := c.DownloadModel("test/model", "typed-decisions", t.TempDir(), true); err != nil {
			t.Fatalf("download model: %v", err)
		}
		return requested
	}
	contains := func(paths []string, want string) bool {
		for _, p := range paths {
			if strings.Contains(p, want) {
				return true
			}
		}
		return false
	}

	got := download(t, siblings)
	if !contains(got, "/test/model/resolve/main/typed-decisions/model.onnx") {
		t.Fatalf("the site's checkpoint was not the resolved graph; requests were %v", got)
	}
	for _, p := range got {
		if strings.Contains(p, "int8") || strings.Contains(p, "_data") || strings.Contains(p, "multilingual/") {
			t.Errorf("unexpected request %q: nothing should be substituted for this shape", p)
		}
	}

	// The pin is real: give the repository an int8 member and it wins.
	withInt8 := append(append([]string{}, siblings...), "typed-decisions/model_int8.onnx")
	got = download(t, withInt8)
	if !contains(got, "/test/model/resolve/main/typed-decisions/model_int8.onnx") {
		t.Fatalf("an int8 member was not preferred, so this test would not catch a substitution; requests were %v", got)
	}
}

func TestDownloadModelPrefersQuantized(t *testing.T) {
	c := newTestClient(t,
		[]string{"model.onnx", "onnx/model_quantized.onnx", "tokenizer.json"},
		map[string]string{"model_quantized.onnx": "int8", "model.onnx": "fp32"},
	)
	dest := t.TempDir()
	if err := c.DownloadModel("test/model", "", dest, true); err != nil {
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
	if err := c.DownloadModel("test/model", "", t.TempDir(), false); err == nil {
		t.Fatal("expected an error when the repo has no ONNX files")
	}
}

// TestDownloadModelSubfolder resolves the ONNX and the nested tokenizer under a
// named subfolder and lands both at destDir under their conventional names. The
// server only serves full paths, so this exercises the nested tokenizer/
// fallback (multilingual/tokenizer.json is absent).
func TestDownloadModelSubfolder(t *testing.T) {
	files := map[string]string{
		"/test/model/resolve/main/multilingual/model.onnx":                      "multilingual-weights",
		"/test/model/resolve/main/multilingual/tokenizer/tokenizer.json":        "nested-tokenizer",
		"/test/model/resolve/main/multilingual/tokenizer/tokenizer_config.json": "{}",
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/models/test/model" {
			_ = json.NewEncoder(w).Encode(modelAPIResponse{Siblings: []FileInfo{
				{Path: "english/model.onnx"},
				{Path: "multilingual/model.onnx"},
				{Path: "multilingual/tokenizer/tokenizer.json"},
				{Path: "multilingual/tokenizer/tokenizer_config.json"},
			}})
			return
		}
		if body, ok := files[r.URL.Path]; ok {
			_, _ = w.Write([]byte(body))
			return
		}
		http.NotFound(w, r)
	}))
	t.Cleanup(srv.Close)
	c := &Client{HTTPClient: srv.Client(), BaseURL: srv.URL}

	dest := t.TempDir()
	if err := c.DownloadModel("test/model", "multilingual", dest, false); err != nil {
		t.Fatalf("download model: %v", err)
	}
	for name, want := range map[string]string{
		"model.onnx":            "multilingual-weights",
		"tokenizer.json":        "nested-tokenizer",
		"tokenizer_config.json": "{}",
	} {
		got, err := os.ReadFile(filepath.Join(dest, name))
		if err != nil || string(got) != want {
			t.Fatalf("%s = %q err=%v, want %q", name, got, err, want)
		}
	}
}

// TestDownloadModelSubfolderNoONNX fails with an error that names both the repo
// and the subfolder when the folder holds no ONNX file.
func TestDownloadModelSubfolderNoONNX(t *testing.T) {
	c := newTestClient(t, []string{"english/model.onnx", "multilingual/README.md"}, nil)
	err := c.DownloadModel("test/model", "multilingual", t.TempDir(), false)
	if err == nil {
		t.Fatal("expected an error when the subfolder has no ONNX files")
	}
	for _, want := range []string{"test/model", "multilingual"} {
		if !strings.Contains(err.Error(), want) {
			t.Fatalf("error %q does not name %q", err, want)
		}
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
