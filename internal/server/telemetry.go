package server

import "github.com/elcuervo/emb/internal/telemetry"

// telemetrySnapshot renders the live server statistics as the exporter's plain
// snapshot. It reuses infoSnapshot — the same aggregation INFO renders — so the
// exported values cannot drift from the commands.
func (s *Server) telemetrySnapshot() telemetry.Snapshot {
	snap := s.infoSnapshot()
	out := telemetry.Snapshot{
		Uptime:            int64(snap.uptime),
		ActiveRequests:    snap.active,
		Connections:       s.conns.Load(),
		Goroutines:        snap.res.goroutines,
		ModelsLoaded:      int64(snap.models),
		RSSBytes:          snap.res.rssBytes,
		HeapBytes:         snap.res.heapBytes,
		TotalSystemMemory: snap.res.totalSysMem,
		CPUTimeUserUsec:   int64(snap.res.cpuUserUsec),
		CPUTimeSysUsec:    int64(snap.res.cpuSysUsec),
		NetInBytes:        snap.netIn,
		NetOutBytes:       snap.netOut,
		TotalRequests:     snap.totalReq,
		TotalTokens:       snap.totalTok,
		TotalErrors:       snap.totalErr,
		TruncatedTexts:    s.truncatedTexts.Load(),
		TruncatedPairs:    s.truncatedPairs.Load(),
		TruncatedImages:   s.truncatedImages.Load(),
	}
	if snap.cache != nil {
		out.CacheHits = snap.cache.Hits
		out.CacheMisses = snap.cache.Misses
		out.CacheEvictions = snap.cache.Evictions
	}
	out.Models = make([]telemetry.ModelSnapshot, 0, len(snap.byModel))
	for _, m := range snap.byModel {
		out.Models = append(out.Models, telemetry.ModelSnapshot{
			Name:     m.name,
			Requests: m.req,
			Tokens:   m.tok,
			Errors:   m.errs,
		})
	}
	return out
}

// telemetryEnabled reports whether the resolved telemetry configuration would
// export (used by CONFIG GET).
func (s *Server) telemetryEnabled() bool {
	return s.telemetryCfg.Enabled()
}
