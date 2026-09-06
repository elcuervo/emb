package config

import (
	"fmt"
	"os"
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
	MaxPairs *int                   `yaml:"max_pairs"`
	Models   map[string]ModelConfig `yaml:"models"`
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

	if (cfg.TLSCert == "") != (cfg.TLSKey == "") {
		return nil, fmt.Errorf("both tls_cert and tls_key must be set together")
	}

	for name, m := range cfg.Models {
		if m.ModelRepo == "" && m.ONNX == "" {
			return nil, fmt.Errorf("model %q: onnx path or model_repo is required", name)
		}
		if m.ExecutionMode != "" && m.ExecutionMode != "sequential" && m.ExecutionMode != "parallel" {
			return nil, fmt.Errorf("model %q: execution_mode must be \"sequential\", \"parallel\", or unset, got %q", name, m.ExecutionMode)
		}
		cfg.Models[name] = m
	}

	if err := cfg.validatePersistence(); err != nil {
		return nil, err
	}
	if cfg.MaxTexts != nil && *cfg.MaxTexts < 0 {
		return nil, fmt.Errorf("max_texts must be non-negative")
	}
	if cfg.MaxPairs != nil && *cfg.MaxPairs < 0 {
		return nil, fmt.Errorf("max_pairs must be non-negative")
	}

	return &cfg, nil
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
		if err != nil || pct <= 0 || pct > 100 {
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
	if c.CacheFile != "" && strings.TrimSpace(c.Cache) == "" {
		return fmt.Errorf("cache_file requires cache to be enabled")
	}
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

func ParseFlags(args []string) (*FlagConfig, error) {
	fc := &FlagConfig{
		Config: Config{
			Listen: ":6379",
			Models: make(map[string]ModelConfig),
		},
	}
	var currentModel string
	hasModel := false
	hasConfig := false

	for i := 0; i < len(args); i++ {
		arg := args[i]

		switch {
		case arg == "-listen" && i+1 < len(args):
			i++
			fc.Listen = args[i]

		case arg == "-config" && i+1 < len(args):
			i++
			hasConfig = true
			cfg, err := Load(args[i])
			if err != nil {
				return nil, fmt.Errorf("loading config: %w", err)
			}
			fc.Config = *cfg
			// -config implies models, mark hasModel so the check below passes
			hasModel = true

		case arg == "-password" && i+1 < len(args):
			i++
			fc.Password = args[i]

		case arg == "-cache" && i+1 < len(args):
			i++
			fc.Cache = args[i]

		case arg == "-cache-file" && i+1 < len(args):
			i++
			fc.CacheFile = args[i]

		case arg == "-cache-load" && i+1 < len(args):
			i++
			v, err := strconv.ParseBool(args[i])
			if err != nil {
				return nil, fmt.Errorf("parsing -cache-load: %w", err)
			}
			fc.CacheLoad = &v

		case arg == "-cache-save" && i+1 < len(args):
			i++
			fc.CacheSave = args[i]

		case arg == "-cache-save-on-shutdown" && i+1 < len(args):
			i++
			v, err := strconv.ParseBool(args[i])
			if err != nil {
				return nil, fmt.Errorf("parsing -cache-save-on-shutdown: %w", err)
			}
			fc.CacheSaveOnShutdown = &v

		case arg == "-cache-restore-limit" && i+1 < len(args):
			i++
			fc.CacheRestoreLimit = args[i]

		case arg == "-cache-restore-reserve" && i+1 < len(args):
			i++
			fc.CacheRestoreReserve = args[i]

		case arg == "-cache-save-rate-limit" && i+1 < len(args):
			i++
			fc.CacheSaveRateLimit = args[i]

		case arg == "-idle-timeout" && i+1 < len(args):
			i++
			d, err := time.ParseDuration(args[i])
			if err != nil {
				return nil, fmt.Errorf("parsing -idle-timeout: %w", err)
			}
			if d < 0 {
				return nil, fmt.Errorf("-idle-timeout must not be negative")
			}
			dur := Duration(d)
			fc.IdleTimeout = &dur

		case arg == "-max-connections" && i+1 < len(args):
			i++
			fc.MaxConnections, _ = strconv.Atoi(args[i])

		case arg == "-max-concurrent-requests" && i+1 < len(args):
			i++
			fc.MaxConcurrentRequests, _ = strconv.Atoi(args[i])

		case arg == "-max-texts" && i+1 < len(args):
			i++
			v, _ := strconv.Atoi(args[i])
			fc.MaxTexts = &v

		case arg == "-max-pairs" && i+1 < len(args):
			i++
			v, _ := strconv.Atoi(args[i])
			fc.MaxPairs = &v

		case arg == "-tls-cert" && i+1 < len(args):
			i++
			fc.TLSCert = args[i]

		case arg == "-tls-key" && i+1 < len(args):
			i++
			fc.TLSKey = args[i]

		case arg == "-ort-lib" && i+1 < len(args):
			i++
			fc.OrtLib = args[i]

		case arg == "-version":
			return nil, fmt.Errorf("__version__")

		case arg == "-model" && i+1 < len(args):
			i++
			currentModel = args[i]
			fc.Models[currentModel] = ModelConfig{}
			hasModel = true

		case strings.HasPrefix(arg, "-model-"):
			if currentModel == "" {
				currentModel = "model"
				fc.Models[currentModel] = ModelConfig{}
			}
			hasModel = true
			m := fc.Models[currentModel]
			val := func() string {
				if i+1 < len(args) {
					i++
					return args[i]
				}
				return ""
			}
			switch arg {
			case "-model-onnx":
				m.ONNX = val()
			case "-model-repo":
				m.ModelRepo = val()
			case "-model-tokenizer":
				m.Tokenizer = val()
			case "-pooling":
				m.Pooling = val()
			case "-normalize":
				m.Normalize = true
			case "-output-tensor":
				m.OutputTensor = val()
			case "-pad-output":
				m.PadOutput = true
			case "-dim":
				m.Dim, _ = strconv.Atoi(val())
			case "-max-length":
				m.MaxLength, _ = strconv.Atoi(val())
			case "-quantize":
				m.Quantize = val()
			case "-workers":
				m.Workers, _ = strconv.Atoi(val())
			case "-tokenize-workers":
				v, _ := strconv.Atoi(val())
				m.TokenizeWorkers = &v
			case "-intra-op-threads":
				m.IntraOpThreads, _ = strconv.Atoi(val())
			case "-inter-op-threads":
				m.InterOpThreads, _ = strconv.Atoi(val())
			}
			fc.Models[currentModel] = m
		}
	}

	if !hasConfig && !hasModel {
		return nil, fmt.Errorf("no models configured; use -config, or -model with -model-onnx/-model-repo")
	}

	if (fc.TLSCert == "") != (fc.TLSKey == "") {
		return nil, fmt.Errorf("both -tls-cert and -tls-key must be set together")
	}

	for name, m := range fc.Models {
		if m.ModelRepo == "" && m.ONNX == "" {
			return nil, fmt.Errorf("model %q: onnx path or model_repo is required", name)
		}
	}

	if err := fc.validatePersistence(); err != nil {
		return nil, err
	}
	if fc.MaxTexts != nil && *fc.MaxTexts < 0 {
		return nil, fmt.Errorf("max_texts must be non-negative")
	}
	if fc.MaxPairs != nil && *fc.MaxPairs < 0 {
		return nil, fmt.Errorf("max_pairs must be non-negative")
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
