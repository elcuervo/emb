// Package hostcheck implements the validation host contract: it reports what a
// measurement host actually is, and whether that host may produce reference
// performance evidence.
//
// The CPU allowance is the server's own resolution (internal/registry), not a
// second cgroup parser, so the report and the server cannot disagree about the
// budget the server plans from. Steal and throttle are sampled from /proc/stat
// and the cgroup cpu.stat over a window: they are the properties that decide
// whether a CPU-utilization-fraction measurement means anything at all.
package hostcheck

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"runtime"
	"strconv"
	"strings"
	"time"

	"github.com/elcuervo/emb/internal/registry"
)

// Verdicts for the whole host.
const (
	// Qualified means the host may produce reference performance evidence.
	Qualified = "qualified"
	// Unqualified means it may not; the failed property names why.
	Unqualified = "unqualified"
)

// Per-property verdicts.
const (
	// VerdictOK means the property was measured inside its limit.
	VerdictOK = "ok"
	// VerdictHigh means the property was measured outside its limit.
	VerdictHigh = "high"
	// VerdictUnmeasured means the property could not be measured on this host.
	VerdictUnmeasured = "unmeasured"
)

// DefaultPlatform is the deployment target: the only architecture recorded
// numbers are comparable on. Cross-ISA comparisons are out of scope, so a host
// that is not this one cannot produce reference evidence.
const DefaultPlatform = "linux/arm64"

// Thresholds are the validation host contract's numeric limits. They are
// recorded in the report so a recorded verdict can be reproduced.
type Thresholds struct {
	// Platform is the deployment target as "os/arch".
	Platform string `json:"platform"`
	// QuotaShortfall is the fraction of the presented CPUs the effective
	// allowance may fall short of before the host counts as shared or
	// burstable. A quota below the presented CPUs makes the server plan from
	// the smaller number while the host may present more.
	QuotaShortfall float64 `json:"quota_shortfall"`
	// MaxStealPercent is the sampled CPU steal ceiling.
	MaxStealPercent float64 `json:"max_steal_percent"`
	// MaxThrottleRatio is the sampled throttled-period share ceiling.
	MaxThrottleRatio float64 `json:"max_throttle_ratio"`
	// SampleWindow is how long steal and throttle are sampled over.
	SampleWindow time.Duration `json:"sample_window_ns"`
}

// DefaultThresholds returns the contract's limits: a quota may fall 10% short
// of the presented CPUs, sampled steal must stay under 1%, throttled periods
// under 10%, sampled over one second.
func DefaultThresholds() Thresholds {
	return Thresholds{
		Platform:         DefaultPlatform,
		QuotaShortfall:   0.10,
		MaxStealPercent:  1.0,
		MaxThrottleRatio: 0.10,
		SampleWindow:     time.Second,
	}
}

// AllowanceView is the server's CPU resolution as reported and recorded: the
// fractional allowance, the file it came from, and the scheduler and affinity
// limits behind it. A host with no cgroup CPU file reports the
// scheduler/affinity source and no quota.
type AllowanceView struct {
	CPUs      float64 `json:"cpus"`
	Source    string  `json:"source"`
	Quota     bool    `json:"quota"`
	Scheduler int     `json:"scheduler"`
	Affinity  int     `json:"affinity"`
}

// MemoryView is the effective memory limit and the host memory behind it.
type MemoryView struct {
	LimitBytes uint64 `json:"limit_bytes"`
	HostBytes  uint64 `json:"host_bytes"`
	Source     string `json:"source"`
}

// StealView is the sampled share of CPU time the hypervisor took away.
type StealView struct {
	Measurable bool    `json:"measurable"`
	Percent    float64 `json:"percent"`
	Verdict    string  `json:"verdict"`
}

// ThrottleView is the sampled share of cgroup CPU periods the kernel throttled.
type ThrottleView struct {
	Measurable bool    `json:"measurable"`
	Ratio      float64 `json:"ratio"`
	Periods    uint64  `json:"periods"`
	Throttled  uint64  `json:"throttled"`
	Verdict    string  `json:"verdict"`
}

// Report is a host's measured shape together with the contract's verdict.
type Report struct {
	OS               string        `json:"os"`
	Arch             string        `json:"arch"`
	PresentedCPUs    int           `json:"presented_cpus"`
	Allowance        AllowanceView `json:"allowance"`
	Memory           MemoryView    `json:"memory"`
	ServerBudget     int           `json:"server_budget"`
	Steal            StealView     `json:"steal"`
	Throttle         ThrottleView  `json:"throttle"`
	Thresholds       Thresholds    `json:"thresholds"`
	Verdict          string        `json:"verdict"`
	FailedProperty   string        `json:"failed_property,omitempty"`
	FailedProperties []string      `json:"failed_properties,omitempty"`
}

// Sources are the kernel files the probe samples.
type Sources struct {
	// ProcStat provides the host's aggregate CPU counters (steal).
	ProcStat string
	// CPUStat provides the cgroup's throttled-period counters.
	CPUStat string
}

// DefaultSources returns the kernel interfaces the probe samples in production.
func DefaultSources() Sources {
	return Sources{ProcStat: "/proc/stat", CPUStat: "/sys/fs/cgroup/cpu.stat"}
}

// Probe measures this host and applies the contract.
func Probe(th Thresholds) Report {
	return probe(DefaultSources(), th)
}

// probe measures the host at sources. Tests point it at fixtures; production
// uses DefaultSources.
func probe(src Sources, th Thresholds) Report {
	allowance := registry.EffectiveCPUAllowance()
	limit := registry.EffectiveMemoryLimit()
	host := registry.TotalSystemMemory()
	memory := MemoryView{LimitBytes: limit, HostBytes: host, Source: "host"}
	if limit > 0 && limit < host {
		memory.Source = "cgroup"
	}
	r := Report{
		OS:            runtime.GOOS,
		Arch:          runtime.GOARCH,
		PresentedCPUs: runtime.NumCPU(),
		Allowance:     allowanceView(allowance),
		Memory:        memory,
		ServerBudget:  registry.EffectiveNumCPU(),
		Steal:         sampleSteal(src.ProcStat, th.SampleWindow, th.MaxStealPercent),
		Throttle:      sampleThrottle(src.CPUStat, th.SampleWindow, th.MaxThrottleRatio),
		Thresholds:    th,
	}
	r.Qualify()
	return r
}

// Qualify applies the contract to the report's measured fields, filling in
// Verdict, the headline FailedProperty and every FailedProperties entry. The
// first failed property is the one to fix.
func (r *Report) Qualify() {
	failed := make([]string, 0, 4)
	platform := r.OS + "/" + r.Arch
	if platform != r.Thresholds.Platform {
		failed = append(failed, fmt.Sprintf("platform %s is not the deployment target %s", platform, r.Thresholds.Platform))
	}
	if r.Allowance.Quota {
		floor := float64(r.PresentedCPUs) * (1 - r.Thresholds.QuotaShortfall)
		if r.Allowance.CPUs < floor {
			failed = append(failed, fmt.Sprintf("cpu quota: the cgroup allows %.2f CPUs of %d presented (server budget %d)",
				r.Allowance.CPUs, r.PresentedCPUs, r.ServerBudget))
		}
	}
	switch {
	case !r.Steal.Measurable:
		failed = append(failed, fmt.Sprintf("cpu steal is not measurable on %s", platform))
	case r.Steal.Percent > r.Thresholds.MaxStealPercent:
		failed = append(failed, fmt.Sprintf("cpu steal %.2f%% exceeds %.2f%%", r.Steal.Percent, r.Thresholds.MaxStealPercent))
	}
	if r.Throttle.Measurable && r.Throttle.Ratio > r.Thresholds.MaxThrottleRatio {
		failed = append(failed, fmt.Sprintf("cpu throttle ratio %.3f exceeds %.3f", r.Throttle.Ratio, r.Thresholds.MaxThrottleRatio))
	}
	r.FailedProperties = failed
	if len(failed) == 0 {
		r.Verdict, r.FailedProperty = Qualified, ""
		return
	}
	r.Verdict, r.FailedProperty = Unqualified, failed[0]
}

// Qualified reports whether the host may produce reference performance
// evidence.
func (r Report) Qualified() bool { return r.Verdict == Qualified }

// String renders the report as the lines the host check prints.
func (r Report) String() string {
	var b strings.Builder
	fmt.Fprintf(&b, "host: %s/%s, %d presented CPUs\n", r.OS, r.Arch, r.PresentedCPUs)
	quota := "no cgroup quota"
	if r.Allowance.Quota {
		quota = "cgroup quota"
	}
	fmt.Fprintf(&b, "cpu: %.2f CPUs from %s (%s), scheduler %d, affinity %d, server budget %d\n",
		r.Allowance.CPUs, r.Allowance.Source, quota, r.Allowance.Scheduler, r.Allowance.Affinity, r.ServerBudget)
	fmt.Fprintf(&b, "memory: %s limit %d bytes of %d host bytes\n", r.Memory.Source, r.Memory.LimitBytes, r.Memory.HostBytes)
	fmt.Fprintf(&b, "steal: %s", r.Steal.Verdict)
	if r.Steal.Measurable {
		fmt.Fprintf(&b, " (%.2f%% over %s)", r.Steal.Percent, r.Thresholds.SampleWindow)
	}
	fmt.Fprintf(&b, "\nthrottle: %s", r.Throttle.Verdict)
	if r.Throttle.Measurable {
		fmt.Fprintf(&b, " (%d of %d periods throttled, ratio %.3f)", r.Throttle.Throttled, r.Throttle.Periods, r.Throttle.Ratio)
	}
	fmt.Fprintf(&b, "\nverdict: %s", r.Verdict)
	if r.Verdict != Qualified {
		fmt.Fprintf(&b, ": %s", strings.Join(r.FailedProperties, "; "))
	}
	b.WriteString("\n")
	return b.String()
}

// Fingerprint is the recorded identity of a measurement host: the shape two
// runs must share before their numbers may be compared, plus the sampled
// steal and throttle verdicts the contract decided on.
type Fingerprint struct {
	OS              string        `json:"os"`
	Arch            string        `json:"arch"`
	PresentedCPUs   int           `json:"presented_cpus"`
	Allowance       AllowanceView `json:"allowance"`
	MemoryLimit     uint64        `json:"memory_limit_bytes"`
	ServerBudget    int           `json:"server_budget"`
	StealVerdict    string        `json:"steal_verdict"`
	ThrottleVerdict string        `json:"throttle_verdict"`
}

// Fingerprint returns the host's recorded identity.
func (r Report) Fingerprint() Fingerprint {
	return Fingerprint{
		OS:              r.OS,
		Arch:            r.Arch,
		PresentedCPUs:   r.PresentedCPUs,
		Allowance:       r.Allowance,
		MemoryLimit:     r.Memory.LimitBytes,
		ServerBudget:    r.ServerBudget,
		StealVerdict:    r.Steal.Verdict,
		ThrottleVerdict: r.Throttle.Verdict,
	}
}

// Identity is the digest two bundles must share before the diff may compare
// them. It covers the stable host shape only: the sampled steal and throttle
// verdicts are recorded beside it but excluded, because they follow the load
// on the host rather than its identity, and a key that drifted with them could
// never be compared against itself. An unqualified candidate run is refused by
// the contract (with the failed property named), not by this digest.
func (f Fingerprint) Identity() string {
	parts := []string{
		f.OS, f.Arch, strconv.Itoa(f.PresentedCPUs),
		strconv.FormatFloat(f.Allowance.CPUs, 'f', 4, 64),
		f.Allowance.Source,
		strconv.FormatBool(f.Allowance.Quota),
		strconv.Itoa(f.Allowance.Scheduler),
		strconv.Itoa(f.Allowance.Affinity),
		strconv.FormatUint(f.MemoryLimit, 10),
		strconv.Itoa(f.ServerBudget),
	}
	sum := sha256.Sum256([]byte(strings.Join(parts, "\x00")))
	return hex.EncodeToString(sum[:])[:16]
}

// allowanceView records the server's resolution.
func allowanceView(a registry.CPUAllowance) AllowanceView {
	return AllowanceView{
		CPUs:      a.CPUs,
		Source:    a.Source,
		Quota:     a.Source != "" && a.Source != "scheduler/affinity",
		Scheduler: a.Scheduler,
		Affinity:  a.Affinity,
	}
}

// sampleSteal measures the stolen-CPU share across window. A platform without
// /proc/stat (or any other read failure) reports the property as unmeasured
// rather than as zero: an unmeasurable property is not a passing one.
func sampleSteal(path string, window time.Duration, maxPercent float64) StealView {
	prev, err := readCPUTimes(path)
	if err != nil {
		return StealView{Verdict: VerdictUnmeasured}
	}
	if window > 0 {
		time.Sleep(window)
	}
	cur, err := readCPUTimes(path)
	if err != nil {
		return StealView{Verdict: VerdictUnmeasured}
	}
	pct := stealPercent(prev, cur)
	verdict := VerdictOK
	if pct > maxPercent {
		verdict = VerdictHigh
	}
	return StealView{Measurable: true, Percent: pct, Verdict: verdict}
}

// sampleThrottle measures the throttled-period share across window. A missing
// cpu.stat (cgroup v1, or a platform without cgroups) reports the property as
// unmeasured.
func sampleThrottle(path string, window time.Duration, maxRatio float64) ThrottleView {
	prev, err := readCPUStat(path)
	if err != nil {
		return ThrottleView{Verdict: VerdictUnmeasured}
	}
	if window > 0 {
		time.Sleep(window)
	}
	cur, err := readCPUStat(path)
	if err != nil {
		return ThrottleView{Verdict: VerdictUnmeasured}
	}
	ratio := throttleRatio(prev, cur)
	verdict := VerdictOK
	if ratio > maxRatio {
		verdict = VerdictHigh
	}
	return ThrottleView{
		Measurable: true,
		Ratio:      ratio,
		Periods:    cur.periods - prev.periods,
		Throttled:  cur.throttled - prev.throttled,
		Verdict:    verdict,
	}
}

// cpuTimes is the aggregate "cpu" line of /proc/stat.
type cpuTimes struct {
	total uint64
	steal uint64
}

// readCPUTimes reads the host's aggregate CPU counters.
func readCPUTimes(path string) (cpuTimes, error) {
	f, err := os.Open(path)
	if err != nil {
		return cpuTimes{}, err
	}
	defer func() { _ = f.Close() }()
	return parseCPUTimes(f)
}

// parseCPUTimes parses the aggregate "cpu" line: user nice system idle iowait
// irq softirq steal guest guest_nice. The guest columns are not summed: the
// kernel already counts guest time inside user and guest_nice inside nice, so
// adding them again would inflate the denominator and understate steal. A steal
// field the kernel does not report counts as zero, which is what a host that
// never loses CPU time reports.
func parseCPUTimes(r io.Reader) (cpuTimes, error) {
	data, err := io.ReadAll(r)
	if err != nil {
		return cpuTimes{}, err
	}
	for _, line := range strings.Split(string(data), "\n") {
		fields := strings.Fields(line)
		if len(fields) < 2 || fields[0] != "cpu" {
			continue
		}
		var t cpuTimes
		for i, raw := range fields[1:] {
			if i > 7 {
				break
			}
			v, err := strconv.ParseUint(raw, 10, 64)
			if err != nil {
				return cpuTimes{}, fmt.Errorf("cpu field %d: %w", i, err)
			}
			t.total += v
			if i == 7 {
				t.steal = v
			}
		}
		return t, nil
	}
	return cpuTimes{}, errors.New("no aggregate cpu line")
}

// stealPercent is the share of CPU time the hypervisor took away between two
// reads.
func stealPercent(prev, cur cpuTimes) float64 {
	if cur.total <= prev.total || cur.steal < prev.steal {
		return 0
	}
	return float64(cur.steal-prev.steal) / float64(cur.total-prev.total) * 100
}

// cpuStat is the subset of the cgroup cpu.stat file the throttle verdict needs.
type cpuStat struct {
	periods   uint64
	throttled uint64
}

// readCPUStat reads the cgroup's throttled-period counters.
func readCPUStat(path string) (cpuStat, error) {
	f, err := os.Open(path)
	if err != nil {
		return cpuStat{}, err
	}
	defer func() { _ = f.Close() }()
	return parseCPUStat(f)
}

// parseCPUStat parses cgroup v2 cpu.stat lines ("nr_periods 12").
func parseCPUStat(r io.Reader) (cpuStat, error) {
	data, err := io.ReadAll(r)
	if err != nil {
		return cpuStat{}, err
	}
	var s cpuStat
	seen := 0
	for _, line := range strings.Split(string(data), "\n") {
		fields := strings.Fields(line)
		if len(fields) != 2 {
			continue
		}
		v, err := strconv.ParseUint(fields[1], 10, 64)
		if err != nil {
			continue
		}
		switch fields[0] {
		case "nr_periods":
			s.periods, seen = v, seen+1
		case "nr_throttled":
			s.throttled, seen = v, seen+1
		}
	}
	if seen < 2 {
		return cpuStat{}, errors.New("cpu.stat: missing nr_periods/nr_throttled")
	}
	return s, nil
}

// throttleRatio is the share of CPU periods the kernel throttled between two
// reads.
func throttleRatio(prev, cur cpuStat) float64 {
	if cur.periods <= prev.periods || cur.throttled < prev.throttled {
		return 0
	}
	return float64(cur.throttled-prev.throttled) / float64(cur.periods-prev.periods)
}
