package registry

import (
	"os"
	"sync"
	"sync/atomic"
)

// memorySafetyDivisor keeps 1/N of the memory limit free for everything the
// reservation does not model: the Go heap, tokenizers, ORT scratch growth and
// the kernel.
const memorySafetyDivisor = 10

// memoryLimit and memoryInUse are seams so tests can drive the headroom
// deterministically.
var (
	memoryLimit = EffectiveMemoryLimit
	memoryInUse = func() uint64 {
		bytes, _ := CurrentMemoryUsage()
		return bytes
	}
)

// sessionEstimate is the expected resident cost of one embedding session: the
// weights file, its external data sidecar and 20% runtime overhead. It is an
// estimate, not a measurement; measured residency enters through the process
// RSS the planner subtracts.
func sessionEstimate(modelPath string) uint64 {
	var size uint64
	for _, path := range []string{modelPath, modelPath + "_data", modelPath + ".data"} {
		if info, err := os.Stat(path); err == nil {
			size += uint64(info.Size())
		}
	}
	return size + size/5
}

// memoryPlanner is the process-wide resident-memory ledger. Headroom is the
// memory limit (less a safety margin) minus current RSS, the cache allowance,
// the minimum reservation of every configured model that is not resident yet,
// and what concurrent loads have already reserved but not yet faulted in.
// Reservations are serialized, so two simultaneous lazy loads cannot each
// claim the same free memory.
type memoryPlanner struct {
	mu       sync.Mutex
	minimum  map[string]uint64 // per configured model: one session
	resident map[string]bool   // models whose pool is built (RSS counts them)
	pending  map[string]uint64 // in-flight reservations not yet in RSS
	cache    atomic.Uint64
}

func newMemoryPlanner() *memoryPlanner {
	return &memoryPlanner{minimum: map[string]uint64{}, resident: map[string]bool{}, pending: map[string]uint64{}}
}

// register records a configured model's minimum reservation.
func (m *memoryPlanner) register(model string, perSession uint64) {
	m.mu.Lock()
	m.minimum[model] = perSession
	m.mu.Unlock()
}

// settle marks a model resident: from now on its sessions are part of RSS.
func (m *memoryPlanner) settle(model string) {
	m.mu.Lock()
	m.resident[model] = true
	m.mu.Unlock()
}

// reserve claims memory for up to want sessions of perSession bytes and returns
// how many fit (possibly zero) with a release that must be called once the
// sessions exist or the load is abandoned. At least one session is held while
// reserved, since a model that is loading will build one regardless; callers
// with an explicit layout decide whether a shortfall is fatal. A zero or
// unknown memory limit means unlimited.
func (m *memoryPlanner) reserve(model string, perSession uint64, want int) (fit int, release func()) {
	m.mu.Lock()
	defer m.mu.Unlock()
	fit = want
	if limit := memoryLimit(); limit > 0 && perSession > 0 {
		budget := limit - limit/memorySafetyDivisor
		taken := memoryInUse() + m.cache.Load()
		for name, floor := range m.minimum {
			if name != model && !m.resident[name] && m.pending[name] == 0 {
				taken += floor
			}
		}
		for name, held := range m.pending {
			if name != model {
				taken += held
			}
		}
		headroom := uint64(0)
		if budget > taken {
			headroom = budget - taken
		}
		fit = min(want, int(headroom/perSession))
	}
	m.pending[model] = uint64(max(fit, 1)) * perSession
	return fit, func() {
		m.mu.Lock()
		delete(m.pending, model)
		m.mu.Unlock()
	}
}
