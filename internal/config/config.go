package config

import (
	"fmt"
	"os"

	"gopkg.in/yaml.v3"
)

type Config struct {
	Listen string                 `yaml:"listen"`
	Models map[string]ModelConfig `yaml:"models"`
}

type BatchingConfig struct {
	Timeout  int `yaml:"timeout"`   // ms to wait before flush (0 = disable)
	MaxBatch int `yaml:"max_batch"` // max texts per batch
}

type ModelConfig struct {
	ONNX         string         `yaml:"onnx"`
	Tokenizer    string         `yaml:"tokenizer"`
	ModelRepo    string         `yaml:"model_repo"`
	Pooling      string         `yaml:"pooling"`
	Normalize    bool           `yaml:"normalize"`
	MaxLength    int            `yaml:"max_length"`
	Dim          int            `yaml:"dim"`
	Preload      bool           `yaml:"preload"`
	Workers      int            `yaml:"workers"`
	OutputTensor string         `yaml:"output_tensor"`
	Batching     BatchingConfig `yaml:"batching"`
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
		if m.ModelRepo == "" && m.ONNX == "" {
			return nil, fmt.Errorf("model %q: onnx path or model_repo is required", name)
		}
		cfg.Models[name] = m
	}

	return &cfg, nil
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
