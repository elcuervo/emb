package registry

import (
	"fmt"
	"time"

	"github.com/elcuervo/emb/internal/config"
	"github.com/elcuervo/emb/internal/onnx"
	"github.com/elcuervo/emb/internal/tokenizer"
)

// CalibrationOutcome records how a CPU-governed model's embedding layout was
// chosen. It is immutable once published. Source is "calibrated", "fallback"
// or "default" (calibration did not run); Reason says why a fallback or default
// was used; Limit is "memory" when the process memory headroom, not the CPU
// ceiling, cut the candidate set. Ceiling is the process CPU budget the
// candidates were bounded by, never the model's weighted share. Duration is the
// total time spent choosing, session construction and cleanup included.
type CalibrationOutcome struct {
	Workers  int
	Threads  int
	Source   string
	Reason   string
	Limit    string
	Duration time.Duration
	Ceiling  int
}

// Calibration returns the model's layout outcome, or false until the embedding
// pool was laid out under the CPU policy (never with cpu_budget off).
func (e *ModelEntry) Calibration() (CalibrationOutcome, bool) {
	if o := e.calibration.Load(); o != nil {
		return *o, true
	}
	return CalibrationOutcome{}, false
}

// autoWidth is the intra-op width of an automatic layout that was not
// calibrated: the whole process budget. The weighted share only schedules
// admission under contention; it must not cap what one busy model can borrow.
func (e *ModelEntry) autoWidth() int { return max(1, e.cpuBudget) }

// cpuLayout is a chosen layout plus the memory reservation that backs it. The
// reservation is held until the pool exists, then released: RSS accounts for
// the sessions from then on.
type cpuLayout struct {
	calibratedLayout
	releaseMemory func()
}

func (l cpuLayout) outcome() *CalibrationOutcome {
	return &CalibrationOutcome{
		Workers: l.Workers, Threads: l.Threads, Source: l.Source, Reason: l.Reason,
		Limit: l.Limit, Duration: l.Elapsed, Ceiling: l.Ceiling,
	}
}

// planCPULayout selects the embedding layout under the process CPU policy.
// Automatic candidates are bounded by the process budget intersected with
// explicit dimensions and the memory headroom, not by this model's share, so an
// idle peer never limits an active model. With calibration disabled (or every
// dimension explicit) the layout is deterministic: explicit values, else one
// session spanning the whole budget (budget/workers wide when workers is set).
// An explicit worker count that cannot fit in memory fails before allocating.
func (e *ModelEntry) planCPULayout(cfg *config.ModelConfig, tok tokenizer.Tokenizer, factoryFor func(int) (onnx.Session, error), batched bool) (cpuLayout, error) {
	ceiling := e.autoWidth()
	resident := 1
	if !batched {
		resident = ceiling
		if cfg.Workers > 0 {
			resident = cfg.Workers
		}
	}
	fit, release := resident, func() {}
	per := sessionEstimate(cfg.ONNX)
	if e.mem != nil {
		fit, release = e.mem.reserve(e.Name, per, resident)
	}
	if !batched && cfg.Workers > 0 && fit < cfg.Workers {
		release()
		return cpuLayout{}, fmt.Errorf("model %q: workers=%d needs about %d MiB per session but the process memory headroom fits %d", e.Name, cfg.Workers, per>>20, fit)
	}
	memCap := max(fit, 1)

	var layout calibratedLayout
	if e.calibrate && (cfg.IntraOpThreads <= 0 || cfg.Workers <= 0) {
		// Trial the batch shape the pool actually serves: a batcher coalesces
		// concurrent requests, so single-row scoring under-provisions width.
		batch := 1
		if batched {
			batch = calibrationBatchRows
			if cfg.Batching.MaxBatch > 0 && cfg.Batching.MaxBatch < batch {
				batch = cfg.Batching.MaxBatch
			}
		}
		layout = calibrateLayout(func(threads int) (onnx.Session, error) {
			sess, err := factoryFor(threads)
			if err != nil {
				return nil, err
			}
			return newBudgetedSession(sess, e.cpuGate, e.Name, threads, nil), nil
		}, tok, cfg.Dim, cfg.MaxLength, ceiling, memCap, cfg.IntraOpThreads, cfg.Workers, batched, batch)
		e.tunedThreads.Store(int64(layout.Threads))
		e.tunedWorkers.Store(int64(layout.Workers))
		if !batched && cfg.Workers <= 0 && memCap < ceiling {
			layout.Limit = "memory"
		}
	} else {
		workers := max(cfg.Workers, 1)
		threads := cfg.IntraOpThreads
		if threads <= 0 {
			threads = max(1, ceiling/workers)
		}
		layout = calibratedLayout{Workers: workers, Threads: threads, Source: "default", Reason: "calibration_disabled"}
		if e.calibrate {
			layout.Reason = "explicit_layout"
		}
	}
	layout.Ceiling = ceiling
	return cpuLayout{calibratedLayout: layout, releaseMemory: release}, nil
}
