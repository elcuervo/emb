package server

import "sync"

// MonitorEvent is one completed EMB / EMB.MULTI request record. Text
// payloads are deliberately excluded (privacy + weight); latency and error
// flags are the debugging signal.
type MonitorEvent struct {
	Seq       uint64 // monotonically increasing, 1-based
	AtUs      int64  // unix microseconds at request start
	Model     string
	Texts     int
	LatencyUs int64
	Err       bool
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

// Add appends an event with the next sequence number, evicting the oldest
// when the ring is full.
func (m *Monitor) Add(e MonitorEvent) {
	m.mu.Lock()
	defer m.mu.Unlock()
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
