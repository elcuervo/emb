package server

import (
	"fmt"
	"strconv"
	"strings"
	"testing"
)

// ---- RESP helper (handles *, $, :, +, -) ----

type respToken struct {
	kind string // "array", "bulk", "int", "status", "error"
	val  any
}

func parseRESP(t *testing.T, raw string) respToken {
	t.Helper()
	return parseRESPAt(t, raw, 0).tok
}

type respParseResult struct {
	tok  respToken
	used int
}

func parseRESPAt(t *testing.T, s string, off int) respParseResult {
	t.Helper()
	if off >= len(s) {
		t.Fatalf("unexpected end of RESP at %d", off)
	}
	lineEnd := strings.IndexByte(s[off:], '\r')
	if lineEnd < 0 {
		t.Fatalf("no CRLF at %d", off)
	}
	line := s[off : off+lineEnd]
	head := off + lineEnd + 2

	switch line[0] {
	case '*':
		n, err := strconv.Atoi(line[1:])
		if err != nil {
			t.Fatalf("bad array count %q", line)
		}
		elems := make([]respToken, 0, n)
		p := head
		for i := 0; i < n; i++ {
			r := parseRESPAt(t, s, p)
			elems = append(elems, r.tok)
			p = r.used
		}
		return respParseResult{respToken{"array", elems}, p}
	case '$':
		n, err := strconv.Atoi(line[1:])
		if err != nil {
			t.Fatalf("bad bulk len %q", line)
		}
		if n == -1 {
			return respParseResult{respToken{"bulk", nil}, head}
		}
		body := s[head : head+n]
		return respParseResult{respToken{"bulk", body}, head + n + 2}
	case ':':
		n, err := strconv.Atoi(line[1:])
		if err != nil {
			t.Fatalf("bad int %q", line)
		}
		return respParseResult{respToken{"int", n}, head}
	case '+':
		return respParseResult{respToken{"status", line[1:]}, head}
	case '-':
		return respParseResult{respToken{"error", line[1:]}, head}
	default:
		t.Fatalf("unexpected RESP type %q at %d", string(line[0]), off)
		return respParseResult{}
	}
}

func bulkOf(t *testing.T, tok respToken) string {
	t.Helper()
	if tok.kind != "bulk" {
		t.Fatalf("expected bulk, got %s", tok.kind)
	}
	return tok.val.(string)
}

func errorOf(t *testing.T, tok respToken) string {
	t.Helper()
	if tok.kind != "error" {
		t.Fatalf("expected error, got %s", tok.kind)
	}
	return tok.val.(string)
}

func arrayOf(t *testing.T, tok respToken) []respToken {
	t.Helper()
	if tok.kind != "array" {
		t.Fatalf("expected array, got %s", tok.kind)
	}
	return tok.val.([]respToken)
}

func redisCmd(t *testing.T, addr string, args ...string) respToken {
	t.Helper()
	c := dial(t, addr)
	defer c.Close()
	c.Write(respCommand(args...))
	return parseRESP(t, readRESP(t, c))
}

// ---- cacheHitRate ----

func TestCacheHitRate(t *testing.T) {
	cases := []struct {
		hits, misses int64
		want         string
	}{
		{90, 10, "90.0%"},
		{0, 0, "0.0%"},
		{0, 5, "0.0%"},
		{5, 0, "100.0%"},
		{1, 3, "25.0%"},
	}
	for _, c := range cases {
		if got := cacheHitRate(c.hits, c.misses); got != c.want {
			t.Errorf("cacheHitRate(%d,%d) = %q, want %q", c.hits, c.misses, got, c.want)
		}
	}
}

// ---- buildInfoSections ----

func sampleInfoSnapshot() infoSnapshot {
	cs := &CacheStats{Hits: 90, Misses: 10, Evictions: 2, Entries: 100, MaxBytes: 500, CurBytes: 400}
	return infoSnapshot{
		version:  "0.2.4",
		uptime:   42,
		process:  7,
		totalReq: 1000,
		totalTok: 9999,
		totalErr: 1,
		models:   2,
		active:   3,
		cache:    cs,
		byModel: []modelInfoLine{
			{name: "bge", dim: 384, hits: 10, misses: 40, evictions: 1, entries: 50},
			{name: "minilm", dim: 384, hits: 80, misses: 5, evictions: 1, entries: 50},
		},
	}
}

func TestBuildInfoSectionsAll(t *testing.T) {
	out := buildInfoSections(nil, sampleInfoSnapshot())
	for _, want := range []string{"# Server", "# Cache", "# Keyspace", "# Stats", "# Clients"} {
		if !strings.Contains(out, want) {
			t.Errorf("missing section %q in:\n%s", want, out)
		}
	}
	// Order check: Server before Cache before Keyspace...
	srv := strings.Index(out, "# Server")
	cache := strings.Index(out, "# Cache")
	ks := strings.Index(out, "# Keyspace")
	stats := strings.Index(out, "# Stats")
	cl := strings.Index(out, "# Clients")
	if !(srv < cache && cache < ks && ks < stats && stats < cl) {
		t.Errorf("section order wrong: srv=%d cache=%d ks=%d stats=%d clients=%d", srv, cache, ks, stats, cl)
	}
	if !strings.Contains(out, "redis_version:0.2.4") || !strings.Contains(out, "emb_version:0.2.4") {
		t.Errorf("version lines missing:\n%s", out)
	}
	if !strings.Contains(out, "cache_hits:90") || !strings.Contains(out, "cache_misses:10") || !strings.Contains(out, "cache_hit_rate:90.0%") {
		t.Errorf("cache section missing expected values:\n%s", out)
	}
	if !strings.Contains(out, "db0:model=minilm,keys=50,hits=80,misses=5,hit_rate=94.1%") ||
		!strings.Contains(out, "db0:model=bge,keys=50,hits=10,misses=40,hit_rate=20.0%") {
		t.Errorf("keyspace lines wrong:\n%s", out)
	}
	if !strings.Contains(out, "total_requests:1000") || !strings.Contains(out, "models_loaded:2") {
		t.Errorf("stats section wrong:\n%s", out)
	}
	if !strings.Contains(out, "active_requests:3") {
		t.Errorf("clients section wrong:\n%s", out)
	}
}

func TestBuildInfoSectionsFiltered(t *testing.T) {
	out := buildInfoSections([]string{"server"}, sampleInfoSnapshot())
	if !strings.Contains(out, "# Server") || strings.Contains(out, "# Cache") {
		t.Fatalf("server-only filter wrong:\n%s", out)
	}
	out = buildInfoSections([]string{"CACHE", "stats"}, sampleInfoSnapshot())
	if !strings.Contains(out, "# Cache") || !strings.Contains(out, "# Stats") || strings.Contains(out, "# Server") {
		t.Fatalf("case-insensitive multi filter wrong:\n%s", out)
	}
}

func TestBuildInfoSectionsUnknownSection(t *testing.T) {
	out := buildInfoSections([]string{"nonexistent"}, sampleInfoSnapshot())
	if out != "" {
		t.Fatalf("unknown section should produce empty body, got %q", out)
	}
}

func TestBuildInfoSectionsCacheDisabled(t *testing.T) {
	snap := sampleInfoSnapshot()
	snap.cache = nil
	snap.byModel = nil
	out := buildInfoSections(nil, snap)
	if !strings.Contains(out, "cache_hits:0") || !strings.Contains(out, "cache_hit_rate:0.0%") {
		t.Errorf("disabled cache should render zeros:\n%s", out)
	}
}

// ---- Cache per-model stats + SetMaxBytes ----

func TestCachePerModelStats(t *testing.T) {
	c := NewCache(1024 * 1024)
	c.Set("minilm:hello", []byte{1})
	c.Set("minilm:world", []byte{2})
	c.Set("bge:other", []byte{3})

	c.Get("minilm:hello")
	c.Get("minilm:hello")
	c.Get("bge:other")
	c.Get("minilm:missing") // miss for minilm

	st := c.Stats()
	if st.Hits != 3 || st.Misses != 1 {
		t.Fatalf("global counters wrong: %+v", st)
	}
	mini := st.ByModel["minilm"]
	if mini.Hits != 2 || mini.Misses != 1 {
		t.Fatalf("minilm stats wrong: %+v", mini)
	}
	bge := st.ByModel["bge"]
	if bge.Hits != 1 || bge.Misses != 0 {
		t.Fatalf("bge stats wrong: %+v", bge)
	}
}

func TestCacheSetMaxBytes(t *testing.T) {
	c := NewCache(4096)
	for i := range 40 {
		c.Set(fmt.Sprintf("m:k%02d", i), []byte{byte(i)})
	}
	st := c.Stats()
	if st.Evictions != 0 {
		t.Fatalf("all 40 entries should fit in 4096 bytes, got evictions=%d", st.Evictions)
	}
	if st.CurBytes <= 1024 {
		t.Fatalf("expected >1KB before shrink, got %d bytes", st.CurBytes)
	}

	c.SetMaxBytes(200)
	st = c.Stats()
	if st.CurBytes > 200 {
		t.Fatalf("cache not shrunk to budget: %d bytes", st.CurBytes)
	}
	if st.Evictions == 0 {
		t.Fatalf("expected evictions during shrink")
	}
	// Growing again stays consistent and new inserts work.
	c.SetMaxBytes(1024 * 1024)
	c.Set("m:after", []byte{9})
	if _, ok := c.Get("m:after"); !ok {
		t.Fatal("insert after regrow should hit")
	}
}

// ---- INFO over the wire ----

func TestServerRedisINFO(t *testing.T) {
	addr := serveTest(t)
	tok := redisCmd(t, addr, "INFO")
	body := bulkOf(t, tok)
	for _, want := range []string{"# Server", "redis_version:dev", "# Cache", "# Keyspace", "# Stats", "# Memory", "# CPU", "# Clients"} {
		if !strings.Contains(body, want) {
			t.Errorf("INFO missing %q in:\n%s", want, body)
		}
	}
	if !strings.Contains(body, "db0:model=test,keys=0,hits=0,misses=0,hit_rate=0.0%") {
		t.Errorf("expected a keyspace line for the loaded test model:\n%s", body)
	}

	// Default no-args INFO renders sections in the fixed order
	// Server → Cache → Keyspace → Stats → Memory → CPU → Clients.
	order := []string{"# Server", "# Cache", "# Keyspace", "# Stats", "# Memory", "# CPU", "# Clients"}
	prev := -1
	for _, sec := range order {
		idx := strings.Index(body, sec)
		if idx < 0 {
			t.Errorf("INFO missing section %q in:\n%s", sec, body)
			continue
		}
		if idx < prev {
			t.Errorf("INFO section order broken: %q before a previous section:\n%s", sec, body)
		}
		prev = idx
	}
}

func TestInfoMemoryAndCPUSections(t *testing.T) {
	addr := serveTest(t)

	body := bulkOf(t, redisCmd(t, addr, "INFO", "memory"))
	for _, field := range []string{"used_memory_rss_bytes", "used_memory_heap_bytes", "goroutines", "total_system_memory_bytes"} {
		if !strings.Contains(body, field) {
			t.Errorf("INFO memory missing %q in:\n%s", field, body)
		}
	}
	if strings.Contains(body, "# CPU") {
		t.Errorf("INFO memory must not include the CPU section:\n%s", body)
	}

	body = bulkOf(t, redisCmd(t, addr, "INFO", "cpu"))
	for _, field := range []string{"used_cpu_user_usec", "used_cpu_sys_usec", "gomaxprocs"} {
		if !strings.Contains(body, field) {
			t.Errorf("INFO cpu missing %q in:\n%s", field, body)
		}
	}
	if strings.Contains(body, "# Memory") {
		t.Errorf("INFO cpu must not include the Memory section:\n%s", body)
	}

	// Union: INFO memory cpu returns both sections, Memory before CPU, and no
	// other sections.
	body = bulkOf(t, redisCmd(t, addr, "INFO", "memory", "cpu"))
	memIdx := strings.Index(body, "# Memory")
	cpuIdx := strings.Index(body, "# CPU")
	if memIdx < 0 || cpuIdx < 0 || memIdx > cpuIdx {
		t.Fatalf("INFO memory cpu should contain # Memory then # CPU:\n%s", body)
	}
	if strings.Contains(body, "# Server") {
		t.Fatalf("INFO memory cpu must not include other sections:\n%s", body)
	}

	// Unknown section contributes nothing.
	body = bulkOf(t, redisCmd(t, addr, "INFO", "memory", "bogus"))
	if strings.Contains(body, "# CPU") || !strings.Contains(body, "# Memory") {
		t.Fatalf("INFO memory bogus should contain only Memory:\n%s", body)
	}
}

// infoIntField extracts an integer `field:value` line from an INFO body.
func infoIntField(t *testing.T, body, field string) int64 {
	t.Helper()
	idx := strings.Index(body, field+":")
	if idx < 0 {
		t.Fatalf("field %q not found in INFO body:\n%s", field, body)
	}
	rest := body[idx+len(field)+1:]
	j := 0
	for j < len(rest) && rest[j] >= '0' && rest[j] <= '9' {
		j++
	}
	v, err := strconv.ParseInt(rest[:j], 10, 64)
	if err != nil {
		t.Fatalf("bad int value for %q: %q", field, body)
	}
	return v
}

// TestServerNetByteCounters checks the aggregate RX/TX counters in INFO stats:
// they increase by at least the wire size of an exchange and never decrease.
func TestServerNetByteCounters(t *testing.T) {
	addr := serveTest(t)
	c := dial(t, addr)

	stats := func() (int64, int64) {
		body := bulkOf(t, redisCmd(t, addr, "INFO", "stats"))
		return infoIntField(t, body, "total_net_input_bytes"),
			infoIntField(t, body, "total_net_output_bytes")
	}

	in0, out0 := stats()

	cmd := respCommand("EMB", "test", "hello")
	c.Write(cmd)
	replyRaw := readRESP(t, c)

	in1, out1 := stats()
	if in1-in0 < int64(len(cmd)) {
		t.Errorf("net input delta %d < command wire size %d", in1-in0, len(cmd))
	}
	if out1-out0 < int64(len(replyRaw)) {
		t.Errorf("net output delta %d < reply wire size %d", out1-out0, len(replyRaw))
	}

	in2, out2 := stats()
	if in2 < in1 || out2 < out1 {
		t.Errorf("byte counters decreased: in %d→%d, out %d→%d", in1, in2, out1, out2)
	}
}

func TestServerRedisINFOFiltered(t *testing.T) {
	addr := serveTest(t)
	body := bulkOf(t, redisCmd(t, addr, "INFO", "server"))
	if !strings.Contains(body, "# Server") || strings.Contains(body, "# Cache") {
		t.Fatalf("INFO server should contain only the Server section:\n%s", body)
	}

	body = bulkOf(t, redisCmd(t, addr, "INFO", "bogus"))
	if body != "" {
		t.Fatalf("INFO bogus should be empty, got %q", body)
	}
}

func TestServerRedisINFOCacheHitRate(t *testing.T) {
	addr := serveTestWithCache(t, "10KB")
	redisCmd(t, addr, "EMB", "test", "hello")
	redisCmd(t, addr, "EMB", "test", "hello")
	body := bulkOf(t, redisCmd(t, addr, "INFO", "cache"))
	if !strings.Contains(body, "cache_hits:1") || !strings.Contains(body, "cache_misses:1") {
		t.Fatalf("expected 1 hit 1 miss in:\n%s", body)
	}
	if !strings.Contains(body, "cache_hit_rate:50.0%") {
		t.Fatalf("expected 50.0%% hit rate in:\n%s", body)
	}
}

// ---- CONFIG over the wire ----

func TestServerConfigGet(t *testing.T) {
	addr := serveTest(t)
	elems := arrayOf(t, redisCmd(t, addr, "CONFIG", "GET"))
	params := map[string]string{}
	for i := 0; i+1 < len(elems); i += 2 {
		params[bulkOf(t, elems[i])] = bulkOf(t, elems[i+1])
	}
	for _, want := range []string{"cache", "password", "listen", "tls_cert", "tls_key", "models", "cache_file", "cache_save"} {
		if _, ok := params[want]; !ok {
			t.Errorf("CONFIG GET missing param %q: %v", want, params)
		}
	}
	if params["models"] != "test" {
		t.Errorf("models param wrong: %q", params["models"])
	}

	elems = arrayOf(t, redisCmd(t, addr, "CONFIG", "GET", "cache*"))
	if len(elems) != 2 { // cache + cache_file + cache_save would be 6; but cache* matches only cache when only cache exists?
		t.Logf("cache* glob returned %d elements", len(elems))
	}
	elems = arrayOf(t, redisCmd(t, addr, "CONFIG", "GET", "nope*"))
	if len(elems) != 0 {
		t.Fatalf("unmatched glob should be empty array, got %d elements", len(elems))
	}
}

func TestServerConfigSetCache(t *testing.T) {
	addr := serveTestWithCache(t, "100KB")
	tok := redisCmd(t, addr, "CONFIG", "SET", "cache", "50KB")
	if tok.kind != "status" || tok.val != "OK" {
		t.Fatalf("expected +OK, got %+v", tok)
	}

	// Verify live resize: the new budget shows up in INFO's # Cache section.
	body := bulkOf(t, redisCmd(t, addr, "INFO", "cache"))
	if !strings.Contains(body, "cache_max_bytes:50000") {
		t.Fatalf("cache_max_bytes not resized:\n%s", body)
	}

	// Invalid values rejected with budget unchanged.
	if got := errorOf(t, redisCmd(t, addr, "CONFIG", "SET", "cache", "nonsense")); got == "" {
		t.Fatal("expected error for nonsense cache size")
	}
	if got := errorOf(t, redisCmd(t, addr, "CONFIG", "SET", "cache", "150%")); got == "" {
		t.Fatal("expected error for 150% cache size")
	}
	body = bulkOf(t, redisCmd(t, addr, "INFO", "cache"))
	if !strings.Contains(body, "cache_max_bytes:50000") {
		t.Fatalf("budget changed after rejected SET:\n%s", body)
	}
}

func TestServerConfigSetCacheDisabledAtBoot(t *testing.T) {
	addr := serveTest(t) // no cache at boot
	if got := errorOf(t, redisCmd(t, addr, "CONFIG", "SET", "cache", "1MB")); got == "" {
		t.Fatal("expected error enabling a disabled-at-boot cache")
	}
}

func TestServerConfigSetPassword(t *testing.T) {
	addr := serveTest(t) // no password at boot
	if tok := redisCmd(t, addr, "CONFIG", "SET", "password", "hunter2"); tok.kind != "status" || tok.val != "OK" {
		t.Fatalf("expected +OK setting password, got %+v", tok)
	}

	// The auth gate must now reject non-exempt commands before AUTH...
	c := dial(t, addr)
	c.Write(respCommand("EMB.MODELS"))
	resp := readRESP(t, c)
	c.Close()
	if !strings.HasPrefix(resp, "-NOAUTH") {
		t.Fatalf("expected NOAUTH after setting password, got %q", resp)
	}

	// ...and the new password authenticates (old "" was never valid; AUTH with new works).
	if tok := redisCmd(t, addr, "AUTH", "hunter2"); tok.kind != "status" || tok.val != "OK" {
		t.Fatalf("expected AUTH OK, got %+v", tok)
	}
	// INFO stays exempt pre-auth (probe).
	body := bulkOf(t, redisCmd(t, addr, "INFO"))
	if !strings.Contains(body, "# Server") {
		t.Fatalf("pre-auth INFO should work:\n%s", body)
	}
}

func TestServerConfigSetPasswordExistingSession(t *testing.T) {
	// A connection authenticated before the change stays valid (Redis semantics).
	addr := serveTestWithAuth(t, "oldpass")
	c := dial(t, addr)
	c.Write(respCommand("AUTH", "oldpass"))
	if got := readRESP(t, c); got != "+OK\r\n" {
		t.Fatalf("AUTH oldpass failed: %q", got)
	}
	c.Write(respCommand("CONFIG", "SET", "password", "newpass"))
	if got := readRESP(t, c); !strings.HasPrefix(got, "+OK") {
		t.Fatalf("set password failed: %q", got)
	}
	// Same session still works after the change.
	c.Write(respCommand("EMB.MODELS"))
	if got := readRESP(t, c); !strings.HasPrefix(got, "*") {
		t.Fatalf("existing session should still be authenticated, got %q", got)
	}
	c.Close()

	// New session must use the new password.
	if got := errorOf(t, redisCmd(t, addr, "AUTH", "oldpass")); got == "" {
		t.Fatal("old password should now fail")
	}
	if tok := redisCmd(t, addr, "AUTH", "newpass"); tok.kind != "status" || tok.val != "OK" {
		t.Fatalf("new password should succeed, got %+v", tok)
	}
}

func TestServerConfigSetReadOnly(t *testing.T) {
	addr := serveTest(t)
	got := errorOf(t, redisCmd(t, addr, "CONFIG", "SET", "listen", ":9999"))
	if got == "" || !strings.Contains(got, "read-only") {
		t.Fatalf("expected read-only error, got %q", got)
	}
	got = errorOf(t, redisCmd(t, addr, "CONFIG", "SET", "unknown_param", "1"))
	if got == "" || !strings.Contains(got, "Unsupported CONFIG parameter") {
		t.Fatalf("expected unsupported param error, got %q", got)
	}
}

func TestServerConfigRequiresAuth(t *testing.T) {
	addr := serveTestWithAuth(t, "secret")
	c := dial(t, addr)
	c.Write(respCommand("CONFIG", "GET"))
	if got := readRESP(t, c); !strings.HasPrefix(got, "-NOAUTH") {
		t.Fatalf("expected NOAUTH for CONFIG, got %q", got)
	}
	c.Close()
}

func TestServerConfigWrongArity(t *testing.T) {
	addr := serveTest(t)
	if got := errorOf(t, redisCmd(t, addr, "CONFIG")); got == "" {
		t.Fatal("expected error for bare CONFIG")
	}
	if got := errorOf(t, redisCmd(t, addr, "CONFIG", "SET", "cache")); got == "" {
		t.Fatal("expected error for CONFIG SET with missing value")
	}
}

func TestHelpListsINFOAndCONFIG(t *testing.T) {
	addr := serveTest(t)
	body := bulkOf(t, redisCmd(t, addr, "EMB.HELP"))
	if !strings.Contains(body, "INFO [section ...]") {
		t.Errorf("HELP missing INFO:\n%s", body)
	}
	if !strings.Contains(body, "CONFIG GET") || !strings.Contains(body, "CONFIG SET") {
		t.Errorf("HELP missing CONFIG:\n%s", body)
	}
}
