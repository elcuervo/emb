// Command evalbench is a RESP load client for scripted inference. It sends
// EMB.EVAL for each corpus text at a list of concurrency levels and reports
// p50/p90/p99 latency and req/s, optionally dumping every reply so two builds
// can be compared for byte/value parity.
//
// The request set is deterministic: request i evaluates corpus index
// (i*batch + j) % len(corpus), so two runs against the same corpus produce the
// same replies regardless of concurrency.
package main

import (
	"encoding/base64"
	"flag"
	"fmt"
	"os"
	"sort"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/elcuervo/emb/internal/resp"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintf(os.Stderr, "evalbench: %v\n", err)
		os.Exit(1)
	}
}

type stringList []string

func (s *stringList) String() string     { return strings.Join(*s, " ") }
func (s *stringList) Set(v string) error { *s = append(*s, v); return nil }

type options struct {
	addr        string
	password    string
	model       string
	scriptPath  string
	corpusPath  string
	args        stringList
	concurrency string
	n           int
	batch       int
	dumpPath    string
	timeout     time.Duration
	mode        string
	shape       string
	shapePhase  time.Duration
	shapeBurst  int
}

func run() error {
	opt := options{}
	flag.StringVar(&opt.addr, "addr", "127.0.0.1:6379", "emb server address host:port")
	flag.StringVar(&opt.password, "password", "", "server password (AUTH)")
	flag.StringVar(&opt.model, "model", "", "model name (required)")
	flag.StringVar(&opt.scriptPath, "script", "", "Lua script file (required)")
	flag.StringVar(&opt.corpusPath, "corpus", "", "corpus file, one text per line (required)")
	flag.Var(&opt.args, "arg", "script ARGV entry (repeatable)")
	flag.StringVar(&opt.concurrency, "concurrency", "1,4,8,16", "comma-separated concurrency levels")
	flag.IntVar(&opt.n, "n", 200, "requests per concurrency level")
	flag.IntVar(&opt.batch, "batch", 1, "texts per request")
	flag.StringVar(&opt.dumpPath, "dump", "", "write one base64 reply per request (request order)")
	flag.StringVar(&opt.mode, "mode", "eval", "eval (EMB.EVAL script) or emb (EMB embedding)")
	flag.StringVar(&opt.shape, "shape", "", "traffic-shape run: serial, burst, or mixed (overrides -concurrency/-n)")
	flag.DurationVar(&opt.shapePhase, "shape-phase", 3*time.Second, "duration of each shape phase")
	flag.IntVar(&opt.shapeBurst, "shape-burst", 16, "burst concurrency for the burst and mixed shapes")
	flag.DurationVar(&opt.timeout, "timeout", 30*time.Second, "per-request timeout")
	flag.Parse()

	if opt.model == "" || opt.corpusPath == "" {
		flag.Usage()
		return fmt.Errorf("-model and -corpus are required")
	}
	if opt.mode != "eval" && opt.mode != "emb" {
		return fmt.Errorf("-mode must be eval or emb, got %q", opt.mode)
	}
	if opt.mode == "eval" && opt.scriptPath == "" {
		return fmt.Errorf("-script is required in eval mode")
	}
	if opt.mode == "emb" && opt.batch != 1 {
		return fmt.Errorf("-batch must be 1 in emb mode")
	}
	if opt.shape != "" && opt.shape != "serial" && opt.shape != "burst" && opt.shape != "mixed" {
		return fmt.Errorf("-shape must be serial, burst, or mixed, got %q", opt.shape)
	}
	if opt.n < 1 {
		return fmt.Errorf("-n must be positive")
	}
	if opt.batch < 1 {
		return fmt.Errorf("-batch must be positive")
	}
	script, err := os.ReadFile(opt.scriptPath)
	if err != nil && opt.mode == "eval" {
		return fmt.Errorf("reading script: %w", err)
	}
	corpus, err := readCorpus(opt.corpusPath)
	if err != nil {
		return err
	}
	if len(corpus) == 0 {
		return fmt.Errorf("corpus %q is empty", opt.corpusPath)
	}

	levels, err := parseLevels(opt.concurrency)
	if err != nil {
		return err
	}

	if opt.shape != "" {
		return runShape(opt, string(script), corpus)
	}

	var dump *os.File
	if opt.dumpPath != "" {
		dump, err = os.Create(opt.dumpPath)
		if err != nil {
			return fmt.Errorf("creating dump: %w", err)
		}
		defer func() { _ = dump.Close() }()
	}

	for _, c := range levels {
		res, err := runLevel(opt, string(script), corpus, c)
		if err != nil {
			return fmt.Errorf("concurrency %d: %w", c, err)
		}
		fmt.Printf("concurrency=%d n=%d req/s=%.1f p50=%.2fms p90=%.2fms p99=%.2fms errors=%d\n",
			c, res.n, res.reqPerSec, ms(res.p50), ms(res.p90), ms(res.p99), res.errors)
		if dump != nil {
			for i, reply := range res.replies {
				if _, err := fmt.Fprintf(dump, "%d %s\n", i, base64.StdEncoding.EncodeToString([]byte(reply))); err != nil {
					return err
				}
			}
		}
	}
	return nil
}

// requestArgv builds the command for one request. In emb mode it is a single
// EMB; otherwise an EMB.EVAL with the script and ARGV.
func requestArgv(opt options, script string, texts []string) []string {
	if opt.mode == "emb" {
		return []string{"EMB", opt.model, texts[0]}
	}
	argv := make([]string, 0, 4+len(texts)+len(opt.args))
	argv = append(argv, "EMB.EVAL", opt.model, script, strconv.Itoa(len(texts)))
	argv = append(argv, texts...)
	return append(argv, opt.args...)
}

// shapePhase is one traffic phase: a concurrency level run for a duration.
type shapePhase struct {
	name string
	conc int
}

func shapePhases(shape string, burst int) []shapePhase {
	switch shape {
	case "serial":
		return []shapePhase{{"serial", 1}}
	case "burst":
		return []shapePhase{{"burst", burst}}
	default: // mixed
		return []shapePhase{{"serial", 1}, {"burst", burst}, {"serial", 1}}
	}
}

// runShape drives fixed serial/burst phases and prints the model's classified
// traffic class and concurrency allowance at the end of each, proving the
// autotuner reacts to the traffic shape.
func runShape(opt options, script string, corpus []string) error {
	if len(corpus) == 0 {
		return fmt.Errorf("corpus is empty")
	}
	info := resp.NewClient(opt.addr, opt.password, false)
	info.SetTimeout(opt.timeout)
	if err := info.Dial(); err != nil {
		return fmt.Errorf("opening INFO connection: %w", err)
	}
	defer func() { _ = info.Close() }()

	for _, ph := range shapePhases(opt.shape, opt.shapeBurst) {
		var next atomic.Int64
		var firstErr atomic.Pointer[string]
		var done atomic.Int64
		deadline := time.Now().Add(opt.shapePhase)
		var wg sync.WaitGroup
		for w := 0; w < ph.conc; w++ {
			wg.Add(1)
			go func() {
				defer wg.Done()
				client := resp.NewClient(opt.addr, opt.password, false)
				client.SetTimeout(opt.timeout)
				if err := client.Dial(); err != nil {
					s := err.Error()
					firstErr.CompareAndSwap(nil, &s)
					return
				}
				defer func() { _ = client.Close() }()
				for time.Now().Before(deadline) {
					i := int(next.Add(1)) - 1
					argv := requestArgv(opt, script, pickTexts(corpus, i, opt.batch))
					if err := client.WriteArgv(argv...); err != nil {
						s := err.Error()
						firstErr.CompareAndSwap(nil, &s)
						return
					}
					if err := client.Flush(); err != nil {
						s := err.Error()
						firstErr.CompareAndSwap(nil, &s)
						return
					}
					rep, err := client.ReadReply()
					if err == nil {
						err = rep.Err()
					}
					if err != nil {
						s := err.Error()
						firstErr.CompareAndSwap(nil, &s)
						return
					}
					done.Add(1)
				}
			}()
		}
		start := time.Now()
		wg.Wait()
		elapsed := time.Since(start)
		if p := firstErr.Load(); p != nil {
			return fmt.Errorf("shape %s: %s", ph.name, *p)
		}
		state, err := readAutotune(info, opt.model)
		if err != nil {
			return fmt.Errorf("reading autotune state: %w", err)
		}
		fmt.Printf("shape=%s concurrency=%d req/s=%.1f %s\n", ph.name, ph.conc, float64(done.Load())/elapsed.Seconds(), state)
	}
	return nil
}

// readAutotune reads the script autotune fields from EMB.INFO <model>.
func readAutotune(c *resp.Client, model string) (string, error) {
	if err := c.WriteArgv("EMB.INFO", model); err != nil {
		return "", err
	}
	if err := c.Flush(); err != nil {
		return "", err
	}
	rep, err := c.ReadReply()
	if err != nil {
		return "", err
	}
	if err := rep.Err(); err != nil {
		return "", err
	}
	fields := make(map[string]string, len(rep.Elems)/2)
	for i := 0; i+1 < len(rep.Elems); i += 2 {
		fields[rep.Elems[i].Str] = replyScalar(rep.Elems[i+1])
	}
	return fmt.Sprintf("class=%s inflight=%s conc=%s/%s autotune=%s",
		fields["script_traffic_class"], fields["script_inflight"],
		fields["script_concurrency_current"], fields["script_concurrency_target"],
		fields["script_autotune_active"]), nil
}

func replyScalar(r resp.Reply) string {
	switch r.Type {
	case ':':
		return strconv.FormatInt(r.Int, 10)
	case '$', '+':
		return r.Str
	}
	return ""
}

func readCorpus(path string) ([]string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("reading corpus: %w", err)
	}
	var texts []string
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if line != "" {
			texts = append(texts, line)
		}
	}
	return texts, nil
}

func parseLevels(s string) ([]int, error) {
	var levels []int
	for _, part := range strings.Split(s, ",") {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		n, err := strconv.Atoi(part)
		if err != nil || n < 1 {
			return nil, fmt.Errorf("invalid concurrency %q", part)
		}
		levels = append(levels, n)
	}
	if len(levels) == 0 {
		return nil, fmt.Errorf("no concurrency levels")
	}
	return levels, nil
}

type levelResult struct {
	n         int
	errors    int64
	reqPerSec float64
	p50       time.Duration
	p90       time.Duration
	p99       time.Duration
	replies   []string
}

func runLevel(opt options, script string, corpus []string, concurrency int) (levelResult, error) {
	var next atomic.Int64
	var firstErr atomic.Pointer[string]
	fail := func(err error) {
		s := err.Error()
		firstErr.CompareAndSwap(nil, &s)
	}

	lat := make([]time.Duration, opt.n)
	replies := make([]string, opt.n)
	start := time.Now()
	var wg sync.WaitGroup
	for w := 0; w < concurrency; w++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			client := resp.NewClient(opt.addr, opt.password, false)
			client.SetTimeout(opt.timeout)
			if err := client.Dial(); err != nil {
				fail(fmt.Errorf("dial: %w", err))
				return
			}
			defer func() { _ = client.Close() }()
			for {
				i := int(next.Add(1)) - 1
				if i >= opt.n {
					return
				}
				texts := pickTexts(corpus, i, opt.batch)
				argv := requestArgv(opt, script, texts)

				t0 := time.Now()
				err := client.WriteArgv(argv...)
				if err == nil {
					err = client.Flush()
				}
				var rep resp.Reply
				if err == nil {
					rep, err = client.ReadReply()
				}
				lat[i] = time.Since(t0)
				if err != nil {
					fail(err)
					continue
				}
				if rep.Err() != nil {
					fail(fmt.Errorf("reply %d: %w", i, rep.Err()))
					continue
				}
				replies[i] = canonical(rep)
			}
		}()
	}
	wg.Wait()
	elapsed := time.Since(start)

	if p := firstErr.Load(); p != nil {
		return levelResult{}, fmt.Errorf("%s", *p)
	}

	sorted := append([]time.Duration(nil), lat...)
	sort.Slice(sorted, func(i, j int) bool { return sorted[i] < sorted[j] })
	return levelResult{
		n:         opt.n,
		reqPerSec: float64(opt.n) / elapsed.Seconds(),
		p50:       percentile(sorted, 0.50),
		p90:       percentile(sorted, 0.90),
		p99:       percentile(sorted, 0.99),
		replies:   replies,
	}, nil
}

// pickTexts returns the deterministic texts for request i: batch consecutive
// corpus entries starting at i*batch, wrapping.
func pickTexts(corpus []string, i, batch int) []string {
	texts := make([]string, batch)
	for j := 0; j < batch; j++ {
		texts[j] = corpus[(i*batch+j)%len(corpus)]
	}
	return texts
}

// percentile returns the nearest-rank percentile (1-indexed, clamped).
func percentile(sorted []time.Duration, p float64) time.Duration {
	if len(sorted) == 0 {
		return 0
	}
	idx := int(p*float64(len(sorted)) + 0.5)
	if idx < 1 {
		idx = 1
	}
	if idx > len(sorted) {
		idx = len(sorted)
	}
	return sorted[idx-1]
}

func ms(d time.Duration) float64 { return float64(d.Microseconds()) / 1000 }

// canonical renders a reply as a stable text that captures its value, so two
// runs can be compared byte-for-byte regardless of RESP encoding.
func canonical(r resp.Reply) string {
	switch r.Type {
	case '$', '+':
		return fmt.Sprintf("%c%d:%s", r.Type, len(r.Str), r.Str)
	case ':':
		return fmt.Sprintf(":%d", r.Int)
	case ',':
		return fmt.Sprintf(",%v", r.Float)
	case '-':
		return "-" + r.Str
	case '_':
		return "_"
	case '*', '%':
		if r.Nil {
			return "*nil"
		}
		var b strings.Builder
		b.WriteByte(r.Type)
		b.WriteByte('[')
		for i, e := range r.Elems {
			if i > 0 {
				b.WriteByte(',')
			}
			b.WriteString(canonical(e))
		}
		b.WriteByte(']')
		return b.String()
	}
	return string(r.Type)
}
