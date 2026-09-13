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

func TestLoadScriptsPathResolution(t *testing.T) {
	dir := t.TempDir()
	scriptsDir := filepath.Join(dir, "scripts")
	os.MkdirAll(scriptsDir, 0755)
	scriptPath := filepath.Join(scriptsDir, "classify.lua")
	os.WriteFile(scriptPath, []byte("return KEYS[1]"), 0644)

	cfgPath := filepath.Join(dir, "config.yaml")
	os.WriteFile(cfgPath, []byte(`
models:
  test:
    onnx: ./model.onnx
    scripts:
      - scripts/classify.lua
      - /absolute/script.lua
`), 0644)

	cfg, err := Load(cfgPath)
	if err != nil {
		t.Fatal(err)
	}
	m := cfg.Models["test"]
	if len(m.Scripts) != 2 {
		t.Fatalf("expected 2 scripts, got %d", len(m.Scripts))
	}
	wantRel := filepath.Join(dir, "scripts", "classify.lua")
	if m.Scripts[0] != wantRel {
		t.Fatalf("relative path: got %q, want %q", m.Scripts[0], wantRel)
	}
	if m.Scripts[1] != "/absolute/script.lua" {
		t.Fatalf("absolute path: got %q, want /absolute/script.lua", m.Scripts[1])
	}
}

func TestLoadScriptsMissingFile(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "config.yaml")
	os.WriteFile(cfgPath, []byte(`
models:
  test:
    onnx: ./model.onnx
    scripts:
      - scripts/nonexistent.lua
`), 0644)

	cfg, err := Load(cfgPath)
	if err != nil {
		t.Fatal(err)
	}
	// Load resolves paths but does NOT validate file existence; the server
	// (or caller) reads the file and reports the error at boot time.
	m := cfg.Models["test"]
	if len(m.Scripts) != 1 {
		t.Fatalf("expected 1 script, got %d", len(m.Scripts))
	}
	want := filepath.Join(dir, "scripts", "nonexistent.lua")
	if m.Scripts[0] != want {
		t.Fatalf("resolved path: got %q, want %q", m.Scripts[0], want)
	}
}

func TestLoadImageConfig(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "config.yaml")
	os.WriteFile(cfgPath, []byte(`
max_images: 128
max_command_bytes: 1048576
max_image_bytes: 524288
max_image_pixels: 4194304
models:
  clip:
    onnx: ./model.onnx
    image:
      input: pixel_values
      size: 224
      crop: center
      resample: bicubic
      rescale: 0.00392156862745098
      mean: [0.48145466, 0.4578275, 0.40821073]
      std: [0.26862954, 0.26130258, 0.27577711]
`), 0644)

	cfg, err := Load(cfgPath)
	if err != nil {
		t.Fatal(err)
	}
	img := cfg.Models["clip"].Image
	if img == nil {
		t.Fatal("image block not parsed")
	}
	if img.Input != "pixel_values" || img.Size != 224 || img.Crop != "center" || img.Resample != "bicubic" {
		t.Fatalf("unexpected image block: %#v", *img)
	}
	if img.Rescale == nil || *img.Rescale != 0.00392156862745098 {
		t.Fatalf("rescale = %v", img.Rescale)
	}
	if len(img.Mean) != 3 || len(img.Std) != 3 {
		t.Fatalf("mean/std = %v/%v", img.Mean, img.Std)
	}
	if cfg.MaxImages == nil || *cfg.MaxImages != 128 {
		t.Fatalf("max_images = %v", cfg.MaxImages)
	}
	if cfg.EffectiveMaxCommandBytes() != 1048576 || cfg.EffectiveMaxImageBytes() != 524288 || cfg.EffectiveMaxImagePixels() != 4194304 {
		t.Fatalf("limits not applied: %d/%d/%d", cfg.EffectiveMaxCommandBytes(), cfg.EffectiveMaxImageBytes(), cfg.EffectiveMaxImagePixels())
	}
}

func TestEffectiveImageLimitsDefaults(t *testing.T) {
	cfg := Config{}
	if cfg.EffectiveMaxCommandBytes() != DefaultMaxCommandBytes {
		t.Fatalf("default command cap = %d", cfg.EffectiveMaxCommandBytes())
	}
	if cfg.EffectiveMaxImageBytes() != DefaultMaxImageBytes {
		t.Fatalf("default image byte cap = %d", cfg.EffectiveMaxImageBytes())
	}
	if cfg.EffectiveMaxImagePixels() != DefaultMaxImagePixels {
		t.Fatalf("default pixel cap = %d", cfg.EffectiveMaxImagePixels())
	}
	// Explicit 0 disables each bound.
	zero := int64(0)
	disabled := Config{MaxCommandBytes: &zero, MaxImageBytes: &zero, MaxImagePixels: &zero}
	if disabled.EffectiveMaxCommandBytes() != 0 || disabled.EffectiveMaxImageBytes() != 0 || disabled.EffectiveMaxImagePixels() != 0 {
		t.Fatal("explicit 0 must disable the caps")
	}
}

func TestLoadImageConfigValidation(t *testing.T) {
	cases := []struct {
		name    string
		image   string
		wantErr string
	}{
		{"mean without std", "image:\n      mean: [0.5, 0.5, 0.5]\n", "set together"},
		{"std without mean", "image:\n      std: [0.5, 0.5, 0.5]\n", "set together"},
		{"mean length", "image:\n      mean: [0.5, 0.5]\n      std: [0.5, 0.5]\n", "3 channels"},
		{"std length", "image:\n      mean: [0.5, 0.5, 0.5]\n      std: [0.5]\n", "3 channels"},
		{"bad crop", "image:\n      crop: middle\n", "crop"},
		{"bad resample", "image:\n      resample: cubic\n", "resample"},
		{"negative size", "image:\n      size: -1\n", "size"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			cfgPath := filepath.Join(dir, "config.yaml")
			os.WriteFile(cfgPath, []byte("models:\n  m:\n    onnx: ./m.onnx\n    "+tc.image), 0644)
			_, err := Load(cfgPath)
			if err == nil || !strings.Contains(err.Error(), tc.wantErr) {
				t.Fatalf("want error containing %q, got %v", tc.wantErr, err)
			}
		})
	}
}

func TestLoadImageLimitsNegative(t *testing.T) {
	for _, key := range []string{"max_images", "max_command_bytes", "max_image_bytes", "max_image_pixels"} {
		dir := t.TempDir()
		cfgPath := filepath.Join(dir, "config.yaml")
		os.WriteFile(cfgPath, []byte(key+": -1\nmodels:\n  m:\n    onnx: ./m.onnx\n"), 0644)
		if _, err := Load(cfgPath); err == nil || !strings.Contains(err.Error(), "non-negative") {
			t.Fatalf("%s: want non-negative error, got %v", key, err)
		}
	}
}

func TestRepoExampleConfigLoads(t *testing.T) {
	// The documented example config must stay parseable (it is shipped and
	// referenced by the README); registry load-time validation is separate.
	cfg, err := Load("../../config.yaml")
	if err != nil {
		t.Fatalf("repo config.yaml failed to load: %v", err)
	}
	if len(cfg.Models) == 0 {
		t.Fatal("repo config.yaml has no models")
	}
}

func TestLoadReservedModelNames(t *testing.T) {
	for _, name := range []string{"BLOB", "blob", "VALUES", "values"} {
		dir := t.TempDir()
		cfgPath := filepath.Join(dir, "config.yaml")
		os.WriteFile(cfgPath, []byte("models:\n  "+name+":\n    onnx: ./m.onnx\n"), 0644)
		_, err := Load(cfgPath)
		if err == nil {
			t.Fatalf("expected reserved-name error for %q", name)
		}
		if !strings.Contains(err.Error(), "reserved") {
			t.Fatalf("expected reserved-word error for %q, got %v", name, err)
		}
	}
}
