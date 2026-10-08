package registry

import (
	"log"
	"time"

	"github.com/elcuervo/emb/internal/permit"
)

// Autotune sampling constants. They are deliberately not configurable: the
// values land inside the measured best band and keeping them fixed avoids a
// tuning matrix.
const (
	autotuneWindow    = time.Second
	autotuneCPUHigh   = 0.90
	autotuneWaitRatio = 0.25
	// Expand to the cap after one throughput window; contract only after
	// sustained (10 s) latency so a short serial lull never starves a burst.
	autotuneGrowAfter   = 1
	autotuneShrinkAfter = 10
)

// startSampler starts the traffic sampler for a scripted model. It is a no-op
// once started.
func (r *ScriptResources) startSampler(cores int) {
	r.samplerOnce.Do(func() {
		r.samplerWG.Add(1)
		go r.runSampler(cores)
	})
}

// stopSampler stops the sampler (if any) and waits for it to exit, so callers
// can close the sessions without racing an in-flight classification.
func (r *ScriptResources) stopSampler() {
	if r.stop == nil {
		return
	}
	r.stopOnce.Do(func() { close(r.stop) })
	r.samplerWG.Wait()
}

// runSampler periodically classifies the script path's traffic and adapts the
// per-session concurrency allowance within the configured cap.
func (r *ScriptResources) runSampler(cores int) {
	defer r.samplerWG.Done()

	ctrl := permit.NewController(r.cap, int(r.allowance.Load()), autotuneGrowAfter, autotuneShrinkAfter)
	prev := r.sample()
	t := time.NewTicker(autotuneWindow)
	defer t.Stop()
	for {
		select {
		case <-r.stop:
			return
		case <-t.C:
			cur := r.sample()
			class := permit.Classify(prev, cur, autotuneWindow, cores, autotuneCPUHigh, autotuneWaitRatio)
			prev = cur
			r.class.Store(class)

			allowance, changed, recommend := ctrl.Observe(class)
			if changed {
				r.allowance.Store(int64(allowance))
				r.pool.SetCapacity(allowance * len(r.sessions))
			}
			if recommend {
				log.Printf("  %s: script inference is CPU-saturated; consider raising script_workers or intra_op_threads", r.name)
			}
		}
	}
}

// sample reads the script path's counters and the process CPU times.
func (r *ScriptResources) sample() permit.Sample {
	wait, run, runs, _ := r.DispatchCounters()
	user, sys := ProcessCPUTimes()
	return permit.Sample{
		Runs:           runs,
		DispatchWaitUs: wait,
		RunUs:          run,
		InFlight:       int64(r.pool.InFlight()),
		Sessions:       int64(len(r.sessions)),
		CPUUserUsec:    user,
		CPUSysUsec:     sys,
	}
}
