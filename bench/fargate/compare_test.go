package main

// Tests for the release comparator: structural rejections, the statistical
// verdict, and the harness self-verification cases. Every bundle is synthetic;
// no server, model, Docker or network is involved.

import (
	"encoding/json"
	"net"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/elcuervo/emb/bench/fargate/driver"
)

var requiredNames = []string{assertOfferedAccounted, assertRepliesValid, assertGovernorEnabled, assertCacheExpectation}

func testProfile() profile {
	return profile{
		Name: "control",
		CPUs: []string{"1", "2"},
		Phases: []phaseSpec{
			{Name: "steady", Gated: true, Sustainable: true},
			{Name: "burst"}, // overload: not sustainable, busy replies tolerated
			{Name: "recovery", RecoveryDeadlineSeconds: 30},
		},
		Deadlines:          deadlines{RequestSeconds: 10, HealthSeconds: 5, RecoverySeconds: 30},
		RequiredAssertions: requiredNames,
	}
}

func passAssertions() []assertion {
	var as []assertion
	for _, n := range requiredNames {
		as = append(as, assertion{Name: n, Outcome: outcomePass})
	}
	return as
}

// testRun is one healthy run: 120 s, no loss, p50 = lat, p95 = 2 lat, p99 = 4 lat.
func testRun(phase, cpus string, round int, pps, lat float64) phaseRun {
	success := int64(pps * 120)
	return phaseRun{
		Round: round, Phase: phase, CPUs: cpus, Dispatch: "parallel", Cache: "warm", Rate: pps,
		Offered: success, Success: success, Seconds: 120, PairsPerSecond: pps,
		Scheduled:       driver.Quantiles{Count: int(success), P50: lat, P95: 2 * lat, P99: 4 * lat},
		RecoverySeconds: have(5),
		Assertions:      passAssertions(),
	}
}

// testArm builds an arm over the whole profile matrix. jitter spreads the
// rounds symmetrically so a quiet arm still varies a little.
func testArm(role string, rounds int, pps, lat, jitter float64) *result {
	p := testProfile()
	e := &regressionEvidence{
		Role: role, Mode: modeReference, Profile: p.Name,
		Approval: approval{Approved: role == roleKnownGood, Reference: "digest comparison 2026-10-13"},
		Artifact: artifact{Digest: "sha256:" + strings.Repeat("a", 64)},
		Runtime:  runtimeInfo{GoVersion: "go1.26.0", ORTVersion: unavailable, CPUModel: "test"},
		Inputs: armInputs{
			Models:       []modelInputs{testModel()},
			ConfigSHA256: strings.Repeat("c", 64),
			CorpusSHA256: strings.Repeat("d", 64),
		},
		Limits: map[string]string{"cpu_allowance": "4.00", "cpu_budget_enabled": "1", "max_concurrent_requests": "32", "max_connections": unavailable},
	}
	for _, ph := range p.Phases {
		for _, cpus := range p.cpusFor(ph) {
			for r := 1; r <= rounds; r++ {
				scale := 1 + jitter*float64(r-2)
				e.Runs = append(e.Runs, testRun(ph.Name, cpus, r, pps*scale, lat))
			}
		}
	}
	return &result{Schema: bundleSchema, Host: testHost(), Model: testModel(), Regression: e}
}

func goodArm() *result { return testArm(roleKnownGood, 3, 10, 100, 0.005) }

func find(t *testing.T, v verdict, name string) assertion {
	t.Helper()
	for _, a := range v.Checks {
		if a.Name == name {
			return a
		}
	}
	t.Fatalf("verdict has no %s check: %+v", name, v.Checks)
	return assertion{}
}

func wantCheck(t *testing.T, v verdict, status, name, outcome string) {
	t.Helper()
	if v.Status != status {
		t.Fatalf("status = %s, want %s\n%+v", v.Status, status, v.Checks)
	}
	if got := find(t, v, name); got.Outcome != outcome {
		t.Fatalf("%s = %s (%s), want %s", name, got.Outcome, got.Detail, outcome)
	}
}

// ---- bundle schema (3.1) ----

func TestRegressionBundleRoundTripsAndNamesItsEvidence(t *testing.T) {
	arm := testArm(roleKnownGood, 3, 10, 100, 0.005)
	arm.Regression.Runs[0].AdmissionWaitUs = missing("older release")
	dir := t.TempDir()
	path := writeBundleFile(t, dir, "good.json", arm)
	got, err := loadResult(path)
	if err != nil {
		t.Fatal(err)
	}
	if got.Schema != bundleSchema || got.Regression == nil || len(got.Regression.Runs) != len(arm.Regression.Runs) {
		t.Fatalf("regression section did not round-trip: %+v", got.Regression)
	}
	if m := got.Regression.Runs[0].AdmissionWaitUs; m.ok() || m.Unavailable != "older release" {
		t.Fatalf("an unavailable metric must stay unavailable with its reason, got %+v", m)
	}
	if !got.Regression.Runs[1].RecoverySeconds.ok() || got.Regression.Runs[1].RecoverySeconds.val() != 5 {
		t.Fatalf("a measured metric must round-trip, got %+v", got.Regression.Runs[1].RecoverySeconds)
	}
	if (metric{}).ok() {
		t.Fatal("the zero metric must read as unavailable, never as a measured zero")
	}
}

func TestBundleRecordsEveryRegressionField(t *testing.T) {
	arm := testArm(roleKnownGood, 3, 10, 100, 0)
	arm.Regression.Startup = startup{ReadySeconds: have(3), Calibration: unavailable}
	var doc map[string]any
	decode(t, arm, &doc)
	for _, path := range []string{
		"schema", "regression.role", "regression.mode", "regression.approval.approved", "regression.profile",
		"regression.artifact.digest", "regression.runtime.go_version", "regression.runtime.ort_version", "regression.runtime.cpu_model",
		"regression.inputs.models.0.onnx.sha256", "regression.inputs.models.0.tokenizer.sha256",
		"regression.inputs.config_sha256", "regression.inputs.corpus_sha256",
		"regression.observed_limits.cpu_allowance", "regression.startup.ready_seconds.value", "regression.startup.calibration",
		"regression.runs.0.round", "regression.runs.0.phase", "regression.runs.0.cpus",
		"regression.runs.0.offered", "regression.runs.0.success", "regression.runs.0.errors", "regression.runs.0.timeouts", "regression.runs.0.dropped",
		"regression.runs.0.pairs_per_second", "regression.runs.0.pair_latency_from_scheduled.p99_ms",
		"regression.runs.0.pair_latency_from_wire_start", "regression.runs.0.error_kinds",
		"regression.runs.0.admission_wait_us", "regression.runs.0.embed_run_us", "regression.runs.0.cpu_seconds",
		"regression.runs.0.throttled_periods", "regression.runs.0.rss_mb", "regression.runs.0.recovery_seconds.value",
		"regression.runs.0.assertions.0.name", "regression.runs.0.assertions.0.outcome",
	} {
		mustHavePath(t, doc, path)
	}
}

func TestPhaseRunRecordsUnavailableForCountersAnOlderReleaseLacks(t *testing.T) {
	pr := driver.PhaseResult{
		Cache: driver.CacheWarm,
		Result: driver.Result{Offered: 4, Success: 4, Seconds: 1, Throughput: 4,
			Legs: map[string]driver.LegResult{"minilm": {Success: 4}}, ErrorKinds: map[string]int64{}},
		Counters: driver.Counters{
			Stats:  map[string]int64{"cache_hits": 4, "cache_misses": 0, "cpu_user_usec": 3_000_000, "cpu_sys_usec": 1_000_000},
			Models: map[string]map[string]int64{"minilm": {"embed_run_us": 700, "embed_dispatch_wait_us": 20}},
		},
		After: driver.Snapshot{Stats: map[string]string{"mem": "512", "cpu_budget_enabled": "1"}},
	}
	run := phaseRunFrom(runSpec{Round: 1, Phase: "steady", CPUs: "1", Dispatch: "single", Rate: 4}, pr)
	if run.AdmissionWaitUs.ok() || !strings.Contains(run.AdmissionWaitUs.Unavailable, "cpu_admission_wait_us") {
		t.Fatalf("a counter the server lacks must be unavailable naming it, got %+v", run.AdmissionWaitUs)
	}
	if !run.CPUSeconds.ok() || run.CPUSeconds.val() != 4 || !run.EmbedRunUs.ok() || run.EmbedRunUs.val() != 700 || run.RSSMB.val() != 512 {
		t.Fatalf("available counters must be recorded: %+v", run)
	}
	if run.ThrottledPeriods.ok() || run.Restarts.ok() {
		t.Fatal("what the driver cannot see must be unavailable")
	}
	for _, a := range run.Assertions {
		if a.Name == assertNoRestartOOM && a.Outcome != outcomeSkipped {
			t.Fatalf("restart/OOM unknown must be skipped, never a pass: %+v", a)
		}
		if a.Name != assertNoRestartOOM && a.Outcome != outcomePass {
			t.Fatalf("healthy run assertion %s = %s: %s", a.Name, a.Outcome, a.Detail)
		}
	}
	s := startupFrom(map[string]map[string]string{"minilm": {"workers": "2", "intra_op_threads": "1"}}, missing("x"))
	if s.Calibration != unavailable || s.Layouts["minilm"]["workers"] != "2" {
		t.Fatalf("startup = %+v", s)
	}
}

// ---- fail-closed comparator (3.2) ----

func TestComparatorRejectsStructurallyBadEvidenceBeforeAnyRatio(t *testing.T) {
	cases := []struct {
		name   string
		check  string
		mutate func(good, cand *result)
	}{
		{"empty candidate bundle", checkRequiredCells, func(_, c *result) { c.Regression.Runs = nil }},
		{"empty baseline bundle", checkRequiredCells, func(g, _ *result) { g.Regression.Runs = nil }},
		{"missing cell", checkRequiredCells, func(_, c *result) {
			c.Regression.Runs = dropCell(c.Regression.Runs, "recovery", "2")
		}},
		{"baseline lost a cell too (matrix comes from the profile)", checkRequiredCells, func(g, c *result) {
			g.Regression.Runs = dropCell(g.Regression.Runs, "burst", "1")
			c.Regression.Runs = dropCell(c.Regression.Runs, "burst", "1")
		}},
		{"duplicate cell", checkRequiredCells, func(_, c *result) { c.Regression.Runs = append(c.Regression.Runs, c.Regression.Runs[0]) }},
		{"rounds not interleaved", checkRequiredCells, func(_, c *result) { c.Regression.Runs = c.Regression.Runs[1:] }},
		{"different onnx digest", checkInputsMatch, func(_, c *result) { c.Regression.Inputs.Models[0].ONNX.SHA256 = strings.Repeat("0", 64) }},
		{"different tokenizer digest", checkInputsMatch, func(_, c *result) { c.Regression.Inputs.Models[0].Tokenizer.SHA256 = strings.Repeat("0", 64) }},
		{"different config digest", checkInputsMatch, func(_, c *result) { c.Regression.Inputs.ConfigSHA256 = strings.Repeat("0", 64) }},
		{"different corpus digest", checkInputsMatch, func(_, c *result) { c.Regression.Inputs.CorpusSHA256 = strings.Repeat("0", 64) }},
		{"invalid required assertion", checkRunsComplete, func(_, c *result) { setAssertion(&c.Regression.Runs[0], assertGovernorEnabled, outcomeInvalid) }},
		{"skipped required assertion", checkRunsComplete, func(_, c *result) { setAssertion(&c.Regression.Runs[0], assertCacheExpectation, outcomeSkipped) }},
		{"missing required assertion", checkRunsComplete, func(_, c *result) { c.Regression.Runs[0].Assertions = c.Regression.Runs[0].Assertions[1:] }},
		{"baseline fails a required assertion", checkRunsComplete, func(g, _ *result) { setAssertion(&g.Regression.Runs[0], assertRepliesValid, outcomeFail) }},
		{"incomplete run", checkRunsComplete, func(_, c *result) { c.Regression.Runs[0].Offered++ }},
		{"run that offered nothing", checkRunsComplete, func(_, c *result) { c.Regression.Runs[0].Offered, c.Regression.Runs[0].Success = 0, 0 }},
		{"recovery run without a recovery time", checkRunsComplete, func(_, c *result) {
			for i := range c.Regression.Runs {
				if c.Regression.Runs[i].Phase == "recovery" {
					c.Regression.Runs[i].RecoverySeconds = metric{}
				}
			}
		}},
		{"zero-success control", checkControlNonzero, func(g, _ *result) {
			r := &g.Regression.Runs[0]
			r.Errors, r.Success, r.PairsPerSecond = r.Offered, 0, 0
		}},
		{"non-approved baseline", checkBaselineApproved, func(g, _ *result) { g.Regression.Approval.Approved = false }},
		{"approval without a reference", checkBaselineApproved, func(g, _ *result) { g.Regression.Approval.Reference = "" }},
		{"baseline role is not known-good", checkBaselineApproved, func(g, _ *result) { g.Regression.Role = roleCandidate }},
		{"no artifact digest", checkArtifactProvenance, func(_, c *result) { c.Regression.Artifact.Digest = "" }},
		{"unavailable artifact digest", checkArtifactProvenance, func(g, _ *result) { g.Regression.Artifact.Digest = unavailable }},
		{"modes differ", checkEvidenceMode, func(_, c *result) { c.Regression.Mode = modeConstrainedReproduction }},
		{"reference claim from a non-gold host", checkEvidenceMode, func(_, c *result) { c.Host.Gold = false }},
		{"limits differ", checkLimitsMatch, func(_, c *result) { c.Regression.Limits["cpu_allowance"] = "2.00" }},
		{"hosts differ", checkLimitsMatch, func(_, c *result) { c.Host = otherHost() }},
		{"pre-regression bundle", checkBundleSchema, func(_, c *result) { c.Schema, c.Regression = 0, nil }},
		{"wrong profile", checkBundleSchema, func(_, c *result) { c.Regression.Profile = "other" }},
		{"outcome outside the schema", checkBundleSchema, func(_, c *result) { c.Regression.Runs[0].Assertions[0].Outcome = "green" }},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			good, cand := goodArm(), testArm(roleCandidate, 3, 10, 100, 0.005)
			tc.mutate(good, cand)
			v := compareArms(testProfile(), good, cand)
			wantCheck(t, v, statusInvalid, tc.check, outcomeInvalid)
			for _, a := range v.Checks {
				if a.Name == checkThroughputFloor || a.Name == checkLatencyP95 || a.Name == checkNoise {
					t.Fatalf("%s was evaluated although the evidence is invalid", a.Name)
				}
			}
		})
	}
}

func TestComparatorRejectsAnInvalidProfile(t *testing.T) {
	for name, mutate := range map[string]func(*profile){
		"no deadlines declared":    func(p *profile) { p.Deadlines = deadlines{} },
		"throughput loosened":      func(p *profile) { p.Thresholds.MinThroughput = 0.5 },
		"latency loosened":         func(p *profile) { p.Thresholds.MaxLatency = 2 },
		"fewer than three rounds":  func(p *profile) { p.Thresholds.MinRounds = 2 },
		"shorter than 120 seconds": func(p *profile) { p.Thresholds.MinSeconds = 30 },
		"fewer than 1000 pairs":    func(p *profile) { p.Thresholds.MinPairs = 10 },
		"no phases":                func(p *profile) { p.Phases = nil },
	} {
		t.Run(name, func(t *testing.T) {
			p := testProfile()
			mutate(&p)
			wantCheck(t, compareArms(p, goodArm(), testArm(roleCandidate, 3, 10, 100, 0.005)), statusInvalid, checkProfileValid, outcomeInvalid)
		})
	}
}

func dropCell(runs []phaseRun, phase, cpus string) []phaseRun {
	var out []phaseRun
	for _, r := range runs {
		if r.Phase != phase || r.CPUs != cpus {
			out = append(out, r)
		}
	}
	return out
}

func setAssertion(r *phaseRun, name, outcome string) {
	for i := range r.Assertions {
		if r.Assertions[i].Name == name {
			r.Assertions[i].Outcome = outcome
		}
	}
}

// ---- verdict (3.3) ----

func TestVerdictPassesAGoodCandidate(t *testing.T) {
	// 97% throughput, 5% slower tails: inside the 95% / 110% guardrails.
	v := compareArms(testProfile(), goodArm(), testArm(roleCandidate, 3, 9.7, 105, 0.005))
	if v.Status != statusPass {
		t.Fatalf("status = %s\n%+v", v.Status, v.Checks)
	}
	for _, n := range []string{checkRounds, checkSampleSize, checkNoise, checkThroughputFloor, checkLatencyP50, checkLatencyP95, checkLatencyP99, checkNoUnexpectedLoss} {
		if find(t, v, n).Outcome != outcomePass {
			t.Fatalf("%s did not pass: %+v", n, v.Checks)
		}
	}
}

func TestVerdictFailsOnThroughputLatencyLossAndRecovery(t *testing.T) {
	cases := []struct {
		name   string
		check  string
		cand   func() *result
		mutate func(*result)
	}{
		{"throughput below 95%", checkThroughputFloor, func() *result { return testArm(roleCandidate, 3, 9.0, 100, 0.005) }, nil},
		{"p50 above 110%", checkLatencyP50, func() *result { return testArm(roleCandidate, 3, 10, 120, 0.005) }, nil},
		{"p95 above 110%", checkLatencyP95, func() *result { return testArm(roleCandidate, 3, 10, 100, 0.005) }, func(c *result) {
			for i := range c.Regression.Runs {
				c.Regression.Runs[i].Scheduled.P95 *= 1.3
			}
		}},
		{"p99 above 110%", checkLatencyP99, func() *result { return testArm(roleCandidate, 3, 10, 100, 0.005) }, func(c *result) {
			for i := range c.Regression.Runs {
				c.Regression.Runs[i].Scheduled.P99 *= 1.3
			}
		}},
		{"an error at the sustainable rate", checkNoUnexpectedLoss, func() *result { return testArm(roleCandidate, 3, 10, 100, 0.005) }, func(c *result) {
			r := &c.Regression.Runs[0] // steady @ 1, round 1
			r.Success--
			r.Errors++
		}},
		{"a timeout at the sustainable rate", checkNoUnexpectedLoss, func() *result { return testArm(roleCandidate, 3, 10, 100, 0.005) }, func(c *result) {
			r := &c.Regression.Runs[0]
			r.Success--
			r.Timeouts++
		}},
		{"a drop at the sustainable rate", checkNoUnexpectedLoss, func() *result { return testArm(roleCandidate, 3, 10, 100, 0.005) }, func(c *result) {
			r := &c.Regression.Runs[0]
			r.Success--
			r.Dropped++
		}},
		{"recovery past the deadline", checkRecoveryDeadline, func() *result { return testArm(roleCandidate, 3, 10, 100, 0.005) }, func(c *result) {
			for i := range c.Regression.Runs {
				if c.Regression.Runs[i].Phase == "recovery" {
					c.Regression.Runs[i].RecoverySeconds = have(45)
				}
			}
		}},
		{"a failing required assertion", checkCandidateAsserts, func() *result { return testArm(roleCandidate, 3, 10, 100, 0.005) }, func(c *result) {
			setAssertion(&c.Regression.Runs[0], assertGovernorEnabled, outcomeFail)
		}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			cand := tc.cand()
			if tc.mutate != nil {
				tc.mutate(cand)
			}
			wantCheck(t, compareArms(testProfile(), goodArm(), cand), statusFail, tc.check, outcomeFail)
		})
	}
}

func TestVerdictBusyRepliesInTheOverloadPhaseAreNotLoss(t *testing.T) {
	cand := testArm(roleCandidate, 3, 10, 100, 0.005)
	for i := range cand.Regression.Runs {
		if r := &cand.Regression.Runs[i]; r.Phase == "burst" {
			r.Success -= 50
			r.Errors += 50
			r.BusyRejections = 50
		}
	}
	if v := compareArms(testProfile(), goodArm(), cand); v.Status != statusPass {
		t.Fatalf("explicit busy rejections in the overload phase must not fail the run: %s\n%+v", v.Status, v.Checks)
	}
}

func TestVerdictCollapseFailsTheThroughputFloor(t *testing.T) {
	v := compareArms(testProfile(), goodArm(), testArm(roleCandidate, 3, 2, 100, 0.005))
	wantCheck(t, v, statusFail, checkThroughputFloor, outcomeFail)
}

func TestVerdictIsInconclusiveNeverPassingOnNoisyOrThinEvidence(t *testing.T) {
	collapsed := func() *result { return testArm(roleCandidate, 3, 2, 100, 0.005) }
	cases := []struct {
		name  string
		check string
		good  func() *result
		cand  func() *result
	}{
		{"noisy good-versus-good", checkNoise, func() *result { return testArm(roleKnownGood, 3, 10, 100, 0.06) }, collapsed},
		{"fewer than three rounds", checkRounds, func() *result { return testArm(roleKnownGood, 2, 10, 100, 0.005) }, func() *result { return testArm(roleCandidate, 2, 10, 100, 0.005) }},
		{"a gated run under 120 seconds", checkSampleSize, goodArm, func() *result {
			c := testArm(roleCandidate, 3, 10, 100, 0.005)
			c.Regression.Runs[0].Seconds = 60
			return c
		}},
		{"a baseline gated run under 1000 pairs", checkSampleSize, func() *result {
			g := goodArm()
			g.Regression.Runs[0].Success = 900
			g.Regression.Runs[0].Offered = 900
			return g
		}, func() *result { return testArm(roleCandidate, 3, 10, 100, 0.005) }},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			v := compareArms(testProfile(), tc.good(), tc.cand())
			wantCheck(t, v, statusInconclusive, tc.check, outcomeInconclusive)
			// The ratio gates are withheld, not passed, and thresholds were not widened.
			if got := find(t, v, checkThroughputFloor).Outcome; got != outcomeInconclusive {
				t.Fatalf("throughput-floor = %s, want inconclusive", got)
			}
		})
	}
}

// ---- self-verification (3.4) ----

// fakeEmbServer answers every EMB with an application error, and EMB.STATS /
// EMB.INFO with the minimum a phase needs.
func fakeErrorServer(t *testing.T) string {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = ln.Close() })
	go func() {
		for {
			conn, err := ln.Accept()
			if err != nil {
				return
			}
			go func() {
				defer func() { _ = conn.Close() }()
				buf := make([]byte, 4096)
				for {
					n, err := conn.Read(buf)
					if err != nil {
						return
					}
					cmd := string(buf[:n])
					switch {
					case strings.Contains(cmd, "EMB.STATS"):
						_, _ = conn.Write([]byte("*6\r\n$18\r\ncpu_budget_enabled\r\n:1\r\n$10\r\ncache_hits\r\n:0\r\n$12\r\ncache_misses\r\n:0\r\n"))
					case strings.Contains(cmd, "EMB.INFO"):
						_, _ = conn.Write([]byte("*2\r\n$8\r\nrequests\r\n:0\r\n"))
					default:
						_, _ = conn.Write([]byte("-ERR model exploded\r\n"))
					}
				}
			}()
		}
	}()
	return ln.Addr().String()
}

func TestSelfVerificationCases(t *testing.T) {
	t.Run("application-error replies fail replies-valid", func(t *testing.T) {
		pr, err := driver.RunPhase(driver.PhaseConfig{
			Config: driver.Config{
				Addr: fakeErrorServer(t), Models: []string{"minilm"}, Dispatch: driver.Single, Rate: 20,
				Duration: 200 * time.Millisecond, MaxInFlight: 2, RequestTimeout: time.Second, DrainTimeout: time.Second, Dim: 4,
			},
			Cache: driver.CacheCold, Corpus: driver.NewCorpus(1, 3, "t"), Info: true,
		})
		if err != nil {
			t.Fatal(err)
		}
		run := phaseRunFrom(runSpec{Round: 1, Phase: "steady", CPUs: "1", Dispatch: "single", Rate: 20}, pr)
		if run.Success != 0 || run.Errors != run.Offered {
			t.Fatalf("every -ERR reply must be an error: %+v", run)
		}
		var got assertion
		for _, a := range run.Assertions {
			if a.Name == assertRepliesValid {
				got = a
			}
		}
		if got.Outcome != outcomeFail {
			t.Fatalf("replies-valid = %+v, want fail", got)
		}

		// The same evidence in a candidate arm fails the comparison by name.
		cand := testArm(roleCandidate, 3, 10, 100, 0.005)
		for i := range cand.Regression.Runs {
			r := &cand.Regression.Runs[i]
			r.Errors, r.Success, r.PairsPerSecond, r.ErrorKinds = r.Offered, 0, 0, map[string]int64{driver.KindServer: r.Offered}
			setAssertion(r, assertRepliesValid, outcomeFail)
		}
		v := compareArms(testProfile(), goodArm(), cand)
		wantCheck(t, v, statusFail, checkCandidateAsserts, outcomeFail)
		if find(t, v, checkNoUnexpectedLoss).Outcome != outcomeFail {
			t.Fatal("application errors at the sustainable rate must also trip no-unexpected-loss")
		}
	})

	t.Run("empty bundle fails required-cells", func(t *testing.T) {
		cand := testArm(roleCandidate, 3, 10, 100, 0.005)
		cand.Regression.Runs = nil
		wantCheck(t, compareArms(testProfile(), goodArm(), cand), statusInvalid, checkRequiredCells, outcomeInvalid)
	})

	t.Run("zero control work fails control-nonzero", func(t *testing.T) {
		good := goodArm()
		for i := range good.Regression.Runs {
			r := &good.Regression.Runs[i]
			r.Errors, r.Success, r.PairsPerSecond = r.Offered, 0, 0
		}
		wantCheck(t, compareArms(testProfile(), good, testArm(roleCandidate, 3, 10, 100, 0.005)), statusInvalid, checkControlNonzero, outcomeInvalid)
	})

	t.Run("deliberate throughput collapse fails throughput-floor", func(t *testing.T) {
		wantCheck(t, compareArms(testProfile(), goodArm(), testArm(roleCandidate, 3, 1, 100, 0.005)), statusFail, checkThroughputFloor, outcomeFail)
	})

	t.Run("good-versus-good passes", func(t *testing.T) {
		v := compareArms(testProfile(), goodArm(), testArm(roleKnownGood, 3, 10, 100, 0.005))
		if v.Status != statusPass {
			t.Fatalf("status = %s\n%+v", v.Status, v.Checks)
		}
	})
}

// ---- CLI path ----

func TestCompareBundlesExpectFailRequiresTheNamedCheck(t *testing.T) {
	dir := t.TempDir()
	profilePath := filepath.Join(dir, "profile.json")
	writeJSON(t, profilePath, testProfile())
	good := writeBundleFile(t, dir, "good.json", goodArm())
	pass := writeBundleFile(t, dir, "pass.json", testArm(roleCandidate, 3, 10, 100, 0.005))
	bad := writeBundleFile(t, dir, "bad.json", testArm(roleCandidate, 3, 1, 100, 0.005))
	empty := testArm(roleCandidate, 3, 10, 100, 0.005)
	empty.Regression.Runs = nil
	broken := writeBundleFile(t, dir, "broken.json", empty)

	var ok bool
	var err error
	out := captureStdout(t, func() { ok, err = compareBundles(profilePath, good, pass, "") })
	if err != nil || !ok || !strings.Contains(out, "verdict: pass") {
		t.Fatalf("good-vs-candidate pass: ok=%v err=%v\n%s", ok, err, out)
	}
	captureStdout(t, func() { ok, err = compareBundles(profilePath, good, bad, "") })
	if err != nil || ok {
		t.Fatalf("a collapsed candidate must not pass: ok=%v err=%v", ok, err)
	}
	captureStdout(t, func() { ok, err = compareBundles(profilePath, good, bad, checkThroughputFloor) })
	if err != nil || !ok {
		t.Fatalf("known-bad failing the intended check must be accepted: ok=%v err=%v", ok, err)
	}
	captureStdout(t, func() { ok, err = compareBundles(profilePath, good, bad, checkLatencyP95) })
	if ok || err == nil {
		t.Fatal("known-bad failing a different check must not count as reproducing the regression")
	}
	captureStdout(t, func() { ok, err = compareBundles(profilePath, good, broken, checkThroughputFloor) })
	if ok || err == nil {
		t.Fatal("invalid evidence (a missing fixture) must never count as the intended failure")
	}
	captureStdout(t, func() { ok, err = compareBundles(profilePath, good, pass, checkThroughputFloor) })
	if ok || err == nil {
		t.Fatal("a passing arm cannot satisfy -expect-fail")
	}
	if _, err := loadProfile(filepath.Join(dir, "missing.json")); err == nil {
		t.Fatal("a missing profile must be an error")
	}
}

func decode(t *testing.T, v any, into *map[string]any) {
	t.Helper()
	raw, err := json.Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(raw, into); err != nil {
		t.Fatal(err)
	}
}

func writeJSON(t *testing.T, path string, v any) {
	t.Helper()
	raw, err := json.Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, raw, 0o600); err != nil {
		t.Fatal(err)
	}
}

func TestCommittedControlProfileIsValid(t *testing.T) {
	p, err := loadProfile("../regression/control-profile.json")
	if err != nil {
		t.Fatal(err)
	}
	if problems := profileProblems(p); len(problems) > 0 {
		t.Fatalf("committed control profile is invalid: %v", problems)
	}
	for _, want := range []string{"cold-first-use", "preloaded-startup", "warm-one-hot-idle-peer", "paired-serial", "paired-parallel", "emb-multi", "mixed-warm", "mixed-cold", "sustained", "burst", "recovery"} {
		if !slices.ContainsFunc(p.Phases, func(ph phaseSpec) bool { return ph.Name == want }) {
			t.Errorf("control profile lacks required phase %s", want)
		}
	}
	var sustained phaseSpec
	for _, ph := range p.Phases {
		if ph.Name == "sustained" {
			sustained = ph
		}
	}
	for _, c := range []string{"0.5", "1", "1.5", "2", "4"} {
		if !slices.Contains(p.cpusFor(sustained), c) {
			t.Errorf("sustained phase lacks the %s CPU control", c)
		}
	}
}
