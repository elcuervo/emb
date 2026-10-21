package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestParseShutdownFlags(t *testing.T) {
	cases := []struct {
		in      string
		want    ShutdownFlags
		wantStr string
		wantErr bool
	}{
		{in: "", wantStr: "default"},
		{in: "default", wantStr: "default"},
		{in: "NOSAVE now", want: ShutdownFlags{NoSave: true, Now: true}, wantStr: "nosave now"},
		{in: "now nosave", want: ShutdownFlags{NoSave: true, Now: true}, wantStr: "nosave now"},
		{in: "save force", want: ShutdownFlags{Save: true, Force: true}, wantStr: "save force"},
		{in: "save nosave", wantErr: true},
		{in: "save nosave now", wantErr: true},
		{in: "later", wantErr: true},
		{in: "now now", want: ShutdownFlags{Now: true}, wantStr: "now"},
	}
	for _, c := range cases {
		got, err := ParseShutdownFlags(c.in)
		if c.wantErr {
			if err == nil {
				t.Errorf("ParseShutdownFlags(%q) accepted invalid flags", c.in)
			}
			continue
		}
		if err != nil {
			t.Errorf("ParseShutdownFlags(%q): %v", c.in, err)
			continue
		}
		if got != c.want {
			t.Errorf("ParseShutdownFlags(%q) = %+v, want %+v", c.in, got, c.want)
		}
		if got.String() != c.wantStr {
			t.Errorf("ParseShutdownFlags(%q).String() = %q, want %q", c.in, got.String(), c.wantStr)
		}
		// Round-trip: the canonical string parses back to the same flags.
		again, err := ParseShutdownFlags(got.String())
		if err != nil || again != got {
			t.Errorf("round trip of %q = %+v, %v", got.String(), again, err)
		}
	}
}

func TestShutdownFlagsSaveEnabled(t *testing.T) {
	cases := []struct {
		flags ShutdownFlags
		base  bool
		want  bool
	}{
		{ShutdownFlags{}, true, true},
		{ShutdownFlags{}, false, false},
		{ShutdownFlags{NoSave: true}, true, false},
		{ShutdownFlags{Save: true}, false, true},
	}
	for _, c := range cases {
		if got := c.flags.SaveEnabled(c.base); got != c.want {
			t.Errorf("%+v.SaveEnabled(%t) = %t, want %t", c.flags, c.base, got, c.want)
		}
	}
}

func writeShutdownConfig(t *testing.T, body string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "config.yaml")
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestLoadShutdownConfig(t *testing.T) {
	t.Run("defaults", func(t *testing.T) {
		cfg, err := Load(writeShutdownConfig(t, "models:\n  m:\n    model_repo: repo\n"))
		if err != nil {
			t.Fatal(err)
		}
		if cfg.ShutdownTimeout != 0 {
			t.Fatalf("shutdown_timeout default = %d, want 0", cfg.ShutdownTimeout)
		}
		term, err := cfg.ShutdownSigtermFlags()
		if err != nil || term != (ShutdownFlags{}) {
			t.Fatalf("shutdown_on_sigterm default = %+v, %v", term, err)
		}
		intr, err := cfg.ShutdownSigintFlags()
		if err != nil || intr != (ShutdownFlags{}) {
			t.Fatalf("shutdown_on_sigint default = %+v, %v", intr, err)
		}
	})

	t.Run("values", func(t *testing.T) {
		cfg, err := Load(writeShutdownConfig(t, `
models:
  m:
    model_repo: repo
shutdown_timeout: 15
shutdown_on_sigterm: nosave now
shutdown_on_sigint: save
`))
		if err != nil {
			t.Fatal(err)
		}
		if cfg.ShutdownTimeout != 15 {
			t.Fatalf("shutdown_timeout = %d, want 15", cfg.ShutdownTimeout)
		}
		if term, _ := cfg.ShutdownSigtermFlags(); term != (ShutdownFlags{NoSave: true, Now: true}) {
			t.Fatalf("shutdown_on_sigterm = %+v", term)
		}
		if intr, _ := cfg.ShutdownSigintFlags(); intr != (ShutdownFlags{Save: true}) {
			t.Fatalf("shutdown_on_sigint = %+v", intr)
		}
	})

	t.Run("invalid", func(t *testing.T) {
		for name, body := range map[string]string{
			"negative timeout": "shutdown_timeout: -1\n",
			"save nosave":      "shutdown_on_sigterm: save nosave\n",
			"unknown word":     "shutdown_on_sigint: later\n",
		} {
			if _, err := Load(writeShutdownConfig(t, "models:\n  m:\n    model_repo: repo\n"+body)); err == nil {
				t.Errorf("%s: invalid shutdown config accepted", name)
			}
		}
		if _, err := Load(writeShutdownConfig(t, "models:\n  m:\n    model_repo: repo\nshutdown_timeout: 1.5\n")); err == nil {
			t.Error("fractional shutdown_timeout accepted")
		}
	})
}

func TestParseFlagsShutdown(t *testing.T) {
	fc, err := ParseFlags([]string{
		"-model", "test", "-model-repo", "repo",
		"-shutdown-timeout", "10",
		"-shutdown-on-sigterm", "NOSAVE now",
		"-shutdown-on-sigint", "save force",
	})
	if err != nil {
		t.Fatal(err)
	}
	if fc.ShutdownTimeout != 10 {
		t.Fatalf("shutdown timeout = %d, want 10", fc.ShutdownTimeout)
	}
	if fc.ShutdownOnSigterm != "nosave now" || fc.ShutdownOnSigint != "save force" {
		t.Fatalf("signal flags = %q / %q", fc.ShutdownOnSigterm, fc.ShutdownOnSigint)
	}
	for _, args := range [][]string{
		{"-model", "test", "-model-repo", "repo", "-shutdown-timeout", "-1"},
		{"-model", "test", "-model-repo", "repo", "-shutdown-timeout", "1.5"},
		{"-model", "test", "-model-repo", "repo", "-shutdown-on-sigterm", "save nosave"},
		{"-model", "test", "-model-repo", "repo", "-shutdown-on-sigint", "later"},
	} {
		if _, err := ParseFlags(args); err == nil {
			t.Errorf("invalid shutdown flags accepted: %v", args)
		}
	}
}
