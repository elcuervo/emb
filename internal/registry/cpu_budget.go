package registry

import (
	"context"
	"fmt"
	"log"
	"math"
	"sort"
	"strconv"
	"sync/atomic"
	"time"

	"github.com/elcuervo/emb/internal/config"
	"github.com/elcuervo/emb/internal/onnx"
	"github.com/elcuervo/emb/internal/permit"
)

type CPUPlan struct {
	Allowance CPUAllowance
	Capacity  int
	Budget    int
	Reserve   int
	Enabled   bool
	Explicit  bool
	Warning   string
	Active    int
	Peak      int
	// Admission pressure at the gate: cumulative wait and timeouts, current waiters.
	AdmissionWaitUs   int64
	AdmissionWaiters  int
	AdmissionTimeouts int64
}

func ResolveCPUPlan(cfg *config.Config, allowance CPUAllowance) (CPUPlan, error) {
	plan := CPUPlan{Allowance: allowance, Enabled: cfg.CPUBudget != "off"}
	if !plan.Enabled {
		return plan, nil
	}
	capacity := allowance.Capacity()
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

func modelThreadShares(models map[string]config.ModelConfig, budget int) map[string]int {
	names := make([]string, 0, len(models))
	maxWeight := 0.0
	for name, model := range models {
		names = append(names, name)
		maxWeight = math.Max(maxWeight, model.AdmissionWeight())
	}
	if len(names) == 0 {
		return map[string]int{}
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
	}
	return shares
}

func logCPUPlan(plan CPUPlan) {
	if !plan.Enabled {
		log.Printf("CPU inference budget disabled; using legacy session sizing")
		return
	}
	log.Printf("CPU inference budget: allowance=%.2f source=%s scheduler=%d affinity=%d capacity=%d reserve=%d budget=%d", plan.Allowance.CPUs, plan.Allowance.Source, plan.Allowance.Scheduler, plan.Allowance.Affinity, plan.Capacity, plan.Reserve, plan.Budget)
	if plan.Allowance.Provenance != "" {
		log.Printf("CPU allowance fallback: %s", plan.Allowance.Provenance)
	}
	if plan.Warning != "" {
		log.Printf("WARNING: %s", plan.Warning)
	}
}

// budgetedSession admits every Run through the process gate. The gate wait is
// kept out of the call's run time: it accrues in pending, which the pipeline
// takes after each batch (see pipeline.admissionTimed), and in total, the
// path's cumulative admission wait.
type budgetedSession struct {
	onnx.Session
	gate    *permit.Budget
	model   string
	width   int
	pending atomic.Int64 // microseconds since the last TakeAdmissionWait
	total   *atomic.Int64
}

func newBudgetedSession(sess onnx.Session, gate *permit.Budget, model string, width int, total *atomic.Int64) *budgetedSession {
	return &budgetedSession{Session: sess, gate: gate, model: model, width: width, total: total}
}

func (s *budgetedSession) Run(ids, mask []int64, batch, length, dim int) ([]float32, error) {
	start := time.Now()
	release, err := s.gate.Acquire(context.Background(), s.model, s.width)
	waited := time.Since(start).Microseconds()
	s.pending.Add(waited)
	if s.total != nil {
		s.total.Add(waited)
	}
	if err != nil {
		return nil, err
	}
	defer release()
	return s.Session.Run(ids, mask, batch, length, dim)
}

// TakeAdmissionWait returns and clears the gate wait accrued by Run.
func (s *budgetedSession) TakeAdmissionWait() time.Duration {
	return time.Duration(s.pending.Swap(0)) * time.Microsecond
}

// budgetedNamedSession charges a named-tensor session through its shared
// accounting: the session's width while any caller runs, plus one token per
// further concurrent caller.
type budgetedNamedSession struct {
	onnx.NamedSession
	shared *permit.Shared
	total  *atomic.Int64 // the path's cumulative admission wait
}

func (s budgetedNamedSession) RunNamed(inputs []onnx.NamedTensor) (map[string]onnx.NamedTensor, error) {
	out, _, err := s.runAdmitted(inputs)
	return out, err
}

// runAdmitted is RunNamed that also reports how long the call queued at the
// gate, so the caller can keep that out of its run time.
func (s budgetedNamedSession) runAdmitted(inputs []onnx.NamedTensor) (map[string]onnx.NamedTensor, time.Duration, error) {
	start := time.Now()
	exit, err := s.shared.Enter(context.Background())
	waited := time.Since(start)
	if s.total != nil {
		s.total.Add(waited.Microseconds())
	}
	if err != nil {
		return nil, waited, err
	}
	defer exit()
	out, err := s.NamedSession.RunNamed(inputs)
	return out, waited, err
}

// admittedNamedSession is a named session that reports its gate wait per call.
type admittedNamedSession interface {
	runAdmitted(inputs []onnx.NamedTensor) (map[string]onnx.NamedTensor, time.Duration, error)
}
