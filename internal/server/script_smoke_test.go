package server

import (
	"bufio"
	"net"
	"os"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/elcuervo/emb/internal/config"
	"github.com/elcuervo/emb/internal/registry"
)

// The autotune smoke gate: the per-change performance test of the scripted
// path's runtime contract. It drives the REAL sampler (production 1 s window)
// and the real session pool against the committed testdata/laya miniature, so
// it needs no model download and no Docker, and asserts on state (traffic
// class, concurrency allowance, layout, run counters) rather than on wall-clock
// durations. Phase lengths follow from the controller's own dwell: a shrink
// proof costs the sampler's ten sustained-latency windows.
//
// Enabled by EMB_BENCH_SMOKE=1 (`just bench-smoke`, and the CGo CI job). The
// pure dwell arithmetic stays covered by internal/permit's deterministic unit
// tests; this gate exists for the wiring those tests cannot reach: sampler ->
// pool capacity -> counters -> EMB.INFO.
const (
	// smokeArm is this gate's arm name in its summary line. `-run Smoke`
	// selects two arms — this adaptive autotune arm and the fixed-concurrency
	// control arm in emb_smoke_test.go — so the summary names the arm it
	// summarizes and a check can require proof from the arm it is checking
	// rather than any summary the run happened to print.
	smokeArm = "autotune"
	// smokeModel is the fixture model name; the preset reads fixed inputs, so
	// every request in every arm produces the same reply.
	smokeModel = "smoke"
	// smokeCap is script_callers_per_session: the allowance ceiling.
	smokeCap = 4
	// smokeWorkers is script_workers: one session, so a burst queues behind it
	// and the allowance is the whole concurrency budget.
	smokeWorkers = 1
	// smokeIntra is intra_op_threads: one thread per session, so the burst's
	// CPU is bounded by the pool capacity and the denominator stays legible.
	smokeIntra = 1
	// smokeQuietWindows is how long the sustained serial phase runs before the
	// allowance must have halved: the controller's autotuneShrinkAfter dwell is
	// 10 consecutive latency windows.
	smokeQuietWindows = 12
	// smokeIdleWait bounds how long the idle phase waits for the sampler to
	// classify idle (the window is one second, and a burst's tail can take a
	// window to clear).
	smokeIdleWait = 6 * time.Second
	// smokeSaturationThreshold mirrors the sampler's autotuneCPUHigh: the CPU
	// fraction at or above which a class of `saturated` is required.
	smokeSaturationThreshold = 0.90
	// smokeThroughputFloorPct is how far the adaptive arm's throughput may fall
	// below the fixed-concurrency control arm measured in the same run. Both
	// arms run the same layout and cap, so the adaptive arm has to be at least
	// as good once it has expanded; the floor absorbs the client and scheduler
	// noise of a shared runner while still catching a collapse.
	smokeThroughputFloorPct = 15.0
	// smokeCoresShape, smokeCoresSaturating and smokeCoresUnreachable are the
	// GOMAXPROCS pins that fix the classifier's CPU-fraction denominator
	// (registry.EffectiveNumCPU falls back to GOMAXPROCS when no cgroup quota is
	// enforced). Pinning it is what makes a class assertion deterministic
	// instead of host-dependent.
	smokeCoresShape       = 12
	smokeCoresSaturating  = 1
	smokeCoresUnreachable = 16
)

// smokeEnabled gates the whole gate on the environment.
func smokeEnabled(t *testing.T) {
	t.Helper()
	if os.Getenv("EMB_BENCH_SMOKE") == "" {
		t.Skip("set EMB_BENCH_SMOKE=1 to run the autotune smoke gate")
	}
	if !ortOK {
		// The gate was explicitly asked for: report the broken environment
		// rather than skipping into a green run that measured nothing.
		t.Fatal("EMB_BENCH_SMOKE=1 but the ONNX runtime is unavailable: run inside nix develop")
	}
}

// smokeInfo is the per-model state the gate asserts on, read from EMB.INFO.
type smokeInfo struct {
	class     string
	inflight  int
	current   int
	target    int
	active    bool
	runs      int
	sessions  int
	intra     int
	spinning  bool
	cacheHits int
}

// smokeRead samples EMB.INFO for the fixture model.
func smokeRead(t *testing.T, addr string) smokeInfo {
	t.Helper()
	f := statsFields(t, redisCmd(t, addr, "EMB.INFO", smokeModel))
	str := func(key string) string { return bulkOf(t, f[key]) }
	num := func(key string) int { return intOf(t, f[key]) }
	info := smokeInfo{
		class:    str("script_traffic_class"),
		inflight: num("script_inflight"),
		current:  num("script_concurrency_current"),
		target:   num("script_concurrency_target"),
		active:   num("script_autotune_active") != 0,
		runs:     num("script_runs"),
		sessions: num("script_sessions"),
		intra:    num("intra_op_threads"),
		spinning: num("allow_spinning") != 0,
	}
	if tok, ok := f["cache_hits"]; ok {
		info.cacheHits = intOf(t, tok)
	}
	return info
}

// smokeServer is one gate arm's in-process server.
type smokeServer struct {
	addr  string
	sha   string
	cores int
}

// smokeConfig is the fixture model's layout: one session at the cap, one
// intra-op thread, and the laya miniature the repository already commits.
func smokeConfig() config.ModelConfig {
	return config.ModelConfig{
		ONNX:                    "../../testdata/laya/model.onnx",
		Tokenizer:               "../../testdata/laya/tokenizer/tokenizer.json",
		MaxLength:               64,
		ScriptWorkers:           smokeWorkers,
		ScriptCallersPerSession: smokeCap,
		IntraOpThreads:          smokeIntra,
		// One inter-op thread as well: the burst's CPU is then bounded by the
		// pool capacity, which is what makes the class assertions reproducible
		// instead of a race between the pool and ORT's own parallelism.
		InterOpThreads: 1,
	}
}

// smokeStart starts a server with the fixture preset preloaded. mutate may
// adjust the model's configuration (autotune, capacity) before the model is
// loaded, and cacheCfg is the server's reply-cache size ("" disables it).
func smokeStart(t *testing.T, mutate func(*config.ModelConfig), cacheCfg string) *smokeServer {
	t.Helper()
	cfg := smokeConfig()
	if mutate != nil {
		mutate(&cfg)
	}
	reg := registry.New()
	entry, err := registry.LoadModel(cfg, smokeModel)
	if err != nil {
		t.Fatalf("loading the %s fixture: %v", smokeModel, err)
	}
	reg.Add(smokeModel, entry)
	addr := getFreeAddr()
	srv := New(addr, reg, "", cacheCfg, nil)
	go srv.ListenAndServe()
	t.Cleanup(func() {
		_ = srv.Close()
		_ = reg.Close()
	})
	time.Sleep(50 * time.Millisecond)

	src, err := os.ReadFile("../../testdata/laya/smoke.lua")
	if err != nil {
		t.Fatalf("reading the smoke fixture preset: %v", err)
	}
	sha, err := srv.PreloadScript(smokeModel, string(src))
	if err != nil {
		t.Fatalf("preloading the smoke fixture preset: %v", err)
	}
	s := &smokeServer{addr: addr, sha: sha, cores: registry.EffectiveNumCPU()}
	// Warm the script path: the model's autotune fields are only published once
	// its resources are open, so an arm that read state first would read zeros.
	if reply := smokeReply(t, s); reply == "" {
		t.Fatal("the smoke fixture preset returned an empty reply")
	}
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if smokeRead(t, s.addr).target > 0 {
			return s
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatal("the script path never published its autotune state")
	return s
}

// smokeWaitClass polls the class until it matches, reporting whether it did.
func smokeWaitClass(t *testing.T, addr, want string, timeout time.Duration) bool {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if smokeRead(t, addr).class == want {
			return true
		}
		time.Sleep(100 * time.Millisecond)
	}
	return false
}

// withGOMAXPROCS pins the classifier's CPU-fraction denominator for one arm.
func withGOMAXPROCS(t *testing.T, n int) {
	t.Helper()
	prev := runtime.GOMAXPROCS(n)
	t.Cleanup(func() { runtime.GOMAXPROCS(prev) })
}

// smokePartition reports the CPU partition the gate runs under, reusing the
// app/bench knobs the Ruby harness uses (the justfile's app_cpus/bench_cpus).
// The server's schedulable budget is the app partition; the load generator
// shares the process with it, so on a host with enough cores the partition is a
// real split of the schedulable budget.
func smokePartition() (app, bench, host int, applied bool) {
	app, bench = 6, 4
	if v := os.Getenv("EMB_BENCH_APP_CPUS"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			app = n
		}
	}
	if v := os.Getenv("EMB_BENCH_BENCH_CPUS"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			bench = n
		}
	}
	host = runtime.NumCPU()
	applied = app+bench <= host
	if !applied {
		// The host cannot hold both partitions: keep the server's budget a
		// strict subset of it rather than a slice it cannot honour.
		app = max(1, host-1)
	}
	return app, bench, host, applied
}

// ---- load generation -------------------------------------------------------

// smokeClient is one connection issuing one request at a time.
type smokeClient struct {
	conn *net.TCPConn
	r    *bufio.Reader
}

func smokeDial(addr string) (*smokeClient, error) {
	conn, err := net.Dial("tcp", addr)
	if err != nil {
		return nil, err
	}
	return &smokeClient{conn: conn.(*net.TCPConn), r: bufio.NewReader(conn)}, nil
}

// call issues one fixture request and returns the reply's exact bytes.
func (c *smokeClient) call(sha string) (string, error) {
	if _, err := c.conn.Write(respCommand("EMB.EVSHA", smokeModel, sha, "1", "x")); err != nil {
		return "", err
	}
	var buf []byte
	for {
		chunk := make([]byte, 4096)
		n, err := c.r.Read(chunk)
		if err != nil {
			return "", err
		}
		buf = append(buf, chunk[:n]...)
		if consumed, ok := respValueLen(buf); ok && consumed == len(buf) {
			return string(buf), nil
		}
	}
}

func (c *smokeClient) close() { _ = c.conn.Close() }

// smokeReply issues one request on its own connection and returns the reply.
func smokeReply(t *testing.T, s *smokeServer) string {
	t.Helper()
	c, err := smokeDial(s.addr)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer c.close()
	reply, err := c.call(s.sha)
	if err != nil {
		t.Fatalf("fixture request: %v", err)
	}
	return reply
}

// smokeSerial issues one request at a time with a pause between them, until
// stop closes: the load shape that must classify latency.
func smokeSerial(addr, sha string, pause time.Duration, stop <-chan struct{}) (int64, error) {
	c, err := smokeDial(addr)
	if err != nil {
		return 0, err
	}
	defer c.close()
	var n int64
	for {
		select {
		case <-stop:
			return n, nil
		default:
		}
		if _, err := c.call(sha); err != nil {
			return n, err
		}
		n++
		time.Sleep(pause)
	}
}

// smokeBurst drives clients concurrent clients until stop closes and returns
// the requests they completed. A failed request ends that client: a short
// window must not be mistaken for throughput.
func smokeBurst(addr, sha string, clients int, stop <-chan struct{}) int64 {
	var done atomic.Int64
	var wg sync.WaitGroup
	for range clients {
		wg.Add(1)
		go func() {
			defer wg.Done()
			c, err := smokeDial(addr)
			if err != nil {
				return
			}
			defer c.close()
			for {
				select {
				case <-stop:
					return
				default:
				}
				if _, err := c.call(sha); err != nil {
					return
				}
				done.Add(1)
			}
		}()
	}
	wg.Wait()
	return done.Load()
}

// smokeCPUSlice measures the process CPU spent over d as a fraction of
// d x cores: the quantity the classifier thresholds at autotuneCPUHigh. The
// load generator and the server share this process, which is why a burst that
// cannot reach the threshold is reported as skipped rather than passed.
//
// A reading that does not advance is treated as a quiet slice, not as a spike:
// the platform's process CPU counters can come back as zero when the kernel
// query fails (gopsutil ignores the failure of the darwin proc_pidinfo call),
// and an unsigned delta over that would report an impossible fraction and fail
// the saturation assertion on a host that is fine.
func smokeCPUSlice(d time.Duration, cores int) float64 {
	if cores < 1 {
		cores = 1
	}
	u0, s0 := registry.ProcessCPUTimes()
	time.Sleep(d)
	u1, s1 := registry.ProcessCPUTimes()
	if u1 < u0 || s1 < s0 {
		return 0
	}
	used := float64((u1-u0)+(s1-s0)) / 1e6
	return used / (d.Seconds() * float64(cores))
}

// ---- phases ----------------------------------------------------------------

// smokeSerialPhase drives serial traffic for d, sampling the class.
func smokeSerialPhase(t *testing.T, s *smokeServer, d, pause time.Duration) []smokeInfo {
	t.Helper()
	stop := make(chan struct{})
	var wg sync.WaitGroup
	var requests atomic.Int64
	wg.Add(1)
	go func() {
		defer wg.Done()
		n, _ := smokeSerial(s.addr, s.sha, pause, stop)
		requests.Store(n)
	}()

	var samples []smokeInfo
	deadline := time.Now().Add(d)
	for time.Now().Before(deadline) {
		samples = append(samples, smokeRead(t, s.addr))
		time.Sleep(200 * time.Millisecond)
	}
	close(stop)
	wg.Wait()
	t.Logf("emb-smoke: serial phase %s: %d requests, cores=%d", d, requests.Load(), s.cores)
	return samples
}

// smokeBurstPhase drives a burst for d, sampling the class and the CPU fraction
// the burst actually reached.
func smokeBurstPhase(t *testing.T, s *smokeServer, clients int, d time.Duration) (samples []smokeInfo, peakCPU float64, completed int64) {
	t.Helper()
	stop := make(chan struct{})
	var wg sync.WaitGroup
	var done atomic.Int64
	wg.Add(1)
	go func() {
		defer wg.Done()
		done.Store(smokeBurst(s.addr, s.sha, clients, stop))
	}()

	deadline := time.Now().Add(d)
	for time.Now().Before(deadline) {
		samples = append(samples, smokeRead(t, s.addr))
		if f := smokeCPUSlice(250*time.Millisecond, s.cores); f > peakCPU {
			peakCPU = f
		}
	}
	close(stop)
	wg.Wait()
	t.Logf("emb-smoke: burst phase %s: %d clients, %d requests, peak CPU fraction %.2f of %d cores",
		d, clients, done.Load(), peakCPU, s.cores)
	return samples, peakCPU, done.Load()
}

// smokeIdlePhase samples with no traffic flowing. It first waits, within a
// bound, for the sampler to classify idle: the class only changes on the
// sampler's own one-second tick, so a burst's tail can spill into the first
// window after it stops and a fixed-length phase would assert on the sampler's
// phase alignment rather than on the property. The wait is reported, and if it
// expires the samples below simply never contain idle, which is what the
// assertion checks.
func smokeIdlePhase(t *testing.T, s *smokeServer, d time.Duration) []smokeInfo {
	t.Helper()
	waitDeadline := time.Now().Add(smokeIdleWait)
	for time.Now().Before(waitDeadline) {
		if smokeRead(t, s.addr).class == "idle" {
			t.Logf("emb-smoke: idle phase: classified idle after %s", smokeIdleWait-time.Until(waitDeadline))
			break
		}
		time.Sleep(100 * time.Millisecond)
	}
	var samples []smokeInfo
	deadline := time.Now().Add(d)
	for time.Now().Before(deadline) {
		samples = append(samples, smokeRead(t, s.addr))
		time.Sleep(200 * time.Millisecond)
	}
	return samples
}

// smokeCachePhase serves the same request repeatedly (all hits after the first)
// for d, sampling the class and the run counter.
func smokeCachePhase(t *testing.T, s *smokeServer, d time.Duration) (samples []smokeInfo, requests int64) {
	t.Helper()
	stop := make(chan struct{})
	var wg sync.WaitGroup
	var done atomic.Int64
	wg.Add(1)
	go func() {
		defer wg.Done()
		n, _ := smokeSerial(s.addr, s.sha, 5*time.Millisecond, stop)
		done.Store(n)
	}()

	deadline := time.Now().Add(d)
	for time.Now().Before(deadline) {
		samples = append(samples, smokeRead(t, s.addr))
		time.Sleep(200 * time.Millisecond)
	}
	close(stop)
	wg.Wait()
	return samples, done.Load()
}

// smokeTimedBurst runs a timed burst and returns its completed requests per
// second.
func smokeTimedBurst(s *smokeServer, clients int, d time.Duration) float64 {
	stop := make(chan struct{})
	var wg sync.WaitGroup
	var done atomic.Int64
	wg.Add(1)
	go func() {
		defer wg.Done()
		done.Store(smokeBurst(s.addr, s.sha, clients, stop))
	}()
	time.Sleep(d)
	close(stop)
	wg.Wait()
	return float64(done.Load()) / d.Seconds()
}

// ---- samples ---------------------------------------------------------------

// smokeClasses collapses a sample run into the ordered class transitions it saw.
func smokeClasses(samples []smokeInfo) string {
	seen := make([]string, 0, len(samples))
	for _, s := range samples {
		if len(seen) == 0 || seen[len(seen)-1] != s.class {
			seen = append(seen, s.class)
		}
	}
	return strings.Join(seen, ",")
}

func smokeSeen(samples []smokeInfo, class string) bool {
	for _, s := range samples {
		if s.class == class {
			return true
		}
	}
	return false
}

// smokePeakInflight is the highest in-flight concurrency the sampler saw: the
// evidence behind a class, since a serial phase that queues is not latency.
func smokePeakInflight(samples []smokeInfo) int {
	peak := 0
	for _, s := range samples {
		peak = max(peak, s.inflight)
	}
	return peak
}

func smokeAllowanceBounds(samples []smokeInfo) (low, high int) {
	low, high = samples[0].current, samples[0].current
	for _, s := range samples {
		low = min(low, s.current)
		high = max(high, s.current)
	}
	return low, high
}

// smokeAtMostChanges reports the number of times the allowance changed.
func smokeAllowanceChanges(samples []smokeInfo) int {
	changes := 0
	for i := 1; i < len(samples); i++ {
		if samples[i].current != samples[i-1].current {
			changes++
		}
	}
	return changes
}

// smokeSaturationVerdict decides the saturation assertion from what the burst
// produced: a pass when the class reached `saturated`, a skip naming the host
// property when the burst could not reach the CPU fraction that requires, and a
// failure when it reached the fraction without the class following.
func smokeSaturationVerdict(saturated bool, peakCPU float64, cores int) (result, detail string) {
	switch {
	case saturated:
		return "pass", "the burst classified saturated (peak CPU fraction " + trim2(peakCPU) + " of " + strconv.Itoa(cores) + " cores)"
	case peakCPU >= smokeSaturationThreshold:
		return "fail", "the burst reached a CPU fraction of " + trim2(peakCPU) + " of " + strconv.Itoa(cores) + " cores without classifying saturated"
	default:
		return "skipped", "the burst reached only " + trim2(peakCPU) + " of the " + trim2(smokeSaturationThreshold) +
			" CPU fraction required, on " + strconv.Itoa(cores) + " cores: this host cannot exercise the assertion"
	}
}

// ---- assertions ------------------------------------------------------------

// smokeOutcome is one assertion's recorded result.
type smokeOutcome struct {
	name   string
	result string // pass | fail | skipped
	detail string
}

// smokeGate collects the gate's assertion outcomes. Behavioural assertions are
// exact (a class, an allowance); only throughput is tolerance-gated. An
// assertion the host cannot exercise is recorded as skipped with the host
// property that prevented it, never as a pass.
type smokeGate struct {
	outcomes []smokeOutcome
}

func newSmokeGate() *smokeGate { return &smokeGate{} }

func (g *smokeGate) check(t *testing.T, name string, ok bool, detail string) bool {
	t.Helper()
	result := "pass"
	if !ok {
		result = "fail"
		t.Errorf("emb-smoke: assertion %s failed: %s", name, detail)
	}
	g.outcomes = append(g.outcomes, smokeOutcome{name: name, result: result, detail: detail})
	return ok
}

func (g *smokeGate) skip(name, property string) {
	g.outcomes = append(g.outcomes, smokeOutcome{name: name, result: "skipped", detail: property})
}

// report prints every outcome and the machine-readable summary the CI job
// asserts on, and fails the test when any assertion failed.
func (g *smokeGate) report(t *testing.T) {
	t.Helper()
	pass, fail, skipped := 0, 0, 0
	for _, o := range g.outcomes {
		t.Logf("emb-smoke assertion %-20s %-7s %s", o.name, o.result, o.detail)
		switch o.result {
		case "pass":
			pass++
		case "fail":
			fail++
		default:
			// A skip stays a skip, naming the host property that prevented the
			// assertion: it is never a pass, and never dropped from the counts
			// (so an assertion that did not run cannot hide in this arm's total).
			skipped++
		}
	}
	t.Logf("emb-smoke: arm=%s assertions=%d pass=%d fail=%d skipped=%d",
		smokeArm, len(g.outcomes), pass, fail, skipped)
	if fail > 0 {
		t.Fatalf("emb-smoke: %d assertion(s) failed", fail)
	}
}

func trim0(f float64) string { return strconv.FormatFloat(f, 'f', 0, 64) }
func trim1(f float64) string { return strconv.FormatFloat(f, 'f', 1, 64) }
func trim2(f float64) string { return strconv.FormatFloat(f, 'f', 2, 64) }

func trim1s(vals []float64) string {
	parts := make([]string, 0, len(vals))
	for _, v := range vals {
		parts = append(parts, trim1(v))
	}
	return "[" + strings.Join(parts, " ") + "]"
}

// median returns the middle of the sorted samples.
func median(rates []float64) float64 {
	sorted := append([]float64{}, rates...)
	for i := 1; i < len(sorted); i++ {
		for j := i; j > 0 && sorted[j] < sorted[j-1]; j-- {
			sorted[j], sorted[j-1] = sorted[j-1], sorted[j]
		}
	}
	n := len(sorted)
	if n == 0 {
		return 0
	}
	if n%2 == 1 {
		return sorted[n/2]
	}
	return (sorted[n/2-1] + sorted[n/2]) / 2
}

// ---- the gate --------------------------------------------------------------

// TestAutotuneSmokeGate is the per-change gate. Run it with EMB_BENCH_SMOKE=1
// (`just bench-smoke`); every assertion's outcome is printed, and an assertion
// this host cannot exercise is reported as skipped with the host property that
// prevented it.
func TestAutotuneSmokeGate(t *testing.T) {
	smokeEnabled(t)

	app, bench, host, applied := smokePartition()
	t.Logf("emb-smoke: partition app=%d bench=%d host=%d applied=%t", app, bench, host, applied)

	g := newSmokeGate()
	g.check(t, "partition-reported", app >= 1 && app < host,
		"app="+strconv.Itoa(app)+" bench="+strconv.Itoa(bench)+" host="+strconv.Itoa(host)+" applied="+strconv.FormatBool(applied))

	t.Run("shape", func(t *testing.T) { smokeShapeArm(t, g) })
	t.Run("saturation", func(t *testing.T) { smokeSaturationArm(t, g) })
	t.Run("unreachable-saturation", func(t *testing.T) { smokeUnreachableArm(t, g) })
	t.Run("kill-switch", func(t *testing.T) { smokeKillSwitchArm(t, g) })
	t.Run("capacity-profiles", func(t *testing.T) { smokeProfileArm(t, g) })
	t.Run("cache-idle", func(t *testing.T) { smokeCacheArm(t, g) })
	t.Run("reply-identity", func(t *testing.T) { smokeIdentityArm(t, g) })
	t.Run("relative-throughput", func(t *testing.T) { smokeThroughputArm(t, g, app) })

	g.report(t)
}

// smokeShapeArm drives the contract's shape: serial -> idle -> sustained serial
// (the allowance halves) -> burst -> serial again, and asserts the class
// sequence, the allowance transitions and bounds, and that adaptation never
// reshapes the layout.
func smokeShapeArm(t *testing.T, g *smokeGate) {
	withGOMAXPROCS(t, smokeCoresShape)
	s := smokeStart(t, nil, "")
	start := smokeRead(t, s.addr)
	t.Logf("emb-smoke: shape arm cores=%d start class=%s current=%d", s.cores, start.class, start.current)

	serial1 := smokeSerialPhase(t, s, 3*time.Second, 20*time.Millisecond)
	idle := smokeIdlePhase(t, s, 2*time.Second)
	serial2 := smokeSerialPhase(t, s, time.Duration(smokeQuietWindows)*time.Second, 20*time.Millisecond)
	burst, peakCPU, _ := smokeBurstPhase(t, s, 8, 5*time.Second)
	serial3 := smokeSerialPhase(t, s, 5*time.Second, 20*time.Millisecond)
	all := make([]smokeInfo, 0, len(serial1)+len(idle)+len(serial2)+len(burst)+len(serial3))
	for _, group := range [][]smokeInfo{serial1, idle, serial2, burst, serial3} {
		all = append(all, group...)
	}

	g.check(t, "class-sequence", smokeSeen(serial1, "latency") && smokeSeen(idle, "idle") &&
		smokeSeen(serial2, "latency") && (smokeSeen(burst, "throughput") || smokeSeen(burst, "saturated")) &&
		smokeSeen(serial3, "latency"),
		"serial="+smokeClasses(serial1)+" idle="+smokeClasses(idle)+" sustained="+smokeClasses(serial2)+
			" burst="+smokeClasses(burst)+" after="+smokeClasses(serial3)+
			" (peak in-flight: serial "+strconv.Itoa(smokePeakInflight(serial1))+
			", burst "+strconv.Itoa(smokePeakInflight(burst))+")")

	low, high := smokeAllowanceBounds(all)
	g.check(t, "allowance-bounds", low >= 1 && high <= smokeCap,
		"the allowance stayed in ["+strconv.Itoa(low)+","+strconv.Itoa(high)+"] against the cap "+strconv.Itoa(smokeCap)+" and the floor 1")

	halved := 0
	for _, sm := range serial2 {
		if sm.current < smokeCap {
			halved = sm.current
			break
		}
	}
	g.check(t, "allowance-halved", halved == smokeCap/2,
		"after "+strconv.Itoa(smokeQuietWindows)+" sustained latency windows the allowance was "+strconv.Itoa(halved)+
			", want "+strconv.Itoa(smokeCap/2))
	g.check(t, "allowance-damped", smokeAllowanceChanges(serial2) <= 2,
		strconv.Itoa(smokeAllowanceChanges(serial2))+" allowance changes across "+strconv.Itoa(len(serial2))+" sustained windows")

	// The burst expands the allowance back to the cap on its first throughput
	// window. A host whose burst saturates before it queues cannot exercise
	// that, so it is reported with the host property instead of passed.
	if smokeSeen(burst, "throughput") {
		restored := 0
		for _, sm := range burst {
			restored = max(restored, sm.current)
		}
		g.check(t, "allowance-restored", restored == smokeCap,
			"the first throughput window restored the allowance to "+strconv.Itoa(restored)+" of the cap "+strconv.Itoa(smokeCap))
	} else {
		g.skip("allowance-restored", "the burst classified "+smokeClasses(burst)+
			" and never queued on "+strconv.Itoa(s.cores)+" cores: no throughput window to restore the allowance on")
	}

	result, detail := smokeSaturationVerdict(smokeSeen(burst, "saturated"), peakCPU, s.cores)
	if result == "skipped" {
		g.skip("saturation", detail)
	} else {
		g.check(t, "saturation", result == "pass", detail)
	}

	end := smokeRead(t, s.addr)
	// Layout immutability is checked against every sample, not just the ends:
	// an adaptation that reshaped the reported session or thread count and then
	// restored it would pass an endpoints-only comparison.
	layout := start.sessions == end.sessions && start.intra == end.intra
	drift := ""
	for _, sm := range all {
		if sm.sessions != start.sessions || sm.intra != start.intra {
			layout = false
			drift = "drift to sessions=" + strconv.Itoa(sm.sessions) + " intra_op_threads=" + strconv.Itoa(sm.intra) +
				" while the allowance was " + strconv.Itoa(sm.current)
			break
		}
	}
	layoutDetail := "sessions " + strconv.Itoa(start.sessions) + "->" + strconv.Itoa(end.sessions) +
		", intra_op_threads " + strconv.Itoa(start.intra) + "->" + strconv.Itoa(end.intra) + " across " + strconv.Itoa(len(all)) + " samples"
	if drift != "" {
		layoutDetail += "; " + drift
	}
	g.check(t, "layout-immutable", layout, layoutDetail)
}

// smokeSaturationArm pins the CPU fraction denominator to one core, the shape
// where a burst on a one-thread session can actually reach the saturation
// threshold: the assertion the shape arm reports as skipped is enforced here.
func smokeSaturationArm(t *testing.T, g *smokeGate) {
	withGOMAXPROCS(t, smokeCoresSaturating)
	s := smokeStart(t, nil, "")
	if s.cores != smokeCoresSaturating {
		g.skip("saturation-enforced", "a cgroup quota of "+strconv.Itoa(s.cores)+
			" cores overrides the pinned budget, so the burst's reach is not the gate's to control")
		return
	}
	smokeSerialPhase(t, s, 2*time.Second, 20*time.Millisecond)
	burst, peakCPU, _ := smokeBurstPhase(t, s, 8, 5*time.Second)

	result, detail := smokeSaturationVerdict(smokeSeen(burst, "saturated"), peakCPU, s.cores)
	if result == "skipped" {
		g.skip("saturation-enforced", detail)
		return
	}
	g.check(t, "saturation-enforced", result == "pass", detail+" at the pinned one-core budget")
}

// smokeUnreachableArm is the other half of the honesty rule: a burst that
// cannot reach the threshold must be reported as skipped with the host property
// that prevented it, and never as a pass.
func smokeUnreachableArm(t *testing.T, g *smokeGate) {
	withGOMAXPROCS(t, smokeCoresUnreachable)
	s := smokeStart(t, nil, "")
	if s.cores != smokeCoresUnreachable {
		// A reported skip rather than a test-level one, for symmetry with the
		// saturating arm: the assertion still belongs to this arm's counts, with
		// the host property that put it out of reach.
		g.skip("unreachable-reported-skipped", "a cgroup quota of "+strconv.Itoa(s.cores)+
			" cores overrides the pinned budget, so an unreachable saturating phase is not the gate's to control")
		return
	}
	smokeSerialPhase(t, s, 2*time.Second, 20*time.Millisecond)
	burst, peakCPU, _ := smokeBurstPhase(t, s, 8, 4*time.Second)

	result, detail := smokeSaturationVerdict(smokeSeen(burst, "saturated"), peakCPU, s.cores)
	g.check(t, "unreachable-reported-skipped", result == "skipped",
		"an unreachable saturating phase reported "+result+": "+detail)
}

// smokeKillSwitchArm asserts the disable switch holds concurrency at the
// configured value under a burst that would otherwise expand it.
func smokeKillSwitchArm(t *testing.T, g *smokeGate) {
	withGOMAXPROCS(t, smokeCoresShape)
	s := smokeStart(t, func(c *config.ModelConfig) { c.Autotune = "off" }, "")
	start := smokeRead(t, s.addr)
	burst, _, _ := smokeBurstPhase(t, s, 8, 3*time.Second)

	held := start.current == smokeCap && start.target == smokeCap && !start.active
	for _, sm := range burst {
		held = held && sm.current == smokeCap && sm.target == smokeCap && !sm.active
	}
	g.check(t, "kill-switch", held,
		"autotune=off held concurrency at "+strconv.Itoa(start.current)+"/"+strconv.Itoa(start.target)+
			" with active="+strconv.FormatBool(start.active)+" across a burst (the class stays idle: the switch runs no sampler)")
}

// smokeProfileArm asserts each capacity profile pins its layout and reports its
// autotune state.
func smokeProfileArm(t *testing.T, g *smokeGate) {
	withGOMAXPROCS(t, smokeCoresShape)

	low := smokeStart(t, func(c *config.ModelConfig) { c.Capacity = "latency" }, "")
	lowInfo := smokeRead(t, low.addr)
	g.check(t, "profile-latency", lowInfo.current == 1 && !lowInfo.active,
		"capacity=latency pinned concurrency "+strconv.Itoa(lowInfo.current)+" with active="+strconv.FormatBool(lowInfo.active)+
			" (reported spinning="+strconv.FormatBool(lowInfo.spinning)+")")

	high := smokeStart(t, func(c *config.ModelConfig) { c.Capacity = "throughput" }, "")
	highInfo := smokeRead(t, high.addr)
	g.check(t, "profile-throughput", highInfo.current == smokeCap && highInfo.active,
		"capacity=throughput pinned concurrency "+strconv.Itoa(highInfo.current)+" with active="+strconv.FormatBool(highInfo.active)+
			" (reported spinning="+strconv.FormatBool(highInfo.spinning)+")")
}

// smokeCacheArm asserts a window served entirely from the reply cache is idle
// and does not advance the run counters: hits are not traffic.
func smokeCacheArm(t *testing.T, g *smokeGate) {
	withGOMAXPROCS(t, smokeCoresShape)
	s := smokeStart(t, nil, "1GB")

	// Warm the cache: the second identical request is served from it.
	first := smokeReply(t, s)
	second := smokeReply(t, s)
	if first != second {
		t.Fatalf("a cached reply differs from the one it was cached from: %q vs %q", first, second)
	}
	// A window that contains the warmup inference is not a cache-served window.
	// The sampler starts at `idle`, so idle alone is also the pre-first-window
	// value: wait for the warmup's latency window to be published first (which
	// also proves the sampler is running), then for the idle window after it.
	if !smokeWaitClass(t, s.addr, "latency", 5*time.Second) {
		t.Fatal("the sampler never classified the warmup window as latency")
	}
	if !smokeWaitClass(t, s.addr, "idle", 5*time.Second) {
		t.Fatal("the sampler never published an idle window after the warmup request")
	}
	warm := smokeRead(t, s.addr)

	samples, requests := smokeCachePhase(t, s, 3*time.Second)
	allIdle := len(samples) > 0
	for _, sm := range samples {
		allIdle = allIdle && sm.class == "idle"
	}
	last := samples[len(samples)-1]
	hits := last.cacheHits - warm.cacheHits
	g.check(t, "cache-idle", allIdle && last.runs == warm.runs && hits > 0,
		"classes "+smokeClasses(samples)+" with runs "+strconv.Itoa(warm.runs)+"->"+strconv.Itoa(last.runs)+
			" across "+strconv.Itoa(int(requests))+" requests and "+strconv.Itoa(hits)+" reply-cache hits")
}

// smokeIdentityArm asserts the reply is byte-identical under every profile and
// switch: adaptation must not change what a request returns.
func smokeIdentityArm(t *testing.T, g *smokeGate) {
	withGOMAXPROCS(t, smokeCoresShape)
	arms := []struct {
		name   string
		mutate func(*config.ModelConfig)
		cache  string
	}{
		{"auto", nil, ""},
		{"autotune-off", func(c *config.ModelConfig) { c.Autotune = "off" }, ""},
		{"capacity-latency", func(c *config.ModelConfig) { c.Capacity = "latency" }, ""},
		{"capacity-throughput", func(c *config.ModelConfig) { c.Capacity = "throughput" }, ""},
		{"cache-hit", nil, "1GB"},
	}
	var want string
	identical := true
	details := make([]string, 0, len(arms))
	for i, arm := range arms {
		s := smokeStart(t, arm.mutate, arm.cache)
		reply := smokeReply(t, s)
		if arm.cache != "" {
			// The identical second request must be a reply-cache hit.
			reply = smokeReply(t, s)
		}
		if i == 0 {
			want = reply
		} else if reply != want {
			identical = false
		}
		details = append(details, arm.name+"="+reply)
	}
	g.check(t, "reply-identity", identical, strings.Join(details, " "))
}

// smokeThroughputArm gates the adaptive arm against a fixed-concurrency control
// arm measured in the same run, at the same layout and cap, medians over
// repeated rounds with the warmup excluded.
func smokeThroughputArm(t *testing.T, g *smokeGate, appCPUs int) {
	withGOMAXPROCS(t, appCPUs)
	adaptive := smokeStart(t, nil, "")
	control := smokeStart(t, func(c *config.ModelConfig) { c.Autotune = "off" }, "")
	g.check(t, "control-layout", adaptive.cores == appCPUs && control.cores == appCPUs,
		"both arms ran at cores="+strconv.Itoa(adaptive.cores)+" (partition app="+strconv.Itoa(appCPUs)+")")

	const (
		clients = 8
		rounds  = 3
	)
	round := 2 * time.Second
	// Warmup, excluded from the medians: it also brings the adaptive arm to the
	// cap (its first throughput window expands the allowance).
	smokeTimedBurst(adaptive, clients, time.Second)
	smokeTimedBurst(control, clients, time.Second)

	adaptiveRates := make([]float64, 0, rounds)
	controlRates := make([]float64, 0, rounds)
	for range rounds {
		adaptiveRates = append(adaptiveRates, smokeTimedBurst(adaptive, clients, round))
		controlRates = append(controlRates, smokeTimedBurst(control, clients, round))
	}
	adaptiveMedian := median(adaptiveRates)
	controlMedian := median(controlRates)
	floor := controlMedian * (100 - smokeThroughputFloorPct) / 100
	g.check(t, "control-arm", adaptiveMedian >= floor,
		"adaptive "+trim1(adaptiveMedian)+" req/s against the fixed-concurrency control "+trim1(controlMedian)+
			" req/s (floor "+trim1(floor)+" = "+trim0(smokeThroughputFloorPct)+"% below the control); adaptive rounds "+
			trim1s(adaptiveRates)+", control rounds "+trim1s(controlRates))
}
