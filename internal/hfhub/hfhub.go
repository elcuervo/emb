package hfhub

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"sort"
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

// FindQuantizedONNX prefers pre-quantized weights in the order Xenova-style
// repos ship them: onnx/model_quantized.onnx → model_quantized.onnx →
// onnx/quantized/model.onnx.
func (c *Client) FindQuantizedONNX(files []FileInfo) *FileInfo {
	for _, name := range []string{"onnx/model_quantized.onnx", "model_quantized.onnx", "onnx/quantized/model.onnx"} {
		for _, f := range files {
			if f.Path == name {
				return &f
			}
		}
	}
	return nil
}

func (c *Client) Download(repo, filePath, destDir string) (string, error) {
	destPath := filepath.Join(destDir, filepath.Base(filePath))

	if _, err := os.Stat(destPath); err == nil {
		return destPath, nil
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

	f, err := os.Create(destPath)
	if err != nil {
		return "", fmt.Errorf("creating %s: %w", destPath, err)
	}
	defer func() { _ = f.Close() }()

	if _, err := io.Copy(f, resp.Body); err != nil {
		return "", fmt.Errorf("writing %s: %w", destPath, err)
	}

	return destPath, nil
}

// DownloadModel fetches an embedding model into destDir. When preferQuantized
// is true it downloads the pre-quantized weights when the repo ships them,
// falling back to fp32 otherwise.
func (c *Client) DownloadModel(repo, destDir string, preferQuantized bool) error {
	files, err := c.ListFiles(repo)
	if err != nil {
		return err
	}

	var onnxFile *FileInfo
	if preferQuantized {
		onnxFile = c.FindQuantizedONNX(files)
	}
	if onnxFile == nil {
		onnxFile = c.FindONNX(files)
	}
	if onnxFile == nil {
		return fmt.Errorf("no ONNX files in repo %q (use optimum-cli to export manually)", repo)
	}

	if err := os.MkdirAll(destDir, 0755); err != nil {
		return fmt.Errorf("creating directory %s: %w", destDir, err)
	}

	_, err = c.Download(repo, onnxFile.Path, destDir)
	if err != nil {
		return fmt.Errorf("downloading ONNX model: %w", err)
	}

	// Download tokenizer and config files (best-effort, non-fatal if missing)
	extraFiles := []string{"tokenizer.json", "config.json", "tokenizer_config.json", "special_tokens_map.json"}
	for _, f := range extraFiles {
		if _, downloadErr := c.Download(repo, f, destDir); downloadErr != nil {
			// Some models might not have all these files
			continue
		}
	}

	return nil
}
