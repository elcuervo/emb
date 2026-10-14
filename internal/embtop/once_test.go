package embtop

import (
	"bytes"
	"fmt"
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"
)

// TestRunOnceRates runs the headless mode against a scripted server at a
// real 1s poll interval — the exact shape of a live run — and asserts the
// rate fields are computed from counter deltas.
func TestRunOnceRates(t *testing.T) {
	var round int
	total := int64(0)
	cpu := int64(0)
	modelReqs := int64(0)
	respond := func(cmd []string, _ int) []byte {
		switch strings.ToUpper(cmd[0]) {
		case "EMB.MODELS":
			return modelsReply([]string{"m", "384"})
		case "EMB.INFO":
			return infoReply(nil, map[string]int64{"requests": modelReqs, "tokens": modelReqs * 4})
		case "EMB.STATS":
			// Requests arrive between polls; each EMB.STATS sees cumulative totals.
			if round >= 1 {
				total += 38
				cpu += 315000
				modelReqs = total
			}
			round++
			return statsReply(map[string]int64{
				"uptime_secs": int64(round), "total_requests": total,
				"total_tokens": total * 4, "cpu_user_usec": cpu,
			})
		case "MONITOR":
			seq := int64(1000 + round)
			return encodeArray(
				encodeArray(encodeInt(seq), encodeInt(1_700_000_000_000_000), encodeBulk("m"), encodeInt(1), encodeInt(int64(400)+int64(round)), encodeInt(0)),
			)
		case "INFO":
			return encodeBulk("gomaxprocs:4\r\n")
		default:
			return encodeError("ERR unknown " + cmd[0])
		}
	}
	_, addr := startScripted(t, respond)
	c := NewClient(addr, "", false)
	var out bytes.Buffer
	err := RunOnce(c, time.Second, 3, &out)
	if err != nil {
		t.Fatalf("RunOnce: %v", err)
	}
	lines := strings.Split(strings.TrimSpace(out.String()), "\n")
	if len(lines) != 3 {
		t.Fatalf("want 3 lines, got %d:\n%s", len(lines), out.String())
	}
	l2 := lines[1]
	l3 := lines[2]
	fmt.Println(l2)
	fmt.Println(l3)
	for _, field := range []string{"req_rate"} {
		v, ok := parseKV(l2, field)
		if !ok {
			t.Fatalf("missing %s in %q", field, l2)
		}
		f, err := strconv.ParseFloat(v, 64)
		if err != nil || f <= 0 {
			t.Fatalf("expected %s > 0 (bug: rate not computed), got %q in %q", field, v, l2)
		}
	}
	if !strings.Contains(l3, "model:m") {
		t.Fatalf("missing per-model section in %q", l3)
	}
	if v, ok := parseKV(l3, "lat_p50_us"); ok {
		if f, err := strconv.ParseFloat(v, 64); err != nil || f < 0 {
			t.Fatalf("bad lat_p50_us in %q (%v)", l3, err)
		}
	} else {
		t.Fatalf("missing lat_p50_us in %q", l3)
	}
	// Per-model section: parse fields after "model:m".
	modelSec := l3[strings.Index(l3, "model:m"):]
	if v, ok := parseKV(modelSec, "req_rate"); ok {
		if f, err := strconv.ParseFloat(v, 64); err != nil || f <= 0 {
			t.Fatalf("expected per-model req_rate > 0 on line 3, got %q in %q", v, l3)
		}
	} else {
		t.Fatalf("no per-model req_rate in section %q", modelSec)
	}
}

func parseKV(line, key string) (string, bool) {
	for _, f := range strings.Fields(line) {
		if strings.HasPrefix(f, key+"=") {
			return strings.TrimPrefix(f, key+"="), true
		}
	}
	return "", false
}

// TestRunOnceKeepsFirstSeenModelOrder guards that the headless lines do not
// follow the server's per-call EMB.MODELS enumeration order.
func TestRunOnceKeepsFirstSeenModelOrder(t *testing.T) {
	orders := [][]string{{"z", "a", "m"}, {"m", "z", "a"}, {"a", "m", "z"}}
	round := 0
	respond := func(cmd []string, _ int) []byte {
		switch strings.ToUpper(cmd[0]) {
		case "EMB.MODELS":
			var rows [][]string
			for _, n := range orders[round%len(orders)] {
				rows = append(rows, []string{n, "8"})
			}
			return modelsReply(rows...)
		case "EMB.INFO":
			return infoReply(nil, map[string]int64{"requests": 1, "tokens": 2})
		case "EMB.STATS":
			round++
			return statsReply(map[string]int64{"uptime_secs": int64(round)})
		case "MONITOR":
			return encodeArray()
		case "INFO":
			return encodeBulk("gomaxprocs:4\r\n")
		default:
			return encodeError("ERR unknown " + cmd[0])
		}
	}
	_, addr := startScripted(t, respond)
	c := NewClient(addr, "", false)
	var out bytes.Buffer
	if err := RunOnce(c, time.Millisecond, 3, &out); err != nil {
		t.Fatalf("RunOnce: %v", err)
	}
	lines := strings.Split(strings.TrimSpace(out.String()), "\n")
	if len(lines) != 3 {
		t.Fatalf("want 3 lines, got %d:\n%s", len(lines), out.String())
	}
	want := []string{"z", "a", "m"}
	for i, line := range lines[1:] {
		if got := modelSectionOrder(line); !slices.Equal(got, want) {
			t.Errorf("line %d model order = %v, want %v\n%s", i+2, got, want, line)
		}
	}
}

func TestFleetAggregateCacheHitRateStaysBounded(t *testing.T) {
	// Two nodes at 100% and 0% must aggregate to 50%, not 150%.
	a := &onceNode{label: "a", sampler: NewSampler(4), res: &PollResult{CacheHits: 100}, sampled: true}
	a.sampler.Push(&PollResult{CacheHits: 100})
	b := &onceNode{label: "b", sampler: NewSampler(4), res: &PollResult{CacheMisses: 100}, sampled: true}
	b.sampler.Push(&PollResult{CacheMisses: 100})

	out := string(appendFleetAggregate(nil, []*onceNode{a, b}, FleetRecommendation{}))
	v, ok := parseKV(out, "cache_hit_rate")
	if !ok {
		t.Fatalf("no cache_hit_rate in %q", out)
	}
	f, err := strconv.ParseFloat(v, 64)
	if err != nil {
		t.Fatalf("cache_hit_rate %q: %v", v, err)
	}
	if f != 50 {
		t.Fatalf("aggregate cache_hit_rate = %v, want 50 (ratio of summed counters)", f)
	}
}

func modelSectionOrder(line string) []string {
	var out []string
	for _, f := range strings.Fields(line) {
		if strings.HasPrefix(f, "model:") {
			out = append(out, strings.TrimPrefix(f, "model:"))
		}
	}
	return out
}

// planResponder answers a full poll for one node with a process CPU plan.
func planResponder(budget, active, peak, allowanceCenti int64) func([]string, int) []byte {
	return func(cmd []string, _ int) []byte {
		switch strings.ToUpper(cmd[0]) {
		case "EMB.MODELS":
			return modelsReply([]string{"m", "384"})
		case "EMB.INFO":
			return infoReply(nil, map[string]int64{
				"cpu_budget_enabled": 1, "cpu_budget": budget, "cpu_thread_share": budget,
				"intra_op_threads": budget, "workers": 1, "requests": 4,
			})
		case "EMB.STATS":
			return statsReply(map[string]int64{
				"uptime_secs": 1, "models_loaded": 1, "total_requests": 4,
				"cpu_budget_enabled": 1, "cpu_allowance": allowanceCenti, "cpu_capacity": 10,
				"cpu_budget": budget, "cpu_active_tokens": active, "cpu_peak_tokens": peak,
			})
		case "INFO":
			return encodeBulk("gomaxprocs:10\r\n")
		case "MONITOR":
			return encodeArray()
		default:
			return encodeError("ERR unknown " + cmd[0])
		}
	}
}

func numKV(t *testing.T, line, key string) float64 {
	t.Helper()
	raw, ok := parseKV(line, key)
	if !ok {
		t.Fatalf("field %q missing from %q", key, line)
	}
	v, err := strconv.ParseFloat(raw, 64)
	if err != nil {
		t.Fatalf("field %q = %q is not numeric: %v", key, raw, err)
	}
	return v
}

// The headless single-node line must carry the process CPU plan and the
// calibrated per-model layout, so a script sees the same numbers the dashboard
// renders.
func TestRunOnceSingleNodeReportsCPUPlan(t *testing.T) {
	_, addr := startScripted(t, planResponder(8, 2, 5, 1000))
	c := NewClient(addr, "", false)
	var out bytes.Buffer
	if err := RunOnce(c, time.Second, 2, &out); err != nil {
		t.Fatalf("RunOnce: %v", err)
	}
	lines := strings.Split(strings.TrimSpace(out.String()), "\n")
	line := lines[len(lines)-1]
	for key, want := range map[string]float64{
		"cpu_budget": 8, "cpu_active_tokens": 2, "cpu_peak_tokens": 5,
		"cpu_allowance": 10, "cpu_budget_enabled": 1,
	} {
		if got := numKV(t, line, key); got != want {
			t.Errorf("%s = %v, want %v", key, got, want)
		}
	}
	if got := numKV(t, line, "intra_op_threads"); got != 8 {
		t.Errorf("per-model intra_op_threads = %v, want 8", got)
	}
}

// Fleet mode sums independent per-node plans for the aggregate and reports each
// node's own budget in its section.
func TestRunOnceFleetReportsCPUPlan(t *testing.T) {
	_, addrA := startScripted(t, planResponder(8, 2, 5, 1000))
	_, addrB := startScripted(t, planResponder(4, 1, 3, 400))
	ca, cb := NewClient(addrA, "", false), NewClient(addrB, "", false)
	var out bytes.Buffer
	if err := RunOnceFleet([]*Client{ca, cb}, []string{"a", "b"}, time.Second, 2, &out); err != nil {
		t.Fatalf("RunOnceFleet: %v", err)
	}
	lines := strings.Split(strings.TrimSpace(out.String()), "\n")
	line := lines[len(lines)-1]
	if got := numKV(t, line, "cpu_budget"); got != 12 {
		t.Errorf("fleet cpu_budget = %v, want 12 (independent plans summed)", got)
	}
	if got := numKV(t, line, "cpu_active_tokens"); got != 3 {
		t.Errorf("fleet cpu_active_tokens = %v, want 3", got)
	}
	if got := numKV(t, line, "cpu_allowance"); got != 14 {
		t.Errorf("fleet cpu_allowance = %v, want 14 cores", got)
	}
	if !strings.Contains(line, "node:a") || !strings.Contains(line, "node:b") {
		t.Fatalf("fleet line missing node sections: %q", line)
	}
	// Each node section keeps its own budget, so a single node's plan is not
	// mistaken for the fleet total.
	start := strings.Index(line, "node:a")
	end := strings.Index(line, "node:b")
	if start < 0 || end < start {
		t.Fatalf("node sections not in expected order: %q", line)
	}
	if got := numKV(t, line[start:end], "cpu_budget"); got != 8 {
		t.Errorf("node:a cpu_budget = %v, want 8", got)
	}
	if got := numKV(t, line[end:], "cpu_budget"); got != 4 {
		t.Errorf("node:b cpu_budget = %v, want 4", got)
	}
}

// boundResponder answers a full poll for a node that holds its whole CPU budget
// with admission waits timing out: the sizing recommendation's CPU rule.
func boundResponder(timeouts int64) func([]string, int) []byte {
	return func(cmd []string, _ int) []byte {
		switch strings.ToUpper(cmd[0]) {
		case "EMB.MODELS":
			return modelsReply([]string{"m", "384"})
		case "EMB.INFO":
			return infoReply(nil, map[string]int64{"requests": 4, "tokens": 16})
		case "EMB.STATS":
			return statsReply(map[string]int64{
				"uptime_secs": 1, "models_loaded": 1, "total_requests": 4,
				"cpu_budget_enabled": 1, "cpu_allowance": 800, "cpu_capacity": 8,
				"cpu_budget": 8, "cpu_active_tokens": 8, "cpu_peak_tokens": 8,
				"cpu_admission_timeouts": timeouts,
			})
		case "INFO":
			return encodeBulk("gomaxprocs:8\r\n")
		case "MONITOR":
			return encodeArray()
		default:
			return encodeError("ERR unknown " + cmd[0])
		}
	}
}

// The headless lines carry the sizing recommendation a script needs, beside the
// per-node and fleet numbers.
func TestRunOnceCarriesRecommendation(t *testing.T) {
	var round int
	respond := func(cmd []string, _ int) []byte {
		if strings.ToUpper(cmd[0]) == "EMB.STATS" {
			round++
			return boundResponder(int64(round*5))(cmd, 0)
		}
		return boundResponder(0)(cmd, 0)
	}
	_, addr := startScripted(t, respond)
	c := NewClient(addr, "", false)
	var out bytes.Buffer
	if err := RunOnce(c, time.Second, 2, &out); err != nil {
		t.Fatalf("RunOnce: %v", err)
	}
	lines := strings.Split(strings.TrimSpace(out.String()), "\n")
	if got, ok := parseKV(lines[0], "rec"); ok {
		t.Errorf("the first poll has one sample and cannot recommend, got rec=%s", got)
	}
	if got, ok := parseKV(lines[len(lines)-1], "rec"); !ok || got != ResourceCPU {
		t.Fatalf("last line rec = %q (ok=%v), want %s:\n%s", got, ok, ResourceCPU, out.String())
	}
}

// The fleet line carries the fleet's own recommendation and each node's own.
func TestRunOnceFleetCarriesRecommendation(t *testing.T) {
	var a, b int
	nodeA := func(cmd []string, _ int) []byte {
		if strings.ToUpper(cmd[0]) == "EMB.STATS" {
			a++
			return boundResponder(int64(a*5))(cmd, 0)
		}
		return boundResponder(0)(cmd, 0)
	}
	nodeB := func(cmd []string, _ int) []byte {
		if strings.ToUpper(cmd[0]) == "EMB.STATS" {
			b++
			return boundResponder(int64(b*5))(cmd, 0)
		}
		return boundResponder(0)(cmd, 0)
	}
	_, addrA := startScripted(t, nodeA)
	_, addrB := startScripted(t, nodeB)
	ca, cb := NewClient(addrA, "", false), NewClient(addrB, "", false)
	var out bytes.Buffer
	if err := RunOnceFleet([]*Client{ca, cb}, []string{"a", "b"}, time.Second, 2, &out); err != nil {
		t.Fatalf("RunOnceFleet: %v", err)
	}
	line := strings.Split(strings.TrimSpace(out.String()), "\n")[1]
	// Both nodes hold their whole budget, so the fleet names the resource and
	// the capacity it is out of.
	if got, ok := parseKV(line, "rec"); !ok || got != ResourceCPU+"+"+ResourceNodes {
		t.Errorf("fleet rec = %q (ok=%v), want %s+%s:\n%s", got, ok, ResourceCPU, ResourceNodes, line)
	}
	start := strings.Index(line, "node:a")
	end := strings.Index(line, "node:b")
	if start < 0 || end < start {
		t.Fatalf("node sections not in expected order: %q", line)
	}
	if got, ok := parseKV(line[start:end], "rec"); !ok || got != ResourceCPU {
		t.Errorf("node:a rec = %q (ok=%v), want %s", got, ok, ResourceCPU)
	}
	if got, ok := parseKV(line[end:], "rec"); !ok || got != ResourceCPU {
		t.Errorf("node:b rec = %q (ok=%v), want %s", got, ok, ResourceCPU)
	}
}

// The single-node line must carry the advisory with the node's own fields, not
// after the model sections: a reader takes the aggregate as everything before
// the first ` model:`.
func TestRunOnceRecommendationSitsWithTheAggregate(t *testing.T) {
	var round int
	respond := func(cmd []string, _ int) []byte {
		if strings.ToUpper(cmd[0]) == "EMB.STATS" {
			round++
			return boundResponder(int64(round*5))(cmd, 0)
		}
		return boundResponder(0)(cmd, 0)
	}
	_, addr := startScripted(t, respond)
	c := NewClient(addr, "", false)
	var out bytes.Buffer
	if err := RunOnce(c, time.Second, 2, &out); err != nil {
		t.Fatalf("RunOnce: %v", err)
	}
	line := strings.Split(strings.TrimSpace(out.String()), "\n")[1]
	if _, ok := parseKV(line, "rec"); !ok {
		t.Fatalf("no rec on the line: %q", line)
	}
	aggregate := line
	if i := strings.Index(line, " model:"); i >= 0 {
		aggregate = line[:i]
	}
	if _, ok := parseKV(aggregate, "rec"); !ok {
		t.Fatalf("rec is not in the node's own fields: %q", line)
	}
}

// A fleet whose only advice is a node to inspect must still say so: the token
// cannot be dropped just because no resource is binding.
func TestRunOnceFleetNamesTheNodeWhenNothingBinds(t *testing.T) {
	states := []*onceNode{
		{label: "a", sampler: NewSampler(4), res: &PollResult{}, sampled: true},
		{label: "b", sampler: NewSampler(4), res: &PollResult{}, sampled: true},
	}
	for _, st := range states {
		st.sampler.Push(&PollResult{})
	}
	// a is far slower than its peer at comparable load.
	states[0].sampler.PushEvents([]Event{{Model: "m", LatencyUs: 1000}})
	states[1].sampler.PushEvents([]Event{{Model: "m", LatencyUs: 9000}})

	rec := fleetRecommendation(states)
	if rec.First != "b" {
		t.Fatalf("First = %q, want the slow peer b", rec.First)
	}
	line := string(appendFleetAggregate(nil, states, rec))
	if got, ok := parseKV(line, "rec_first"); !ok || got != "b" {
		t.Fatalf("rec_first = %q (ok=%v), want b:\n%s", got, ok, line)
	}
}
