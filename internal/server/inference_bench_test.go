package server

import (
	"bufio"
	"encoding/json"
	"math/rand"
	"net"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/elcuervo/emb/internal/config"
	"github.com/elcuervo/emb/internal/registry"
)

// Seeded local inference benchmark. It offers embedding calls on a fixed
// open-loop schedule (independent of response completion, so a slower layout
// cannot look faster by doing less work) and records offered/completed/dropped
// pairs, per-leg and pair latency, CPU seconds per pair and per ORT row, RSS,
// cache hits/misses, errors, and the selected layout. Run it with:
//
//	just bench-inference
//
// It reuses the pinned local model fixtures and skips when they are absent. The
// result directory is a self-contained JSON report; machine metadata is
// recorded so cross-machine comparisons stay honest.

const (
	inferenceBenchSeed        = 20261012
	inferenceBenchPairRate    = 10.0 // pairs/s, the reported production load shape
	inferenceBenchDuration    = 2 * time.Second
	inferenceBenchMaxInFlight = 8
)

var inferenceBenchWords = strings.Fields(`the quick brown fox jumps over a lazy dog while embedders summarize photographs of mountains rivers cities and people in long descriptive captions that vary in length`)

// inferenceBenchUnique keeps cold-pass text unique across every pass in the
// process, so a previously warmed cache cannot answer a cold request.
var inferenceBenchUnique atomic.Int64

// inferenceCacheCounters is the reply cache's cumulative hit/miss pair.
type inferenceCacheCounters struct{ hits, misses int64 }

func inferenceCacheSnapshot(srv *Server) inferenceCacheCounters {
	if srv.cache == nil {
		return inferenceCacheCounters{}
	}
	st := srv.cache.Stats()
	return inferenceCacheCounters{hits: st.Hits, misses: st.Misses}
}

type inferenceLegLatency struct {
	P50 float64 `json:"p50_ms"`
	P95 float64 `json:"p95_ms"`
	P99 float64 `json:"p99_ms"`
}

type inferenceModelLayout struct {
	Model       string `json:"model"`
	Sessions    int    `json:"sessions"`
	IntraOp     int    `json:"intra_op_threads"`
	ThreadShare int    `json:"cpu_thread_share"`
	Spinning    bool   `json:"allow_spinning"`
}

type inferenceBenchResult struct {
	Name            string                         `json:"name"`
	Dispatch        string                         `json:"dispatch"`
	Cache           string                         `json:"cache"`
	Rate            float64                        `json:"offered_pairs_per_second"`
	Seconds         float64                        `json:"seconds"`
	Offered         int64                          `json:"offered"`
	Completed       int64                          `json:"completed"`
	Dropped         int64                          `json:"dropped"`
	Errors          int64                          `json:"errors"`
	Rows            int64                          `json:"ort_rows"`
	Legs            map[string]inferenceLegLatency `json:"leg_latency"`
	Pair            inferenceLegLatency            `json:"pair_latency"`
	CPUSecondsPair  float64                        `json:"cpu_seconds_per_pair"`
	CPUSecondsRow   float64                        `json:"cpu_seconds_per_ort_row"`
	RSSMB           int                            `json:"rss_mb"`
	CacheHits       int64                          `json:"cache_hits"`
	CacheMisses     int64                          `json:"cache_misses"`
	SelectedLayout  []inferenceModelLayout         `json:"selected_layout"`
	ProcessBudget   int                            `json:"process_budget"`
	ProcessCapacity int                            `json:"process_capacity"`
	CPUAllowance    float64                        `json:"cpu_allowance"`
}

type inferenceBenchReport struct {
	Generated  time.Time              `json:"generated"`
	Seed       int64                  `json:"seed"`
	GoVersion  string                 `json:"go_version"`
	GOOS       string                 `json:"goos"`
	GOARCH     string                 `json:"goarch"`
	NumCPU     int                    `json:"num_cpu"`
	GOMAXPROCS int                    `json:"gomaxprocs"`
	Rate       float64                `json:"pair_rate"`
	Results    []inferenceBenchResult `json:"results"`
}

// BenchmarkInferencePairs runs the seeded single-model, paired-serial, and
// paired-parallel workloads under warm and cold cache, one open-loop pass each.
func BenchmarkInferencePairs(b *testing.B) {
	if _, err := os.Stat("../../models/bge/model.onnx"); err != nil {
		b.Skipf("second benchmark model not present: %v (run: just download-model)", err)
	}
	rate := inferenceBenchPairRate
	if v := os.Getenv("EMB_BENCH_PAIR_RATE"); v != "" {
		if f, err := strconv.ParseFloat(v, 64); err == nil && f > 0 {
			rate = f
		}
	}
	duration := inferenceBenchDuration
	if v := os.Getenv("EMB_BENCH_DURATION"); v != "" {
		if f, err := strconv.ParseFloat(v, 64); err == nil && f > 0 {
			duration = time.Duration(f * float64(time.Second))
		}
	}

	addr, srv := serveInferencePairBench(b)
	report := inferenceBenchReport{
		Generated:  time.Now().UTC(),
		Seed:       inferenceBenchSeed,
		GoVersion:  runtime.Version(),
		GOOS:       runtime.GOOS,
		GOARCH:     runtime.GOARCH,
		NumCPU:     runtime.NumCPU(),
		GOMAXPROCS: runtime.GOMAXPROCS(0),
		Rate:       rate,
	}

	cases := []struct {
		name     string
		dispatch string
		legs     []string
	}{
		{"single-minilm", "single", []string{"minilm"}},
		{"paired-serial", "serial", []string{"minilm", "bge"}},
		{"paired-parallel", "parallel", []string{"minilm", "bge"}},
	}
	for _, tc := range cases {
		for _, cache := range []string{"warm", "cold"} {
			b.Run(tc.name+"/"+cache, func(b *testing.B) {
				for b.Loop() {
					res := runInferenceBenchPass(b, addr, srv, tc.dispatch, tc.legs, cache, rate, duration)
					report.Results = append(report.Results, res)
					b.ReportMetric(float64(res.Completed)/res.Seconds, "completed_pairs/s")
					b.ReportMetric(res.CPUSecondsPair, "cpu_s/pair")
					b.ReportMetric(float64(res.RSSMB), "rss_mb")
					b.ReportMetric(float64(res.Dropped), "dropped")
				}
			})
		}
	}
	writeInferenceBenchReport(b, report)
}

// serveInferencePairBench starts one real server holding minilm and bge with a
// generous reply cache (cold passes use unique text instead of disabling it).
func serveInferencePairBench(b *testing.B) (string, *Server) {
	b.Helper()
	if !ortOK {
		b.Skip("onnx runtime unavailable (run inside nix develop)")
	}
	reg := registry.New()
	for _, name := range []string{"minilm", "bge"} {
		entry, err := registry.LoadModel(config.ModelConfig{
			ONNX:      "../../models/" + name + "/model.onnx",
			Tokenizer: "../../models/" + name + "/tokenizer.json",
			Dim:       384,
			MaxLength: 128,
			Pooling:   "mean",
			Normalize: false,
		}, name)
		if err != nil {
			b.Skipf("benchmark model %s not present: %v (run: just download-model)", name, err)
		}
		reg.Add(name, entry)
	}
	addr := getFreeAddr()
	srv := New(addr, reg, "", "1GB", nil)
	go srv.ListenAndServe()
	b.Cleanup(func() {
		_ = srv.Close()
		_ = reg.Close()
	})
	time.Sleep(100 * time.Millisecond)
	return addr, srv
}

type inferenceLegConn struct {
	conn *net.TCPConn
	r    *bufio.Reader
}

type inferencePairConns struct{ legs []inferenceLegConn }

// runInferenceBenchPass offers pairs at the fixed rate for the duration and
// waits for every launched pair, so offered = completed + dropped + errors.
func runInferenceBenchPass(b *testing.B, addr string, srv *Server, dispatch string, legs []string, cache string, rate float64, duration time.Duration) inferenceBenchResult {
	b.Helper()
	pool := make(chan *inferencePairConns, inferenceBenchMaxInFlight)
	for i := 0; i < inferenceBenchMaxInFlight; i++ {
		pc := &inferencePairConns{}
		for range legs {
			conn, r := benchConn(b, addr)
			pc.legs = append(pc.legs, inferenceLegConn{conn: conn, r: r})
		}
		pool <- pc
	}

	rng := rand.New(rand.NewSource(inferenceBenchSeed))
	warmCorpus := make([]string, 8)
	for i := range warmCorpus {
		warmCorpus[i] = benchInferenceText(rng, i%3)
	}

	var offered, completed, dropped, errored, rows atomic.Int64
	var mu sync.Mutex
	legLatency := map[string][]time.Duration{}
	var pairLatency []time.Duration
	cpuBeforeUser, cpuBeforeSys := registry.ProcessCPUTimes()
	cacheBefore := inferenceCacheSnapshot(srv)
	started := time.Now()
	deadline := started.Add(duration)
	interval := time.Duration(float64(time.Second) / rate)
	next := started
	var wg sync.WaitGroup
	for i := 0; time.Now().Before(deadline); i++ {
		if wait := time.Until(next); wait > 0 {
			time.Sleep(wait)
		}
		next = next.Add(interval)
		offered.Add(1)
		select {
		case pc := <-pool:
			texts := make([]string, len(legs))
			for li := range legs {
				if cache == "cold" {
					// A fresh corpus shared across passes, so cold passes always
					// miss even after an earlier warm pass filled the cache.
					texts[li] = benchInferenceText(rng, li%3) + " " + strconv.FormatInt(inferenceBenchUnique.Add(1), 10)
				} else {
					texts[li] = warmCorpus[i%len(warmCorpus)]
				}
			}
			wg.Add(1)
			go func(pc *inferencePairConns, texts []string) {
				defer func() {
					pool <- pc
					wg.Done()
				}()
				start := time.Now()
				// The parallel branch calls step from two goroutines, so the failure
				// count is atomic: both legs failing in one pair is a normal race.
				var errs atomic.Int64
				step := func(li int, model string) {
					d, err := benchInferenceCall(pc.legs[li], model, texts[li])
					mu.Lock()
					legLatency[model] = append(legLatency[model], d)
					mu.Unlock()
					if err != nil {
						errs.Add(1)
					}
				}
				if dispatch == "parallel" {
					var legWG sync.WaitGroup
					for li, model := range legs {
						legWG.Add(1)
						go func(li int, model string) {
							defer legWG.Done()
							step(li, model)
						}(li, model)
					}
					legWG.Wait()
				} else {
					for li, model := range legs {
						step(li, model)
					}
				}
				mu.Lock()
				pairLatency = append(pairLatency, time.Since(start))
				mu.Unlock()
				rows.Add(int64(len(legs)))
				if errs.Load() > 0 {
					errored.Add(1)
					return
				}
				completed.Add(1)
			}(pc, texts)
		default:
			dropped.Add(1)
		}
	}
	wg.Wait()
	elapsed := time.Since(started).Seconds()
	cpuAfterUser, cpuAfterSys := registry.ProcessCPUTimes()
	cpuSeconds := float64((cpuAfterUser-cpuBeforeUser)+(cpuAfterSys-cpuBeforeSys)) / 1e6

	res := inferenceBenchResult{
		Name:      dispatch + "/" + cache,
		Dispatch:  dispatch,
		Cache:     cache,
		Rate:      rate,
		Seconds:   elapsed,
		Offered:   offered.Load(),
		Completed: completed.Load(),
		Dropped:   dropped.Load(),
		Errors:    errored.Load(),
		Rows:      rows.Load(),
		Legs:      map[string]inferenceLegLatency{},
		Pair:      latencySummary(pairLatency),
		RSSMB:     currentRSSMB(),
	}
	if res.Completed > 0 {
		res.CPUSecondsPair = cpuSeconds / float64(res.Completed)
	}
	if res.Rows > 0 {
		res.CPUSecondsRow = cpuSeconds / float64(res.Rows)
	}
	for model, lats := range legLatency {
		res.Legs[model] = latencySummary(lats)
	}
	if srv.cache != nil {
		after := inferenceCacheSnapshot(srv)
		res.CacheHits = after.hits - cacheBefore.hits
		res.CacheMisses = after.misses - cacheBefore.misses
	}
	res.SelectedLayout, res.ProcessBudget, res.ProcessCapacity, res.CPUAllowance = inferenceLayout(srv)
	return res
}

// benchInferenceCall runs one EMB command and returns its latency.
func benchInferenceCall(leg inferenceLegConn, model, text string) (time.Duration, error) {
	start := time.Now()
	if _, err := leg.conn.Write(respCommand("EMB", model, text)); err != nil {
		return time.Since(start), err
	}
	if err := drainValue(leg.r); err != nil {
		return time.Since(start), err
	}
	return time.Since(start), nil
}

func benchInferenceText(rng *rand.Rand, class int) string {
	words := []int{8, 40, 160}[class]
	var sb strings.Builder
	for i := 0; i < words; i++ {
		if i > 0 {
			sb.WriteByte(' ')
		}
		sb.WriteString(inferenceBenchWords[rng.Intn(len(inferenceBenchWords))])
	}
	return sb.String()
}

func latencySummary(latencies []time.Duration) inferenceLegLatency {
	if len(latencies) == 0 {
		return inferenceLegLatency{}
	}
	sort.Slice(latencies, func(i, j int) bool { return latencies[i] < latencies[j] })
	return inferenceLegLatency{
		P50: benchMs(latencies[len(latencies)*50/100]),
		P95: benchMs(latencies[min(len(latencies)-1, len(latencies)*95/100)]),
		P99: benchMs(latencies[min(len(latencies)-1, len(latencies)*99/100)]),
	}
}

func benchMs(d time.Duration) float64 { return float64(d.Microseconds()) / 1000 }

func currentRSSMB() int {
	bytes, _ := registry.CurrentMemoryUsage()
	return int(bytes >> 20)
}

// inferenceLayout records the selected layout after the pass forced every pool
// to load: per-model sessions/width and the process budget.
func inferenceLayout(srv *Server) ([]inferenceModelLayout, int, int, float64) {
	status := srv.reg.CPUStatus()
	entries := srv.reg.List()
	layout := make([]inferenceModelLayout, 0, len(entries))
	for _, entry := range entries {
		_, _, share, threads, spinning := entry.CPUSettings()
		sessions := 0
		if entry.Pool != nil {
			sessions = entry.Pool.Stats().NumWorkers
		}
		layout = append(layout, inferenceModelLayout{
			Model:       entry.Name,
			Sessions:    sessions,
			IntraOp:     threads,
			ThreadShare: share,
			Spinning:    spinning,
		})
	}
	return layout, status.Budget, status.Capacity, status.Allowance.CPUs
}

func writeInferenceBenchReport(b *testing.B, report inferenceBenchReport) {
	b.Helper()
	dir := os.Getenv("EMB_BENCH_OUT")
	if dir == "" {
		dir = filepath.Join("..", "..", "bench-out", "inference")
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		b.Fatalf("creating result directory: %v", err)
	}
	path := filepath.Join(dir, "inference.json")
	raw, err := json.MarshalIndent(report, "", "  ")
	if err != nil {
		b.Fatalf("encoding report: %v", err)
	}
	if err := os.WriteFile(path, raw, 0o644); err != nil {
		b.Fatalf("writing report: %v", err)
	}
	for _, res := range report.Results {
		b.Logf("%s offered=%d completed=%d dropped=%d errors=%d rows=%d pair_p95=%.2fms cpu/pair=%.4fs cpu/row=%.4fs rss=%dMB hits=%d misses=%d",
			res.Name, res.Offered, res.Completed, res.Dropped, res.Errors, res.Rows, res.Pair.P95, res.CPUSecondsPair, res.CPUSecondsRow, res.RSSMB, res.CacheHits, res.CacheMisses)
	}
	b.Logf("wrote %s", path)
}
