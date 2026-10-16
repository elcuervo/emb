package embtop

import (
	"bytes"
	"strings"
	"testing"
	"time"
)

// monitorRowsReply builds a MONITOR reply of flat event rows, the RESP2 shape.
func monitorRowsReply(rows ...[]int64) []byte {
	encoded := make([][]byte, 0, len(rows))
	for i, row := range rows {
		parts := make([][]byte, 0, len(row))
		for j, v := range row {
			if j == 2 {
				parts = append(parts, encodeBulk("m"))
				continue
			}
			parts = append(parts, encodeInt(v))
		}
		// A model name occupies the third slot, so the row's values after it
		// shift by nothing: the placeholder above keeps the count right.
		parts[2] = encodeBulk("m")
		_ = i
		encoded = append(encoded, encodeArray(parts...))
	}
	return encodeArray(encoded...)
}

// eventRow is one MONITOR event as its eleven fields.
func eventRow(seq, latencyUs, cachedTexts, attributed, dispatchUs, admitUs, runUs int64) []int64 {
	return []int64{seq, 1_700_000_000_000_000, 0, 1, latencyUs, 0, cachedTexts, attributed, dispatchUs, admitUs, runUs}
}

// attributedResponder serves a node whose window mixes cache-served requests
// with runs that waited at the CPU gate. withPhases false stands in for a
// server that publishes neither the per-model phases nor the event
// attribution: the older-server case.
func attributedResponder(withPhases bool, events []byte) func([]string, int) []byte {
	var round int
	sent := false
	return func(cmd []string, _ int) []byte {
		switch strings.ToUpper(cmd[0]) {
		case "EMB.MODELS":
			return modelsReply([]string{"m", "384"})
		case "EMB.INFO":
			round++
			fields := map[string]int64{
				"requests": int64(round) * 40, "tokens": int64(round) * 160,
				"cache_hits": int64(round) * 16, "cache_misses": int64(round) * 24,
				"cache_entries": 10, "cache_evictions": 0,
			}
			if withPhases {
				fields["embed_dispatch_wait_us"] = int64(round) * 40_000
				fields["embed_admission_wait_us"] = int64(round) * 900_000
				fields["embed_run_us"] = int64(round) * 200_000
				fields["embed_runs"] = int64(round) * 8
				fields["embed_sessions_busy"] = int64(round) % 2
				fields["embed_sessions_total"] = 1
			}
			return infoReply(map[string]string{"cache_hit_rate": "50.0%"}, fields)
		case "EMB.STATS":
			return statsReply(map[string]int64{
				"uptime_secs": 1, "models_loaded": 1, "total_requests": int64(round) * 40,
				"cache_hits": int64(round) * 16, "cache_misses": int64(round) * 24,
				"cpu_budget_enabled": 1, "cpu_allowance": 200, "cpu_capacity": 2,
				"cpu_budget": 2, "cpu_active_tokens": 2, "cpu_peak_tokens": 2,
			})
		case "INFO":
			return encodeBulk("gomaxprocs:2\r\n")
		case "MONITOR":
			// MONITOR is incremental: the client keeps the events it has
			// already been given, so a replay would double-count the window.
			if sent {
				return encodeArray()
			}
			sent = true
			return events
		default:
			return encodeError("ERR unknown " + cmd[0])
		}
	}
}

// The headless line must carry the attribution the dashboard renders, under
// token names a script can read, beside the tokens it already carried.
func TestRunOnceCarriesTheAttribution(t *testing.T) {
	events := monitorRowsReply(
		eventRow(1, 3, 1, 1, 0, 0, 0),
		eventRow(2, 4, 1, 1, 0, 0, 0),
		eventRow(3, 1_900_000, 0, 1, 200, 1_700_000, 190_000),
		eventRow(4, 900_000, 0, 1, 100, 800_000, 90_000),
		eventRow(5, 400_000, 0, 1, 50, 300_000, 40_000),
		eventRow(6, 5, 1, 1, 0, 0, 0),
		eventRow(7, 6, 1, 1, 0, 0, 0),
		eventRow(8, 7, 1, 1, 0, 0, 0),
	)
	_, addr := startScripted(t, attributedResponder(true, events))
	c := NewClient(addr, "", false)
	var out bytes.Buffer
	if err := RunOnce(c, time.Second, 3, &out); err != nil {
		t.Fatalf("RunOnce: %v", err)
	}
	line := lastLine(out.String())

	// The tokens it already carried keep their names.
	for _, key := range []string{"lat_p95_us", "lat_p50_us", "cpu_budget", "cache_hits"} {
		if _, ok := parseKV(line, key); !ok {
			t.Errorf("existing token %q disappeared:\n%s", key, line)
		}
	}
	// The attribution arrives beside them.
	if got, ok := parseKV(line, "act"); !ok || got == "" {
		t.Errorf("no action token: %q", line)
	}
	// The cached share is the window's, the same one the dashboard states
	// beside the advice: this fixture's event ring holds 5 cache answers of 8
	// (62.5%), while the window served 32 of 80 requests (40%).
	if got, ok := parseKV(line, "cached_pct"); !ok || got != "40.0" {
		t.Errorf("cached share = %q (ok=%v), want the window's 40.0:\n%s", got, ok, line)
	}
	if got, ok := parseKV(line, "lat_p95_run_us"); !ok || got != "1900000" {
		t.Errorf("uncached p95 = %q (ok=%v), want the runs' 1900000", got, ok)
	}
	if got, ok := parseKV(line, "lat_p95_run_n"); !ok || got != "3" {
		t.Errorf("uncached tail size = %q (ok=%v), want the window's 3 runs", got, ok)
	}
	if got, ok := parseKV(line, "phase"); !ok || got != string(PhaseAdmission) {
		t.Errorf("dominant phase = %q (ok=%v), want %q", got, ok, PhaseAdmission)
	}
	if got, ok := parseKV(line, "phase_share"); !ok || got == "" {
		t.Errorf("no phase share: %q", line)
	}
	if _, ok := parseKV(line, "phase_admit_us"); !ok {
		t.Errorf("no phase means: %q", line)
	}

	// The per-model section carries the model's own window.
	model := modelSection(line, "m")
	if model == "" {
		t.Fatalf("no model section:\n%s", line)
	}
	for _, key := range []string{"runs", "wait_us", "admit_us", "run_us", "cache_served_reqs"} {
		if _, ok := parseKV(model, key); !ok {
			t.Errorf("model section is missing %q: %q", key, model)
		}
	}
	// The means are microseconds per run, not a duration's nanoseconds: the
	// fixture's window is 200000us of run over 8 runs.
	if got, _ := parseKV(model, "run_us"); got != "25000" {
		t.Errorf("the model's run mean = %q, want 25000us per run", got)
	}
	if got, _ := parseKV(model, "admit_us"); got != "112500" {
		t.Errorf("the model's gate-wait mean = %q, want 112500us per run", got)
	}
	if got, _ := parseKV(model, "wait_us"); got != "5000" {
		t.Errorf("the model's dispatch mean = %q, want 5000us per run", got)
	}
}

// An older server sends events without the attribution: the line keeps its
// tokens and carries no action or phase, rather than reading absent phases as a
// measured run.
func TestRunOnceWithoutAttributionKeepsItsTokens(t *testing.T) {
	plain := func(seq int64, latency int64) []int64 {
		return []int64{seq, 1_700_000_000_000_000, 0, 1, latency, 0}
	}
	events := monitorRowsReply(plain(1, 400), plain(2, 500), plain(3, 600))
	_, addr := startScripted(t, attributedResponder(false, events))
	c := NewClient(addr, "", false)
	var out bytes.Buffer
	if err := RunOnce(c, time.Second, 3, &out); err != nil {
		t.Fatalf("RunOnce: %v", err)
	}
	line := lastLine(out.String())
	if _, ok := parseKV(line, "lat_p95_us"); !ok {
		t.Errorf("the service percentile disappeared:\n%s", line)
	}
	if got, ok := parseKV(line, "lat_p95_run_us"); ok {
		t.Errorf("an unattributed window reported a run tail: %q", got)
	}
	if got, ok := parseKV(line, "phase"); ok {
		t.Errorf("an unattributed window reported a phase: %q", got)
	}
}

// A poll whose event ring and whose counter window disagree must still be
// attributed one way: -once writes the share the dashboard states beside the
// advice — the window's — so the two surfaces cannot report a different cache
// share for the same poll.
func TestOnceCacheShareFollowsTheWindowThePanelRenders(t *testing.T) {
	s := NewSampler(4)
	at := time.Unix(0, 0)
	s.SetClock(func() time.Time { return at })

	// The window served 10 of every 100 requests from the cache ...
	var last *PollResult
	for i := range 2 {
		last = &PollResult{
			UptimeSecs: 1, TotalRequests: int64(100 * (i + 1)),
			CacheHits: int64(10 * (i + 1)), CacheMisses: int64(90 * (i + 1)),
			ModelsLoaded: 1,
		}
		s.Push(last)
		at = at.Add(time.Second)
	}
	// ... while the ring happens to hold only requests it answered entirely.
	s.PushEvents([]Event{
		{Seq: 1, Model: "m", Texts: 1, CachedTexts: 1, Attributed: true, LatencyUs: 5},
		{Seq: 2, Model: "m", Texts: 1, CachedTexts: 1, Attributed: true, LatencyUs: 6},
	})

	ring := s.PathCounts().Share()
	if ring == 10 {
		t.Fatalf("the fixture's ring and window agree at %.1f%%, so it cannot show which one is read", ring)
	}

	// What the dashboard states beside the advice for the same sampler.
	panel := Advise(SamplerAdviceInput(s)).CachedPct
	if panel != 10 {
		t.Fatalf("the dashboard's cached share = %v, want the window's 10", panel)
	}

	var out bytes.Buffer
	if err := writeOnceLine(&out, s.Latest, last, nil, s); err != nil {
		t.Fatalf("writeOnceLine: %v", err)
	}
	line := strings.TrimSpace(out.String())
	got, ok := parseKV(line, "cached_pct")
	if !ok {
		t.Fatalf("no cached share on the line: %q", line)
	}
	if got != "10.0" {
		t.Errorf("cached_pct = %q, want the window's 10.0 (the ring's share is %.1f%%):\n%s",
			got, ring, line)
	}
}

// lastLine is the newest once-line, which has the full window behind it.
func lastLine(out string) string {
	lines := strings.Split(strings.TrimSpace(out), "\n")
	return lines[len(lines)-1]
}

// modelSection extracts one ` model:<name> ...` section from a once-line.
func modelSection(line, name string) string {
	idx := strings.Index(line, " model:"+name+" ")
	if idx < 0 {
		return ""
	}
	rest := line[idx+1:]
	if next := strings.Index(rest, " model:"); next >= 0 {
		rest = rest[:next]
	}
	return rest
}
