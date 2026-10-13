package main

// Regression evidence: the part of the bundle that the release comparator reads.
// One bundle describes one arm (known-good, known-bad or candidate) of an
// interleaved comparison: where its artifact came from, which inputs it measured,
// which limits the server itself observed, and one phaseRun per round x phase x
// CPU bandwidth. Everything a run could not observe is recorded unavailable with
// the reason, never as zero.

import (
	"fmt"
	"os"
	"strconv"
	"strings"

	"github.com/elcuervo/emb/bench/fargate/driver"
)

// bundleSchema versions the regression section. A bundle without it (the
// pre-regression cell bundles) carries no regression evidence and is never
// accepted by the comparator.
const bundleSchema = 2

// unavailable marks a string field an older release or the driver cannot supply.
const unavailable = "unavailable"

// Arm roles and evidence modes.
const (
	roleKnownGood = "known-good"
	roleKnownBad  = "known-bad"
	roleCandidate = "candidate"

	modeReference               = "reference"
	modeConstrainedReproduction = "constrained-reproduction"
)

// metric is a measurement that may be absent. The zero value is unavailable, so
// a forgotten field can never read as a measured zero.
type metric struct {
	Value       *float64 `json:"value,omitempty"`
	Unavailable string   `json:"unavailable,omitempty"`
}

func have(v float64) metric { return metric{Value: &v} }

func missing(reason string) metric { return metric{Unavailable: reason} }

func (m metric) ok() bool { return m.Value != nil }

// val is the measured value; callers check ok first.
func (m metric) val() float64 {
	if m.Value == nil {
		return 0
	}
	return *m.Value
}

type approval struct {
	Approved  bool   `json:"approved"`
	Reference string `json:"reference,omitempty"` // what approved it, e.g. a digest comparison record
}

type artifact struct {
	Image    string `json:"image,omitempty"`
	Revision string `json:"revision,omitempty"`
	Digest   string `json:"digest"` // binary or image digest
}

type runtimeInfo struct {
	GoVersion  string `json:"go_version"`
	ORTVersion string `json:"ort_version"`
	CPUModel   string `json:"cpu_model"`
}

// armInputs are the hashes two arms must share to be comparable.
type armInputs struct {
	Models       []modelInputs `json:"models"`
	ConfigSHA256 string        `json:"config_sha256"`
	CorpusSHA256 string        `json:"corpus_sha256"`
}

type startup struct {
	ReadySeconds metric                       `json:"ready_seconds"`
	Layouts      map[string]map[string]string `json:"layouts,omitempty"` // per model, from EMB.INFO
	Calibration  string                       `json:"calibration"`       // source/reason/duration, or unavailable
}

// phaseRun is one round of one phase at one CPU bandwidth.
type phaseRun struct {
	Round    int     `json:"round"`
	Phase    string  `json:"phase"`
	CPUs     string  `json:"cpus"` // bandwidth label, e.g. "0.5"
	Dispatch string  `json:"dispatch"`
	Cache    string  `json:"cache"`
	Rate     float64 `json:"offered_rate"`

	Offered        int64   `json:"offered"`
	Success        int64   `json:"success"`
	Errors         int64   `json:"errors"`
	Timeouts       int64   `json:"timeouts"`
	Dropped        int64   `json:"dropped"`
	BusyRejections int64   `json:"busy_rejections"`
	Seconds        float64 `json:"seconds"`
	PairsPerSecond float64 `json:"pairs_per_second"`

	Scheduled  driver.Quantiles            `json:"pair_latency_from_scheduled"`
	Wire       driver.Quantiles            `json:"pair_latency_from_wire_start"`
	Legs       map[string]driver.LegResult `json:"legs"`
	ErrorKinds map[string]int64            `json:"error_kinds"`

	Inference           map[string]map[string]int64 `json:"inference_counters,omitempty"` // per model, EMB.INFO deltas
	CacheHits           metric                      `json:"cache_hits"`
	CacheMisses         metric                      `json:"cache_misses"`
	AdmissionWaitUs     metric                      `json:"admission_wait_us"`
	EmbedRunUs          metric                      `json:"embed_run_us"`
	EmbedDispatchWaitUs metric                      `json:"embed_dispatch_wait_us"`
	CPUSeconds          metric                      `json:"cpu_seconds"`
	ThrottledPeriods    metric                      `json:"throttled_periods"`
	RSSMB               metric                      `json:"rss_mb"`
	Restarts            metric                      `json:"restarts"`
	OOMKills            metric                      `json:"oom_kills"`
	RecoverySeconds     metric                      `json:"recovery_seconds"`

	Assertions []assertion `json:"assertions"`
}

// regressionEvidence is the arm-level record the comparator consumes.
type regressionEvidence struct {
	Role     string            `json:"role"`
	Mode     string            `json:"mode"`
	Approval approval          `json:"approval"`
	Profile  string            `json:"profile"`
	Artifact artifact          `json:"artifact"`
	Runtime  runtimeInfo       `json:"runtime"`
	Inputs   armInputs         `json:"inputs"`
	Limits   map[string]string `json:"observed_limits"` // EMB.STATS limits as the server reported them
	Startup  startup           `json:"startup"`
	Runs     []phaseRun        `json:"runs"`
}

// Run assertion names.
const (
	assertOfferedAccounted = "offered-accounted"
	assertRepliesValid     = "replies-valid"
	assertGovernorEnabled  = "governor-enabled"
	assertCacheExpectation = "cache-expectation"
	assertNoRestartOOM     = "no-restart-oom"
)

// limitKeys are the EMB.STATS fields recorded as the server-observed limits.
var limitKeys = []string{
	"cpu_budget_enabled", "cpu_allowance", "cpu_allowance_source", "cpu_capacity",
	"cpu_reserve", "cpu_budget", "max_connections", "max_concurrent_requests",
}

// layoutKeys are the EMB.INFO fields recorded as a model's selected layout.
var layoutKeys = []string{"workers", "intra_op_threads", "cpu_thread_share", "cpu_budget", "batch_determinism", "batching_timeout_ms"}

// calibrationKeys are the EMB.INFO fields that describe calibration, once the
// server exposes them; older releases do not, and the bundle says so.
var calibrationKeys = []string{"calibration_source", "calibration_reason", "calibration_duration_ms", "calibration_ceiling"}

// observedLimits copies the server's limit fields; absent fields are unavailable.
func observedLimits(stats map[string]string) map[string]string {
	out := make(map[string]string, len(limitKeys))
	for _, k := range limitKeys {
		if v, ok := stats[k]; ok {
			out[k] = v
		} else {
			out[k] = unavailable
		}
	}
	return out
}

// startupFrom records layouts and calibration metadata from an EMB.INFO read.
func startupFrom(models map[string]map[string]string, ready metric) startup {
	s := startup{ReadySeconds: ready, Layouts: map[string]map[string]string{}, Calibration: unavailable}
	var cal []string
	for name, info := range models {
		layout := map[string]string{}
		for _, k := range layoutKeys {
			if v, ok := info[k]; ok {
				layout[k] = v
			}
		}
		s.Layouts[name] = layout
		for _, k := range calibrationKeys {
			if v, ok := info[k]; ok {
				cal = append(cal, name+"."+k+"="+v)
			}
		}
	}
	if len(cal) > 0 {
		s.Calibration = strings.Join(cal, " ")
	}
	return s
}

// runSpec names the cell a driven pass belongs to.
type runSpec struct {
	Round    int
	Phase    string
	CPUs     string
	Dispatch string
	Rate     float64
	// Orchestrator holds what only the process supervisor can see: restarts,
	// oom_kills, throttled_periods and recovery_seconds.
	Orchestrator map[string]float64
}

func statMetric(c driver.Counters, key string) metric {
	if v, ok := c.Stats[key]; ok {
		return have(float64(v))
	}
	return missing("the server does not report " + key)
}

func modelSum(c driver.Counters, key string) metric {
	if len(c.Models) == 0 {
		return missing("EMB.INFO was not observed for this phase")
	}
	var sum int64
	for name, m := range c.Models {
		v, ok := m[key]
		if !ok {
			return missing("EMB.INFO for " + name + " does not report " + key)
		}
		sum += v
	}
	return have(float64(sum))
}

func orchestratorMetric(o map[string]float64, key string) metric {
	if v, ok := o[key]; ok {
		return have(v)
	}
	return missing("not observable by the driver; supplied by the orchestrator when available")
}

// phaseRunFrom turns a driven pass into one bundle run with its named assertions.
func phaseRunFrom(s runSpec, pr driver.PhaseResult) phaseRun {
	r := pr.Result
	run := phaseRun{
		Round: s.Round, Phase: s.Phase, CPUs: s.CPUs, Dispatch: s.Dispatch, Cache: pr.Cache, Rate: s.Rate,
		Offered: r.Offered, Success: r.Success, Errors: r.Errors, Timeouts: r.Timeouts, Dropped: r.Dropped,
		BusyRejections: r.ErrorKinds[driver.KindBusy],
		Seconds:        r.Seconds, PairsPerSecond: r.Throughput,
		Scheduled: r.Scheduled, Wire: r.Wire, Legs: r.Legs, ErrorKinds: r.ErrorKinds,
		Inference:           pr.Counters.Models,
		CacheHits:           statMetric(pr.Counters, "cache_hits"),
		CacheMisses:         statMetric(pr.Counters, "cache_misses"),
		AdmissionWaitUs:     statMetric(pr.Counters, "cpu_admission_wait_us"),
		EmbedRunUs:          modelSum(pr.Counters, "embed_run_us"),
		EmbedDispatchWaitUs: modelSum(pr.Counters, "embed_dispatch_wait_us"),
		ThrottledPeriods:    orchestratorMetric(s.Orchestrator, "throttled_periods"),
		Restarts:            orchestratorMetric(s.Orchestrator, "restarts"),
		OOMKills:            orchestratorMetric(s.Orchestrator, "oom_kills"),
		RecoverySeconds:     orchestratorMetric(s.Orchestrator, "recovery_seconds"),
	}
	user, okU := pr.Counters.Stats["cpu_user_usec"]
	sys, okS := pr.Counters.Stats["cpu_sys_usec"]
	if okU && okS {
		run.CPUSeconds = have(float64(user+sys) / 1e6)
	} else {
		run.CPUSeconds = missing("the server does not report cpu_user_usec/cpu_sys_usec")
	}
	if mem, err := strconv.ParseFloat(pr.After.Stats["mem"], 64); err == nil {
		run.RSSMB = have(mem)
	} else {
		run.RSSMB = missing("the server does not report mem")
	}
	run.Assertions = runAssertions(run, pr)
	return run
}

// runAssertions are the claims a run makes about itself at record time.
func runAssertions(run phaseRun, pr driver.PhaseResult) []assertion {
	var out []assertion
	if err := pr.Result.Accounted(); err != nil {
		out = append(out, assertion{Name: assertOfferedAccounted, Outcome: outcomeFail, Detail: err.Error()})
	} else {
		out = append(out, assertion{Name: assertOfferedAccounted, Outcome: outcomePass, Detail: fmt.Sprintf("%d offered, all classified", run.Offered)})
	}

	// Every reply the server sent must be a valid embedding: an application
	// error, null or malformed blob is a failed run even when throughput looks fine.
	var bad int64
	for _, k := range []string{driver.KindServer, driver.KindNull, driver.KindMalformed, driver.KindNonFinite} {
		bad += run.ErrorKinds[k]
	}
	switch {
	case bad > 0:
		out = append(out, assertion{Name: assertRepliesValid, Outcome: outcomeFail, Detail: fmt.Sprintf("%d invalid replies %v", bad, run.ErrorKinds)})
	case run.Success == 0:
		out = append(out, assertion{Name: assertRepliesValid, Outcome: outcomeFail, Detail: "no valid embedding was returned"})
	default:
		out = append(out, assertion{Name: assertRepliesValid, Outcome: outcomePass, Detail: fmt.Sprintf("%d pairs returned valid embeddings", run.Success)})
	}

	switch enabled := pr.After.Stats["cpu_budget_enabled"]; enabled {
	case "1":
		out = append(out, assertion{Name: assertGovernorEnabled, Outcome: outcomePass, Detail: "cpu_budget_enabled 1"})
	case "":
		out = append(out, assertion{Name: assertGovernorEnabled, Outcome: outcomeInvalid, Detail: "the server does not report cpu_budget_enabled"})
	default:
		out = append(out, assertion{Name: assertGovernorEnabled, Outcome: outcomeFail, Detail: "cpu_budget_enabled " + enabled})
	}

	if err := pr.CheckCache(); err != nil {
		out = append(out, assertion{Name: assertCacheExpectation, Outcome: outcomeFail, Detail: err.Error()})
	} else {
		out = append(out, assertion{Name: assertCacheExpectation, Outcome: outcomePass, Detail: pr.Cache + " phase matches the server's cache counters"})
	}

	if run.Restarts.ok() && run.OOMKills.ok() {
		if run.Restarts.val() == 0 && run.OOMKills.val() == 0 {
			out = append(out, assertion{Name: assertNoRestartOOM, Outcome: outcomePass, Detail: "no restart or OOM kill"})
		} else {
			out = append(out, assertion{Name: assertNoRestartOOM, Outcome: outcomeFail, Detail: fmt.Sprintf("%v restarts, %v OOM kills", run.Restarts.val(), run.OOMKills.val())})
		}
	} else {
		out = append(out, assertion{Name: assertNoRestartOOM, Outcome: outcomeSkipped, Detail: "restart/OOM counters were not supplied"})
	}
	return out
}

// cpuModel names the CPU the driver host reports; the server's CPU is the same
// machine only when the driver shares it, so the bundle records where it read it.
func cpuModel() string {
	if raw, err := os.ReadFile("/proc/cpuinfo"); err == nil {
		for _, line := range strings.Split(string(raw), "\n") {
			if k, v, ok := strings.Cut(line, ":"); ok && strings.HasPrefix(strings.TrimSpace(k), "model name") {
				return strings.TrimSpace(v)
			}
		}
	}
	if out, err := runCmd("sysctl", "-n", "machdep.cpu.brand_string"); err == nil && strings.TrimSpace(out) != "" {
		return strings.TrimSpace(out)
	}
	return unavailable + ": no /proc/cpuinfo model name or sysctl brand string"
}
