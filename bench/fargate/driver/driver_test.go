package driver

import (
	"bufio"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"math"
	"net"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/elcuervo/emb/internal/resp"
)

const testDim = 4

// fakeServer speaks just enough RESP for the driver: it parses each command
// and answers with whatever the handler returns; a nil answer stalls the
// connection until the test ends, which is how a hung reply is simulated.
type fakeServer struct {
	addr string
	stop chan struct{}
}

func newFakeServer(t *testing.T, handle func(args []string) []byte) *fakeServer {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	fs := &fakeServer{addr: ln.Addr().String(), stop: make(chan struct{})}
	t.Cleanup(func() { close(fs.stop); _ = ln.Close() })
	go func() {
		for {
			conn, err := ln.Accept()
			if err != nil {
				return
			}
			go func() {
				defer func() { _ = conn.Close() }()
				r := bufio.NewReader(conn)
				for {
					args, err := readCommand(r)
					if err != nil {
						return
					}
					out := handle(args)
					if out == nil {
						<-fs.stop
						return
					}
					if _, err := conn.Write(out); err != nil {
						return
					}
				}
			}()
		}
	}()
	return fs
}

func readCommand(r *bufio.Reader) ([]string, error) {
	line, err := r.ReadString('\n')
	if err != nil {
		return nil, err
	}
	n, err := strconv.Atoi(strings.TrimSpace(line[1:]))
	if err != nil {
		return nil, err
	}
	args := make([]string, n)
	for i := range args {
		hdr, err := r.ReadString('\n')
		if err != nil {
			return nil, err
		}
		size, err := strconv.Atoi(strings.TrimSpace(hdr[1:]))
		if err != nil {
			return nil, err
		}
		buf := make([]byte, size+2)
		if _, err := io.ReadFull(r, buf); err != nil {
			return nil, err
		}
		args[i] = string(buf[:size])
	}
	return args, nil
}

func blob(dim int, v float32) string {
	b := make([]byte, dim*4)
	for i := range dim {
		binary.LittleEndian.PutUint32(b[i*4:], math.Float32bits(v))
	}
	return string(b)
}

func bulk(s string) []byte { return []byte(fmt.Sprintf("$%d\r\n%s\r\n", len(s), s)) }

func testConfig(addr string) Config {
	return Config{
		Addr: addr, Models: []string{"minilm"}, Dispatch: Single,
		Rate: 40, Duration: 250 * time.Millisecond, MaxInFlight: 4,
		RequestTimeout: 200 * time.Millisecond, DrainTimeout: 300 * time.Millisecond,
		Dim: testDim, Texts: func(i int) string { return "text " + strconv.Itoa(i) },
	}
}

func TestJudgeClassifiesEveryReplyShape(t *testing.T) {
	good := resp.Reply{Type: '$', Str: blob(testDim, 0.5)}
	nan := resp.Reply{Type: '$', Str: blob(testDim, float32(math.NaN()))}
	inf := resp.Reply{Type: '$', Str: blob(testDim, float32(math.Inf(1)))}
	cases := []struct {
		name string
		rep  resp.Reply
		err  error
		out  Outcome
		kind string
	}{
		{"valid", good, nil, OutcomeSuccess, ""},
		{"-ERR reply", resp.Reply{Type: '-', Str: "ERR model exploded"}, nil, OutcomeError, KindServer},
		{"busy rejection", resp.Reply{Type: '-', Str: "ERR busy: CPU admission timeout"}, nil, OutcomeError, KindBusy},
		{"null bulk", resp.Reply{Type: '$', Nil: true}, nil, OutcomeError, KindNull},
		{"null array", resp.Reply{Type: '*', Nil: true}, nil, OutcomeError, KindNull},
		{"short blob", resp.Reply{Type: '$', Str: blob(testDim-1, 0.5)}, nil, OutcomeError, KindMalformed},
		{"long blob", resp.Reply{Type: '$', Str: blob(testDim+1, 0.5)}, nil, OutcomeError, KindMalformed},
		{"status instead of blob", resp.Reply{Type: '+', Str: "OK"}, nil, OutcomeError, KindMalformed},
		{"NaN", nan, nil, OutcomeError, KindNonFinite},
		{"Inf", inf, nil, OutcomeError, KindNonFinite},
		{"decode failure", resp.Reply{}, errors.New("resp: unexpected prefix 'x'"), OutcomeError, KindMalformed},
		{"EOF", resp.Reply{}, io.EOF, OutcomeError, KindTransport},
		{"deadline", resp.Reply{}, &net.OpError{Op: "read", Err: timeoutErr{}}, OutcomeTimeout, KindTimeout},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			out, kind := Judge(tc.rep, tc.err, testDim)
			if out != tc.out || kind != tc.kind {
				t.Fatalf("Judge = (%s, %q), want (%s, %q)", out, kind, tc.out, tc.kind)
			}
		})
	}
}

type timeoutErr struct{}

func (timeoutErr) Error() string   { return "i/o timeout" }
func (timeoutErr) Timeout() bool   { return true }
func (timeoutErr) Temporary() bool { return true }

func TestRunCountsApplicationErrorsAsErrorsNotSuccess(t *testing.T) {
	fs := newFakeServer(t, func([]string) []byte { return []byte("-ERR model exploded\r\n") })
	res, err := Run(testConfig(fs.addr))
	if err != nil {
		t.Fatal(err)
	}
	if res.Success != 0 || res.Errors != res.Offered || res.ErrorKinds[KindServer] != res.Offered {
		t.Fatalf("every -ERR reply must be an error, got %+v", res)
	}
	if res.Throughput != 0 {
		t.Fatalf("throughput = %v, want 0 when nothing succeeded", res.Throughput)
	}
}

func TestRunStalledReplyIsATimeoutAndTheDrainIsBounded(t *testing.T) {
	var mu sync.Mutex
	seen := 0
	fs := newFakeServer(t, func([]string) []byte {
		mu.Lock()
		seen++
		stall := seen == 3
		mu.Unlock()
		if stall {
			return nil
		}
		return bulk(blob(testDim, 1))
	})
	cfg := testConfig(fs.addr)
	start := time.Now()
	res, err := Run(cfg)
	if err != nil {
		t.Fatal(err)
	}
	if res.Timeouts != 1 || res.Success != res.Offered-1 || res.Errors != 0 || res.Dropped != 0 {
		t.Fatalf("one stalled reply must be exactly one timeout, got %+v", res)
	}
	if bound := cfg.Duration + cfg.DrainTimeout + 300*time.Millisecond; time.Since(start) > bound {
		t.Fatalf("run took %v, want within %v", time.Since(start), bound)
	}
}

func TestRunDrainTimeoutSealsStillRunningPairs(t *testing.T) {
	fs := newFakeServer(t, func([]string) []byte { return nil })
	cfg := testConfig(fs.addr)
	cfg.RequestTimeout = time.Minute // only the hard stop can end these pairs
	cfg.Rate, cfg.Duration, cfg.DrainTimeout = 20, 200*time.Millisecond, 100*time.Millisecond
	start := time.Now()
	res, err := Run(cfg)
	if err != nil {
		t.Fatal(err)
	}
	if res.Success != 0 || res.Timeouts+res.Dropped != res.Offered {
		t.Fatalf("hung pairs must be timeouts or drops, got %+v", res)
	}
	if time.Since(start) > cfg.Duration+cfg.DrainTimeout+300*time.Millisecond {
		t.Fatalf("drain not bounded: %v", time.Since(start))
	}
}

func TestRunDropsWhenNoSlotIsFreeAndStillAccounts(t *testing.T) {
	fs := newFakeServer(t, func([]string) []byte {
		time.Sleep(120 * time.Millisecond)
		return bulk(blob(testDim, 1))
	})
	cfg := testConfig(fs.addr)
	cfg.MaxInFlight = 1
	res, err := Run(cfg)
	if err != nil {
		t.Fatal(err)
	}
	if res.Dropped == 0 || res.Success == 0 {
		t.Fatalf("want drops and successes with one slot, got %+v", res)
	}
	if err := res.Accounted(); err != nil {
		t.Fatal(err)
	}
}

func TestRunReportsLatencyFromScheduledAndWireStart(t *testing.T) {
	fs := newFakeServer(t, func([]string) []byte { return bulk(blob(testDim, 1)) })
	res, err := Run(testConfig(fs.addr))
	if err != nil {
		t.Fatal(err)
	}
	if res.Scheduled.Count == 0 || res.Wire.Count != res.Scheduled.Count {
		t.Fatalf("both latency views must cover the same successful pairs: %+v / %+v", res.Scheduled, res.Wire)
	}
	if res.Scheduled.P50 < res.Wire.P50 {
		t.Fatalf("latency from the schedule (%v) cannot be below wire latency (%v)", res.Scheduled.P50, res.Wire.P50)
	}
	if res.Throughput <= 0 || math.Abs(res.Throughput-float64(res.Success)/res.Seconds) > 1e-9 {
		t.Fatalf("throughput must be successes over wall seconds: %+v", res)
	}
}

func TestRunParallelPairFailsWhenOneLegFails(t *testing.T) {
	fs := newFakeServer(t, func(args []string) []byte {
		if args[1] == "bge" {
			return []byte("-ERR no bge\r\n")
		}
		return bulk(blob(testDim, 1))
	})
	cfg := testConfig(fs.addr)
	cfg.Models, cfg.Dispatch = []string{"minilm", "bge"}, Parallel
	res, err := Run(cfg)
	if err != nil {
		t.Fatal(err)
	}
	if res.Success != 0 || res.Errors != res.Offered {
		t.Fatalf("a pair with a failed leg is an error pair: %+v", res)
	}
	if res.Legs["minilm"].Success == 0 || res.Legs["bge"].Errors == 0 {
		t.Fatalf("per-leg outcomes must stay visible: %+v", res.Legs)
	}
}

func TestSummarizeP90IsLiteral(t *testing.T) {
	var ds []time.Duration
	for i := 1; i <= 10; i++ {
		ds = append(ds, time.Duration(i)*time.Millisecond)
	}
	q := Summarize(ds)
	if q.P50 != 5 || q.P90 != 9 || q.P95 != 10 || q.P99 != 10 {
		t.Fatalf("nearest-rank quantiles = %+v", q)
	}
	if q := Summarize([]time.Duration{time.Millisecond, 2 * time.Millisecond, 3 * time.Millisecond}); q.P90 != 3 {
		t.Fatalf("p90 of three samples = %v, want the largest", q.P90)
	}
}

func TestCorpusCyclesClassesPerRequestAndKeepsColdUnique(t *testing.T) {
	c := NewCorpus(7, 9, "n1")
	for i := range 12 {
		want := classWords[i%3]
		if got := len(strings.Fields(c.Text(CacheWarm, i))); got != want {
			t.Fatalf("warm request %d has %d words, want %d (class cycles per request)", i, got, want)
		}
	}
	seen := map[string]bool{}
	for i := range 300 {
		txt := c.Text(CacheCold, i)
		if seen[txt] {
			t.Fatalf("cold text %d repeated", i)
		}
		seen[txt] = true
	}
	for _, w := range c.Warm() {
		if seen[w] {
			t.Fatal("a cold text collided with the warm corpus")
		}
	}
	if NewCorpus(7, 9, "n1").Digest() != c.Digest() || NewCorpus(8, 9, "n1").Digest() == c.Digest() {
		t.Fatal("corpus digest must follow the seed and be stable")
	}
}

// cacheServer is a fake emb with a real reply cache: EMB hits or misses by
// text, EMB.STATS reports the counters, EMB.INFO the per-model request count.
func cacheServer(t *testing.T) (*fakeServer, func(string)) {
	t.Helper()
	var mu sync.Mutex
	cache := map[string]bool{}
	var hits, misses, requests int
	fs := newFakeServer(t, func(args []string) []byte {
		mu.Lock()
		defer mu.Unlock()
		switch args[0] {
		case "EMB":
			requests++
			key := args[1] + "|" + args[2]
			if cache[key] {
				hits++
			} else {
				misses++
				cache[key] = true
			}
			return bulk(blob(testDim, 1))
		case "EMB.STATS":
			return []byte(fmt.Sprintf("*4\r\n$10\r\ncache_hits\r\n:%d\r\n$12\r\ncache_misses\r\n:%d\r\n", hits, misses))
		case "EMB.INFO":
			return []byte(fmt.Sprintf("*2\r\n$8\r\nrequests\r\n:%d\r\n", requests))
		}
		return []byte("-ERR unknown\r\n")
	})
	seed := func(text string) {
		mu.Lock()
		cache["minilm|"+text] = true
		mu.Unlock()
	}
	return fs, seed
}

func phaseConfig(addr, cache string) PhaseConfig {
	return PhaseConfig{Config: testConfig(addr), Cache: cache, Corpus: NewCorpus(1, 3, "run"), Info: true}
}

func TestPhaseWarmHasZeroMissesAndColdHasOnlyMisses(t *testing.T) {
	fs, _ := cacheServer(t)
	for _, cache := range []string{CacheWarm, CacheCold} {
		pr, err := RunPhase(phaseConfig(fs.addr, cache))
		if err != nil {
			t.Fatalf("%s: %v", cache, err)
		}
		if err := pr.CheckCache(); err != nil {
			t.Fatalf("%s: %v", cache, err)
		}
		if got := pr.Counters.Models["minilm"]["requests"]; got != pr.Result.Success {
			t.Fatalf("%s: server inference counter delta %d, want %d successful requests", cache, got, pr.Result.Success)
		}
	}
}

func TestCheckCacheRejectsAColdPhaseThatHitsAndAWarmOneThatMisses(t *testing.T) {
	fs, seed := cacheServer(t)
	cold := phaseConfig(fs.addr, CacheCold)
	// Pre-seed the exact cold texts the pass will send: the cache answers them.
	probe := NewCorpus(1, 3, "run")
	for i := range 20 {
		seed(probe.Text(CacheCold, i))
	}
	pr, err := RunPhase(cold)
	if err != nil {
		t.Fatal(err)
	}
	if pr.CheckCache() == nil {
		t.Fatal("a cold phase answered from the cache must fail the cache check")
	}

	pr.Cache = CacheWarm
	pr.Counters.Stats = map[string]int64{"cache_hits": 0, "cache_misses": 5}
	if pr.CheckCache() == nil {
		t.Fatal("a warm phase with misses must fail the cache check")
	}
	pr.Counters.Stats = nil
	if pr.CheckCache() == nil {
		t.Fatal("missing cache counters must fail the cache check, not pass it")
	}
}

func TestPrewarmFailsOnABadReply(t *testing.T) {
	fs := newFakeServer(t, func([]string) []byte { return []byte("$-1\r\n") })
	if err := Prewarm(testConfig(fs.addr), NewCorpus(1, 3, "x")); err == nil {
		t.Fatal("prewarm against a server returning nulls must fail")
	}
}

func TestRunMultiDispatchValidatesEveryElement(t *testing.T) {
	array := func(elems ...[]byte) []byte {
		out := []byte(fmt.Sprintf("*%d\r\n", len(elems)))
		for _, e := range elems {
			out = append(out, e...)
		}
		return out
	}
	var mu sync.Mutex
	bad := false
	fs := newFakeServer(t, func(args []string) []byte {
		if args[0] != "EMB.MULTI" || len(args) != 5 {
			return []byte("-ERR unexpected command\r\n")
		}
		mu.Lock()
		defer mu.Unlock()
		if bad {
			return array(bulk(blob(testDim, 1)), []byte("$-1\r\n"))
		}
		return array(bulk(blob(testDim, 1)), bulk(blob(testDim, 2)))
	})
	cfg := testConfig(fs.addr)
	cfg.Models, cfg.Dispatch = []string{"minilm", "bge"}, Multi
	res, err := Run(cfg)
	if err != nil {
		t.Fatal(err)
	}
	if res.Success != res.Offered || res.Legs["minilm"].Success != res.Offered || res.Legs["bge"].Success != res.Offered {
		t.Fatalf("valid EMB.MULTI replies must succeed per leg: %+v", res)
	}

	mu.Lock()
	bad = true
	mu.Unlock()
	res, err = Run(cfg)
	if err != nil {
		t.Fatal(err)
	}
	if res.Success != 0 || res.ErrorKinds[KindNull] != res.Offered || res.Legs["minilm"].Success != res.Offered {
		t.Fatalf("a null element must fail only its leg and the pair: %+v", res)
	}
}
