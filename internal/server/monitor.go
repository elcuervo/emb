package server

import (
	"sync"
	"time"
)

// MonitorEvent is one completed inference request record (EMB, EMB.MULTI,
// EMB.IMG, EMB.IMGMULTI, EMB.EVAL, EMB.EVSHA). Text and image payloads are
// deliberately excluded (privacy + weight); latency and error flags are the
// debugging signal. For image commands Texts is the request's image count.
type MonitorEvent struct {
	Seq       uint64 // monotonically increasing, 1-based
	AtUs      int64  // unix microseconds at request start
	Model     string
	Texts     int
	LatencyUs int64
	Err       bool

	// CachedTexts is how many of Texts the reply cache answered without
	// inference (for images, images whose every requested output was cached).
	// Always measured.
	CachedTexts int
	// Attributed reports whether the phases below were measured for this
	// request. It is true once the request reached its inference path (the
	// embedding pool, or for images embedImages); a request that failed before
	// any inference carries Attributed false with zero phases — absent
	// attribution, never a claim that its run cost nothing.
	Attributed bool
	// PreprocessUs is image preprocessing time (decode + resize + tensor
	// normalization), measured around each image's plan.Tensor call. Zero for
	// text requests and for image requests answered entirely from cache.
	PreprocessUs int64
	// DispatchUs, AdmitUs and RunUs are the time this request's work that was
	// not cache-answered spent waiting for a session or batch window, waiting at
	// the process CPU admission gate, and running. For images these cover the
	// image-session pool. AdmitUs is kept out of RunUs.
	DispatchUs int64
	AdmitUs    int64
	RunUs      int64
}

// attributed returns e with one request's path and phases attached, so a record
// site does not repeat the field mapping.
func (e MonitorEvent) attributed(a requestAttribution) MonitorEvent {
	e.CachedTexts = a.cachedTexts
	e.Attributed = a.attributed
	e.PreprocessUs = a.preprocess.Microseconds()
	e.DispatchUs = a.dispatchWait.Microseconds()
	e.AdmitUs = a.admissionWait.Microseconds()
	e.RunUs = a.run.Microseconds()
	return e
}

// Monitor is a bounded, seq-numbered ring buffer of recent request events.
// Clients fetch incrementally via MONITOR (events with seq > last seen).
type Monitor struct {
	mu   sync.Mutex
	seq  uint64
	cap  int
	head int // next write slot
	size int
	buf  []MonitorEvent
}

// NewMonitor returns a Monitor holding at most capacity events.
func NewMonitor(capacity int) *Monitor {
	if capacity < 1 {
		capacity = 1
	}
	return &Monitor{cap: capacity, buf: make([]MonitorEvent, capacity)}
}

// maxMonitorModelLen bounds the model identifier retained per event: model
// names are client-controlled and the ring holds thousands of events, so a
// hostile caller must not be able to pin memory with huge names (CWE-770).
const maxMonitorModelLen = 128

// Add appends an event with the next sequence number, evicting the oldest
// when the ring is full.
func (m *Monitor) Add(e MonitorEvent) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if len(e.Model) > maxMonitorModelLen {
		e.Model = e.Model[:maxMonitorModelLen]
	}
	m.seq++
	e.Seq = m.seq
	m.buf[m.head] = e
	m.head = (m.head + 1) % m.cap
	if m.size < m.cap {
		m.size++
	}
}

// LastSeq returns the highest assigned sequence number (0 when empty).
func (m *Monitor) LastSeq() uint64 {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.seq
}

// Since returns up to limit events with Seq > after, oldest first. When the
// ring has already overwritten some of those events, the list starts at the
// oldest surviving event — the caller detects the gap via sequence checks.
func (m *Monitor) Since(after uint64, limit int) []MonitorEvent {
	m.mu.Lock()
	defer m.mu.Unlock()
	if limit < 1 {
		limit = 1
	}
	if m.size == 0 {
		return nil
	}
	if limit > m.size {
		limit = m.size
	}
	oldest := (m.head - m.size + m.cap) % m.cap
	out := make([]MonitorEvent, 0, limit)
	for i := 0; i < m.size && len(out) < limit; i++ {
		e := m.buf[(oldest+i)%m.cap]
		if e.Seq > after {
			out = append(out, e)
		}
	}
	return out
}

// recordRequest records one completed inference request: the bounded MONITOR
// ring event plus the per-model request-latency histogram observation. Every
// inference command (EMB, EMB.MULTI, EMB.IMG, EMB.IMGMULTI, EMB.EVAL,
// EMB.EVSHA) goes through this helper, so the ring's population and the
// histogram's are identical by construction — failed requests included.
//
// The histogram itself only tracks the models the registry is configured with
// (s.latency was built with Registry.HasModel as its gate), so a
// client-supplied model name can neither allocate a series nor evict a
// legitimate one; such a request still produces its MONITOR event.
func (s *Server) recordRequest(e MonitorEvent) {
	s.monitor.Add(e)
	s.latency.Observe(e.Model, time.Duration(e.LatencyUs)*time.Microsecond)
}
