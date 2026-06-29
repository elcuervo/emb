package config

import (
	"fmt"
	"os"

	"gopkg.in/yaml.v3"
)

type Config struct {
	Listen string                `yaml:"listen"`
	Models map[string]ModelConfig `yaml:"models"`
}

type ModelConfig struct {
	ONNX      string `yaml:"onnx"`
	Tokenizer string `yaml:"tokenizer"`
	Pooling   string `yaml:"pooling"`
	Normalize bool   `yaml:"normalize"`
	MaxLength int    `yaml:"max_length"`
	Dim       int    `yaml:"dim"`
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
		if m.Pooling == "" {
			m.Pooling = "mean"
		}
		if m.MaxLength <= 0 {
			m.MaxLength = 512
		}
		if m.ONNX == "" {
			return nil, fmt.Errorf("model %q: onnx path is required", name)
		}
		if m.Tokenizer == "" {
			return nil, fmt.Errorf("model %q: tokenizer path is required", name)
		}
		if m.Dim <= 0 {
			return nil, fmt.Errorf("model %q: dim must be > 0", name)
		}
		cfg.Models[name] = m
	}

	return &cfg, nil
}

func (m ModelConfig) Validate() error {
	if _, err := os.Stat(m.ONNX); err != nil {
		return fmt.Errorf("onnx file %q: %w", m.ONNX, err)
	}
	if _, err := os.Stat(m.Tokenizer); err != nil {
		return fmt.Errorf("tokenizer file %q: %w", m.Tokenizer, err)
	}
	return nil
}
