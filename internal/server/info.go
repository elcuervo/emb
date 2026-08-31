package server

import (
	"fmt"
	"os"
	"sort"
	"strings"
	"time"

	"github.com/tidwall/redcon"
)

// cacheHitRate formats hits/(hits+misses) as "%.1f%%"; 0.0% when there is no
// cache activity. Shared semantics with EMB.INFO's cache_hit_rate.
func cacheHitRate(hits, misses int64) string {
	total := hits + misses
	if total <= 0 {
		return "0.0%"
	}
	return fmt.Sprintf("%.1f%%", float64(hits)/float64(total)*100)
}

// infoSnapshot is the immutable view the INFO builder renders. One snapshot
// struct feeds both the sectioned INFO and (via its fields) the existing
// EMB.STATS array writer, so the two cannot drift.
type infoSnapshot struct {
	version  string
	uptime   int
	process  int
	totalReq int64
	totalTok int64
	totalErr int64
	models   int
	active   int64
	cache    *CacheStats // nil when the cache is disabled at boot
	byModel  []modelInfoLine
}

// modelInfoLine is one row of the Keyspace section (per-loaded-model cache
// activity). Cache activity absent for a model renders as zeros.
type modelInfoLine struct {
	name               string
	dim                int
	hits, misses       int64
	evictions, entries int64
}

// infoSnapshot gathers the server's current statistics under one consistent
// view: pool stats aggregated across loaded models (guarding nil pools like
// handleSTATS does), plus cache stats with per-model breakdown.
func (s *Server) infoSnapshot() infoSnapshot {
	models := s.reg.List()
	names := make([]string, 0, len(models))
	byName := make(map[string]*registryEntrySnapshot, len(models))
	for _, m := range models {
		names = append(names, m.Name)
		res := &registryEntrySnapshot{dim: m.Dim}
		if m.Pool != nil {
			st := m.Pool.Stats()
			res.req, res.tok = st.Requests, st.Tokens
		}
		byName[m.Name] = res
	}
	sort.Strings(names)

	var cacheStats *CacheStats
	var perModel map[string]CacheModelStats
	if s.cache != nil {
		cs := s.cache.Stats()
		cacheStats = &cs
		perModel = cs.ByModel
	}

	lines := make([]modelInfoLine, 0, len(names))
	var totalReq, totalTok int64
	for _, name := range names {
		res := byName[name]
		totalReq += res.req
		totalTok += res.tok
		var c CacheModelStats
		if perModel != nil {
			if v, ok := perModel[name]; ok {
				c = v
			}
		}
		lines = append(lines, modelInfoLine{
			name:      name,
			dim:       res.dim,
			hits:      c.Hits,
			misses:    c.Misses,
			evictions: c.Evictions,
			entries:   c.Entries,
		})
	}

	return infoSnapshot{
		version:  s.version,
		uptime:   int(time.Since(s.started).Seconds()),
		process:  os.Getpid(),
		totalReq: totalReq,
		totalTok: totalTok,
		totalErr: s.reg.TotalErrors(),
		models:   len(models),
		active:   s.activeReqs.Load(),
		cache:    cacheStats,
		byModel:  lines,
	}
}

// registryEntrySnapshot is a lightweight per-model view used while assembling
// the snapshot (avoids keeping registry references past the gather).
type registryEntrySnapshot struct {
	dim int
	req int64
	tok int64
}

// buildInfoSections renders Redis-format INFO sections. An empty `which`
// selects all sections; named sections select subsets (case-insensitive);
// unknown names contribute nothing. Section order is fixed.
func buildInfoSections(which []string, snap infoSnapshot) string {
	selected := map[string]bool{}
	if len(which) == 0 {
		selected = map[string]bool{"server": true, "cache": true, "keyspace": true, "stats": true, "clients": true}
	} else {
		for _, w := range which {
			selected[strings.ToLower(w)] = true
		}
	}

	var b strings.Builder
	if selected["server"] {
		b.WriteString("# Server\r\n")
		fmt.Fprintf(&b, "redis_version:%s\r\n", snap.version)
		fmt.Fprintf(&b, "emb_version:%s\r\n", snap.version)
		fmt.Fprintf(&b, "uptime_secs:%d\r\n", snap.uptime)
		fmt.Fprintf(&b, "process_id:%d\r\n\r\n", snap.process)
	}
	if selected["cache"] {
		b.WriteString("# Cache\r\n")
		hits, misses := int64(0), int64(0)
		if snap.cache != nil {
			hits, misses = snap.cache.Hits, snap.cache.Misses
		}
		fmt.Fprintf(&b, "cache_hits:%d\r\n", hits)
		fmt.Fprintf(&b, "cache_misses:%d\r\n", misses)
		fmt.Fprintf(&b, "cache_hit_rate:%s\r\n", cacheHitRate(hits, misses))
		evictions, entries := int64(0), 0
		maxBytes, curBytes := int64(0), int64(0)
		if snap.cache != nil {
			evictions, entries = snap.cache.Evictions, snap.cache.Entries
			maxBytes, curBytes = snap.cache.MaxBytes, snap.cache.CurBytes
		}
		fmt.Fprintf(&b, "cache_evictions:%d\r\n", evictions)
		fmt.Fprintf(&b, "cache_entries:%d\r\n", entries)
		fmt.Fprintf(&b, "cache_max_bytes:%d\r\n", maxBytes)
		fmt.Fprintf(&b, "cache_memory_bytes:%d\r\n\r\n", curBytes)
	}
	if selected["keyspace"] {
		b.WriteString("# Keyspace\r\n")
		if len(snap.byModel) == 0 {
			b.WriteString("\r\n")
		} else {
			for _, m := range snap.byModel {
				fmt.Fprintf(&b, "db0:model=%s,keys=%d,hits=%d,misses=%d,hit_rate=%s\r\n",
					m.name, m.entries, m.hits, m.misses, cacheHitRate(m.hits, m.misses))
			}
			b.WriteString("\r\n")
		}
	}
	if selected["stats"] {
		b.WriteString("# Stats\r\n")
		fmt.Fprintf(&b, "total_requests:%d\r\n", snap.totalReq)
		fmt.Fprintf(&b, "total_tokens:%d\r\n", snap.totalTok)
		fmt.Fprintf(&b, "total_errors:%d\r\n", snap.totalErr)
		fmt.Fprintf(&b, "models_loaded:%d\r\n\r\n", snap.models)
	}
	if selected["clients"] {
		b.WriteString("# Clients\r\n")
		fmt.Fprintf(&b, "active_requests:%d\r\n", snap.active)
	}
	return b.String()
}

func (s *Server) handleInfo(conn redcon.Conn, cmd redcon.Command) {
	var which []string
	for _, a := range cmd.Args[1:] {
		which = append(which, string(a))
	}
	conn.WriteBulkString(buildInfoSections(which, s.infoSnapshot()))
}
