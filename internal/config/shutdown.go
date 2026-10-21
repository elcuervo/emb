package config

import (
	"fmt"
	"math"
	"strings"

	"gopkg.in/yaml.v3"
)

// DefaultShutdownFlags is the flag set a signal runs the shutdown with when
// shutdown_on_sigterm/shutdown_on_sigint is unset.
const DefaultShutdownFlags = "default"

// ShutdownTimeout is the shutdown grace period in integer seconds. A plain int
// field would let YAML silently truncate a fractional value, so the type
// rejects one instead.
type ShutdownTimeout int

// UnmarshalYAML accepts an integral, non-negative number of seconds.
func (t *ShutdownTimeout) UnmarshalYAML(node *yaml.Node) error {
	var seconds float64
	if err := node.Decode(&seconds); err != nil {
		return fmt.Errorf("shutdown_timeout must be a non-negative integer number of seconds: %w", err)
	}
	if seconds < 0 || seconds != math.Trunc(seconds) {
		return fmt.Errorf("shutdown_timeout must be a non-negative integer number of seconds, got %v", seconds)
	}
	*t = ShutdownTimeout(seconds)
	return nil
}

// ShutdownFlags is the parsed form of shutdown_on_sigterm, shutdown_on_sigint,
// and the SHUTDOWN command's arguments. The zero value means "no flag given".
type ShutdownFlags struct {
	Save   bool
	NoSave bool
	Now    bool
	Force  bool
}

// ParseShutdownFlags parses a case-insensitive, space-separated combination of
// default, save, nosave, now, and force. It rejects unknown words and save with
// nosave, as Redis rejects them for the SHUTDOWN command. An empty string is
// the default flag set.
func ParseShutdownFlags(s string) (ShutdownFlags, error) {
	var f ShutdownFlags
	for _, word := range strings.Fields(strings.ToLower(s)) {
		switch word {
		case "default":
		case "save":
			f.Save = true
		case "nosave":
			f.NoSave = true
		case "now":
			f.Now = true
		case "force":
			f.Force = true
		default:
			return ShutdownFlags{}, fmt.Errorf("unknown shutdown flag %q (want default|save|nosave|now|force)", word)
		}
	}
	if f.Save && f.NoSave {
		return ShutdownFlags{}, fmt.Errorf("save and nosave cannot be combined")
	}
	return f, nil
}

// String formats the flags in canonical order, "default" when none is set.
func (f ShutdownFlags) String() string {
	parts := make([]string, 0, 4)
	for _, flag := range []struct {
		set  bool
		name string
	}{{f.Save, "save"}, {f.NoSave, "nosave"}, {f.Now, "now"}, {f.Force, "force"}} {
		if flag.set {
			parts = append(parts, flag.name)
		}
	}
	if len(parts) == 0 {
		return DefaultShutdownFlags
	}
	return strings.Join(parts, " ")
}

// SaveEnabled resolves whether the final snapshot is written, from these flags
// and cache_save_on_shutdown: NOSAVE wins, SAVE wins over the base setting, and
// otherwise the base setting decides.
func (f ShutdownFlags) SaveEnabled(base bool) bool {
	if f.NoSave {
		return false
	}
	if f.Save {
		return true
	}
	return base
}

// ShutdownSigtermFlags resolves the flags for SIGTERM from the configuration.
func (c Config) ShutdownSigtermFlags() (ShutdownFlags, error) {
	return c.shutdownSignalFlags("shutdown_on_sigterm", c.ShutdownOnSigterm)
}

// ShutdownSigintFlags resolves the flags for SIGINT from the configuration.
func (c Config) ShutdownSigintFlags() (ShutdownFlags, error) {
	return c.shutdownSignalFlags("shutdown_on_sigint", c.ShutdownOnSigint)
}

func (c Config) shutdownSignalFlags(name, value string) (ShutdownFlags, error) {
	f, err := ParseShutdownFlags(value)
	if err != nil {
		return ShutdownFlags{}, fmt.Errorf("%s: %w", name, err)
	}
	return f, nil
}
