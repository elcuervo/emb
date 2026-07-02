package config

import (
	"fmt"
	"os"
	"strconv"
	"strings"

	"gopkg.in/yaml.v3"
)

type Config struct {
	Listen   string                 `yaml:"listen"`
	Password string                 `yaml:"password"`
	TLSCert  string                 `yaml:"tls_cert"`
	TLSKey   string                 `yaml:"tls_key"`
	Cache    string                 `yaml:"cache"`
	Models   map[string]ModelConfig `yaml:"models"`
}

type BatchingConfig struct {
	Timeout  int `yaml:"timeout"`
	MaxBatch int `yaml:"max_batch"`
}

type ModelConfig struct {
	ONNX           string         `yaml:"onnx"`
	Tokenizer      string         `yaml:"tokenizer"`
	ModelRepo      string         `yaml:"model_repo"`
	Pooling        string         `yaml:"pooling"`
	Normalize      bool           `yaml:"normalize"`
	MaxLength      int            `yaml:"max_length"`
	Dim            int            `yaml:"dim"`
	Preload        bool           `yaml:"preload"`
	Workers        int            `yaml:"workers"`
	OutputTensor   string         `yaml:"output_tensor"`
	PadOutput      bool           `yaml:"pad_output"`
	Batching       BatchingConfig `yaml:"batching"`
	IntraOpThreads int            `yaml:"intra_op_threads"`
	InterOpThreads int            `yaml:"inter_op_threads"`
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
		cfg.Models[name] = m
	}

	return &cfg, nil
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
			case "-workers":
				m.Workers, _ = strconv.Atoi(val())
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
