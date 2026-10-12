package main

// Fargate-shaped benchmark harness for emb on ARM64/Graviton CPU.
//
// Builds the server image for --platform, runs it in a Docker container bounded
// by --cpus/--memory at the Fargate vCPU tiers, drives fixed-length / unique-text
// / mixed-length / cache-hit workloads with redis-benchmark and a pure-Ruby RESP
// driver, and emits one evidence bundle: the host fingerprint (the validation
// host contract's verdict), revision, model digests, configuration, per-cell
// metrics, and the outcome of every assertion the run made. Only `-mode
// baseline` insists on a qualified host; the diff refuses any pair whose
// fingerprint identities differ.
//
// Run from the repo root inside `nix develop`:
//
//	go run ./bench/fargate -mode run
//	go run ./bench/fargate -mode baseline
//	go run ./bench/fargate -mode diff -before <a.json> -after <b.json>

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/elcuervo/emb/bench/hostcheck"
	"github.com/elcuervo/emb/internal/tokenizer"
)

type options struct {
	platform        string
	cpus            []int
	clients         []int
	pipelines       []int
	count           int
	modelDir        string
	model           string
	maxLen          int
	image           string
	outDir          string
	mode            string
	before          string
	after           string
	reqGate         float64
	p50Gate         float64
	mixedCount      int
	shortLen        int
	longLen         int
	shortRatio      float64
	maxBatch        int
	batchingTimeout int
	maxBatchTokens  int
	skipBuild       bool
	keep            bool
}

func parseFlags() (*options, error) {
	o := &options{}
	var cpus, clients, pipelines string
	flag.StringVar(&cpus, "cpus", "1,2,4,8", "comma-separated Fargate vCPU tiers")
	flag.StringVar(&clients, "clients", "1,8,16", "comma-separated client counts")
	flag.StringVar(&pipelines, "pipeline", "1,8", "comma-separated pipeline depths")
	flag.StringVar(&o.platform, "platform", "linux/arm64", "docker platform for the Fargate replica")
	flag.IntVar(&o.count, "count", 500, "requests per redis-benchmark cell")
	flag.StringVar(&o.modelDir, "model-dir", "./models/minilm", "model directory (onnx + tokenizer.json)")
	flag.StringVar(&o.model, "model", "minilm", "model name used in EMB commands and config")
	flag.IntVar(&o.maxLen, "max-length", 512, "max_length for the harness server config")
	flag.StringVar(&o.image, "image", "elcuervo/emb:fargate-bench", "docker image tag for the server")
	flag.StringVar(&o.outDir, "out", "bench/fargate/out", "output directory for per-cell runs")
	flag.StringVar(&o.mode, "mode", "run", "run | baseline | diff")
	flag.StringVar(&o.before, "before", "", "baseline JSON (diff mode)")
	flag.StringVar(&o.after, "after", "", "candidate JSON (diff mode)")
	flag.Float64Var(&o.reqGate, "req-gate", 0.05, "req/s tolerance for diff (fraction)")
	flag.Float64Var(&o.p50Gate, "p50-gate", 0.10, "p50 tolerance for diff (fraction)")
	flag.IntVar(&o.mixedCount, "mixed-count", 0, "override request count for the mixed-length workload")
	flag.IntVar(&o.shortLen, "short-len", 8, "approximate token length of short mixed texts")
	flag.IntVar(&o.longLen, "long-len", 500, "approximate token length of long mixed texts")
	flag.Float64Var(&o.shortRatio, "short-ratio", 0.8, "fraction of short texts in the mixed workload")
	flag.IntVar(&o.maxBatch, "max-batch", 32, "server batching max_batch used for padded-slots computation")
	flag.IntVar(&o.batchingTimeout, "batching-timeout", 0, "enable server batching window (ms); 0 = disabled")
	flag.IntVar(&o.maxBatchTokens, "max-batch-tokens", 0, "server batching max_batch_tokens budget; 0 = count-only")
	flag.BoolVar(&o.skipBuild, "skip-build", false, "skip the docker image build")
	flag.BoolVar(&o.keep, "keep", false, "keep server containers running after each tier")
	flag.Parse()

	var err error
	if o.cpus, err = parseInts(cpus); err != nil {
		return nil, fmt.Errorf("cpus: %w", err)
	}
	if o.clients, err = parseInts(clients); err != nil {
		return nil, fmt.Errorf("clients: %w", err)
	}
	if o.pipelines, err = parseInts(pipelines); err != nil {
		return nil, fmt.Errorf("pipeline: %w", err)
	}
	if o.shortRatio <= 0 || o.shortRatio >= 1 {
		return nil, fmt.Errorf("short-ratio must be in (0,1)")
	}
	return o, nil
}

func parseInts(s string) ([]int, error) {
	parts := strings.Split(s, ",")
	out := make([]int, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p == "" {
			continue
		}
		v, err := strconv.Atoi(p)
		if err != nil {
			return nil, err
		}
		out = append(out, v)
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("empty list")
	}
	return out, nil
}

// ---- bundle model ----

// hostBundle is the fingerprint a bundle records: the shape two runs must share
// before their numbers may be compared (architecture, effective CPU allowance
// and its source, memory limit, server budget), the digest of that shape, the
// validation host contract's verdict, and the properties the contract failed.
// The embedded fingerprint keeps os/arch/gold where readers of the
// pre-fingerprint bundles found them.
type hostBundle struct {
	hostcheck.Fingerprint
	Identity         string   `json:"identity"`
	Gold             bool     `json:"gold"`
	FailedProperty   string   `json:"failed_property,omitempty"`
	FailedProperties []string `json:"failed_properties,omitempty"`
}

// Assertion outcomes. The list is closed: invalid is an unmet precondition (the
// run could not reach what the assertion needs) and skipped is a run that could
// not exercise the assertion; neither is ever a pass.
const (
	outcomePass    = "pass"
	outcomeFail    = "fail"
	outcomeInvalid = "invalid"
	outcomeSkipped = "skipped"
)

// Assertion names the harness records.
const (
	assertHostContract  = "host-contract"
	assertCellsComplete = "cells-complete"
)

// assertion is one claim the run made about itself, with its outcome.
type assertion struct {
	Name    string `json:"name"`
	Outcome string `json:"outcome"`
	Detail  string `json:"detail,omitempty"`
}

// fileDigest names one measured model input, its size and its content hash: a
// model or tokenizer that changed under a baseline must be visible as a digest
// change, not as mysterious drift.
type fileDigest struct {
	Path   string `json:"path"`
	Bytes  int64  `json:"bytes"`
	SHA256 string `json:"sha256"`
}

// modelInputs are the model files this run actually measured.
type modelInputs struct {
	Name      string     `json:"name"`
	ONNX      fileDigest `json:"onnx"`
	Tokenizer fileDigest `json:"tokenizer"`
}

// harnessConfig is the run's own configuration, recorded so any cell can be
// reproduced from the bundle alone.
type harnessConfig struct {
	Platform     string  `json:"platform"`
	Model        string  `json:"model"`
	MaxLength    int     `json:"max_length"`
	CPUs         []int   `json:"cpus"`
	Clients      []int   `json:"clients"`
	Pipelines    []int   `json:"pipelines"`
	CountPerCell int     `json:"count_per_cell"`
	ReqGate      float64 `json:"req_gate"`
	P50Gate      float64 `json:"p50_gate"`
}

type benchCell struct {
	Tier     int      `json:"tier"`
	Workload string   `json:"workload"`
	Clients  int      `json:"clients"`
	Pipeline int      `json:"pipeline"`
	Reqs     float64  `json:"req_s"`
	P50      float64  `json:"p50_ms"`
	P90      float64  `json:"p90_ms"`
	P99      float64  `json:"p99_ms"`
	PadEff   *float64 `json:"padding_efficiency,omitempty"`
}

// result is the recorded evidence bundle: who measured it (revision), on what
// (host fingerprint), with what (model digests), how (config), what came out
// (cells), and what the run asserts about itself (assertions).
type result struct {
	Revision   string        `json:"revision"`
	Created    string        `json:"created"`
	Host       hostBundle    `json:"host"`
	Model      modelInputs   `json:"model"`
	Config     harnessConfig `json:"config"`
	Cells      []benchCell   `json:"cells"`
	Assertions []assertion   `json:"assertions"`
}

func tierMemory(tier int) string {
	switch tier {
	case 1:
		return "2g"
	case 2:
		return "4g"
	case 4:
		return "8g"
	default:
		return "16g"
	}
}

func main() {
	o, err := parseFlags()
	if err != nil {
		fatal("%v", err)
	}

	switch o.mode {
	case "diff":
		if o.before == "" || o.after == "" {
			fatal("diff mode requires -before and -after")
		}
		ok, err := diffBundles(o.before, o.after, o.reqGate, o.p50Gate)
		if err != nil {
			fatal("%v", err)
		}
		if !ok {
			os.Exit(1)
		}
		return
	case "run", "baseline":
	default:
		fatal("unknown mode %q (want run|baseline|diff)", o.mode)
	}

	report := hostcheck.Probe(hostcheck.DefaultThresholds())
	if err := runMain(o, report); err != nil {
		fatal("%v", err)
	}
}

// runMain prints the host report, refuses a baseline capture off an unqualified
// host before any path is created, and records the run. A run on an unqualified
// host is still recorded (marked non-gold): only a baseline is refused, because
// a baseline is the reference every later diff is measured against.
func runMain(o *options, report hostcheck.Report) error {
	fmt.Print(report.String())
	if err := baselineRefusal(o.mode, report); err != nil {
		return err
	}
	return runBenchmark(o, report)
}

// baselineRefusal returns nil when mode may persist a baseline on this host.
// Every baseline capture passes through it before a path is created or a byte
// written, so a refusal cannot leave a partial baseline behind.
func baselineRefusal(mode string, report hostcheck.Report) error {
	if mode != "baseline" || report.Qualified() {
		return nil
	}
	return fmt.Errorf("refusing to capture a baseline on an unqualified host: %s", report.FailedProperty)
}

// newBundle starts a bundle: revision, host fingerprint, model digests, the
// run's configuration, and the host-contract assertion. Cells and the
// cells-complete assertion are appended by the run itself.
func newBundle(o *options, report hostcheck.Report) (*result, error) {
	model, err := modelDigests(o)
	if err != nil {
		return nil, err
	}
	return &result{
		Revision:   gitSha(),
		Created:    time.Now().UTC().Format(time.RFC3339),
		Host:       hostBundleFrom(report),
		Model:      model,
		Config:     configFrom(o),
		Assertions: []assertion{hostContractAssertion(report)},
	}, nil
}

// finish records whether the requested matrix completed.
func (r *result) finish(expected int, runErr error) {
	r.Assertions = append(r.Assertions, cellsCompleteAssertion(expected, len(r.Cells), runErr))
}

// hostBundleFrom is the recorded fingerprint plus the contract's verdict. An
// empty failure list is recorded as absent: the bundle must be exactly what a
// JSON round trip yields, and "no properties failed" is one record, not two.
func hostBundleFrom(report hostcheck.Report) hostBundle {
	fp := report.Fingerprint()
	failed := report.FailedProperties
	if len(failed) == 0 {
		failed = nil
	}
	return hostBundle{
		Fingerprint:      fp,
		Identity:         fp.Identity(),
		Gold:             report.Qualified(),
		FailedProperty:   report.FailedProperty,
		FailedProperties: failed,
	}
}

// hostContractAssertion records the validation host contract's verdict. An
// unqualified host fails the assertion and names the property that failed.
func hostContractAssertion(report hostcheck.Report) assertion {
	if report.Qualified() {
		return assertion{
			Name:    assertHostContract,
			Outcome: outcomePass,
			Detail: fmt.Sprintf("qualified %s/%s: %.2f CPUs from %s, server budget %d",
				report.OS, report.Arch, report.Allowance.CPUs, report.Allowance.Source, report.ServerBudget),
		}
	}
	return assertion{
		Name:    assertHostContract,
		Outcome: outcomeFail,
		Detail:  report.FailedProperty,
	}
}

// cellsCompleteAssertion records whether the matrix produced every cell. A run
// that could not finish never reached the precondition a pass needs, so it is
// invalid.
func cellsCompleteAssertion(expected, produced int, runErr error) assertion {
	detail := fmt.Sprintf("%d of %d cells measured", produced, expected)
	switch {
	case runErr != nil:
		return assertion{
			Name:    assertCellsComplete,
			Outcome: outcomeInvalid,
			Detail:  fmt.Sprintf("the run could not finish the matrix: %v (%s)", runErr, detail),
		}
	case produced < expected:
		return assertion{Name: assertCellsComplete, Outcome: outcomeInvalid, Detail: detail}
	default:
		return assertion{Name: assertCellsComplete, Outcome: outcomePass, Detail: detail}
	}
}

// checkAssertions rejects an outcome outside the four the schema defines, so a
// typo cannot record an unmet precondition or an unexercised assertion as a
// pass.
func checkAssertions(as []assertion) error {
	for _, a := range as {
		switch a.Outcome {
		case outcomePass, outcomeFail, outcomeInvalid, outcomeSkipped:
		default:
			return fmt.Errorf("assertion %q has outcome %q (want pass|fail|invalid|skipped)", a.Name, a.Outcome)
		}
	}
	return nil
}

// expectedCells is the matrix the flags request: per tier, every pipeline ×
// client pair yields the two redis-benchmark workloads and a cache-hit cell, and
// every client count yields one mixed-length cell.
func expectedCells(o *options) int {
	perTier := 3*len(o.pipelines)*len(o.clients) + len(o.clients)
	return perTier * len(o.cpus)
}

// configFrom records the run's own knobs.
func configFrom(o *options) harnessConfig {
	return harnessConfig{
		Platform:     o.platform,
		Model:        o.model,
		MaxLength:    o.maxLen,
		CPUs:         o.cpus,
		Clients:      o.clients,
		Pipelines:    o.pipelines,
		CountPerCell: o.count,
		ReqGate:      o.reqGate,
		P50Gate:      o.p50Gate,
	}
}

// modelDigests hashes the model inputs this run measures.
func modelDigests(o *options) (modelInputs, error) {
	onnx, err := digestFile(filepath.Join(o.modelDir, "model.onnx"))
	if err != nil {
		return modelInputs{}, err
	}
	tok, err := digestFile(filepath.Join(o.modelDir, "tokenizer.json"))
	if err != nil {
		return modelInputs{}, err
	}
	return modelInputs{Name: o.model, ONNX: onnx, Tokenizer: tok}, nil
}

// digestFile returns the size and SHA-256 of one model input.
func digestFile(path string) (fileDigest, error) {
	// #nosec G304 -- path comes from flags; the harness control plane is trusted.
	f, err := os.Open(path)
	if err != nil {
		return fileDigest{}, err
	}
	defer func() { _ = f.Close() }()
	h := sha256.New()
	n, err := io.Copy(h, f)
	if err != nil {
		return fileDigest{}, err
	}
	return fileDigest{Path: path, Bytes: n, SHA256: hex.EncodeToString(h.Sum(nil))}, nil
}

func fatal(format string, args ...any) {
	fmt.Fprintf(os.Stderr, "error: "+format+"\n", args...)
	os.Exit(1)
}

// findBin resolves an executable by name (os.Path + nix develop PATH) with
// absolute-path fallbacks for host tools (docker/git may live outside the
// `nix develop` profile, e.g. /opt/homebrew or /usr/local).
func findBin(candidates []string) string {
	home := os.Getenv("HOME")
	for _, c := range candidates {
		if p, err := exec.LookPath(c); err == nil {
			return p
		}
		for _, dir := range []string{"/usr/local/bin", "/opt/homebrew/bin", "/usr/bin", home + "/.local/bin", "/run/current-system/sw/bin"} {
			p := filepath.Join(dir, c)
			// #nosec G703 -- fixed dir + fixed candidate name; not user input.
			if _, err := os.Stat(p); err == nil {
				return p
			}
		}
	}
	return candidates[len(candidates)-1]
}

var (
	dockerBin = findBin([]string{"docker"})
	gitBin    = findBin([]string{"git"})
)

// ---- repo / docker plumbing ----

func findRepoRoot() (string, error) {
	dir, err := os.Getwd()
	if err != nil {
		return "", err
	}
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir, nil
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return "", fmt.Errorf("go.mod not found above %s", dir)
		}
		dir = parent
	}
}

func runCmd(name string, args ...string) (string, error) {
	// #nosec G204 -- harness runs trusted tooling (docker/redis/ruby/git) by design.
	cmd := exec.Command(name, args...)
	out, err := cmd.CombinedOutput()
	return string(out), err
}

func freePort() (int, error) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return 0, err
	}
	defer ln.Close()
	return ln.Addr().(*net.TCPAddr).Port, nil
}

func gitSha() string {
	out, err := runCmd(gitBin, "rev-parse", "--short", "HEAD")
	if err != nil {
		return "unknown"
	}
	return strings.TrimSpace(out)
}

// ---- server lifecycle ----

func buildImage(o *options) error {
	if o.skipBuild {
		return nil
	}
	if err := checkDocker(); err != nil {
		return err
	}
	fmt.Printf("using docker: %s\n", dockerBin)
	fmt.Printf("building image %s (%s)...\n", o.image, o.platform)
	// #nosec G204 -- build args come from flags; harness control plane is trusted.
	out, err := exec.Command(dockerBin, "buildx", "build",
		"--platform", o.platform, "--load", "-t", o.image, ".").CombinedOutput()
	if err != nil {
		return fmt.Errorf("docker build failed: %w\n%s", err, out)
	}
	return nil
}

func checkDocker() error {
	if _, err := runCmd(dockerBin, "info"); err != nil {
		return fmt.Errorf("docker unavailable (harness needs Docker to emulate the Fargate CPU quota); looked for %s: %w", dockerBin, err)
	}
	return nil
}

func serverConfigYaml(o *options) string {
	batching := ""
	if o.batchingTimeout > 0 {
		batching = fmt.Sprintf("    batching:\n      timeout: %d\n      max_batch: %d\n      max_batch_tokens: %d\n",
			o.batchingTimeout, o.maxBatch, o.maxBatchTokens)
	} else {
		// Batching now defaults ON; emit an explicit timeout: 0 so harness
		// pool-mode configs (baselines) keep pool semantics.
		batching = "    batching:\n      timeout: 0\n"
	}
	return fmt.Sprintf(`listen: ":6379"

models:
  %s:
    onnx: /model/model.onnx
    tokenizer: /model/tokenizer.json
    max_length: %d
    pooling: mean
    normalize: true
    preload: true
%s`, o.model, o.maxLen, batching)
}

func startServer(o *options, tier int, cache bool) (port int, name string, err error) {
	if err := checkDocker(); err != nil {
		return 0, "", err
	}
	port, err = freePort()
	if err != nil {
		return 0, "", err
	}
	name = fmt.Sprintf("emb-fargate-%d-%d", os.Getpid(), port)

	cfg := serverConfigYaml(o)
	cfgPath := filepath.Join(o.outDir, fmt.Sprintf("fargate-%d.yaml", tier))
	if err := os.WriteFile(cfgPath, []byte(cfg), 0o600); err != nil {
		return 0, "", err
	}
	absModel, err := filepath.Abs(o.modelDir)
	if err != nil {
		return 0, "", err
	}
	absOut, err := filepath.Abs(o.outDir)
	if err != nil {
		return 0, "", err
	}

	args := []string{"run", "-d", "--name", name,
		"--cpus", strconv.Itoa(tier),
		"--memory", tierMemory(tier),
		// Pin GOMAXPROCS to the tier so the server's Go runtime sees the same
		// CPU quota Fargate would expose via the task cgroup. Without this, hosts
		// where the container CPU quota is invisible to Go (Docker/OrbStack on
		// macOS) run every tier with host GOMAXPROCS, so intra_op_threads and
		// fan-out defaults are computed from the wrong core count. The env mirrors
		// the gold (linux/arm64) cgroup behavior on every host.
		"-e", "GOMAXPROCS=" + strconv.Itoa(tier),
		"-p", fmt.Sprintf("%d:6379", port),
		"-v", absModel + ":/model",
		"-v", absOut + ":/etc/emb",
		"--platform", o.platform,
		"--entrypoint", "emb", o.image,
		"-config", filepath.Join("/etc/emb", fmt.Sprintf("fargate-%d.yaml", tier)),
	}
	if cache {
		args = append(args, "-cache", "auto")
	}
	out, err := runCmd(dockerBin, args...)
	if err != nil {
		return 0, "", fmt.Errorf("docker run: %w\n%s", err, out)
	}
	return port, name, nil
}

func stopServer(name string, keep bool) {
	if keep {
		return
	}
	// #nosec G204 -- cleanup of a harness-created container.
	_ = exec.Command(dockerBin, "rm", "-f", name).Run()
}

func waitReady(port int, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		out, err := runCmd("redis-cli", "-h", "127.0.0.1", "-p", strconv.Itoa(port), "EMB.READY")
		if err == nil && strings.Contains(out, "OK") {
			return nil
		}
		time.Sleep(500 * time.Millisecond)
	}
	return fmt.Errorf("server on :%d not ready within %s", port, timeout)
}

// ---- workloads ----

type bench struct {
	rps float64
	p50 float64
	p90 float64
	p95 float64
	p99 float64
}

var (
	reRps  = regexp.MustCompile(`([0-9.]+)\s+requests per second`)
	rePctS = regexp.MustCompile(`p((?:[0-9]+(?:\.[0-9]+)?))=([0-9.]+)\s+msec`)
)

func redisBench(port, clients, pipeline, count int, args ...string) (*bench, error) {
	cmdArgs := []string{"redis-benchmark", "-h", "127.0.0.1", "-p", strconv.Itoa(port),
		"-c", strconv.Itoa(clients), "-P", strconv.Itoa(pipeline), "-n", strconv.Itoa(count)}
	cmdArgs = append(cmdArgs, args...)
	out, err := runCmd(cmdArgs[0], cmdArgs[1:]...)
	if err != nil {
		return nil, fmt.Errorf("redis-benchmark: %w\n%s", err, out)
	}
	return parseBench(out)
}

func parseBench(out string) (*bench, error) {
	b := &bench{}
	if m := reRps.FindStringSubmatch(out); len(m) == 2 {
		b.rps, _ = strconv.ParseFloat(m[1], 64)
	}
	// Prefer the latency summary table (redis >= 7):
	//   latency summary (msec):
	//           avg       min       p50       p95       p99       max
	//        3.311     1.808     2.287     6.079    29.791    31.487
	lines := strings.Split(out, "\n")
	for i, ln := range lines {
		if !strings.Contains(ln, "latency summary (msec):") {
			continue
		}
		if i+2 >= len(lines) {
			break
		}
		headers := strings.Fields(lines[i+1])
		values := strings.Fields(lines[i+2])
		table := make(map[string]float64, len(headers))
		for j, h := range headers {
			if j >= len(values) {
				break
			}
			if v, err := strconv.ParseFloat(values[j], 64); err == nil {
				table[h] = v
			}
		}
		b.p50 = table["p50"]
		b.p95 = table["p95"]
		b.p99 = table["p99"]
		break
	}
	// Fallback: single-line percentiles (redis < 7), e.g. `p50=2.287 msec`.
	if b.p50 == 0 {
		pcts := map[string]float64{}
		for _, mm := range rePctS.FindAllStringSubmatch(out, -1) {
			if v, err := strconv.ParseFloat(mm[2], 64); err == nil {
				pcts[mm[1]] = v
			}
		}
		b.p50 = pcts["50"]
		b.p90 = pcts["90"]
		if b.p90 == 0 {
			b.p90 = pcts["95"]
		}
		b.p99 = pcts["99"]
	}
	// redis-benchmark does not emit p90; approximate it with p95.
	if b.p90 == 0 {
		b.p90 = b.p95
	}
	if b.rps == 0 {
		return nil, fmt.Errorf("no throughput in redis-benchmark output:\n%s", out)
	}
	return b, nil
}

func pct(sorted []float64, p float64) float64 {
	if len(sorted) == 0 {
		return 0
	}
	idx := int(p / 100.0 * float64(len(sorted)-1))
	return sorted[idx]
}

// median64 returns the median of the values (copies and sorts its input).
func median64(vals []float64) float64 {
	if len(vals) == 0 {
		return 0
	}
	sorted := append([]float64(nil), vals...)
	sort.Float64s(sorted)
	return sorted[len(sorted)/2]
}

func readLatencies(path string) ([]float64, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	lines := strings.Fields(string(data))
	out := make([]float64, 0, len(lines))
	for _, l := range lines {
		if v, err := strconv.ParseFloat(l, 64); err == nil {
			out = append(out, v)
		}
	}
	sort.Float64s(out)
	return out, nil
}

// corpus builds the deterministic mixed-length text set (80/20 short/long by
// default) and returns it. The same set is fed to the Ruby driver and to the
// tokenizer-based padding-efficiency computation.
func corpus(o *options, count int) []string {
	words := strings.Fields("apple banana cherry date elderberry fig grape honeydew kiwi lemon mango nectarine orange papaya quince raspberry strawberry tangerine ugli vanilla watermelon yam zucchini")
	texts := make([]string, count)
	for i := 0; i < count; i++ {
		if i%10 < int(o.shortRatio*10) {
			texts[i] = fmt.Sprintf("benchmark text %d", i)
			continue
		}
		rep := o.longLen/7 + 1
		var sb strings.Builder
		sb.Grow(rep * 10)
		for j := 0; j < rep; j++ {
			sb.WriteString(words[j%len(words)])
			sb.WriteByte(' ')
		}
		texts[i] = strings.TrimSpace(sb.String())
	}
	return texts
}

func paddingEfficiency(o *options, texts []string) *float64 {
	tokPath := filepath.Join(o.modelDir, "tokenizer.json")
	if _, err := os.Stat(tokPath); err != nil {
		return nil
	}
	tok, err := tokenizer.NewTokenizer(tokPath, false)
	if err != nil {
		fmt.Printf("warning: padding efficiency unavailable: %v\n", err)
		return nil
	}
	defer func() { _ = tok.Close() }()

	seqs := make([]int, len(texts))
	for i, t := range texts {
		ids, _, err := tok.Encode(t, o.maxLen)
		if err != nil {
			return nil
		}
		seqs[i] = len(ids)
	}

	real, slots := 0, 0
	for i := 0; i < len(seqs); i += o.maxBatch {
		end := i + o.maxBatch
		if end > len(seqs) {
			end = len(seqs)
		}
		windowSeqLen := 0
		for j := i; j < end; j++ {
			real += seqs[j]
			if seqs[j] > windowSeqLen {
				windowSeqLen = seqs[j]
			}
		}
		slots += (end - i) * windowSeqLen
	}
	if slots == 0 {
		return nil
	}
	eff := float64(real) / float64(slots)
	return &eff
}

// ---- orchestration ----

func runBenchmark(o *options, report hostcheck.Report) error {
	if err := os.MkdirAll(o.outDir, 0o755); err != nil {
		return err
	}
	if err := buildImage(o); err != nil {
		return err
	}
	res, err := newBundle(o, report)
	if err != nil {
		return err
	}

	expected := expectedCells(o)
	if err := runMatrix(o, res); err != nil {
		res.finish(expected, err)
		// A run that could not finish the matrix is evidence of the attempt,
		// so a `run` records it with cells-complete invalid. A baseline never
		// lands here (refused up front), and a partial one would poison every
		// later diff anyway. A write that fails is reported alongside the
		// matrix failure rather than swallowed: the record of the attempt is
		// the point of writing it at all.
		if o.mode != "baseline" {
			if werr := writeBundle(o, res); werr != nil {
				return fmt.Errorf("%w (the partial bundle could not be written: %v)", err, werr)
			}
		}
		return err
	}
	res.finish(expected, nil)
	return writeBundle(o, res)
}

// writeBundle persists the bundle, refusing to record an outcome outside the
// schema first: a bundle that got its own assertions wrong is not evidence.
func writeBundle(o *options, res *result) error {
	if err := checkAssertions(res.Assertions); err != nil {
		return err
	}
	path := resultPath(o)
	out, err := json.MarshalIndent(res, "", "  ")
	if err != nil {
		return err
	}
	if err := os.WriteFile(path, out, 0o600); err != nil {
		return err
	}
	fmt.Printf("bundle: %s (%d cells)\n", path, len(res.Cells))
	return nil
}

// runMatrix drives the workload matrix and appends a cell per measurement.
func runMatrix(o *options, res *result) error {
	for _, tier := range o.cpus {
		port, name, err := startServer(o, tier, false)
		if err != nil {
			return err
		}
		fmt.Printf("tier=%d port=%d waiting for ready...\n", tier, port)
		if err := waitReady(port, 3*time.Minute); err != nil {
			stopServer(name, o.keep)
			return err
		}
		for _, pipe := range o.pipelines {
			for _, n := range o.clients {
				if err := collectRedisCells(o, res, tier, port, "fixed-length", n, pipe); err != nil {
					stopServer(name, o.keep)
					return err
				}
				if err := collectRedisCells(o, res, tier, port, "unique-text", n, pipe); err != nil {
					stopServer(name, o.keep)
					return err
				}
			}
		}
		if err := collectMixedCell(o, res, tier, port); err != nil {
			stopServer(name, o.keep)
			return err
		}
		stopServer(name, o.keep)

		// cache-hit runs on a server launched with -cache auto
		cport, cname, err := startServer(o, tier, true)
		if err != nil {
			return err
		}
		if err := waitReady(cport, 3*time.Minute); err != nil {
			stopServer(cname, o.keep)
			return err
		}
		// warm the cache so the cell measures cache hits, not the first inference
		if _, err := runCmd("redis-cli", "-h", "127.0.0.1", "-p", strconv.Itoa(cport), "EMB", o.model, "hello world"); err != nil {
			stopServer(cname, o.keep)
			return err
		}
		for _, pipe := range o.pipelines {
			for _, n := range o.clients {
				if err := collectRedisCells(o, res, tier, cport, "cache-hit", n, pipe); err != nil {
					stopServer(cname, o.keep)
					return err
				}
			}
		}
		stopServer(cname, o.keep)
	}
	return nil
}

func collectRedisCells(o *options, res *result, tier, port int, workload string, clients, pipeline int) error {
	var text string
	switch workload {
	case "fixed-length", "cache-hit":
		text = "hello world"
	case "unique-text":
		text = "benchmark text __rand_int__"
	default:
		return fmt.Errorf("unknown redis workload %q", workload)
	}
	count := o.count
	if workload == "cache-hit" {
		// cache cells finish in milliseconds; use a count floor so the wall
		// time is long enough that sub-millisecond quantization is negligible.
		if count < 5000 {
			count = 5000
		}
	}
	var rps, p50, p90, p99 []float64
	reps := 3 // median-of-3 per the harness noise gate
	for i := 0; i < reps; i++ {
		b, err := redisBench(port, clients, pipeline, count, "EMB", o.model, text)
		if err != nil {
			return fmt.Errorf("%s tier=%d clients=%d pipe=%d: %w", workload, tier, clients, pipeline, err)
		}
		rps = append(rps, b.rps)
		p50 = append(p50, b.p50)
		p90 = append(p90, b.p90)
		p99 = append(p99, b.p99)
	}
	res.Cells = append(res.Cells, benchCell{
		Tier: tier, Workload: workload, Clients: clients, Pipeline: pipeline,
		Reqs: median64(rps), P50: median64(p50), P90: median64(p90), P99: median64(p99),
	})
	fmt.Printf("  %-12s tier=%d clients=%d pipe=%d req/s=%.0f p50=%.2f p90=%.2f p99=%.2f (median-of-3)\n",
		workload, tier, clients, pipeline, median64(rps), median64(p50), median64(p90), median64(p99))
	return nil
}

func collectMixedCell(o *options, res *result, tier, port int) error {
	count := o.count
	if o.mixedCount > 0 {
		count = o.mixedCount
	}
	texts := corpus(o, count)
	corpusPath := filepath.Join(o.outDir, fmt.Sprintf("mixed-%d.txt", tier))
	if err := os.WriteFile(corpusPath, []byte(strings.Join(texts, "\n")+"\n"), 0o600); err != nil {
		return err
	}

	for _, n := range o.clients {
		latsPath := filepath.Join(o.outDir, fmt.Sprintf("mixed-%d-%d.lats", tier, n))
		reps := 3 // median-of-3 per the harness noise gate
		var rps, p50s, p90s, p99s, effs []float64
		for i := 0; i < reps; i++ {
			// #nosec G204 -- corpus/count/model from flags; harness control plane trusted.
			cmd := exec.Command("ruby", filepath.Join("bench", "fargate", "load.rb"),
				"--port", strconv.Itoa(port), "--clients", strconv.Itoa(n), "--count", strconv.Itoa(count),
				"--model", o.model, "--corpus", corpusPath, "--latencies", latsPath)
			out, err := cmd.CombinedOutput()
			if err != nil {
				return fmt.Errorf("mixed workload tier=%d clients=%d: %w\n%s", tier, n, err, out)
			}
			lats, err := readLatencies(latsPath)
			if err != nil {
				return err
			}
			wall := 0.0
			for _, l := range lats {
				wall += l
			}
			rps = append(rps, float64(len(lats))/(wall/1000.0))
			p50s = append(p50s, pct(lats, 50))
			p90s = append(p90s, pct(lats, 90))
			p99s = append(p99s, pct(lats, 99))
			eff := paddingEfficiency(o, texts)
			if eff != nil {
				effs = append(effs, *eff)
			}
		}
		var effPtr *float64
		if len(effs) > 0 {
			v := median64(effs)
			effPtr = &v
		}
		res.Cells = append(res.Cells, benchCell{
			Tier: tier, Workload: "mixed-length", Clients: n, Pipeline: 1,
			Reqs: median64(rps), P50: median64(p50s), P90: median64(p90s), P99: median64(p99s),
			PadEff: effPtr,
		})
		ev := 0.0
		if effPtr != nil {
			ev = *effPtr
		}
		fmt.Printf("  %-12s tier=%d clients=%d req/s=%.0f p50=%.2f p90=%.2f p99=%.2f pad_eff=%.2f (median-of-3)\n",
			"mixed-length", tier, n, median64(rps), median64(p50s), median64(p90s), median64(p99s), ev)
	}
	return nil
}

func resultPath(o *options) string {
	sha := gitSha()
	if o.mode == "baseline" {
		repoRoot, err := findRepoRoot()
		if err != nil {
			return filepath.Join(o.outDir, "baseline."+sha+".json")
		}
		return filepath.Join(repoRoot, "bench", "fargate", "baseline."+sha+".json")
	}
	return filepath.Join(o.outDir, fmt.Sprintf("run.%s.%d.json", sha, time.Now().Unix()))
}

// ---- diff ----

func loadResult(path string) (*result, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var r result
	if err := json.Unmarshal(data, &r); err != nil {
		return nil, err
	}
	return &r, nil
}

func cellKey(c benchCell) string {
	return fmt.Sprintf("%d|%s|%d|%d", c.Tier, c.Workload, c.Clients, c.Pipeline)
}

func delta(b, a float64) float64 {
	if b == 0 {
		return 0
	}
	return (a - b) / b * 100
}

// diffBundles is -mode diff minus flags and exit codes: it loads both bundles,
// refuses an incomparable pair (printing both fingerprints), and applies the
// per-cell tolerances. The bool is the cell verdict.
func diffBundles(beforePath, afterPath string, reqGate, p50Gate float64) (bool, error) {
	before, err := loadResult(beforePath)
	if err != nil {
		return false, err
	}
	after, err := loadResult(afterPath)
	if err != nil {
		return false, err
	}
	detail, err := compareHosts(beforePath, before, afterPath, after)
	if err != nil {
		fmt.Print(detail)
		return false, err
	}
	return diffRun(before, after, reqGate, p50Gate), nil
}

// compareHosts decides whether two bundles may be compared. It refuses a pair
// whose fingerprint identities differ, and a legacy bundle that carries no
// fingerprint at all. Both refusals return the text to print, which names both
// fingerprints field by field: comparing numbers across two hosts is a
// convention until the tooling refuses to do it.
func compareHosts(beforePath string, before *result, afterPath string, after *result) (string, error) {
	detail := fingerprintText(beforePath, before.Host) + fingerprintText(afterPath, after.Host)
	switch {
	case before.Host.Identity == "" && after.Host.Identity == "":
		return detail, fmt.Errorf("refusing to compare %s and %s: neither carries a host fingerprint (legacy schema); re-record both on a qualified host", beforePath, afterPath)
	case before.Host.Identity == "":
		return detail, fmt.Errorf("refusing to compare: %s carries no host fingerprint (legacy schema recorded before fingerprints); re-record it on a qualified host", beforePath)
	case after.Host.Identity == "":
		return detail, fmt.Errorf("refusing to compare: %s carries no host fingerprint (legacy schema recorded before fingerprints); re-record it on a qualified host", afterPath)
	case before.Host.Identity != after.Host.Identity:
		return detail, fmt.Errorf("refusing to compare %s and %s: host fingerprints differ (%s vs %s)", beforePath, afterPath, before.Host.Identity, after.Host.Identity)
	default:
		return detail, nil
	}
}

// fingerprintText renders one bundle's fingerprint field by field, so a refusal
// shows what actually differed instead of only two opaque digests.
func fingerprintText(label string, h hostBundle) string {
	var b strings.Builder
	fmt.Fprintf(&b, "%s\n", label)
	fmt.Fprintf(&b, "  os/arch         %s/%s\n", h.OS, h.Arch)
	fmt.Fprintf(&b, "  presented CPUs  %d\n", h.PresentedCPUs)
	fmt.Fprintf(&b, "  allowance       %.2f CPUs from %s (quota=%t, scheduler=%d, affinity=%d)\n",
		h.Allowance.CPUs, h.Allowance.Source, h.Allowance.Quota, h.Allowance.Scheduler, h.Allowance.Affinity)
	fmt.Fprintf(&b, "  memory limit    %d bytes\n", h.MemoryLimit)
	fmt.Fprintf(&b, "  server budget   %d\n", h.ServerBudget)
	fmt.Fprintf(&b, "  steal/throttle  %s / %s\n", h.StealVerdict, h.ThrottleVerdict)
	fmt.Fprintf(&b, "  gold            %t\n", h.Gold)
	if h.FailedProperty != "" {
		fmt.Fprintf(&b, "  failed property %s\n", h.FailedProperty)
	}
	fmt.Fprintf(&b, "  identity        %q\n", h.Identity)
	return b.String()
}

func diffRun(before, after *result, reqGate, p50Gate float64) bool {
	afterIdx := make(map[string]benchCell, len(after.Cells))
	for _, c := range after.Cells {
		afterIdx[cellKey(c)] = c
	}
	beforeIdx := make(map[string]benchCell, len(before.Cells))
	for _, c := range before.Cells {
		beforeIdx[cellKey(c)] = c
	}
	keys := make([]string, 0, len(before.Cells))
	for k := range beforeIdx {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	fmt.Printf("%-4s %-12s %3s %4s %11s %11s %7s %9s %9s %7s  %s\n",
		"tier", "workload", "cl", "pipe", "req/s(b)", "req/s(a)", "delta%", "p50(b)", "p50(a)", "delta%", "verdict")

	pass, fail := 0, 0
	for _, k := range keys {
		b := beforeIdx[k]
		a, ok := afterIdx[k]
		if !ok {
			c := beforeIdx[k]
			fmt.Printf("%-4d %-12s %3d %4d   MISSING IN AFTER\n", c.Tier, c.Workload, c.Clients, c.Pipeline)
			fail++
			continue
		}
		dReq := delta(b.Reqs, a.Reqs)
		dP50 := delta(b.P50, a.P50)
		verdict := "PASS"
		// Cache-saturated cells (e.g. cache-hit at >5000 req/s) finish in
		// sub-millisecond to tens-of-ms windows where timer quantization
		// dominates; req/s is not a stable capacity measure there. Gate those
		// on p50 only. Inference-bound cells gate on both req/s and p50.
		requireReqs := b.Reqs < 5000
		// req/s regressions beyond the gate fail; p50 regressions beyond the
		// gate fail. Improvements pass regardless of magnitude.
		if (requireReqs && dReq < -reqGate*100) || (dP50 > p50Gate*100) {
			verdict = "FAIL"
			fail++
		} else {
			pass++
		}
		fmt.Printf("%-4d %-12s %3d %4d %11.1f %11.1f %+6.1f%% %9.2f %9.2f %+6.1f%%  %s\n",
			a.Tier, a.Workload, a.Clients, a.Pipeline,
			b.Reqs, a.Reqs, dReq, b.P50, a.P50, dP50, verdict)
	}
	_ = pct
	fmt.Printf("diff: %d pass, %d fail\n", pass, fail)
	if fail > 0 {
		fmt.Printf("FAIL: %d cells outside tolerances (inference cells: req/s >= -%.0f%%, p50 <= +%.0f%%; saturated cells: p50 <= +%.0f%%)\n",
			fail, reqGate*100, p50Gate*100, p50Gate*100)
		return false
	}
	return true
}
