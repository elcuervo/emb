package server

import (
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"testing"
	"time"

	"github.com/elcuervo/emb/bench/fargate/driver"
	"github.com/elcuervo/emb/internal/config"
	"github.com/elcuervo/emb/internal/registry"
)

// Seeded local inference benchmark: a thin wrapper that starts one in-process
// server holding minilm and bge and drives it with the harness's open-loop pair
// driver (bench/fargate/driver), which owns scheduling, reply validation, the
// offered-work accounting and the server counters. Run it with:
//
//	just bench-inference
//
// It reuses the pinned local model fixtures and skips when they are absent.
// The server is built with registry.New(), so this measures scheduling and
// accounting locally; it is not a governor test. The production-shaped gate
// drives a real bin/emb through the same driver (see docs/performance-validation.md).
// CPU and RSS come from the server's own EMB.STATS counters, but the server is
// in this process, so they include the driver.

const (
	inferenceBenchSeed        = 20261012
	inferenceBenchPairRate    = 10.0 // pairs/s, the reported production load shape
	inferenceBenchDuration    = 2 * time.Second
	inferenceBenchMaxInFlight = 8
	inferenceBenchDim         = 384
	inferenceBenchTimeout     = 30 * time.Second
)

type inferenceModelLayout struct {
	Model       string `json:"model"`
	Sessions    int    `json:"sessions"`
	IntraOp     int    `json:"intra_op_threads"`
	ThreadShare int    `json:"cpu_thread_share"`
	Spinning    bool   `json:"allow_spinning"`
}

type inferenceBenchResult struct {
	Name            string                 `json:"name"`
	Dispatch        string                 `json:"dispatch"`
	Cache           string                 `json:"cache"`
	Rate            float64                `json:"offered_pairs_per_second"`
	Result          driver.Result          `json:"result"`
	Counters        driver.Counters        `json:"counters"`
	CPUSecondsPair  float64                `json:"cpu_seconds_per_pair"`
	RSSMB           int                    `json:"rss_mb"`
	SelectedLayout  []inferenceModelLayout `json:"selected_layout"`
	ProcessBudget   int                    `json:"process_budget"`
	ProcessCapacity int                    `json:"process_capacity"`
	CPUAllowance    float64                `json:"cpu_allowance"`
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
// A pass fails the benchmark when it completes no valid pair, sees an invalid
// reply, or its cache counters contradict its warm/cold claim.
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
	corpus := driver.NewCorpus(inferenceBenchSeed, 9, strconv.FormatInt(time.Now().UnixNano(), 36))
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
		dispatch driver.Dispatch
		models   []string
	}{
		{"single-minilm", driver.Single, []string{"minilm"}},
		{"paired-serial", driver.Serial, []string{"minilm", "bge"}},
		{"paired-parallel", driver.Parallel, []string{"minilm", "bge"}},
	}
	for _, tc := range cases {
		for _, cache := range []string{driver.CacheWarm, driver.CacheCold} {
			b.Run(tc.name+"/"+cache, func(b *testing.B) {
				for b.Loop() {
					pr, err := driver.RunPhase(driver.PhaseConfig{
						Config: driver.Config{
							Addr: addr, Models: tc.models, Dispatch: tc.dispatch, Rate: rate, Duration: duration,
							MaxInFlight: inferenceBenchMaxInFlight, RequestTimeout: inferenceBenchTimeout,
							DrainTimeout: inferenceBenchTimeout, Dim: inferenceBenchDim,
						},
						Cache: cache, Corpus: corpus, Info: true,
					})
					if err != nil {
						b.Fatalf("%s/%s: %v", tc.name, cache, err)
					}
					if pr.Result.Success == 0 || pr.Result.Errors > 0 {
						b.Fatalf("%s/%s: %d successful pairs, %d errors %v", tc.name, cache, pr.Result.Success, pr.Result.Errors, pr.Result.ErrorKinds)
					}
					if err := pr.CheckCache(); err != nil {
						b.Fatalf("%s/%s: %v", tc.name, cache, err)
					}
					res := inferenceBenchResultFrom(tc.name, string(tc.dispatch), rate, pr, srv)
					report.Results = append(report.Results, res)
					b.ReportMetric(pr.Result.Throughput, "completed_pairs/s")
					b.ReportMetric(res.CPUSecondsPair, "cpu_s/pair")
					b.ReportMetric(float64(res.RSSMB), "rss_mb")
					b.ReportMetric(float64(pr.Result.Dropped), "dropped")
				}
			})
		}
	}
	writeInferenceBenchReport(b, report)
}

func inferenceBenchResultFrom(name, dispatch string, rate float64, pr driver.PhaseResult, srv *Server) inferenceBenchResult {
	res := inferenceBenchResult{
		Name: name + "/" + pr.Cache, Dispatch: dispatch, Cache: pr.Cache, Rate: rate,
		Result: pr.Result, Counters: pr.Counters,
	}
	if pr.Result.Success > 0 {
		cpuUs := pr.Counters.Stats["cpu_user_usec"] + pr.Counters.Stats["cpu_sys_usec"]
		res.CPUSecondsPair = float64(cpuUs) / 1e6 / float64(pr.Result.Success)
	}
	if mem, err := strconv.Atoi(pr.After.Stats["mem"]); err == nil {
		res.RSSMB = mem
	}
	res.SelectedLayout, res.ProcessBudget, res.ProcessCapacity, res.CPUAllowance = inferenceLayout(srv)
	return res
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
			Dim:       inferenceBenchDim,
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
		r := res.Result
		b.Logf("%s offered=%d success=%d errors=%d timeouts=%d dropped=%d pairs/s=%.2f pair_p95=%.2fms(sched) %.2fms(wire) cpu/pair=%.4fs rss=%dMB hits=%d misses=%d",
			res.Name, r.Offered, r.Success, r.Errors, r.Timeouts, r.Dropped, r.Throughput, r.Scheduled.P95, r.Wire.P95,
			res.CPUSecondsPair, res.RSSMB, res.Counters.Stats["cache_hits"], res.Counters.Stats["cache_misses"])
	}
	b.Logf("wrote %s", path)
}
