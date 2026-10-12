package registry

import (
	"fmt"
	"log"
	"math"
	"sort"
	"strconv"

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
	if plan.Warning != "" {
		log.Printf("WARNING: %s", plan.Warning)
	}
}

type budgetedSession struct {
	onnx.Session
	gate  *permit.Budget
	width int
}

func (s budgetedSession) Run(ids, mask []int64, batch, length, dim int) ([]float32, error) {
	release := s.gate.Acquire(s.width)
	defer release()
	return s.Session.Run(ids, mask, batch, length, dim)
}

type budgetedNamedSession struct {
	onnx.NamedSession
	gate  *permit.Budget
	width int
}

func (s budgetedNamedSession) RunNamed(inputs []onnx.NamedTensor) (map[string]onnx.NamedTensor, error) {
	release := s.gate.Acquire(s.width)
	defer release()
	return s.NamedSession.RunNamed(inputs)
}
