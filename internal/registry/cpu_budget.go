package registry

import (
	"context"
	"fmt"
	"log"
	"math"
	"sort"
	"strconv"
	"time"

	"github.com/elcuervo/emb/internal/config"
	"github.com/elcuervo/emb/internal/onnx"
	"github.com/elcuervo/emb/internal/permit"
)

type CPUPlan struct {
	Allowance  CPUAllowance
	Capacity   int
	Budget     int
	Reserve    int
	QueueBound int
	Enabled    bool
	Explicit   bool
	Warning    string
	Active     int
	Peak       int
	// Admission pressure: the wait callers spent queueing at the gate, the
	// callers queueing now, and the waits that ended without a grant. Kept
	// apart from Active/Peak, which count runs that were admitted.
	AdmissionWaitUs   int64
	AdmissionWaiters  int
	AdmissionTimeouts int
}

func ResolveCPUPlan(cfg *config.Config, allowance CPUAllowance) (CPUPlan, error) {
	plan := CPUPlan{Allowance: allowance, Enabled: cfg.CPUBudget != "off"}
	if !plan.Enabled {
		return plan, nil
	}
	capacity := max(1, int(math.Floor(allowance.CPUs)))
	if allowance.Scheduler > 0 && allowance.Scheduler < capacity {
		capacity = allowance.Scheduler
	}
	plan.Capacity = capacity
	if cfg.CPUBudget != "" && cfg.CPUBudget != "auto" {
		budget, err := strconv.Atoi(cfg.CPUBudget)
		if err != nil || budget < 1 {
			return CPUPlan{}, fmt.Errorf("invalid cpu_budget %q", cfg.CPUBudget)
		}
		if cfg.CPUReserve != nil {
			return CPUPlan{}, fmt.Errorf("cpu_budget and cpu_reserve cannot both be set")
		}
		plan.Budget, plan.Explicit = budget, true
		if budget > capacity {
			plan.Warning = fmt.Sprintf("cpu_budget %d exceeds detected CPU capacity %.2f", budget, allowance.CPUs)
		}
		return plan, nil
	}
	reserve := 0
	if capacity >= 8 {
		reserve = 2
	} else if capacity >= 4 {
		reserve = 1
	}
	if cfg.CPUReserve != nil {
		reserve = *cfg.CPUReserve
	}
	if reserve < 0 || reserve >= capacity {
		return CPUPlan{}, fmt.Errorf("cpu_reserve must be between 0 and %d for detected capacity %d", capacity-1, capacity)
	}
	plan.Reserve, plan.Budget = reserve, capacity-reserve
	return plan, nil
}

// modelThreadShares splits the process budget across models by weight, and
// reclaims the over-allocation when more models are configured than the budget
// has units. Every model is entitled to one unit while the budget has room; the
// floor of one each can exceed the budget (four models, a budget of three), so
// the excess is reclaimed from the smallest shares and smallest weights until the
// shares sum to the budget. A reclaimed model keeps no reservation and runs on
// capacity borrowed from idle peers, which is what the admission controller
// already lends; the second return value names those models for the boot log and
// EMB.INFO.
func modelThreadShares(models map[string]config.ModelConfig, budget int) (map[string]int, []string) {
	names := make([]string, 0, len(models))
	maxWeight := 0.0
	for name, model := range models {
		names = append(names, name)
		maxWeight = math.Max(maxWeight, model.AdmissionWeight())
	}
	if len(names) == 0 {
		return map[string]int{}, nil
	}
	totalWeight := 0.0
	for _, name := range names {
		totalWeight += models[name].AdmissionWeight() / maxWeight
	}
	sort.Strings(names)
	shares := make(map[string]int, len(names))
	remainders := make(map[string]float64, len(names))
	allocated := 0
	for _, name := range names {
		weight := models[name].AdmissionWeight()
		share := float64(budget) * (weight / maxWeight) / totalWeight
		shares[name] = max(1, int(math.Floor(share)))
		allocated += shares[name]
		remainders[name] = share - math.Floor(share)
	}
	if left := budget - allocated; left > 0 {
		sort.Slice(names, func(i, j int) bool {
			if remainders[names[i]] == remainders[names[j]] {
				return names[i] < names[j]
			}
			return remainders[names[i]] > remainders[names[j]]
		})
		for i := 0; i < left; i++ {
			shares[names[i%len(names)]]++
		}
		sort.Strings(names)
	}

	// Reclaim: drop the least entitled reservations until the shares fit the
	// budget. The victim is the model with the smallest share, then the smallest
	// weight, then the first name — deterministic, so the same config always
	// reclaims the same models.
	var reclaimed []string
	for allocated > budget {
		victim := ""
		for _, name := range names {
			if shares[name] == 0 {
				continue
			}
			if victim == "" || shares[name] < shares[victim] ||
				(shares[name] == shares[victim] && models[name].AdmissionWeight() < models[victim].AdmissionWeight()) {
				victim = name
			}
		}
		if victim == "" {
			break
		}
		allocated -= shares[victim]
		shares[victim] = 0
		reclaimed = append(reclaimed, victim)
	}
	return shares, reclaimed
}

// policyPoolLayout decides an unbatched model's worker count and intra-op width
// under the CPU policy. A pool serves several runs at once, so a run that
// consumed the model's whole share made admission exclusive: group 1 measured
// cpu_peak_tokens == capacity and a 2.17x aggregate throughput loss at a fixed
// layout, and an uncalibrated policy default of one session holding the share
// cost 2.7x in tail latency. The share is therefore split so a second run of the
// same width fits the gate, with at least two workers to use it. A batched pool
// has one run slot by construction and keeps the whole share. Explicit settings
// stay authoritative.
func policyPoolLayout(share, configuredWorkers, configuredThreads int, batched bool) (workers, threads int) {
	if batched {
		workers, threads = 1, max(1, share)
	} else {
		workers, threads = 2, max(1, share/2)
	}
	if configuredWorkers > 0 {
		workers = configuredWorkers
	}
	if configuredThreads > 0 {
		threads = configuredThreads
	}
	return workers, threads
}

// logCPUPlan logs the process plan once per registry.
func logCPUPlan(plan CPUPlan) {
	if !plan.Enabled {
		log.Printf("CPU inference budget disabled; using legacy session sizing")
		return
	}
	log.Printf("CPU inference budget: allowance=%.2f source=%s scheduler=%d affinity=%d capacity=%d reserve=%d budget=%d", plan.Allowance.CPUs, plan.Allowance.Source, plan.Allowance.Scheduler, plan.Allowance.Affinity, plan.Capacity, plan.Reserve, plan.Budget)
	if plan.Warning != "" {
		log.Printf("WARNING: %s", plan.Warning)
	}
}

type budgetedSession struct {
	onnx.Session
	gate  *permit.Budget
	width int
}

func (s *budgetedSession) Run(ids, mask []int64, batch, length, dim int) ([]float32, error) {
	return s.RunContext(context.Background(), ids, mask, batch, length, dim)
}

// RunContext admits the run only if the caller is still waiting, and abandons
// the wait when its context ends: admission is bounded from the request rather
// than from the run.
func (s *budgetedSession) RunContext(ctx context.Context, ids, mask []int64, batch, length, dim int) ([]float32, error) {
	out, _, err := s.RunContextAdmitted(ctx, ids, mask, batch, length, dim)
	return out, err
}

// RunContextAdmitted runs the session and hands the wait the run paid at the
// gate back to the caller with the run's result. One session is leased to
// several callers, so the wait cannot be stashed on the session: a concurrent
// run would overwrite it before the run that paid it reads it.
func (s *budgetedSession) RunContextAdmitted(ctx context.Context, ids, mask []int64, batch, length, dim int) ([]float32, time.Duration, error) {
	release, waited, err := s.gate.AcquireContextWait(ctx, s.width)
	if err != nil {
		return nil, waited, err
	}
	defer release()
	out, err := onnx.RunContext(ctx, s.Session, ids, mask, batch, length, dim)
	return out, waited, err
}

type budgetedNamedSession struct {
	onnx.NamedSession
	gate  *permit.Budget
	width int
}

func (s *budgetedNamedSession) RunNamed(inputs []onnx.NamedTensor) (map[string]onnx.NamedTensor, error) {
	out, _, err := s.RunNamedAdmitted(inputs)
	return out, err
}

// RunNamedAdmitted runs the session and hands the wait the run paid at the gate
// back to the caller with the run's result. The scripted and image host APIs
// carry no context of their own, so their wait is bounded by the queue bound
// alone; as with budgetedSession, the wait travels with the run rather than on
// a field a leased session's other callers could overwrite.
func (s *budgetedNamedSession) RunNamedAdmitted(inputs []onnx.NamedTensor) (map[string]onnx.NamedTensor, time.Duration, error) {
	release, waited, err := s.gate.AcquireContextWait(context.Background(), s.width)
	if err != nil {
		return nil, waited, err
	}
	defer release()
	out, err := s.NamedSession.RunNamed(inputs)
	return out, waited, err
}

// admittedNamedSession is implemented by the gate wrappers: the wait a named run
// paid at the gate travels back with the run's own result.
type admittedNamedSession interface {
	RunNamedAdmitted(inputs []onnx.NamedTensor) (map[string]onnx.NamedTensor, time.Duration, error)
}

// runNamedAdmitted runs inputs and reports the wait the run paid at the CPU
// gate, zero for sessions that do not pass through the gate.
func runNamedAdmitted(sess onnx.NamedSession, inputs []onnx.NamedTensor) (map[string]onnx.NamedTensor, time.Duration, error) {
	if a, ok := sess.(admittedNamedSession); ok {
		return a.RunNamedAdmitted(inputs)
	}
	out, err := sess.RunNamed(inputs)
	return out, 0, err
}
