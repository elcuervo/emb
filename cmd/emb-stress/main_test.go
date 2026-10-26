package main

import (
	"bytes"
	"net"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/elcuervo/emb/internal/embtop"
)

// The flag surface must stay self-documenting: every flag the tool takes has a
// usage line, and -h prints them all.
func TestHelpDocumentsEveryFlag(t *testing.T) {
	var buf bytes.Buffer
	if _, err := parseFlags([]string{"-h"}, func(string) string { return "" }, &buf); err == nil {
		t.Fatal("parseFlags(-h) did not report help")
	}
	usage := buf.String()
	for _, flagName := range []string{
		"-config", "-target", "-seed", "-levels", "-dwell", "-warmup",
		"-corpus", "-json", "-password", "-tls", "-tls-server-name", "-version",
	} {
		if !strings.Contains(usage, flagName) {
			t.Errorf("-h does not document %s:\n%s", flagName, usage)
		}
	}
}

func TestParseFlagsDefaultsMatchTheDocumentedOnes(t *testing.T) {
	o, err := parseFlags([]string{"-config", "c.yaml", "-target", "h:1"},
		func(string) string { return "" }, &bytes.Buffer{})
	if err != nil {
		t.Fatal(err)
	}
	if o.levels != defaultLevels || o.dwell != defaultDwell || o.warmup != defaultWarmup || o.seed != 1 {
		t.Fatalf("defaults are not the documented ones: %+v", o)
	}
	if len(o.specs) != 1 || o.specs[0].Addr() != "h:1" {
		t.Fatalf("target was not parsed into one specification: %+v", o.specs)
	}
}

func TestParseFlagsRejectsInvalidRuns(t *testing.T) {
	cases := []struct {
		name string
		args []string
		want string
	}{
		{"no config", []string{"-target", "h:1"}, "-config is required"},
		{"no target", []string{"-config", "c.yaml"}, "-target is required"},
		{"unparseable target", []string{"-config", "c.yaml", "-target", "h:notaport"}, "invalid port"},
		{"empty level list", []string{"-config", "c.yaml", "-target", "h:1", "-levels", ","}, "-levels"},
		{"non-numeric level", []string{"-config", "c.yaml", "-target", "h:1", "-levels", "1,x"}, "not a positive integer"},
		{"zero level", []string{"-config", "c.yaml", "-target", "h:1", "-levels", "0"}, "not a positive integer"},
		{"zero dwell", []string{"-config", "c.yaml", "-target", "h:1", "-dwell", "0s"}, "-dwell must be positive"},
		{"warm-up longer than the dwell", []string{"-config", "c.yaml", "-target", "h:1", "-dwell", "1s", "-warmup", "1s"}, "must be shorter than"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := parseFlags(tc.args, func(string) string { return "" }, &bytes.Buffer{})
			if err == nil {
				t.Fatal("invalid flags were accepted")
			}
			if !strings.Contains(err.Error(), tc.want) {
				t.Errorf("err = %q, want it to mention %q", err, tc.want)
			}
		})
	}
}

func TestParseFlagsReadsThePasswordFromTheEnvironment(t *testing.T) {
	o, err := parseFlags([]string{"-config", "c.yaml", "-target", "h:1"},
		func(k string) string {
			if k == "EMB_STRESS_PASSWORD" {
				return "hunter2"
			}
			return ""
		}, &bytes.Buffer{})
	if err != nil {
		t.Fatal(err)
	}
	if o.password != "hunter2" {
		t.Fatalf("password = %q, want the environment's", o.password)
	}
}

// A missing configuration must fail before any connection is opened: the target
// here points at a port nothing listens on, so a connection attempt would fail
// differently.
func TestMissingConfigFailsBeforeConnecting(t *testing.T) {
	var out, errBuf bytes.Buffer
	code := run([]string{"-config", "/nonexistent/emb.yaml", "-target", "127.0.0.1:9"},
		func(string) string { return "" }, &out, &errBuf)
	if code != 1 {
		t.Fatalf("exit = %d, want 1", code)
	}
	if !strings.Contains(errBuf.String(), "config") {
		t.Fatalf("stderr does not name the configuration: %q", errBuf.String())
	}
}

// A target that resolves to nothing is an error naming it, not a run reported
// against an empty fleet.
func TestUnresolvableTargetFailsNamingIt(t *testing.T) {
	defer func(d time.Duration) { waitBudget = d }(waitBudget)
	waitBudget = 50 * time.Millisecond
	dir := t.TempDir()
	cfg := filepath.Join(dir, "config.yaml")
	if err := os.WriteFile(cfg, []byte("models:\n  m:\n    model_repo: some/repo\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	var out, errBuf bytes.Buffer
	code := run([]string{"-config", cfg, "-target", "emb-stress-no-such-host.invalid"},
		func(string) string { return "" }, &out, &errBuf)
	if code != 1 {
		t.Fatalf("exit = %d, want 1 (stderr %q)", code, errBuf.String())
	}
	if !strings.Contains(errBuf.String(), "emb-stress-no-such-host.invalid") {
		t.Fatalf("the failure does not name the target: %q", errBuf.String())
	}
}

// A configuration that declares no model cannot drive a fleet.
func TestEmptyConfigFails(t *testing.T) {
	dir := t.TempDir()
	cfg := filepath.Join(dir, "config.yaml")
	if err := os.WriteFile(cfg, []byte("listen: 127.0.0.1:6379\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	var out, errBuf bytes.Buffer
	code := run([]string{"-config", cfg, "-target", "127.0.0.1:9"},
		func(string) string { return "" }, &out, &errBuf)
	if code != 1 {
		t.Fatalf("exit = %d, want 1", code)
	}
	if !strings.Contains(errBuf.String(), "no models declared") {
		t.Fatalf("stderr does not name the problem: %q", errBuf.String())
	}
}

func TestVersionFlag(t *testing.T) {
	var out, errBuf bytes.Buffer
	if code := run([]string{"-version"}, func(string) string { return "" }, &out, &errBuf); code != 0 {
		t.Fatalf("exit = %d, want 0", code)
	}
	if strings.TrimSpace(out.String()) != version {
		t.Fatalf("out = %q, want the version %q", out.String(), version)
	}
}

func TestParseLevels(t *testing.T) {
	got, err := parseLevels(" 1 , 2,4 ")
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 3 || got[0] != 1 || got[2] != 4 {
		t.Fatalf("parseLevels = %v, want [1 2 4]", got)
	}
}

// The run's own wait must not be shorter than the levels it holds.
func TestRunIsLongEnoughToSettle(t *testing.T) {
	if defaultWarmup <= 0 || defaultWarmup >= defaultDwell {
		t.Fatalf("default warm-up %s is not inside the dwell %s", defaultWarmup, defaultDwell)
	}
	if defaultDwell < time.Second {
		t.Fatalf("default dwell %s is too short to settle", defaultDwell)
	}
}

// staleFleet serves one EMB.MODELS reply and then stops listening, so the run
// reconciles a model and then cannot open a worker connection. That is the
// shape of a fleet that goes away mid-run, and of a client that runs out of
// sockets.
func staleFleet(t *testing.T) string {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	addr := ln.Addr().String()
	go func() {
		conn, err := ln.Accept()
		if err != nil {
			return
		}
		defer conn.Close()
		// The listener goes away as soon as the fleet has answered once, so
		// reconciliation succeeds and every worker connection after it is
		// refused.
		_ = ln.Close()
		buf := make([]byte, 4096)
		for {
			if _, err := conn.Read(buf); err != nil {
				return
			}
			// EMB.MODELS: one row of [name, dim, status].
			_, _ = conn.Write([]byte("*1\r\n*3\r\n$1\r\nm\r\n:384\r\n$5\r\nready\r\n"))
		}
	}()
	t.Cleanup(func() { _ = ln.Close() })
	return addr
}

// A run that fails after it started must not exit 0: it hands back the samples
// it took, so the exit code is the only thing that tells a script it failed.
func TestFailedRunExitsNonZero(t *testing.T) {
	addr := staleFleet(t)
	dir := t.TempDir()
	cfg := filepath.Join(dir, "config.yaml")
	if err := os.WriteFile(cfg, []byte("models:\n  m:\n    model_repo: some/repo\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	var out, errBuf bytes.Buffer
	code := run([]string{
		"-config", cfg, "-target", addr,
		"-levels", "1", "-dwell", "2s", "-warmup", "1s",
	}, func(string) string { return "" }, &out, &errBuf)

	if code != 1 {
		t.Fatalf("exit = %d, want 1 (stderr %q)", code, errBuf.String())
	}
	if !strings.Contains(errBuf.String(), "worker connection") {
		t.Errorf("stderr does not say what failed: %q", errBuf.String())
	}
}

// The same holds for the machine-readable form: a script reading JSON must see
// the failure in the exit code too.
func TestFailedRunExitsNonZeroAsJSON(t *testing.T) {
	addr := staleFleet(t)
	dir := t.TempDir()
	cfg := filepath.Join(dir, "config.yaml")
	if err := os.WriteFile(cfg, []byte("models:\n  m:\n    model_repo: some/repo\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	var out, errBuf bytes.Buffer
	code := run([]string{
		"-config", cfg, "-target", addr, "-json",
		"-levels", "1", "-dwell", "2s", "-warmup", "1s",
	}, func(string) string { return "" }, &out, &errBuf)

	if code != 1 {
		t.Fatalf("exit = %d, want 1 (stderr %q)", code, errBuf.String())
	}
}

// A row expanded from a name is dialled by its address, so the certificate has
// to be verified against the name that resolved it; the override still wins.
func TestTLSServerNameFollowsTheResolvedName(t *testing.T) {
	expanded := embtop.ResolvedNode{
		Key: "10.0.0.1:6399", Spec: "emb.internal:6399", TLSName: "emb.internal", Addr: "10.0.0.1:6399",
	}
	if got := tlsServerName(&options{}, expanded); got != "emb.internal" {
		t.Errorf("TLS name = %q, want the resolved name", got)
	}
	if got := tlsServerName(&options{tlsServerName: "override"}, expanded); got != "override" {
		t.Errorf("TLS name = %q, want the override", got)
	}
	if got := tlsServerName(&options{}, embtop.ResolvedNode{Addr: "10.0.0.1:6399"}); got != "" {
		t.Errorf("literal row TLS name = %q, want none", got)
	}
}
