package main

// -mode drive: one open-loop pass of the pair driver against an external,
// already-running server, recorded as a run in a regression bundle. The server
// is whatever artifact the orchestrator started (known-good, pre4, candidate)
// through its production startup path; the driver only speaks RESP to it and
// reads its EMB.STATS/EMB.INFO counters.

import (
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"os"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/elcuervo/emb/bench/fargate/driver"
	"github.com/elcuervo/emb/bench/hostcheck"
)

// driveWarmSize is the warm corpus size: three classes, three texts each.
const driveWarmSize = 9

type driveOptions struct {
	addr           string
	models         string
	dispatch       string
	cacheState     string
	rate           float64
	duration       time.Duration
	requestTimeout time.Duration
	drainTimeout   time.Duration
	maxInFlight    int
	dim            int
	phase          string
	round          int
	cpuLimit       string
	profile        string
	bundle         string
	meta           string
	info           bool
	orchestrator   string
	expectFail     string
	seed           int64
}

func (d *driveOptions) register() {
	flag.StringVar(&d.addr, "addr", "", "server address host:port (drive mode)")
	flag.StringVar(&d.models, "models", "minilm,bge", "comma-separated models, one EMB leg each (drive mode)")
	flag.StringVar(&d.dispatch, "dispatch", "parallel", "single | serial | parallel (drive mode)")
	flag.StringVar(&d.cacheState, "cache-state", driver.CacheWarm, "warm | cold (drive mode)")
	flag.Float64Var(&d.rate, "rate", 10, "offered pairs per second (drive mode)")
	flag.DurationVar(&d.duration, "duration", 120*time.Second, "scheduling window (drive mode)")
	flag.DurationVar(&d.requestTimeout, "request-timeout", 10*time.Second, "per-pair deadline from wire start (drive mode)")
	flag.DurationVar(&d.drainTimeout, "drain-timeout", 10*time.Second, "final drain bound after the window (drive mode)")
	flag.IntVar(&d.maxInFlight, "max-in-flight", 8, "pairs in flight before a scheduled pair is dropped (drive mode)")
	flag.IntVar(&d.dim, "dim", 384, "expected embedding dimension (drive mode)")
	flag.StringVar(&d.phase, "phase", "", "profile phase this pass records (drive mode)")
	flag.IntVar(&d.round, "round", 1, "interleaved round number, from 1 (drive mode)")
	flag.StringVar(&d.cpuLimit, "cpu-limit", "", "CPU bandwidth label the server runs under, e.g. 0.5 (drive mode)")
	flag.StringVar(&d.profile, "profile", "", "regression profile JSON (drive and compare modes)")
	flag.StringVar(&d.bundle, "bundle", "", "regression bundle JSON to create or append to (drive mode)")
	flag.StringVar(&d.meta, "meta", "", "arm metadata JSON: role, mode, approval, artifact, config, models (drive mode)")
	flag.BoolVar(&d.info, "info", true, "observe EMB.INFO per model; false for cold-first-use, since EMB.INFO initializes a lazy model (drive mode)")
	flag.StringVar(&d.orchestrator, "orchestrator", "", "k=v,... from the supervisor: restarts, oom_kills, throttled_periods, recovery_seconds, ready_seconds (drive mode)")
	flag.StringVar(&d.expectFail, "expect-fail", "", "compare mode: the check a known-bad arm must fail (and nothing invalid)")
	flag.Int64Var(&d.seed, "seed", 20261012, "corpus seed (drive mode)")
}

// armMeta is what only the person starting the arm knows.
type armMeta struct {
	Role       string      `json:"role"`
	Mode       string      `json:"mode"`
	Approval   approval    `json:"approval"`
	Artifact   artifact    `json:"artifact"`
	Runtime    runtimeInfo `json:"runtime"`
	ConfigFile string      `json:"config_file"`
	Models     []struct {
		Name string `json:"name"`
		Dir  string `json:"dir"` // holds model.onnx and tokenizer.json
	} `json:"models"`
}

func loadMeta(path string) (armMeta, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return armMeta{}, err
	}
	var m armMeta
	dec := json.NewDecoder(strings.NewReader(string(raw)))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&m); err != nil {
		return armMeta{}, fmt.Errorf("meta %s: %w", path, err)
	}
	return m, nil
}

// parseOrchestrator reads k=v pairs; an unknown key is an error, not ignored.
func parseOrchestrator(s string) (map[string]float64, error) {
	out := map[string]float64{}
	for _, kv := range strings.Split(s, ",") {
		if kv = strings.TrimSpace(kv); kv == "" {
			continue
		}
		k, v, ok := strings.Cut(kv, "=")
		f, err := strconv.ParseFloat(v, 64)
		if !ok || err != nil || !slices.Contains([]string{"restarts", "oom_kills", "throttled_periods", "recovery_seconds", "ready_seconds"}, k) {
			return nil, fmt.Errorf("bad -orchestrator entry %q", kv)
		}
		out[k] = f
	}
	return out, nil
}

// armInputsFrom hashes the model, tokenizer and config files the arm serves.
func armInputsFrom(m armMeta, corpusDigest string) (armInputs, error) {
	in := armInputs{CorpusSHA256: corpusDigest}
	for _, mm := range m.Models {
		onnx, err := digestFile(mm.Dir + "/model.onnx")
		if err != nil {
			return in, err
		}
		tok, err := digestFile(mm.Dir + "/tokenizer.json")
		if err != nil {
			return in, err
		}
		in.Models = append(in.Models, modelInputs{Name: mm.Name, ONNX: onnx, Tokenizer: tok})
	}
	cfg, err := digestFile(m.ConfigFile)
	if err != nil {
		return in, err
	}
	in.ConfigSHA256 = cfg.SHA256
	return in, nil
}

func orUnavailable(s string) string {
	if s == "" {
		return unavailable
	}
	return s
}

func runDrive(o *options) error {
	d := o.drive
	if d.addr == "" || d.phase == "" || d.cpuLimit == "" || d.profile == "" || d.bundle == "" || d.meta == "" {
		return errors.New("drive mode requires -addr, -phase, -cpu-limit, -profile, -bundle and -meta")
	}
	p, err := loadProfile(d.profile)
	if err != nil {
		return err
	}
	phase := slices.IndexFunc(p.Phases, func(ph phaseSpec) bool { return ph.Name == d.phase })
	if phase < 0 || !slices.Contains(p.cpusFor(p.Phases[phase]), d.cpuLimit) {
		return fmt.Errorf("phase %q at %q CPUs is not in profile %s's required matrix", d.phase, d.cpuLimit, p.Name)
	}
	meta, err := loadMeta(d.meta)
	if err != nil {
		return err
	}
	orch, err := parseOrchestrator(d.orchestrator)
	if err != nil {
		return err
	}

	models := strings.Split(d.models, ",")
	cfg := driver.Config{
		Addr: d.addr, Models: models, Dispatch: driver.Dispatch(d.dispatch), Rate: d.rate, Duration: d.duration,
		MaxInFlight: d.maxInFlight, RequestTimeout: d.requestTimeout, DrainTimeout: d.drainTimeout, Dim: d.dim,
	}
	if len(models) == 1 {
		cfg.Dispatch = driver.Single
	}
	corpus := driver.NewCorpus(d.seed, driveWarmSize, strconv.FormatInt(time.Now().UnixNano(), 36))
	inputs, err := armInputsFrom(meta, corpus.Digest())
	if err != nil {
		return err
	}

	pr, runErr := driver.RunPhase(driver.PhaseConfig{Config: cfg, Cache: d.cacheState, Corpus: corpus, Info: d.info})
	if runErr != nil && pr.Result.Offered == 0 {
		return runErr // never reached the server: nothing to record
	}
	run := phaseRunFrom(runSpec{Round: d.round, Phase: d.phase, CPUs: d.cpuLimit, Dispatch: string(cfg.Dispatch), Rate: d.rate, Orchestrator: orch}, pr)
	fmt.Printf("%s round %d: offered=%d success=%d errors=%d timeouts=%d dropped=%d pairs/s=%.2f p50=%.1fms p95=%.1fms p99=%.1fms\n",
		runKey(run.Phase, run.CPUs), run.Round, run.Offered, run.Success, run.Errors, run.Timeouts, run.Dropped,
		run.PairsPerSecond, run.Scheduled.P50, run.Scheduled.P95, run.Scheduled.P99)

	res, err := loadOrNewRegressionBundle(d.bundle, p, meta, inputs)
	if err != nil {
		return err
	}
	e := res.Regression
	for _, r := range e.Runs {
		if r.Phase == run.Phase && r.CPUs == run.CPUs && r.Round == run.Round {
			return fmt.Errorf("%s round %d is already recorded in %s", runKey(run.Phase, run.CPUs), run.Round, d.bundle)
		}
	}
	limits := observedLimits(pr.After.Stats)
	for k, v := range limits {
		if old := e.Limits[k]; old != "" && old != unavailable && v != unavailable && old != v {
			return fmt.Errorf("server-observed %s changed within the arm (%s -> %s); record a new bundle", k, old, v)
		}
	}
	e.Limits = limits
	ready := missing("the driver attached to a running server; supply ready_seconds from the orchestrator")
	if v, ok := orch["ready_seconds"]; ok {
		ready = have(v)
	}
	if len(pr.After.Models) > 0 || !e.Startup.ReadySeconds.ok() {
		e.Startup = startupFrom(pr.After.Models, ready)
	}
	e.Runs = append(e.Runs, run)
	if err := writeRegressionBundle(d.bundle, res); err != nil {
		return err
	}
	fmt.Printf("bundle: %s (%d runs)\n", d.bundle, len(e.Runs))

	// A failing run is retained above; it still fails the command.
	var failed []string
	for _, a := range run.Assertions {
		if a.Outcome == outcomeFail || a.Outcome == outcomeInvalid {
			failed = append(failed, fmt.Sprintf("%s %s: %s", a.Outcome, a.Name, a.Detail))
		}
	}
	if runErr != nil {
		failed = append(failed, runErr.Error())
	}
	if len(failed) > 0 {
		return fmt.Errorf("run recorded with failing assertions: %s", strings.Join(failed, "; "))
	}
	return nil
}

// loadOrNewRegressionBundle opens the arm's bundle, refusing to mix a different
// arm into it, or starts one with the driver host's contract verdict.
func loadOrNewRegressionBundle(path string, p profile, m armMeta, in armInputs) (*result, error) {
	runtime := m.Runtime
	runtime.GoVersion, runtime.ORTVersion = orUnavailable(runtime.GoVersion), orUnavailable(runtime.ORTVersion)
	if runtime.CPUModel == "" {
		runtime.CPUModel = cpuModel()
	}
	want := &regressionEvidence{
		Role: m.Role, Mode: m.Mode, Approval: m.Approval, Profile: p.Name,
		Artifact: m.Artifact, Runtime: runtime, Inputs: in,
	}
	if _, err := os.Stat(path); err == nil {
		res, err := loadResult(path)
		if err != nil {
			return nil, err
		}
		got := res.Regression
		if got == nil || res.Schema != bundleSchema {
			return nil, fmt.Errorf("%s is not a regression bundle", path)
		}
		if got.Role != want.Role || got.Mode != want.Mode || got.Profile != want.Profile || got.Artifact != want.Artifact ||
			got.Approval != want.Approval || got.Inputs.ConfigSHA256 != in.ConfigSHA256 || got.Inputs.CorpusSHA256 != in.CorpusSHA256 ||
			!slices.Equal(modelHashes(got.Inputs), modelHashes(in)) {
			return nil, fmt.Errorf("%s was recorded for a different arm, profile or input set; use a new bundle", path)
		}
		return res, nil
	}
	report := hostcheck.Probe(hostcheck.DefaultThresholds())
	res := &result{
		Revision:   gitSha(),
		Created:    time.Now().UTC().Format(time.RFC3339),
		Host:       hostBundleFrom(report),
		Assertions: []assertion{hostContractAssertion(report)},
		Schema:     bundleSchema,
		Regression: want,
	}
	if len(in.Models) > 0 {
		res.Model = in.Models[0]
	}
	return res, nil
}

func modelHashes(in armInputs) []string {
	var out []string
	for _, m := range in.Models {
		out = append(out, m.Name+":"+m.ONNX.SHA256+":"+m.Tokenizer.SHA256)
	}
	return out
}

func writeRegressionBundle(path string, res *result) error {
	if err := checkAssertions(res.Assertions); err != nil {
		return err
	}
	for _, r := range res.Regression.Runs {
		if err := checkAssertions(r.Assertions); err != nil {
			return err
		}
	}
	out, err := json.MarshalIndent(res, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, out, 0o600)
}
