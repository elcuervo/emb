package main

// The release comparator. It reads two regression bundles (a known-good arm and
// a candidate or known-bad arm) against a profile that declares the required
// matrix, and fails closed: every structural problem is `invalid` and is
// reported before any ratio is computed, noisy or thin evidence is
// `inconclusive`, and only a complete, quiet comparison within the thresholds
// passes. The required matrix is iterated from the profile, never from the
// baseline's own keys, so a baseline that lost a cell cannot shrink the gate.

import (
	"encoding/json"
	"fmt"
	"os"
	"slices"
	"sort"
	"strings"
)

// Thresholds a release must meet. A profile may tighten them, never loosen.
const (
	defaultMinThroughput = 0.95 // candidate useful throughput / known-good
	defaultMaxLatency    = 1.10 // candidate pair latency / known-good, p50/p95/p99
	defaultMinRounds     = 3
	defaultMinPairs      = 1000
	defaultMinSeconds    = 120.0
	defaultNoiseTput     = 0.05 // known-good round-to-round throughput spread
	defaultNoiseLatency  = 0.10 // known-good round-to-round latency spread
)

// Verdict statuses; comparison check names follow.
const (
	statusPass         = "pass"
	statusFail         = "fail"
	statusInconclusive = "inconclusive"
	statusInvalid      = "invalid"
)

const (
	checkProfileValid       = "profile-valid"
	checkBundleSchema       = "bundle-schema"
	checkBaselineApproved   = "baseline-approved"
	checkArtifactProvenance = "artifact-provenance"
	checkInputsMatch        = "inputs-match"
	checkEvidenceMode       = "evidence-mode"
	checkLimitsMatch        = "limits-match"
	checkRequiredCells      = "required-cells"
	checkRunsComplete       = "runs-complete"
	checkControlNonzero     = "control-nonzero"
	checkRounds             = "rounds"
	checkSampleSize         = "sample-size"
	checkNoise              = "noise"
	checkThroughputFloor    = "throughput-floor"
	checkLatencyP50         = "latency-p50"
	checkLatencyP95         = "latency-p95"
	checkLatencyP99         = "latency-p99"
	checkNoUnexpectedLoss   = "no-unexpected-loss"
	checkCandidateAsserts   = "candidate-assertions"
	checkRecoveryDeadline   = "recovery-deadline"
)

// deadlines are the production deadline inputs, declared before measurement.
type deadlines struct {
	RequestSeconds  float64 `json:"request_seconds"`
	HealthSeconds   float64 `json:"health_seconds"`
	RecoverySeconds float64 `json:"recovery_seconds"`
}

type thresholds struct {
	MinThroughput   float64 `json:"min_throughput_ratio,omitempty"`
	MaxLatency      float64 `json:"max_latency_ratio,omitempty"`
	MinRounds       int     `json:"min_rounds,omitempty"`
	MinPairs        int64   `json:"min_pairs,omitempty"`
	MinSeconds      float64 `json:"min_seconds,omitempty"`
	NoiseThroughput float64 `json:"noise_throughput,omitempty"`
	NoiseLatency    float64 `json:"noise_latency,omitempty"`
}

// withDefaults fills omitted thresholds with the documented defaults.
func (t thresholds) withDefaults() thresholds {
	if t.MinThroughput == 0 {
		t.MinThroughput = defaultMinThroughput
	}
	if t.MaxLatency == 0 {
		t.MaxLatency = defaultMaxLatency
	}
	if t.MinRounds == 0 {
		t.MinRounds = defaultMinRounds
	}
	if t.MinPairs == 0 {
		t.MinPairs = defaultMinPairs
	}
	if t.MinSeconds == 0 {
		t.MinSeconds = defaultMinSeconds
	}
	if t.NoiseThroughput == 0 {
		t.NoiseThroughput = defaultNoiseTput
	}
	if t.NoiseLatency == 0 {
		t.NoiseLatency = defaultNoiseLatency
	}
	return t
}

// phaseSpec is one required phase. Gated phases are the steady-state tail cells
// compared by ratio; sustainable phases run at a rate the known-good sustains, so
// any error, timeout or drop there is unexpected; a recovery deadline marks a
// phase whose run must report how long the server took to recover.
type phaseSpec struct {
	Name                    string   `json:"name"`
	Gated                   bool     `json:"gated,omitempty"`
	Sustainable             bool     `json:"sustainable,omitempty"`
	RecoveryDeadlineSeconds float64  `json:"recovery_deadline_seconds,omitempty"`
	CPUs                    []string `json:"cpus,omitempty"` // overrides the profile default for this phase
}

// cpusFor is the CPU bandwidths a phase must be measured at: selected
// combinations per phase instead of a Cartesian product.
func (p profile) cpusFor(ph phaseSpec) []string {
	if len(ph.CPUs) > 0 {
		return ph.CPUs
	}
	return p.CPUs
}

func (p profile) cellCount() int {
	n := 0
	for _, ph := range p.Phases {
		n += len(p.cpusFor(ph))
	}
	return n
}

// profile declares what a comparison must contain.
type profile struct {
	Name               string      `json:"name"`
	Note               string      `json:"note,omitempty"`
	CPUs               []string    `json:"cpus,omitempty"` // default CPU bandwidth labels for phases without their own
	Phases             []phaseSpec `json:"phases"`
	Deadlines          deadlines   `json:"deadlines"`
	Thresholds         thresholds  `json:"thresholds"`
	RequiredAssertions []string    `json:"required_assertions"`
}

func loadProfile(path string) (profile, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return profile{}, err
	}
	var p profile
	dec := json.NewDecoder(strings.NewReader(string(raw)))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&p); err != nil {
		return profile{}, fmt.Errorf("profile %s: %w", path, err)
	}
	return p, nil
}

type verdict struct {
	Status string      `json:"status"`
	Checks []assertion `json:"checks"`
}

// checks accumulates named outcomes.
type checks struct{ list []assertion }

func (c *checks) add(name, outcome, format string, args ...any) {
	c.list = append(c.list, assertion{Name: name, Outcome: outcome, Detail: fmt.Sprintf(format, args...)})
}

// require records a structural check: any problem makes the evidence invalid.
func (c *checks) require(name string, problems []string, pass string) {
	if len(problems) == 0 {
		c.add(name, outcomePass, "%s", pass)
		return
	}
	c.add(name, outcomeInvalid, "%s", strings.Join(problems, "; "))
}

func (c *checks) has(outcome string) bool {
	return slices.ContainsFunc(c.list, func(a assertion) bool { return a.Outcome == outcome })
}

func (c *checks) status() string {
	switch {
	case c.has(outcomeInvalid):
		return statusInvalid
	case c.has(outcomeFail):
		return statusFail
	case c.has(outcomeInconclusive):
		return statusInconclusive
	default:
		return statusPass
	}
}

// runKey is a required cell.
func runKey(phase, cpus string) string { return phase + " @ " + cpus + " CPUs" }

// armRuns indexes an arm's runs by cell then round, reporting duplicates and bad rounds.
func armRuns(label string, runs []phaseRun, problems *[]string) map[string]map[int]phaseRun {
	idx := map[string]map[int]phaseRun{}
	for _, r := range runs {
		k := runKey(r.Phase, r.CPUs)
		if r.Round < 1 {
			*problems = append(*problems, fmt.Sprintf("%s %s has round %d (rounds start at 1)", label, k, r.Round))
			continue
		}
		if idx[k] == nil {
			idx[k] = map[int]phaseRun{}
		}
		if _, dup := idx[k][r.Round]; dup {
			*problems = append(*problems, fmt.Sprintf("%s has a duplicate run for %s round %d", label, k, r.Round))
			continue
		}
		idx[k][r.Round] = r
	}
	return idx
}

func sortedRounds(m map[int]phaseRun) []int {
	rounds := make([]int, 0, len(m))
	for r := range m {
		rounds = append(rounds, r)
	}
	sort.Ints(rounds)
	return rounds
}

func profileProblems(p profile) []string {
	var out []string
	if p.Name == "" {
		out = append(out, "profile has no name")
	}
	for _, ph := range p.Phases {
		if len(p.cpusFor(ph)) == 0 {
			out = append(out, fmt.Sprintf("phase %q has no CPU bandwidths (set the profile default or the phase's own list)", ph.Name))
		}
	}
	if len(p.Phases) == 0 {
		out = append(out, "profile declares no phases")
	}
	seen := map[string]bool{}
	for _, ph := range p.Phases {
		if ph.Name == "" || seen[ph.Name] {
			out = append(out, fmt.Sprintf("phase %q is unnamed or repeated", ph.Name))
		}
		seen[ph.Name] = true
	}
	d := p.Deadlines
	if d.RequestSeconds <= 0 || d.HealthSeconds <= 0 || d.RecoverySeconds <= 0 {
		out = append(out, "request, health and recovery deadlines must be declared before measurement")
	}
	t := p.Thresholds.withDefaults()
	if t.MinThroughput < defaultMinThroughput || t.MaxLatency > defaultMaxLatency || t.MinRounds < defaultMinRounds ||
		t.MinPairs < defaultMinPairs || t.MinSeconds < defaultMinSeconds || t.NoiseThroughput <= 0 || t.NoiseLatency <= 0 {
		out = append(out, "profile thresholds may tighten the defaults but never loosen them")
	}
	return out
}

// armProblems validates one bundle's own structure.
func armProblems(label string, p profile, r *result) []string {
	var out []string
	if r == nil || r.Regression == nil || r.Schema != bundleSchema {
		return []string{label + " carries no regression evidence at schema " + fmt.Sprint(bundleSchema)}
	}
	e := r.Regression
	if e.Profile != p.Name {
		out = append(out, fmt.Sprintf("%s was recorded for profile %q, not %q", label, e.Profile, p.Name))
	}
	if !slices.Contains([]string{roleKnownGood, roleKnownBad, roleCandidate}, e.Role) {
		out = append(out, fmt.Sprintf("%s has unknown role %q", label, e.Role))
	}
	if e.Mode != modeReference && e.Mode != modeConstrainedReproduction {
		out = append(out, fmt.Sprintf("%s has unknown evidence mode %q", label, e.Mode))
	}
	if err := checkAssertions(r.Assertions); err != nil {
		out = append(out, label+": "+err.Error())
	}
	for _, run := range e.Runs {
		if err := checkAssertions(run.Assertions); err != nil {
			out = append(out, fmt.Sprintf("%s %s round %d: %v", label, runKey(run.Phase, run.CPUs), run.Round, err))
		}
	}
	return out
}

func digestProblems(label string, e *regressionEvidence) []string {
	var out []string
	if e.Artifact.Digest == "" || strings.HasPrefix(e.Artifact.Digest, unavailable) {
		out = append(out, label+" has no artifact digest")
	}
	return out
}

func inputProblems(good, cand *regressionEvidence) []string {
	var out []string
	g, c := good.Inputs, cand.Inputs
	if len(g.Models) == 0 || len(g.Models) != len(c.Models) {
		return []string{fmt.Sprintf("arms measured %d and %d models", len(g.Models), len(c.Models))}
	}
	for i, gm := range g.Models {
		cm := c.Models[i]
		for _, f := range []struct{ what, a, b string }{
			{"model name", gm.Name, cm.Name},
			{"onnx digest of " + gm.Name, gm.ONNX.SHA256, cm.ONNX.SHA256},
			{"tokenizer digest of " + gm.Name, gm.Tokenizer.SHA256, cm.Tokenizer.SHA256},
		} {
			if f.a == "" || f.a != f.b {
				out = append(out, fmt.Sprintf("%s differs or is missing (%q vs %q)", f.what, f.a, f.b))
			}
		}
	}
	for _, f := range []struct{ what, a, b string }{
		{"config digest", g.ConfigSHA256, c.ConfigSHA256},
		{"corpus digest", g.CorpusSHA256, c.CorpusSHA256},
	} {
		if f.a == "" || f.a != f.b {
			out = append(out, fmt.Sprintf("%s differs or is missing (%q vs %q)", f.what, f.a, f.b))
		}
	}
	return out
}

func modeProblems(good, cand *result) []string {
	var out []string
	if good.Regression.Mode != cand.Regression.Mode {
		out = append(out, fmt.Sprintf("evidence modes differ (%s vs %s)", good.Regression.Mode, cand.Regression.Mode))
	}
	for _, a := range []struct {
		label string
		r     *result
	}{{"known-good", good}, {"candidate", cand}} {
		if a.r.Regression.Mode == modeReference && !a.r.Host.Gold {
			out = append(out, a.label+" claims reference evidence from a host that failed the host contract; record it as "+modeConstrainedReproduction)
		}
	}
	return out
}

func limitProblems(good, cand *result) []string {
	var out []string
	if good.Host.Identity == "" || good.Host.Identity != cand.Host.Identity {
		out = append(out, fmt.Sprintf("host identities differ or are missing (%q vs %q)", good.Host.Identity, cand.Host.Identity))
	}
	for _, k := range limitKeys {
		g, c := good.Regression.Limits[k], cand.Regression.Limits[k]
		if g == "" || c == "" || g == unavailable || c == unavailable {
			continue // an older release may not report it; recorded, not compared
		}
		if g != c {
			out = append(out, fmt.Sprintf("server-observed %s differs (%s vs %s)", k, g, c))
		}
	}
	return out
}

// runProblems checks that one required run is complete and that its required
// assertions were recorded as passes (a candidate's failing assertion is a
// verdict failure, not invalid evidence; a baseline's is invalid).
func runProblems(label string, p profile, spec phaseSpec, r phaseRun, isGood bool) []string {
	k := fmt.Sprintf("%s %s round %d", label, runKey(r.Phase, r.CPUs), r.Round)
	var out []string
	if r.Offered <= 0 || r.Seconds <= 0 {
		out = append(out, k+" offered no work")
	}
	if sum := r.Success + r.Errors + r.Timeouts + r.Dropped; sum != r.Offered {
		out = append(out, fmt.Sprintf("%s is incomplete: offered %d != classified %d", k, r.Offered, sum))
	}
	for _, name := range p.RequiredAssertions {
		var found *assertion
		for i := range r.Assertions {
			if r.Assertions[i].Name == name {
				found = &r.Assertions[i]
			}
		}
		switch {
		case found == nil:
			out = append(out, fmt.Sprintf("%s lacks required assertion %s", k, name))
		case found.Outcome == outcomeInvalid || found.Outcome == outcomeSkipped:
			out = append(out, fmt.Sprintf("%s required assertion %s is %s", k, name, found.Outcome))
		case found.Outcome == outcomeFail && isGood:
			out = append(out, fmt.Sprintf("%s baseline fails required assertion %s: %s", k, name, found.Detail))
		}
	}
	if spec.RecoveryDeadlineSeconds > 0 && !r.RecoverySeconds.ok() {
		out = append(out, k+" reports no recovery time")
	}
	return out
}

// compareArms is the whole verdict. See the file comment for precedence.
func compareArms(p profile, good, cand *result) verdict {
	var c checks
	th := p.Thresholds.withDefaults()

	c.require(checkProfileValid, profileProblems(p), "profile declares its matrix, deadlines and thresholds")
	arm := append(armProblems("known-good", p, good), armProblems("candidate", p, cand)...)
	c.require(checkBundleSchema, arm, "both bundles carry regression evidence at the current schema")
	if c.has(outcomeInvalid) {
		return verdict{Status: statusInvalid, Checks: c.list}
	}
	g, k := good.Regression, cand.Regression

	var baseline []string
	if g.Role != roleKnownGood || !g.Approval.Approved || g.Approval.Reference == "" {
		baseline = append(baseline, "the baseline must be a known-good arm with a recorded approval and its reference")
	}
	c.require(checkBaselineApproved, baseline, "known-good is approved by "+g.Approval.Reference)
	c.require(checkArtifactProvenance, append(digestProblems("known-good", g), digestProblems("candidate", k)...), "both arms name an artifact digest")
	c.require(checkInputsMatch, inputProblems(g, k), "model, tokenizer, config and corpus digests match")
	c.require(checkEvidenceMode, modeProblems(good, cand), "both arms are "+g.Mode+" evidence")
	c.require(checkLimitsMatch, limitProblems(good, cand), "host identity and server-observed limits match")

	// Required matrix from the profile; runs outside it are not evidence for it.
	var cells, completeness, zero []string
	var gp, cp []string
	goodIdx, candIdx := armRuns("known-good", g.Runs, &gp), armRuns("candidate", k.Runs, &cp)
	cells = append(cells, gp...)
	cells = append(cells, cp...)
	if len(g.Runs) == 0 || len(k.Runs) == 0 {
		cells = append(cells, "a bundle has no runs")
	}
	for _, ph := range p.Phases {
		for _, cpus := range p.cpusFor(ph) {
			key := runKey(ph.Name, cpus)
			gr, cr := goodIdx[key], candIdx[key]
			if len(gr) == 0 || len(cr) == 0 {
				cells = append(cells, fmt.Sprintf("required cell %s is missing from %s", key, missingArms(len(gr) == 0, len(cr) == 0)))
				continue
			}
			if !slices.Equal(sortedRounds(gr), sortedRounds(cr)) {
				cells = append(cells, fmt.Sprintf("%s was not interleaved: rounds %v vs %v", key, sortedRounds(gr), sortedRounds(cr)))
			}
			for _, rd := range sortedRounds(gr) {
				completeness = append(completeness, runProblems("known-good", p, ph, gr[rd], true)...)
				if gr[rd].Success == 0 {
					zero = append(zero, fmt.Sprintf("known-good %s round %d completed no valid inference", key, rd))
				}
			}
			for _, rd := range sortedRounds(cr) {
				completeness = append(completeness, runProblems("candidate", p, ph, cr[rd], false)...)
			}
		}
	}
	c.require(checkRequiredCells, cells, fmt.Sprintf("%d required cells present in both arms", p.cellCount()))
	c.require(checkRunsComplete, completeness, "every required run is complete with its required assertions recorded")
	c.require(checkControlNonzero, zero, "every known-good run completed valid inference")
	if c.has(outcomeInvalid) {
		return verdict{Status: statusInvalid, Checks: c.list}
	}

	compareNumbers(&c, p, th, goodIdx, candIdx)
	return verdict{Status: c.status(), Checks: c.list}
}

func missingArms(good, cand bool) string {
	switch {
	case good && cand:
		return "both arms"
	case good:
		return "known-good"
	default:
		return "candidate"
	}
}

// spread is (max-min)/median of the values.
func spread(vals []float64) float64 {
	lo, hi := slices.Min(vals), slices.Max(vals)
	if m := median64(vals); m > 0 {
		return (hi - lo) / m
	}
	return 0
}

func pick(runs map[int]phaseRun, f func(phaseRun) float64) []float64 {
	var out []float64
	for _, rd := range sortedRounds(runs) {
		out = append(out, f(runs[rd]))
	}
	return out
}

// compareNumbers applies the statistical gates. Ratio gates run only when the
// evidence is sufficient and quiet; otherwise they are inconclusive.
func compareNumbers(c *checks, p profile, th thresholds, goodIdx, candIdx map[string]map[int]phaseRun) {
	var thin, noisy, rounds []string
	for _, ph := range p.Phases {
		for _, cpus := range p.cpusFor(ph) {
			key := runKey(ph.Name, cpus)
			if n := len(goodIdx[key]); n < th.MinRounds {
				rounds = append(rounds, fmt.Sprintf("%s has %d rounds, need %d", key, n, th.MinRounds))
			}
			if !ph.Gated {
				continue
			}
			// The baseline must supply the sample (successful pairs and duration);
			// the candidate must run as long, but a candidate with few successes
			// is a throughput failure to report, not thin evidence to excuse.
			for label, idx := range map[string]map[int]phaseRun{"known-good": goodIdx[key], "candidate": candIdx[key]} {
				for _, rd := range sortedRounds(idx) {
					r := idx[rd]
					if (label == "known-good" && r.Success < th.MinPairs) || r.Seconds < th.MinSeconds {
						thin = append(thin, fmt.Sprintf("%s %s round %d has %d successful pairs over %.0fs (need %d over %.0fs)", label, key, rd, r.Success, r.Seconds, th.MinPairs, th.MinSeconds))
					}
				}
			}
			if s := spread(pick(goodIdx[key], func(r phaseRun) float64 { return r.PairsPerSecond })); s > th.NoiseThroughput {
				noisy = append(noisy, fmt.Sprintf("%s known-good throughput spread %.1f%% > %.1f%%", key, s*100, th.NoiseThroughput*100))
			}
			for name, f := range map[string]func(phaseRun) float64{
				"p50": func(r phaseRun) float64 { return r.Scheduled.P50 },
				"p95": func(r phaseRun) float64 { return r.Scheduled.P95 },
				"p99": func(r phaseRun) float64 { return r.Scheduled.P99 },
			} {
				if s := spread(pick(goodIdx[key], f)); s > th.NoiseLatency {
					noisy = append(noisy, fmt.Sprintf("%s known-good %s spread %.1f%% > %.1f%%", key, name, s*100, th.NoiseLatency*100))
				}
			}
		}
	}
	sort.Strings(thin)
	sort.Strings(noisy)
	evidence := true
	for _, g := range []struct {
		name  string
		items []string
		ok    string
	}{
		{checkRounds, rounds, fmt.Sprintf("at least %d interleaved rounds per cell", th.MinRounds)},
		{checkSampleSize, thin, fmt.Sprintf("every gated run has %d pairs over %.0fs", th.MinPairs, th.MinSeconds)},
		{checkNoise, noisy, "known-good is within noise tolerance across rounds"},
	} {
		if len(g.items) == 0 {
			c.add(g.name, outcomePass, "%s", g.ok)
			continue
		}
		evidence = false
		c.add(g.name, outcomeInconclusive, "%s", strings.Join(g.items, "; "))
	}

	ratio := func(name string, f func(phaseRun) float64, lowerBound bool, bound float64) {
		var bad []string
		for _, ph := range p.Phases {
			if !ph.Gated {
				continue
			}
			for _, cpus := range p.cpusFor(ph) {
				key := runKey(ph.Name, cpus)
				g, k := median64(pick(goodIdx[key], f)), median64(pick(candIdx[key], f))
				if g <= 0 {
					bad = append(bad, key+" known-good value is zero")
					continue
				}
				if r := k / g; (lowerBound && r < bound) || (!lowerBound && r > bound) {
					bad = append(bad, fmt.Sprintf("%s %.1f%% of known-good (limit %.0f%%)", key, r*100, bound*100))
				}
			}
		}
		switch {
		case !evidence:
			c.add(name, outcomeInconclusive, "not evaluated: the evidence is thin or noisy")
		case len(bad) > 0:
			c.add(name, outcomeFail, "%s", strings.Join(bad, "; "))
		default:
			c.add(name, outcomePass, "within %.0f%% of known-good", bound*100)
		}
	}
	ratio(checkThroughputFloor, func(r phaseRun) float64 { return r.PairsPerSecond }, true, th.MinThroughput)
	ratio(checkLatencyP50, func(r phaseRun) float64 { return r.Scheduled.P50 }, false, th.MaxLatency)
	ratio(checkLatencyP95, func(r phaseRun) float64 { return r.Scheduled.P95 }, false, th.MaxLatency)
	ratio(checkLatencyP99, func(r phaseRun) float64 { return r.Scheduled.P99 }, false, th.MaxLatency)

	var loss, asserts, recovery []string
	for _, ph := range p.Phases {
		for _, cpus := range p.cpusFor(ph) {
			key := runKey(ph.Name, cpus)
			for _, rd := range sortedRounds(candIdx[key]) {
				r := candIdx[key][rd]
				if ph.Sustainable && r.Errors+r.Timeouts+r.Dropped > 0 {
					loss = append(loss, fmt.Sprintf("%s round %d: %d errors, %d timeouts, %d dropped of %d offered", key, rd, r.Errors, r.Timeouts, r.Dropped, r.Offered))
				}
				for _, a := range r.Assertions {
					if a.Outcome == outcomeFail && slices.Contains(p.RequiredAssertions, a.Name) {
						asserts = append(asserts, fmt.Sprintf("%s round %d %s: %s", key, rd, a.Name, a.Detail))
					}
				}
				if ph.RecoveryDeadlineSeconds > 0 {
					limit := min(ph.RecoveryDeadlineSeconds, p.Deadlines.RecoverySeconds)
					if r.RecoverySeconds.val() > limit {
						recovery = append(recovery, fmt.Sprintf("%s round %d recovered in %.1fs, deadline %.1fs", key, rd, r.RecoverySeconds.val(), limit))
					}
				}
			}
		}
	}
	for _, g := range []struct {
		name  string
		items []string
		ok    string
	}{
		{checkNoUnexpectedLoss, loss, "no errors, timeouts or drops at the sustainable rate"},
		{checkCandidateAsserts, asserts, "the candidate's required assertions passed"},
		{checkRecoveryDeadline, recovery, "recovery is within the declared deadline"},
	} {
		if len(g.items) == 0 {
			c.add(g.name, outcomePass, "%s", g.ok)
		} else {
			c.add(g.name, outcomeFail, "%s", strings.Join(g.items, "; "))
		}
	}
}

func (v verdict) print() {
	for _, a := range v.Checks {
		fmt.Printf("  %-12s %-20s %s\n", a.Outcome, a.Name, a.Detail)
	}
	fmt.Printf("verdict: %s\n", v.Status)
}

// compareBundles is -mode compare minus flags and exit codes. expectFail names
// the check a known-bad arm must trip: the comparison then succeeds only when the
// verdict is fail because of exactly that check, never because of invalid
// evidence (a missing fixture or startup failure must not count as reproducing
// the regression).
func compareBundles(profilePath, goodPath, candPath, expectFail string) (bool, error) {
	p, err := loadProfile(profilePath)
	if err != nil {
		return false, err
	}
	good, err := loadResult(goodPath)
	if err != nil {
		return false, err
	}
	cand, err := loadResult(candPath)
	if err != nil {
		return false, err
	}
	v := compareArms(p, good, cand)
	v.print()
	if expectFail == "" {
		return v.Status == statusPass, nil
	}
	if v.Status != statusFail {
		return false, fmt.Errorf("expected the %s check to fail, but the verdict is %s", expectFail, v.Status)
	}
	for _, a := range v.Checks {
		if a.Name == expectFail && a.Outcome == outcomeFail {
			return true, nil
		}
	}
	return false, fmt.Errorf("the verdict is fail, but not on the expected %s check", expectFail)
}
