package config

import (
	"flag"
	"fmt"
	"io"
	"math"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/docker/go-units"
	"gopkg.in/yaml.v3"
)

type Config struct {
	Listen              string `yaml:"listen"`
	Password            string `yaml:"password"`
	TLSCert             string `yaml:"tls_cert"`
	TLSKey              string `yaml:"tls_key"`
	Cache               string `yaml:"cache"`
	CacheFile           string `yaml:"cache_file"`
	CacheLoad           *bool  `yaml:"cache_load"`
	CacheSave           string `yaml:"cache_save"`
	CacheSaveOnShutdown *bool  `yaml:"cache_save_on_shutdown"`
	CacheRestoreLimit   string `yaml:"cache_restore_limit"`
	CacheRestoreReserve string `yaml:"cache_restore_reserve"`
	CacheSaveRateLimit  string `yaml:"cache_save_rate_limit"`
	// IdleTimeout closes connections that have not sent a command for the
	// duration. nil (default) applies DefaultIdleTimeout; an explicit 0
	// disables reaping entirely (strict Redis semantics).
	IdleTimeout *Duration `yaml:"idle_timeout"`
	// MaxConnections refuses new connections beyond the cap. 0 (default) is
	// unlimited.
	MaxConnections int `yaml:"max_connections"`
	// MaxConcurrentRequests answers EMB/EMB.MULTI with a busy error beyond the
	// cap. 0 (default) is unlimited.
	MaxConcurrentRequests int `yaml:"max_concurrent_requests"`
	// MaxTexts bounds texts per EMB command; commands beyond the cap are
	// truncated (overflow reply slots null). nil = default 4096; 0 = unlimited.
	MaxTexts *int `yaml:"max_texts"`
	// MaxPairs bounds pairs per EMB.MULTI command; commands beyond the cap are
	// truncated (overflow reply slots null). nil = default 4096; 0 = unlimited.
	MaxPairs *int `yaml:"max_pairs"`
	// MaxImages bounds images per EMB.IMG/EMB.IMGMULTI command; commands beyond
	// the cap are truncated (overflow reply slots null). nil = default 4096;
	// 0 = unlimited.
	MaxImages *int `yaml:"max_images"`
	// MaxCommandBytes bounds the buffered bytes of a single command (including
	// binary image payloads); an oversized command is refused before decode or
	// inference. nil = DefaultMaxCommandBytes; 0 = unlimited.
	MaxCommandBytes *int64 `yaml:"max_command_bytes"`
	// MaxImageBytes bounds the byte size of each image argument.
	// nil = DefaultMaxImageBytes; 0 = unlimited.
	MaxImageBytes *int64 `yaml:"max_image_bytes"`
	// MaxImagePixels bounds the decoded pixel count of each image (checked from
	// the header before full decode). nil = DefaultMaxImagePixels; 0 = unlimited.
	MaxImagePixels *int64                 `yaml:"max_image_pixels"`
	Models         map[string]ModelConfig `yaml:"models"`
}

// Default payload caps applied when the corresponding top-level key is unset.
// They are generous enough for real images but bounded so a single command
// cannot pin unbounded memory.
const (
	DefaultMaxCommandBytes = 64 << 20 // 64 MiB buffered per command
	DefaultMaxImageBytes   = 32 << 20 // 32 MiB per image argument
	DefaultMaxImagePixels  = 32 << 20 // 33.5 MP decoded before the full decode
)

// ImageConfig declares a model's server-side image preprocessing. A model
// without a block does not accept EMB.IMG. Fields left at their zero value are
// auto-detected from the ONNX graph and preprocessor_config.json where
// possible; an undetectable required field fails model loading.
type ImageConfig struct {
	// Input is the ONNX input tensor that receives the pixel tensor (for
	// example "pixel_values"). Empty auto-detects the 4D image input.
	Input string `yaml:"input"`
	// Size is the target square edge length. 0 auto-detects from the graph or
	// preprocessor config.
	Size int `yaml:"size"`
	// Crop is "none" (default) or "center".
	Crop string `yaml:"crop"`
	// Resample is "bicubic" (default), "bilinear", "nearest", or "lanczos".
	Resample string `yaml:"resample"`
	// Rescale multiplies each pixel value in [0, 255] after resize
	// (typically 1/255). nil auto-detects (default 1/255 when undetectable).
	Rescale *float64 `yaml:"rescale"`
	// Mean is the per-channel (RGB) normalization mean.
	Mean []float64 `yaml:"mean"`
	// Std is the per-channel (RGB) normalization standard deviation.
	Std []float64 `yaml:"std"`
	// Output is the ONNX output tensor the image embedding is read from. Empty
	// uses the model's output_tensor (correct for a fused export whose text and
	// image branches share one output tensor).
	Output string `yaml:"output"`
}

// validateImageConfig rejects structurally invalid image blocks at config
// load. Preprocessor defaults and graph detection happen later in the registry,
// so absent fields are legal here; only inconsistent or unknown values fail.
func validateImageConfig(name string, img ImageConfig) error {
	switch img.Crop {
	case "", "none", "center":
	default:
		return fmt.Errorf("model %q: image.crop must be \"none\", \"center\", or unset, got %q", name, img.Crop)
	}
	switch img.Resample {
	case "", "nearest", "bilinear", "bicubic", "lanczos":
	default:
		return fmt.Errorf("model %q: image.resample must be one of nearest|bilinear|bicubic|lanczos or unset, got %q", name, img.Resample)
	}
	if img.Size < 0 {
		return fmt.Errorf("model %q: image.size must be non-negative", name)
	}
	if img.Rescale != nil && *img.Rescale < 0 {
		return fmt.Errorf("model %q: image.rescale must be non-negative", name)
	}
	// mean and std travel together: one without the other is an incomplete
	// (partial) image block rather than an auto-detectable omission.
	hasMean, hasStd := img.Mean != nil, img.Std != nil
	if hasMean != hasStd {
		return fmt.Errorf("model %q: image.mean and image.std must be set together", name)
	}
	if hasMean && len(img.Mean) != 3 {
		return fmt.Errorf("model %q: image.mean must have 3 channels, got %d", name, len(img.Mean))
	}
	if hasStd && len(img.Std) != 3 {
		return fmt.Errorf("model %q: image.std must have 3 channels, got %d", name, len(img.Std))
	}
	return nil
}

// DefaultIdleTimeout is the idle-connection TTL applied when idle_timeout is
// unset: generous enough not to surprise pooled clients, bounded enough to
// reap zombie sockets and bound file descriptors.
const DefaultIdleTimeout = 15 * time.Minute

// Duration is time.Duration with lenient YAML decoding: string scalars parse
// with time.ParseDuration ("5m", "90s"), and an integer 0 is accepted as
// "disabled". Other numeric scalars are rejected with a hint, since a bare
// number's unit is ambiguous.
type Duration time.Duration

func (d *Duration) UnmarshalYAML(node *yaml.Node) error {
	switch node.Tag {
	case "!!str":
		v, err := time.ParseDuration(node.Value)
		if err != nil {
			return fmt.Errorf("invalid duration %q: %w", node.Value, err)
		}
		*d = Duration(v)
		return nil
	case "!!int":
		n, err := strconv.ParseInt(node.Value, 10, 64)
		if err != nil {
			return fmt.Errorf("invalid duration %q: %w", node.Value, err)
		}
		if n != 0 {
			return fmt.Errorf("invalid duration %q: numeric values must be 0; use a duration string like \"5m\"", node.Value)
		}
		*d = 0
		return nil
	default:
		return fmt.Errorf("invalid duration %q: expected a duration string like \"5m\" or 0", node.Value)
	}
}

type BatchingConfig struct {
	// Timeout bounds the batching window. nil = enabled with a 1ms default (so
	// any model gets the performance path); 0 = disabled (worker pool); >0 =
	// the window in milliseconds.
	Timeout *int `yaml:"timeout"`
	// MaxBatch bounds a window by request count (default 32).
	MaxBatch int `yaml:"max_batch"`
	// MaxBatchTokens bounds a window by accumulated real tokens (mirrors TEI's
	// max-batch-tokens). nil = default 16384 when batching is enabled; 0 =
	// disable token accounting (count-only behavior); >0 = the token budget.
	MaxBatchTokens *int `yaml:"max_batch_tokens"`
}

type ModelConfig struct {
	ONNX         string `yaml:"onnx"`
	Tokenizer    string `yaml:"tokenizer"`
	ModelRepo    string `yaml:"model_repo"`
	Pooling      string `yaml:"pooling"`
	Normalize    bool   `yaml:"normalize"`
	MaxLength    int    `yaml:"max_length"`
	Dim          int    `yaml:"dim"`
	Preload      bool   `yaml:"preload"`
	Workers      int    `yaml:"workers"`
	OutputTensor string `yaml:"output_tensor"`
	PadOutput    bool   `yaml:"pad_output"`
	// Quantize selects weight precision: auto (prefer pre-quantized ONNX when
	// present), on (require quantized, fail otherwise), off (fp32 always).
	Quantize        string         `yaml:"quantize"`
	TokenizeWorkers *int           `yaml:"tokenize_workers"`
	Batching        BatchingConfig `yaml:"batching"`
	IntraOpThreads  int            `yaml:"intra_op_threads"`
	InterOpThreads  int            `yaml:"inter_op_threads"`
	// ExecutionMode selects the ORT execution mode: "sequential" (default; the
	// right choice for mostly-serial encoder graphs) or "parallel". Empty
	// resolves to sequential.
	ExecutionMode string `yaml:"execution_mode"`
	// ScriptWorkers bounds parallel scripted-model sessions. 0/absent
	// auto-tunes from RAM and model size (min 1), mirroring Workers for the
	// embedding pool; scripted evaluations distribute round-robin across the
	// sessions.
	ScriptWorkers int `yaml:"script_workers"`
	// ScriptPreload warms the scripted session + tokenizer at load time
	// instead of on the first script evaluation.
	ScriptPreload bool `yaml:"script_preload"`
	// Scripts is a list of file paths to Lua scripts that the server SHALL
	// preload at boot. Relative paths are resolved against the config file's
	// directory; absolute paths are used as-is.
	Scripts []string `yaml:"scripts"`
	// Image opts the model into server-side image embedding (EMB.IMG); nil means
	// the model is text-only and rejects image commands.
	Image *ImageConfig `yaml:"image"`
	// ImagePreload warms the image named-session pool at load time instead of on
	// the first EMB.IMG request.
	ImagePreload bool `yaml:"image_preload"`
}

func Load(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("reading config: %w", err)
	}

	var cfg Config
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("parsing config: %w", err)
	}

	if cfg.Listen == "" {
		cfg.Listen = ":6379"
	}

	for name, m := range cfg.Models {
		if strings.EqualFold(name, "blob") || strings.EqualFold(name, "values") {
			return nil, fmt.Errorf("model %q: %q is a reserved word (reserved for the EMB reply-format keyword)", name, name)
		}
		if m.ExecutionMode != "" && m.ExecutionMode != "sequential" && m.ExecutionMode != "parallel" {
			return nil, fmt.Errorf("model %q: execution_mode must be \"sequential\", \"parallel\", or unset, got %q", name, m.ExecutionMode)
		}
		if m.Image != nil {
			if err := validateImageConfig(name, *m.Image); err != nil {
				return nil, err
			}
		}
	}

	configDir := filepath.Dir(path)
	for name, m := range cfg.Models {
		for i, sp := range m.Scripts {
			if !filepath.IsAbs(sp) {
				m.Scripts[i] = filepath.Join(configDir, sp)
			}
		}
		cfg.Models[name] = m
	}

	if err := cfg.validate(); err != nil {
		return nil, err
	}

	return &cfg, nil
}

// validate applies the checks shared by the YAML loader and the CLI parser:
// the TLS pair, per-model onnx-or-repo, persistence, request-size caps, and the
// image/command limits. Path-specific checks (reserved model names, execution
// mode, image config shape in Load; the has-config/has-model requirement in
// ParseFlags) stay with their caller.
func (c *Config) validate() error {
	if (c.TLSCert == "") != (c.TLSKey == "") {
		return fmt.Errorf("both tls_cert and tls_key must be set together")
	}
	for name, m := range c.Models {
		if m.ModelRepo == "" && m.ONNX == "" {
			return fmt.Errorf("model %q: onnx path or model_repo is required", name)
		}
	}
	if err := c.validatePersistence(); err != nil {
		return err
	}
	if c.MaxTexts != nil && *c.MaxTexts < 0 {
		return fmt.Errorf("max_texts must be non-negative")
	}
	if c.MaxPairs != nil && *c.MaxPairs < 0 {
		return fmt.Errorf("max_pairs must be non-negative")
	}
	return c.validateImageLimits()
}

// validateImageLimits rejects negative top-level image/command limits.
func (c Config) validateImageLimits() error {
	if c.MaxImages != nil && *c.MaxImages < 0 {
		return fmt.Errorf("max_images must be non-negative")
	}
	if c.MaxCommandBytes != nil && *c.MaxCommandBytes < 0 {
		return fmt.Errorf("max_command_bytes must be non-negative")
	}
	if c.MaxImageBytes != nil && *c.MaxImageBytes < 0 {
		return fmt.Errorf("max_image_bytes must be non-negative")
	}
	if c.MaxImagePixels != nil && *c.MaxImagePixels < 0 {
		return fmt.Errorf("max_image_pixels must be non-negative")
	}
	return nil
}

// EffectiveMaxCommandBytes resolves the command-size bound: nil keeps the
// default guard, an explicit 0 disables the bound, and a positive value is the
// configured cap.
func (c Config) EffectiveMaxCommandBytes() int64 {
	if c.MaxCommandBytes == nil {
		return DefaultMaxCommandBytes
	}
	return *c.MaxCommandBytes
}

// EffectiveMaxImageBytes resolves the per-image byte cap like
// EffectiveMaxCommandBytes.
func (c Config) EffectiveMaxImageBytes() int64 {
	if c.MaxImageBytes == nil {
		return DefaultMaxImageBytes
	}
	return *c.MaxImageBytes
}

// EffectiveMaxImagePixels resolves the decoded-pixel cap like
// EffectiveMaxCommandBytes.
func (c Config) EffectiveMaxImagePixels() int64 {
	if c.MaxImagePixels == nil {
		return DefaultMaxImagePixels
	}
	return *c.MaxImagePixels
}

func boolValue(v *bool, def bool) bool {
	if v == nil {
		return def
	}
	return *v
}

func (c Config) CacheLoadEnabled() bool         { return boolValue(c.CacheLoad, true) }
func (c Config) CacheShutdownSaveEnabled() bool { return boolValue(c.CacheSaveOnShutdown, true) }

func validateMemorySetting(name, value string, allowAuto bool) error {
	value = strings.TrimSpace(value)
	if value == "" || (allowAuto && strings.EqualFold(value, "auto")) {
		return nil
	}
	if strings.HasSuffix(value, "%") {
		pct, err := strconv.ParseFloat(strings.TrimSuffix(value, "%"), 64)
		if err != nil || math.IsNaN(pct) || pct <= 0 || pct > 100 {
			return fmt.Errorf("%s must be a percentage greater than 0 and at most 100", name)
		}
		return nil
	}
	n, err := units.FromHumanSize(value)
	if err != nil || n <= 0 {
		if allowAuto {
			return fmt.Errorf("%s must be auto, a positive size, or percentage", name)
		}
		return fmt.Errorf("%s must be a positive size or percentage", name)
	}
	return nil
}

func parseRate(value string) (int64, error) {
	v := strings.TrimSpace(value)
	if v == "" || v == "0" {
		return 0, nil
	}
	v = strings.TrimSuffix(strings.TrimSuffix(v, "/s"), "ps")
	n, err := units.FromHumanSize(v)
	if err != nil || n <= 0 {
		return 0, fmt.Errorf("cache_save_rate_limit must be 0 or a positive byte rate such as 100MB/s")
	}
	return n, nil
}

func (c Config) CacheSaveRateBytes() (int64, error) { return parseRate(c.CacheSaveRateLimit) }

func (c Config) validatePersistence() error {
	// A cache_file without a cache is valid but dormant: Server.New keeps it
	// stored until a cache is also active, matching the runtime CONFIG SET
	// path. cache_save still requires a destination file.
	if c.CacheSave != "" {
		if c.CacheFile == "" {
			return fmt.Errorf("cache_save requires cache_file")
		}
		d, err := time.ParseDuration(c.CacheSave)
		if err != nil || d <= 0 {
			return fmt.Errorf("cache_save must be a positive duration")
		}
	}
	if err := validateMemorySetting("cache_restore_limit", c.CacheRestoreLimit, true); err != nil {
		return err
	}
	if err := validateMemorySetting("cache_restore_reserve", c.CacheRestoreReserve, false); err != nil {
		return err
	}
	_, err := c.CacheSaveRateBytes()
	return err
}

type FlagConfig struct {
	Config
	OrtLib string
}

// lenientInt is an int flag that ignores parse errors, preserving the
// historical CLI behavior where a malformed numeric flag became 0 rather than a
// fatal error.
type lenientInt struct{ dst *int }

func (l lenientInt) String() string {
	if l.dst == nil {
		return "0"
	}
	return strconv.Itoa(*l.dst)
}

func (l lenientInt) Set(s string) error {
	n, _ := strconv.Atoi(s)
	*l.dst = n
	return nil
}

// ParseFlags parses the emb CLI. Model options are order-dependent: `-model
// <name>` opens a section, the following `-model-*` flags attach to it, and a
// `-model-*` flag with no preceding `-model` attaches to an implicit "model".
// The standard flag package accepts both -flag and --flag forms.
func ParseFlags(args []string) (*FlagConfig, error) {
	fc := &FlagConfig{
		Config: Config{
			Listen: ":6379",
			Models: make(map[string]ModelConfig),
		},
	}
	fs := flag.NewFlagSet("emb", flag.ContinueOnError)
	fs.SetOutput(io.Discard)

	var (
		currentModel string
		hasModel     bool
		hasConfig    bool
		showVersion  bool
	)

	withModel := func(f func(*ModelConfig)) {
		if currentModel == "" {
			currentModel = "model"
			fc.Models[currentModel] = ModelConfig{}
		}
		m := fc.Models[currentModel]
		f(&m)
		fc.Models[currentModel] = m
		hasModel = true
	}
	modelString := func(name string, set func(*ModelConfig, string)) {
		fs.Func(name, "", func(s string) error {
			withModel(func(m *ModelConfig) { set(m, s) })
			return nil
		})
	}

	fs.StringVar(&fc.Listen, "listen", fc.Listen, "")
	fs.Func("config", "", func(s string) error {
		cfg, err := Load(s)
		if err != nil {
			return fmt.Errorf("loading config: %w", err)
		}
		hasConfig = true
		hasModel = true
		fc.Config = *cfg
		if fc.Models == nil {
			fc.Models = make(map[string]ModelConfig)
		}
		return nil
	})
	fs.StringVar(&fc.Password, "password", "", "")
	fs.StringVar(&fc.Cache, "cache", "", "")
	fs.StringVar(&fc.CacheFile, "cache-file", "", "")
	fs.Func("cache-load", "", func(s string) error {
		v, err := strconv.ParseBool(s)
		if err != nil {
			return fmt.Errorf("parsing -cache-load: %w", err)
		}
		fc.CacheLoad = &v
		return nil
	})
	fs.StringVar(&fc.CacheSave, "cache-save", "", "")
	fs.Func("cache-save-on-shutdown", "", func(s string) error {
		v, err := strconv.ParseBool(s)
		if err != nil {
			return fmt.Errorf("parsing -cache-save-on-shutdown: %w", err)
		}
		fc.CacheSaveOnShutdown = &v
		return nil
	})
	fs.StringVar(&fc.CacheRestoreLimit, "cache-restore-limit", "", "")
	fs.StringVar(&fc.CacheRestoreReserve, "cache-restore-reserve", "", "")
	fs.StringVar(&fc.CacheSaveRateLimit, "cache-save-rate-limit", "", "")
	fs.Func("idle-timeout", "", func(s string) error {
		d, err := time.ParseDuration(s)
		if err != nil {
			return fmt.Errorf("parsing -idle-timeout: %w", err)
		}
		if d < 0 {
			return fmt.Errorf("-idle-timeout must not be negative")
		}
		dur := Duration(d)
		fc.IdleTimeout = &dur
		return nil
	})
	fs.Var(lenientInt{&fc.MaxConnections}, "max-connections", "")
	fs.Var(lenientInt{&fc.MaxConcurrentRequests}, "max-concurrent-requests", "")
	fs.Func("max-texts", "", func(s string) error {
		v, _ := strconv.Atoi(s)
		fc.MaxTexts = &v
		return nil
	})
	fs.Func("max-pairs", "", func(s string) error {
		v, _ := strconv.Atoi(s)
		fc.MaxPairs = &v
		return nil
	})
	fs.StringVar(&fc.TLSCert, "tls-cert", "", "")
	fs.StringVar(&fc.TLSKey, "tls-key", "", "")
	fs.StringVar(&fc.OrtLib, "ort-lib", "", "")
	fs.BoolVar(&showVersion, "version", false, "")

	fs.Func("model", "", func(name string) error {
		currentModel = name
		fc.Models[name] = ModelConfig{}
		hasModel = true
		return nil
	})
	modelString("model-onnx", func(m *ModelConfig, s string) { m.ONNX = s })
	modelString("model-repo", func(m *ModelConfig, s string) { m.ModelRepo = s })
	modelString("model-tokenizer", func(m *ModelConfig, s string) { m.Tokenizer = s })
	modelString("model-pooling", func(m *ModelConfig, s string) { m.Pooling = s })
	modelString("model-output-tensor", func(m *ModelConfig, s string) { m.OutputTensor = s })
	modelString("model-quantize", func(m *ModelConfig, s string) { m.Quantize = s })
	fs.BoolFunc("model-normalize", "", func(string) error {
		withModel(func(m *ModelConfig) { m.Normalize = true })
		return nil
	})
	fs.BoolFunc("model-pad-output", "", func(string) error {
		withModel(func(m *ModelConfig) { m.PadOutput = true })
		return nil
	})
	fs.Func("model-dim", "", func(s string) error {
		withModel(func(m *ModelConfig) { m.Dim, _ = strconv.Atoi(s) })
		return nil
	})
	fs.Func("model-max-length", "", func(s string) error {
		withModel(func(m *ModelConfig) { m.MaxLength, _ = strconv.Atoi(s) })
		return nil
	})
	fs.Func("model-workers", "", func(s string) error {
		withModel(func(m *ModelConfig) { m.Workers, _ = strconv.Atoi(s) })
		return nil
	})
	fs.Func("model-tokenize-workers", "", func(s string) error {
		withModel(func(m *ModelConfig) {
			v, _ := strconv.Atoi(s)
			m.TokenizeWorkers = &v
		})
		return nil
	})
	fs.Func("model-intra-op-threads", "", func(s string) error {
		withModel(func(m *ModelConfig) { m.IntraOpThreads, _ = strconv.Atoi(s) })
		return nil
	})
	fs.Func("model-inter-op-threads", "", func(s string) error {
		withModel(func(m *ModelConfig) { m.InterOpThreads, _ = strconv.Atoi(s) })
		return nil
	})

	if err := fs.Parse(args); err != nil {
		return nil, err
	}
	if showVersion {
		return nil, fmt.Errorf("__version__")
	}
	if !hasConfig && !hasModel {
		return nil, fmt.Errorf("no models configured; use -config, or -model with -model-onnx/-model-repo")
	}
	if err := fc.validate(); err != nil {
		return nil, err
	}
	return fc, nil
}

func (m ModelConfig) Validate() error {
	if m.ModelRepo != "" {
		return nil
	}
	if _, err := os.Stat(m.ONNX); err != nil {
		return fmt.Errorf("onnx file %q: %w", m.ONNX, err)
	}
	if _, err := os.Stat(m.Tokenizer); err != nil {
		return fmt.Errorf("tokenizer file %q: %w", m.Tokenizer, err)
	}
	return nil
}
