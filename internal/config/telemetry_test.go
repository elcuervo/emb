package config

import (
	"testing"
	"time"
)

func envMap(m map[string]string) func(string) string {
	return func(k string) string { return m[k] }
}

func TestTelemetryOverlayFromYAML(t *testing.T) {
	enabled := true
	cfg := Config{Telemetry: TelemetryConfig{
		Enabled:     &enabled,
		Endpoint:    "http://yaml:4318",
		Interval:    "45s",
		Headers:     []string{"dd-api-key=yaml-key", "dd-otel-metric-config={\"resource_attributes_as_tags\": true}"},
		ServiceName: "emb-yaml",
		Host:        "host-yaml",
		Attributes:  map[string]string{"team": "core"},
	}}
	resolved, err := (&FlagConfig{Config: cfg}).ResolveTelemetry(envMap(nil))
	if err != nil {
		t.Fatal(err)
	}
	if resolved.Endpoint != "http://yaml:4318" || resolved.Interval != 45*time.Second {
		t.Fatalf("resolved = %+v", resolved)
	}
	if resolved.ServiceName != "emb-yaml" || resolved.Host != "host-yaml" {
		t.Fatalf("resolved identity = %+v", resolved)
	}
	if resolved.Headers["dd-api-key"] != "yaml-key" {
		t.Fatalf("headers = %+v", resolved.Headers)
	}
	if !resolved.Enabled() {
		t.Fatal("enabled config with an endpoint must export")
	}
}

func TestTelemetryFlagsBeatYAMLAndEnv(t *testing.T) {
	fc, err := ParseFlags([]string{
		"-model", "m", "-model-repo", "org/repo",
		"-otel-endpoint", "http://flag:4318",
		"-otel-interval", "12s",
		"-otel-header", "dd-api-key=flag-key",
		"-otel-service-name", "emb-flag",
	})
	if err != nil {
		t.Fatalf("ParseFlags: %v", err)
	}
	fc.Telemetry = TelemetryConfig{Endpoint: "http://yaml:4318", Interval: "45s"}
	env := envMap(map[string]string{
		"OTEL_EXPORTER_OTLP_ENDPOINT": "http://env:4318",
		"OTEL_SERVICE_NAME":           "emb-env",
	})

	resolved, err := fc.ResolveTelemetry(env)
	if err != nil {
		t.Fatal(err)
	}
	if resolved.Endpoint != "http://flag:4318" {
		t.Errorf("endpoint = %q, want the flag", resolved.Endpoint)
	}
	if resolved.Interval != 12*time.Second {
		t.Errorf("interval = %v, want the flag", resolved.Interval)
	}
	if resolved.ServiceName != "emb-flag" {
		t.Errorf("service name = %q, want the flag", resolved.ServiceName)
	}
	if resolved.Headers["dd-api-key"] != "flag-key" {
		t.Errorf("headers = %+v, want the flag", resolved.Headers)
	}
}

func TestTelemetryInvalidIntervalFailsLoad(t *testing.T) {
	cfg := Config{Telemetry: TelemetryConfig{Interval: "not-a-duration"}}
	if _, err := (&FlagConfig{Config: cfg}).ResolveTelemetry(envMap(nil)); err == nil {
		t.Fatal("an invalid telemetry interval must fail")
	}
	if err := cfg.validate(); err == nil {
		t.Fatal("validate must reject an invalid telemetry interval")
	}
}

func TestTelemetrySDKDisabledEnvStillDisables(t *testing.T) {
	fc := &FlagConfig{Config: Config{Telemetry: TelemetryConfig{Endpoint: "http://yaml:4318"}}}
	resolved, err := fc.ResolveTelemetry(envMap(map[string]string{"OTEL_SDK_DISABLED": "true"}))
	if err != nil {
		t.Fatal(err)
	}
	if resolved.Enabled() {
		t.Fatal("OTEL_SDK_DISABLED must disable export")
	}
}
