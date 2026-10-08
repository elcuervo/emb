// Package telemetry exports a running emb server's statistics as OpenTelemetry
// metrics over OTLP/HTTP.
//
// It builds with CGO_ENABLED=0 and imports nothing from internal/server: the
// server hands it a plain Snapshot. That keeps the encoding and delta logic
// testable without ONNX and reusable by an out-of-process producer later.
package telemetry

// Snapshot is one point-in-time view of the server's cumulative counters,
// gauges, and per-model activity. Every field is a value the server already
// reports through INFO/EMB.STATS; the exporter never derives a value the
// server did not measure.
type Snapshot struct {
	// Gauges.
	Uptime            int64
	ActiveRequests    int64
	Connections       int64
	Goroutines        int64
	ModelsLoaded      int64
	RSSBytes          uint64
	HeapBytes         uint64
	TotalSystemMemory uint64

	// Cumulative counters. Monotonic except across a process restart.
	CPUTimeUserUsec int64
	CPUTimeSysUsec  int64
	NetInBytes      uint64
	NetOutBytes     uint64
	CacheHits       int64
	CacheMisses     int64
	CacheEvictions  int64
	TotalRequests   int64
	TotalTokens     int64
	TotalErrors     int64
	TruncatedTexts  int64
	TruncatedPairs  int64
	TruncatedImages int64

	// Models is the per-model breakdown, one entry per loaded model.
	Models []ModelSnapshot
}

// ModelSnapshot is the per-model slice of a Snapshot.
type ModelSnapshot struct {
	Name     string
	Requests int64
	Tokens   int64
	Errors   int64
}
