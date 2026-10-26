// Command emb-stress drives synthetic embedding load across a DNS-resolved
// fleet, parameterized by an emb server configuration.
//
// It takes the configuration the fleet was started with (which models to
// exercise, and their settings), a target that is host:port or a DNS name with
// an optional port, expands the name into the node set behind it, reconciles
// the configured models against what each node reports, and then offers load to
// every node at each level in turn. It reports the client's own throughput and
// latency, per node and for the fleet; the token-exact figures are the server's,
// read by emb-top.
//
// It is a pure RESP client: no model, no ONNX Runtime, builds with CGO_ENABLED=0.
//
// Usage:
//
//	emb-stress -config server.yaml -target emb.internal -levels 1,2,4
//	emb-stress -config server.yaml -target 127.0.0.1:6379 -json
package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/signal"
	"sort"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/elcuervo/emb/internal/config"
	"github.com/elcuervo/emb/internal/embstress"
	"github.com/elcuervo/emb/internal/embtop"
)

// version is injected at build time via -ldflags "-X main.version=…".
var version = "dev"

// waitBudget is how long the target's records are waited for before the
// resolution failure is reported: the built-in window, shortened by tests so
// the unresolvable-target case does not spend it.
var waitBudget = embtop.WaitBudget

// Defaults for a run long enough for a level to settle. A level's numbers are
// drawn from dwell-warmup, so three levels take dwell × 3.
const (
	defaultLevels = "1,2,4"
	defaultDwell  = 15 * time.Second
	defaultWarmup = 5 * time.Second
)

// options is the parsed command line.
type options struct {
	configPath    string
	target        string
	seed          int64
	levels        string
	dwell         time.Duration
	warmup        time.Duration
	corpusPath    string
	asJSON        bool
	password      string
	useTLS        bool
	tlsServerName string
	showVersion   bool

	specs []embtop.NodeSpec
}

// tlsServerName is the name a client verifies its certificate against: the
// explicit override wins, then the name the row was resolved from. An expanded
// row is dialled by its address, so the name cannot be left to the dial address.
func tlsServerName(o *options, rn embtop.ResolvedNode) string {
	if o.tlsServerName != "" {
		return o.tlsServerName
	}
	return rn.TLSName
}

// parseFlags parses args into options, writing usage and parse errors to
// usage. It is separate from main so the flag surface and its validation are
// covered by a plain test.
func parseFlags(args []string, getenv func(string) string, usage io.Writer) (*options, error) {
	o := &options{}
	fs := flag.NewFlagSet("emb-stress", flag.ContinueOnError)
	fs.SetOutput(usage)
	fs.StringVar(&o.configPath, "config", "", "emb server configuration file: which models to exercise, and their settings")
	fs.StringVar(&o.target, "target", "", "fleet to drive: host:port, or a DNS name expanded to one endpoint per address")
	fs.Int64Var(&o.seed, "seed", 1, "seed for the generated texts; the same seed offers the same request set")
	fs.StringVar(&o.levels, "levels", defaultLevels, "concurrent workers per endpoint at each level, comma-separated")
	fs.DurationVar(&o.dwell, "dwell", defaultDwell, "how long each level is held")
	fs.DurationVar(&o.warmup, "warmup", defaultWarmup, "leading portion of each dwell that is discarded before measuring")
	fs.StringVar(&o.corpusPath, "corpus", "", "corpus file, one text per line, replacing the generated texts")
	fs.BoolVar(&o.asJSON, "json", false, "emit the machine-readable result instead of the human report")
	fs.StringVar(&o.password, "password", getenv("EMB_STRESS_PASSWORD"),
		"AUTH password (prefer $EMB_STRESS_PASSWORD: command-line values are visible in process listings)")
	fs.BoolVar(&o.useTLS, "tls", false, "connect over TLS")
	fs.StringVar(&o.tlsServerName, "tls-server-name", "",
		"TLS name to verify when it differs from the dialed address")
	fs.BoolVar(&o.showVersion, "version", false, "print version and exit")
	if err := fs.Parse(args); err != nil {
		return nil, err
	}
	if o.showVersion {
		return o, nil
	}
	if strings.TrimSpace(o.configPath) == "" {
		return nil, errors.New("-config is required")
	}
	if strings.TrimSpace(o.target) == "" {
		return nil, errors.New("-target is required")
	}
	if _, err := parseLevels(o.levels); err != nil {
		return nil, err
	}
	if o.dwell <= 0 {
		return nil, errors.New("-dwell must be positive")
	}
	if o.warmup < 0 || o.warmup >= o.dwell {
		return nil, fmt.Errorf("-warmup %s must be shorter than -dwell %s", o.warmup, o.dwell)
	}
	spec, err := embtop.ParseNodeSpec(o.target)
	if err != nil {
		return nil, err
	}
	o.specs = []embtop.NodeSpec{spec}
	return o, nil
}

// parseLevels parses the comma-separated worker counts, rejecting anything that
// is not a positive integer.
func parseLevels(csv string) ([]int, error) {
	var out []int
	for _, field := range strings.Split(csv, ",") {
		field = strings.TrimSpace(field)
		if field == "" {
			continue
		}
		n, err := strconv.Atoi(field)
		if err != nil || n < 1 {
			return nil, fmt.Errorf("-levels: %q is not a positive integer", field)
		}
		out = append(out, n)
	}
	if len(out) == 0 {
		return nil, errors.New("-levels must name at least one level")
	}
	return out, nil
}

func main() {
	os.Exit(run(os.Args[1:], os.Getenv, os.Stdout, os.Stderr))
}

// run executes the command and returns its exit code: 2 for a usage error, 1
// for a failed run, 0 on success.
func run(args []string, getenv func(string) string, stdout, stderr io.Writer) int {
	o, err := parseFlags(args, getenv, stderr)
	if err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return 0
		}
		fprintln(stderr, "emb-stress:", err)
		return 2
	}
	if o.showVersion {
		fprintln(stdout, version)
		return 0
	}

	// The configuration is read before any connection is opened, so a bad or
	// missing file fails without driving a fleet.
	cfg, err := config.Load(o.configPath)
	if err != nil {
		fprintln(stderr, "emb-stress: config:", err)
		return 1
	}
	models := configuredModels(cfg)
	if len(models) == 0 {
		fprintln(stderr, "emb-stress: config:", "no models declared in", o.configPath)
		return 1
	}
	levels, _ := parseLevels(o.levels) // validated in parseFlags

	var corpus []string
	if o.corpusPath != "" {
		if corpus, err = embstress.LoadCorpus(o.corpusPath); err != nil {
			fprintln(stderr, "emb-stress:", err)
			return 1
		}
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	// The fleet's records can appear after this run starts, so the target is
	// waited for within the same built-in window `emb-top` uses; a name that
	// never appears is still the resolution error it was.
	resolved, err := embtop.WaitForSpecs(ctx, embtop.NetResolver{}, o.specs, waitBudget)
	if err != nil {
		fprintln(stderr, "emb-stress:", err)
		return 1
	}
	if o.password != "" && !o.useTLS && !embtop.AllLoopback(o.specs) {
		fprintln(stderr, "emb-stress: warning: sending an AUTH password to a non-loopback address without -tls")
	}

	clients := make([]embstress.Endpoint, 0, len(resolved))
	for _, rn := range resolved {
		c := embstress.NewClient(rn.Addr, rn.Addr, o.password, o.useTLS)
		if name := tlsServerName(o, rn); name != "" {
			c.SetTLSServerName(name)
		}
		clients = append(clients, c)
		defer func() { _ = c.Close() }()
	}

	fprintln(stderr, fmt.Sprintf("emb-stress: driving %d endpoint(s) from %q · %d model(s) · levels %v · dwell %s (warm-up discarded) · seed %d",
		len(clients), o.target, len(models), levels, o.dwell, o.seed))

	result, err := embstress.Run(ctx, clients, embstress.Options{
		Target: o.target,
		Models: models,
		Levels: levels,
		Dwell:  o.dwell,
		Warmup: o.warmup,
		Seed:   o.seed,
		Corpus: corpus,
	})
	if err != nil {
		fprintln(stderr, "emb-stress:", err)
		if result == nil {
			return 1
		}
	}
	// A run that failed or exercised nothing is not a result, even when it
	// produced a partial one: an interrupted run reports the samples it took,
	// and a script must still see the failure.
	code := 0
	if err != nil || len(result.Endpoints) == 0 {
		code = 1
	}

	if o.asJSON {
		enc := json.NewEncoder(stdout)
		enc.SetIndent("", "  ")
		if err := enc.Encode(result); err != nil {
			fprintln(stderr, "emb-stress:", err)
			return 1
		}
		return code
	}
	writeReport(stdout, result, len(resolved), len(corpus), o)
	if len(result.Endpoints) == 0 {
		// The drift above names why.
		fprintln(stderr, "emb-stress: nothing was exercised against", o.target)
	}
	return code
}

// configuredModels reads the models the fleet is expected to serve out of the
// server configuration, in a stable name order.
func configuredModels(cfg *config.Config) []embstress.ConfiguredModel {
	names := make([]string, 0, len(cfg.Models))
	for name := range cfg.Models {
		names = append(names, name)
	}
	sort.Strings(names)

	out := make([]embstress.ConfiguredModel, 0, len(names))
	for _, name := range names {
		m := cfg.Models[name]
		out = append(out, embstress.ConfiguredModel{Name: name, Dim: m.Dim, MaxLength: m.MaxLength})
	}
	return out
}

// fprintln writes a line and ignores the error: diagnostics on a broken
// stdout/stderr must not change the exit code.
func fprintln(w io.Writer, a ...any) { _, _ = fmt.Fprintln(w, a...) }

// writeReport renders the human result: the fleet per level, then one section
// per endpoint, then the pairs reconciliation kept out of the workload.
func writeReport(w io.Writer, res *embstress.Result, resolved, corpusTexts int, o *options) {
	texts := "generated word-length classes"
	if corpusTexts > 0 {
		texts = fmt.Sprintf("%d corpus texts", corpusTexts)
	}
	measured := fmt.Sprintf("%d endpoint(s)", len(res.Endpoints))
	if len(res.Endpoints) != resolved {
		measured = fmt.Sprintf("%d of %d endpoint(s)", len(res.Endpoints), resolved)
	}
	_, _ = fmt.Fprintf(w, "emb-stress %s · %s → %s · %d model(s) · seed %d · dwell %s (warm-up discarded) · %s\n",
		version, res.Target, measured, len(res.Models), res.Seed, o.dwell, texts)
	_, _ = fmt.Fprintln(w, "words, not tokens: the server reports the token count; emb-top reads it")
	if len(res.Fleet) > 0 {
		_, _ = fmt.Fprintln(w, "\nFLEET")
		for i, l := range res.Fleet {
			_, _ = fmt.Fprintf(w, "  level %d · %s\n", i+1, l.Describe())
		}
	}
	for _, er := range res.Endpoints {
		_, _ = fmt.Fprintf(w, "\n%s\n", er.Label)
		for i, l := range er.Levels {
			_, _ = fmt.Fprintf(w, "  level %d · %s\n", i+1, l.Describe())
		}
	}
	if len(res.Drift) > 0 {
		_, _ = fmt.Fprintln(w, "\nNOT EXERCISED")
		for _, d := range res.Drift {
			_, _ = fmt.Fprintf(w, "  %s\n", d)
		}
	}
}
