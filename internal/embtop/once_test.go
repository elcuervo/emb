package embtop

import (
	"bytes"
	"fmt"
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
