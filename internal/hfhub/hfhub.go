package hfhub

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

type FileInfo struct {
	Path string `json:"rfilename"`
	Size int64  `json:"size"`
}

type modelAPIResponse struct {
	Siblings []FileInfo `json:"siblings"`
}

type Client struct {
	HTTPClient *http.Client
	BaseURL    string
}

type downloadTempFile interface {
	io.Writer
	Close() error
	Name() string
}

var createDownloadTemp = func(dir, pattern string) (downloadTempFile, error) {
	return os.CreateTemp(dir, pattern)
}

var publishDownload = os.Rename

func New() *Client {
	return &Client{
		HTTPClient: http.DefaultClient,
		BaseURL:    "https://huggingface.co",
	}
}

func (c *Client) modelAPIURL(repo string) string {
	return fmt.Sprintf("%s/api/models/%s", c.BaseURL, repo)
}

func (c *Client) fileURL(repo, filePath string) string {
	return fmt.Sprintf("%s/%s/resolve/main/%s", c.BaseURL, repo, filePath)
}

func (c *Client) ListFiles(repo string) ([]FileInfo, error) {
	resp, err := c.HTTPClient.Get(c.modelAPIURL(repo))
	if err != nil {
		return nil, fmt.Errorf("fetching model info for %q: %w", repo, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("model %q: HTTP %d", repo, resp.StatusCode)
	}

	var data modelAPIResponse
	if err := json.NewDecoder(resp.Body).Decode(&data); err != nil {
		return nil, fmt.Errorf("decoding model info for %q: %w", repo, err)
	}
	return data.Siblings, nil
}

func (c *Client) FindONNX(files []FileInfo) *FileInfo {
	// Priority: model.onnx → onnx/model.onnx → first .onnx
	onnxFiles := make([]FileInfo, 0)
	for _, f := range files {
		if ext := filepath.Ext(f.Path); ext == ".onnx" {
			onnxFiles = append(onnxFiles, f)
		}
	}
	if len(onnxFiles) == 0 {
		return nil
	}

	for _, name := range []string{"model.onnx", "onnx/model.onnx"} {
		for _, f := range onnxFiles {
			if f.Path == name {
				return &f
			}
		}
	}

	sort.Slice(onnxFiles, func(i, j int) bool {
		return onnxFiles[i].Path < onnxFiles[j].Path
	})
	return &onnxFiles[0]
}

// QuantizedWeightNames are the pre-quantized artifact names, in resolution
// priority order: the Xenova-style model_quantized family first, then the
// int8-named exports the decision models publish. Both the Hub member selection
// (FindQuantizedONNX) and local-directory resolution (registry.resolveQuantize)
// read this list, so a repository's int8 member and a directory's int8 file
// cannot disagree about which names count as pre-quantized.
var QuantizedWeightNames = []string{
	"onnx/model_quantized.onnx",
	"model_quantized.onnx",
	"onnx/quantized/model.onnx",
	"model_int8.onnx",
	"onnx/model_int8.onnx",
}

// FindQuantizedONNX prefers pre-quantized weights in the order repos ship them:
// the model_quantized family first, then the int8-named decision-model exports.
func (c *Client) FindQuantizedONNX(files []FileInfo) *FileInfo {
	for _, name := range QuantizedWeightNames {
		for _, f := range files {
			if f.Path == name {
				return &f
			}
		}
	}
	return nil
}

// DownloadExternalData fetches the external-data member of a graph — the
// `<graph>_data` file ONNX Runtime reads beside it — and writes it into destDir
// under the base name the graph expects. graphBase is the graph's own file name,
// looked up at the subfolder root and under onnx/, which is where these
// repositories place it. It reports whether the repository publishes such a
// member: a graph that holds its weights inline has none, which is not an error,
// while a failed transfer is.
func (c *Client) DownloadExternalData(repo, subfolder, graphBase, destDir string) (bool, error) {
	if graphBase == "" || strings.HasSuffix(graphBase, "_data") {
		return false, nil
	}
	files, err := c.ListFiles(repo)
	if err != nil {
		return false, err
	}
	prefix := ""
	if s := strings.Trim(subfolder, "/"); s != "" {
		prefix = s + "/"
	}
	for _, name := range []string{prefix + graphBase + "_data", prefix + "onnx/" + graphBase + "_data"} {
		for _, f := range files {
			if f.Path != name {
				continue
			}
			if _, err := c.Download(repo, name, destDir); err != nil {
				return true, fmt.Errorf("downloading external data %s: %w", name, err)
			}
			return true, nil
		}
	}
	return false, nil
}

func (c *Client) Download(repo, filePath, destDir string) (string, error) {
	destPath := filepath.Join(destDir, filepath.Base(filePath))

	if _, err := os.Stat(destPath); err == nil {
		return destPath, nil
	}

	// The caller may name a destination that does not exist yet (a fresh volume,
	// or one directory shared by several files); Download is the chokepoint that
	// creates the temp file, so it owns making the directory.
	if err := os.MkdirAll(destDir, 0755); err != nil {
		return "", fmt.Errorf("creating directory %s: %w", destDir, err)
	}

	url := c.fileURL(repo, filePath)
	resp, err := c.HTTPClient.Get(url)
	if err != nil {
		return "", fmt.Errorf("downloading %s: %w", filePath, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("downloading %s: HTTP %d", filePath, resp.StatusCode)
	}

	f, err := createDownloadTemp(destDir, "."+filepath.Base(filePath)+".download-*")
	if err != nil {
		return "", fmt.Errorf("creating temporary download for %s: %w", destPath, err)
	}
	tempPath := f.Name()
	closed := false
	defer func() {
		if !closed {
			_ = f.Close()
		}
		_ = os.Remove(tempPath)
	}()

	if _, err := io.Copy(f, resp.Body); err != nil {
		return "", fmt.Errorf("writing %s: %w", destPath, err)
	}
	if err := f.Close(); err != nil {
		closed = true
		return "", fmt.Errorf("closing temporary download for %s: %w", destPath, err)
	}
	closed = true
	if err := publishDownload(tempPath, destPath); err != nil {
		return "", fmt.Errorf("publishing %s: %w", destPath, err)
	}

	return destPath, nil
}

// DownloadModel fetches an embedding model into destDir. subfolder, when set,
// names a folder within the repository that holds the checkpoint; the ONNX and
// supporting files are then resolved under it and written into destDir under
// their conventional (base) names. When preferQuantized is true it downloads
// the pre-quantized weights when the repo ships them, falling back to fp32
// otherwise.
func (c *Client) DownloadModel(repo, subfolder, destDir string, preferQuantized bool) error {
	files, err := c.ListFiles(repo)
	if err != nil {
		return err
	}

	prefix := ""
	if s := strings.Trim(subfolder, "/"); s != "" {
		prefix = s + "/"
	}
	// FindONNX/FindQuantizedONNX and the extra-file loop reason over the
	// conventional layout (model.onnx at the root, tokenizer.json beside it).
	// Treating a subfolder's files as if they were the root keeps those pickers
	// and the destination names unchanged.
	rel := files
	if prefix != "" {
		rel = make([]FileInfo, 0, len(files))
		for _, f := range files {
			if rest, ok := strings.CutPrefix(f.Path, prefix); ok {
				rel = append(rel, FileInfo{Path: rest, Size: f.Size})
			}
		}
	}

	var onnxFile *FileInfo
	if preferQuantized {
		onnxFile = c.FindQuantizedONNX(rel)
	}
	if onnxFile == nil {
		onnxFile = c.FindONNX(rel)
	}
	if onnxFile == nil {
		if prefix != "" {
			return fmt.Errorf("no ONNX files under %q in repo %q (use optimum-cli to export manually)", strings.TrimSuffix(prefix, "/"), repo)
		}
		return fmt.Errorf("no ONNX files in repo %q (use optimum-cli to export manually)", repo)
	}

	if err := os.MkdirAll(destDir, 0755); err != nil {
		return fmt.Errorf("creating directory %s: %w", destDir, err)
	}

	graphDest := filepath.Join(destDir, filepath.Base(onnxFile.Path))
	_, statErr := os.Stat(graphDest)
	graphPreexisting := statErr == nil

	if _, err := c.Download(repo, prefix+onnxFile.Path, destDir); err != nil {
		return fmt.Errorf("downloading ONNX model: %w", err)
	}

	// A split export keeps its weights in a `<graph>_data` member beside the
	// graph, and ONNX Runtime reads that file: the graph alone cannot load.
	if _, err := c.DownloadExternalData(repo, subfolder, filepath.Base(onnxFile.Path), destDir); err != nil {
		if !graphPreexisting {
			// Leave no half-published export behind. A cached graph would
			// short-circuit the next download and fail to load forever.
			_ = os.Remove(graphDest)
		}
		return fmt.Errorf("downloading ONNX external data: %w", err)
	}

	// Download tokenizer, config, and image-preprocessor files (best-effort,
	// non-fatal if missing). preprocessor_config.json carries the image
	// rescale/mean/std/crop/resample/size the image autoconfiguration reads. A
	// subfolder export may nest the tokenizer under tokenizer/, so try both
	// layouts; Download writes to the file's base name either way.
	for _, f := range ExtraModelFiles {
		candidates := []string{f}
		if prefix != "" {
			candidates = []string{prefix + f, prefix + "tokenizer/" + f}
		}
		for _, candidate := range candidates {
			if _, downloadErr := c.Download(repo, candidate, destDir); downloadErr == nil {
				break
			}
		}
	}

	return nil
}

// ExtraModelFiles are the non-weight files DownloadModel fetches alongside the
// ONNX graph (best-effort). Exported for tests.
var ExtraModelFiles = []string{
	"tokenizer.json",
	"config.json",
	"tokenizer_config.json",
	"special_tokens_map.json",
	"preprocessor_config.json",
}
