package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
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
	// Unset idle_timeout: nil, meaning the default TTL applies at server build.
	if cfg.IdleTimeout != nil {
		t.Fatalf("expected nil IdleTimeout, got %v", cfg.IdleTimeout)
	}
	if cfg.MaxConnections != 0 || cfg.MaxConcurrentRequests != 0 {
		t.Fatalf("expected zero caps, got %d/%d", cfg.MaxConnections, cfg.MaxConcurrentRequests)
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

func TestLoadConnectionKnobs(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "config.yaml")
	os.WriteFile(cfgPath, []byte(`
idle_timeout: 5m
max_connections: 10
max_concurrent_requests: 4
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
	if cfg.IdleTimeout == nil || time.Duration(*cfg.IdleTimeout) != 5*60*time.Second {
		t.Fatalf("expected 5m, got %v", cfg.IdleTimeout)
	}
	if cfg.MaxConnections != 10 {
		t.Fatalf("expected 10, got %d", cfg.MaxConnections)
	}
	if cfg.MaxConcurrentRequests != 4 {
		t.Fatalf("expected 4, got %d", cfg.MaxConcurrentRequests)
	}
}

func TestParseFlagsConnectionKnobs(t *testing.T) {
	fc, err := ParseFlags([]string{
		"-model", "test", "-model-onnx", "./model.onnx",
		"-model-tokenizer", "./tok.json", "-model-dim", "128",
		"-idle-timeout", "90s",
		"-max-connections", "7",
		"-max-concurrent-requests", "3",
	})
	if err != nil {
		t.Fatal(err)
	}
	if fc.IdleTimeout == nil || time.Duration(*fc.IdleTimeout) != 90*time.Second {
		t.Fatalf("expected 90s, got %v", fc.IdleTimeout)
	}
	if fc.MaxConnections != 7 {
		t.Fatalf("expected 7, got %d", fc.MaxConnections)
	}
	if fc.MaxConcurrentRequests != 3 {
		t.Fatalf("expected 3, got %d", fc.MaxConcurrentRequests)
	}
}

func TestParseFlagsIdleTimeoutInvalid(t *testing.T) {
	_, err := ParseFlags([]string{
		"-model", "test", "-model-onnx", "./model.onnx",
		"-model-tokenizer", "./tok.json", "-model-dim", "128",
		"-idle-timeout", "banana",
	})
	if err == nil {
		t.Fatal("expected error for invalid duration")
	}
}

func TestParseFlagsIdleTimeoutZeroDisables(t *testing.T) {
	fc, err := ParseFlags([]string{
		"-model", "test", "-model-onnx", "./model.onnx",
		"-model-tokenizer", "./tok.json", "-model-dim", "128",
		"-idle-timeout", "0",
	})
	if err != nil {
		t.Fatal(err)
	}
	if fc.IdleTimeout == nil || time.Duration(*fc.IdleTimeout) != 0 {
		t.Fatalf("expected explicit 0 (disabled), got %v", fc.IdleTimeout)
	}
}

func TestParseFlagsIdleTimeoutNegative(t *testing.T) {
	_, err := ParseFlags([]string{
		"-model", "test", "-model-onnx", "./model.onnx",
		"-model-tokenizer", "./tok.json", "-model-dim", "128",
		"-idle-timeout", "-5m",
	})
	if err == nil {
		t.Fatal("expected error for negative duration")
	}
}

func TestLoadIdleTimeoutZeroDisables(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "config.yaml")
	os.WriteFile(cfgPath, []byte(`
idle_timeout: 0
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
	if cfg.IdleTimeout == nil || time.Duration(*cfg.IdleTimeout) != 0 {
		t.Fatalf("expected explicit 0 (disabled), got %v", cfg.IdleTimeout)
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

func TestLoadIdleTimeoutNumericRejected(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "config.yaml")
	os.WriteFile(cfgPath, []byte(`
idle_timeout: 300
models:
  test:
    onnx: ./model.onnx
    tokenizer: ./tok.json
    dim: 128
`), 0644)

	if _, err := Load(cfgPath); err == nil {
		t.Fatal("expected error for unit-less numeric idle_timeout")
	}
}

func TestParseFlagsRequestSizeCaps(t *testing.T) {
	fc, err := ParseFlags([]string{
		"-model", "test", "-model-onnx", "./model.onnx",
		"-model-tokenizer", "./tok.json", "-model-dim", "128",
		"-max-texts", "1024",
		"-max-pairs", "512",
	})
	if err != nil {
		t.Fatal(err)
	}
	if fc.MaxTexts == nil || *fc.MaxTexts != 1024 {
		t.Fatalf("expected max_texts 1024, got %v", fc.MaxTexts)
	}
	if fc.MaxPairs == nil || *fc.MaxPairs != 512 {
		t.Fatalf("expected max_pairs 512, got %v", fc.MaxPairs)
	}
}

func TestParseFlagsRequestSizeCapsUnset(t *testing.T) {
	fc, err := ParseFlags([]string{"-model", "test", "-model-onnx", "./model.onnx"})
	if err != nil {
		t.Fatal(err)
	}
	if fc.MaxTexts != nil || fc.MaxPairs != nil {
		t.Fatalf("expected unset caps to stay nil (default applied by server), got %v/%v", fc.MaxTexts, fc.MaxPairs)
	}
}

func TestParseFlagsRequestSizeCapsNegative(t *testing.T) {
	if _, err := ParseFlags([]string{
		"-model", "test", "-model-onnx", "./model.onnx",
		"-max-texts", "-1",
	}); err == nil || !strings.Contains(err.Error(), "non-negative") {
		t.Fatalf("expected non-negative error for -max-texts -1, got %v", err)
	}
	if _, err := ParseFlags([]string{
		"-model", "test", "-model-onnx", "./model.onnx",
		"-max-pairs", "-5",
	}); err == nil || !strings.Contains(err.Error(), "non-negative") {
		t.Fatalf("expected non-negative error for -max-pairs -5, got %v", err)
	}
}

func TestLoadScriptedInferenceConfig(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "config.yaml")
	os.WriteFile(cfgPath, []byte(`
models:
  gliner:
    onnx: ./model.onnx
    tokenizer: ./tokenizer.json
    script_workers: 2
    script_preload: true
`), 0644)

	cfg, err := Load(cfgPath)
	if err != nil {
		t.Fatal(err)
	}
	m := cfg.Models["gliner"]
	if m.ScriptWorkers != 2 {
		t.Fatalf("script_workers = %d, want 2", m.ScriptWorkers)
	}
	if !m.ScriptPreload {
		t.Fatal("script_preload not parsed")
	}

	// A model without the keys keeps zero defaults (auto-tune, no preload).
	os.WriteFile(cfgPath, []byte(`
models:
  plain:
    onnx: ./model.onnx
`), 0644)
	cfg, err = Load(cfgPath)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Models["plain"].ScriptWorkers != 0 || cfg.Models["plain"].ScriptPreload {
		t.Fatal("defaults must be 0/false")
	}
}

func TestPersistenceConfigDefaultsAndValidation(t *testing.T) {
	defaults := Config{}
	if !defaults.CacheLoadEnabled() || !defaults.CacheShutdownSaveEnabled() {
		t.Fatal("cache load and shutdown save must default to enabled")
	}

	valid := Config{
		Cache: "auto", CacheFile: "/tmp/cache.embcache", CacheSave: "5m",
		CacheRestoreLimit: "50%", CacheRestoreReserve: "1GB", CacheSaveRateLimit: "100MB/s",
	}
	if err := valid.validatePersistence(); err != nil {
		t.Fatalf("valid persistence config rejected: %v", err)
	}
	// A file-only config is valid but dormant until a cache is also enabled;
	// this mirrors the runtime CONFIG SET cache_file path.
	fileOnly := Config{CacheFile: "/tmp/cache.embcache"}
	if err := fileOnly.validatePersistence(); err != nil {
		t.Fatalf("dormant file-only config rejected: %v", err)
	}
	if rate, err := valid.CacheSaveRateBytes(); err != nil || rate != 100_000_000 {
		t.Fatalf("rate = %d, %v", rate, err)
	}

	tests := []Config{
		{Cache: "1GB", CacheSave: "5m"},
		{Cache: "1GB", CacheFile: "/tmp/cache", CacheSave: "0"},
		{Cache: "1GB", CacheFile: "/tmp/cache", CacheRestoreLimit: "101%"},
		{Cache: "1GB", CacheFile: "/tmp/cache", CacheRestoreLimit: "NaN%"},
		{Cache: "1GB", CacheFile: "/tmp/cache", CacheRestoreReserve: "0%"},
		{Cache: "1GB", CacheFile: "/tmp/cache", CacheRestoreReserve: "NaN%"},
		{Cache: "1GB", CacheFile: "/tmp/cache", CacheSaveRateLimit: "fast"},
	}
	for i, cfg := range tests {
		if err := cfg.validatePersistence(); err == nil {
			t.Errorf("invalid persistence config %d was accepted: %#v", i, cfg)
		}
	}
}

func TestParseFlagsPersistence(t *testing.T) {
	fc, err := ParseFlags([]string{
		"-model", "test", "-model-onnx", "./model.onnx",
		"-cache", "512MB", "-cache-file", "/tmp/cache.embcache",
		"-cache-load", "false", "-cache-save", "30s",
		"-cache-save-on-shutdown", "false", "-cache-restore-limit", "256MB",
		"-cache-restore-reserve", "20%", "-cache-save-rate-limit", "10MB/s",
	})
	if err != nil {
		t.Fatal(err)
	}
	if fc.CacheFile != "/tmp/cache.embcache" || fc.CacheLoadEnabled() || fc.CacheShutdownSaveEnabled() {
		t.Fatalf("persistence flags not retained: %#v", fc.Config)
	}
	if fc.CacheSave != "30s" || fc.CacheRestoreLimit != "256MB" || fc.CacheRestoreReserve != "20%" {
		t.Fatalf("persistence controls not retained: %#v", fc.Config)
	}
}
