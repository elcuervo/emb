package telemetry

import (
	"strconv"
	"strings"
	"time"
)

// DefaultInterval is the export interval when none is configured. It is
// shorter than the OpenTelemetry default (60s) so a fresh dashboard fills
// quickly.
const DefaultInterval = 30 * time.Second

// sendTimeout bounds one export POST so a slow endpoint cannot accumulate
// goroutines or delay shutdown.
const sendTimeout = 10 * time.Second

// Config is the resolved telemetry configuration.
type Config struct {
	Endpoint       string
	Interval       time.Duration
	Headers        map[string]string
	ServiceName    string
	ServiceVersion string
	Host           string
	// Extra holds additional resource attributes (OTEL_RESOURCE_ATTRIBUTES).
	Extra map[string]string
	// Disabled is set by an explicit `enabled: false` or `-otel-disabled`.
	Disabled bool
	// envDisabled records OTEL_SDK_DISABLED=true. It is the standard kill
	// switch: it can never be cleared by a later overlay.
	envDisabled bool
}

// Overlay is a partial configuration. Nil fields are unset, so an overlay only
// applies the sources that actually specified a value.
type Overlay struct {
	Endpoint       *string
	Interval       *time.Duration
	Headers        []string
	ServiceName    *string
	ServiceVersion *string
	Host           *string
	Extra          map[string]string
	Disabled       *bool
}

// EffectiveDisabled reports whether export is off for any reason.
func (c Config) EffectiveDisabled() bool { return c.Disabled || c.envDisabled }

// Enabled reports whether the configuration is complete enough to export.
func (c Config) Enabled() bool { return !c.EffectiveDisabled() && c.Endpoint != "" }

// IntervalOrDefault returns the configured interval, or DefaultInterval.
func (c Config) IntervalOrDefault() time.Duration {
	if c.Interval > 0 {
		return c.Interval
	}
	return DefaultInterval
}

// With returns c with the overlay's set fields applied. The standard
// OTEL_SDK_DISABLED kill switch is preserved across overlays.
func (c Config) With(o Overlay) Config {
	if o.Endpoint != nil {
		c.Endpoint = *o.Endpoint
	}
	if o.Interval != nil {
		c.Interval = *o.Interval
	}
	if o.Headers != nil {
		c.Headers = ParseHeaders(o.Headers)
	}
	if o.ServiceName != nil {
		c.ServiceName = *o.ServiceName
	}
	if o.ServiceVersion != nil {
		c.ServiceVersion = *o.ServiceVersion
	}
	if o.Host != nil {
		c.Host = *o.Host
	}
	if o.Extra != nil {
		c.Extra = o.Extra
	}
	if o.Disabled != nil {
		c.Disabled = *o.Disabled
	}
	return c
}

// FromEnv reads the standard OpenTelemetry environment variables. Unset
// variables leave the corresponding field zero so a later overlay can fill it.
func FromEnv(getenv func(string) string) Config {
	var c Config
	switch {
	case getenv("OTEL_EXPORTER_OTLP_METRICS_ENDPOINT") != "":
		c.Endpoint = getenv("OTEL_EXPORTER_OTLP_METRICS_ENDPOINT")
	case getenv("OTEL_EXPORTER_OTLP_ENDPOINT") != "":
		c.Endpoint = getenv("OTEL_EXPORTER_OTLP_ENDPOINT")
	}
	if v := getenv("OTEL_METRIC_EXPORT_INTERVAL"); v != "" {
		// The standard variable is milliseconds.
		if ms, err := strconv.Atoi(v); err == nil && ms > 0 {
			c.Interval = time.Duration(ms) * time.Millisecond
		}
	}
	c.ServiceName = getenv("OTEL_SERVICE_NAME")
	c.ServiceVersion = getenv("OTEL_SERVICE_VERSION")
	if v := getenv("OTEL_RESOURCE_ATTRIBUTES"); v != "" {
		c.Extra = parseList(v, ",")
	}
	if v := getenv("OTEL_EXPORTER_OTLP_METRICS_HEADERS"); v != "" {
		c.Headers = parseList(v, ",")
	} else if v := getenv("OTEL_EXPORTER_OTLP_HEADERS"); v != "" {
		c.Headers = parseList(v, ",")
	}
	if strings.EqualFold(getenv("OTEL_SDK_DISABLED"), "true") {
		c.envDisabled = true
	}
	return c
}

// ParseHeaders converts "key=value" strings into a map. Entries without "="
// are skipped.
func ParseHeaders(headers []string) map[string]string {
	if len(headers) == 0 {
		return nil
	}
	out := make(map[string]string, len(headers))
	for _, h := range headers {
		k, v, ok := strings.Cut(h, "=")
		if !ok || k == "" {
			continue
		}
		out[k] = v
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

// parseList splits a comma-separated key=value string into a map.
func parseList(s, sep string) map[string]string {
	parts := strings.Split(s, sep)
	out := make(map[string]string, len(parts))
	for _, p := range parts {
		k, v, ok := strings.Cut(strings.TrimSpace(p), "=")
		if !ok || k == "" {
			continue
		}
		out[k] = v
	}
	if len(out) == 0 {
		return nil
	}
	return out
}
