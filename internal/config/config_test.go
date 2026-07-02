package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadDefaults(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "config.yaml")
	os.WriteFile(cfgPath, []byte(`
listen: ":9999"
models:
  test-model:
    onnx: ./model.onnx
    tokenizer: ./tokenizer.json
    pooling: mean
    normalize: false
    max_length: 512
    dim: 384
`), 0644)

	cfg, err := Load(cfgPath)
	if err != nil {
		t.Fatal(err)
	}

	if cfg.Listen != ":9999" {
		t.Fatalf("expected :9999, got %s", cfg.Listen)
	}
	m, ok := cfg.Models["test-model"]
	if !ok {
		t.Fatal("expected test-model")
	}
	if m.Pooling != "mean" {
		t.Fatalf("expected mean, got %s", m.Pooling)
	}
	if m.MaxLength != 512 {
		t.Fatalf("expected 512, got %d", m.MaxLength)
	}
	if m.Normalize {
		t.Fatal("expected normalize=false")
	}
	if m.Dim != 384 {
		t.Fatalf("expected 384, got %d", m.Dim)
	}
}

func TestLoadMinimalConfig(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "config.yaml")
	os.WriteFile(cfgPath, []byte(`
models:
  test:
    model_repo: some/repo
`), 0644)

	cfg, err := Load(cfgPath)
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := cfg.Models["test"]; !ok {
		t.Fatal("expected test model")
	}
}

func TestLoadInvalidDim(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "config.yaml")
	os.WriteFile(cfgPath, []byte(`
models:
  test:
    dim: 0
`), 0644)

	_, err := Load(cfgPath)
	if err == nil {
		t.Fatal("expected error for missing onnx")
	}
}

func TestLoadFileNotFound(t *testing.T) {
	_, err := Load("/nonexistent/path/config.yaml")
	if err == nil {
		t.Fatal("expected error for missing file")
	}
}

func TestValidate(t *testing.T) {
	dir := t.TempDir()
	onnxPath := filepath.Join(dir, "model.onnx")
	tokPath := filepath.Join(dir, "tokenizer.json")
	os.WriteFile(onnxPath, []byte("dummy"), 0644)
	os.WriteFile(tokPath, []byte("{}"), 0644)

	m := ModelConfig{ONNX: onnxPath, Tokenizer: tokPath}
	if err := m.Validate(); err != nil {
		t.Fatal(err)
	}
}

func TestValidateMissingFiles(t *testing.T) {
	m := ModelConfig{ONNX: "/nonexistent.onnx", Tokenizer: "./nonexistent.json"}
	if err := m.Validate(); err == nil {
		t.Fatal("expected error")
	}
}

func TestParseFlagsTLSCertOnly(t *testing.T) {
	_, err := ParseFlags([]string{
		"-model", "test", "-model-onnx", "./model.onnx",
		"-model-tokenizer", "./tok.json", "-model-dim", "128",
		"-tls-cert", "/etc/cert.pem",
	})
	if err == nil {
		t.Fatal("expected error: tls_cert without tls_key")
	}
}

func TestParseFlagsTLSKeyOnly(t *testing.T) {
	_, err := ParseFlags([]string{
		"-model", "test", "-model-onnx", "./model.onnx",
		"-model-tokenizer", "./tok.json", "-model-dim", "128",
		"-tls-key", "/etc/key.pem",
	})
	if err == nil {
		t.Fatal("expected error: tls_key without tls_cert")
	}
}

func TestLoadTLSBothSet(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "config.yaml")
	os.WriteFile(cfgPath, []byte(`
tls_cert: /etc/cert.pem
tls_key: /etc/key.pem
models:
  test:
    onnx: ./model.onnx
    tokenizer: ./tok.json
    dim: 128
`), 0644)

	cfg, err := Load(cfgPath)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.TLSCert != "/etc/cert.pem" {
		t.Fatalf("expected /etc/cert.pem, got %s", cfg.TLSCert)
	}
	if cfg.TLSKey != "/etc/key.pem" {
		t.Fatalf("expected /etc/key.pem, got %s", cfg.TLSKey)
	}
}

func TestLoadTLSCertOnly(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "config.yaml")
	os.WriteFile(cfgPath, []byte(`
tls_cert: /etc/cert.pem
models:
  test:
    onnx: ./model.onnx
    tokenizer: ./tok.json
    dim: 128
`), 0644)

	_, err := Load(cfgPath)
	if err == nil {
		t.Fatal("expected error: tls_cert without tls_key")
	}
}

func TestListenDefault(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "config.yaml")
	os.WriteFile(cfgPath, []byte(`
models:
  test:
    onnx: ./model.onnx
    tokenizer: ./tok.json
    dim: 128
`), 0644)

	cfg, err := Load(cfgPath)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Listen != ":6379" {
		t.Fatalf("expected :6379, got %s", cfg.Listen)
	}
}
