package server

import (
	"bytes"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"math"
	"net"
	"os"
	"sort"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/elcuervo/emb/internal/config"
	"github.com/elcuervo/emb/internal/registry"
)

// The plain-EMB gate: the per-change performance test of the embedding path's
// runtime contract, the counterpart of TestScriptSmoke for the scripted path.
// It drives bulk `EMB` through the real in-process server against
// `testdata/plain-emb` (a committed miniature plus a fixed corpus), so it needs
// no model download and no Docker, and it asserts on counted state and stated
// latency bounds rather than on one privileged cause. It reproduces the
// incident's layout class on purpose: a plain request queue that can outrun the
// process CPU budget, where the failure mode is an unbounded wait rather than a
// bounded refusal.
//
// Enabled by EMB_BENCH_SMOKE=1 (`just bench-plain-emb`, and the CGo CI job);
// with EMB_PLAIN_EMB_UPDATE=1 it regenerates `testdata/plain-emb/expected.json`
// from a real run instead of asserting (see the fixture's README).
//
// Arms, and what each one can see:
//
//	plain     batching on, CPU policy layout  - pinned embeddings, burst p99,
//	          the reply cache, drain, health, calibration off the request path
//	pool      batching off, 16 sessions of one thread against a budget of two
//	          - the admission queue bound, counted refusals, reported wait
//	deadline  pool, with the admission bound derived from the measured request
//	          - a wait that ends without a grant, counted, and none without end
//	layout    batching off, policy layout     - two concurrent runs admissible
//	lazy      no preload, not-ready at boot   - the first request drives the
//	          off-request-path load, is answered not-ready inside the bound, and
//	          the model then serves (the reachable calibration-on-request-path arm)
//	fairness  two models, one budget          - a narrow waiter served while a
//	          wider head is parked (the wide model's width is the whole budget)
//
// Assertion names are an interface: `bench/smoke/verify-gate.sh` plants
// regressions that must turn this gate red *naming* the assertion they broke.
// The machine-readable contract is one summary line per arm, naming the arm it
// summarizes (see embArmName):
//
//	emb-smoke: arm=control assertions=12 pass=12 fail=0 skipped=0
//	emb-smoke: failed=<assertion>          (one line per failure)
const (
	// embArmName is this gate's arm name in its summary line. `-run Smoke` selects
	// two arms — this fixed-concurrency control arm (explicit workers and
	// threads, no autotuning) and the adaptive autotune arm in
	// script_smoke_test.go — so the summary names the arm it summarizes and a
	// check can require proof from the arm it is checking rather than any
	// summary the run happened to print.
	embArmName = "control"
	// embModelName is the fixture model's name in the server config.
	embModelName = "plain"
	// embBudget is the explicit process CPU budget every arm runs under: small
	// enough that a burst outruns it, and explicit so the plan does not depend
	// on the measuring host's core count.
	embBudget = 2
	// embFixtureDim and embFixtureMaxLength are the fixture model's shape; the
	// pins in expected.json were taken with exactly these.
	embFixtureDim       = 128
	embFixtureMaxLength = 2048
	// embBurstP99Bound is the stated bound for the batch-on burst's p99. It is a
	// collapse detector, not a register of the measured value: the reference run
	// measures far under it (the README records the number), and it sits well
	// below the incident's 3.9 s first-request stall.
	embBurstP99Bound = 250 * time.Millisecond
	// embFirstCallBound bounds the first plain request served by a preloaded
	// model. Calibration runs at load; a request that has to pay for it takes
	// orders of magnitude longer.
	embFirstCallBound = 100 * time.Millisecond
	// embDrainQuiet is how long the counters must stay frozen after the burst.
	embDrainQuiet = 400 * time.Millisecond
	// embGaugeDeadline bounds how long in-flight gauges may take to fall back.
	embGaugeDeadline = 3 * time.Second
	// embBurstClients and embBurstTextsPerClient shape the batch-on burst: more
	// commands than the arm can admit at once, so the queue is exercised rather
	// than raced.
	embBurstClients        = 48
	embBurstTextsPerClient = 4
	// embPoolWorkers is the pool arm's over-provisioning: more run slots than
	// the budget admits, which is what makes the admission gate observable. The
	// incident's production config had the same shape (four sessions of width
	// three against a budget of three).
	embPoolWorkers = 16
	// embSerialSamples is how many serial requests the pool arm measures: the
	// deadline arm's bound is derived from their median, so the bound scales
	// with the host instead of with a constant.
	embSerialSamples = 5
	// embFairBudget, embFairWideThreads and embFairBoundMultiple shape the
	// fairness arm: the wide model's width is the whole budget (so its runs can
	// only be admitted alone), and a narrow probe must be served within this
	// multiple of its own measured serial latency while that head is parked.
	embFairBudget        = 3
	embFairWideThreads   = embFairBudget
	embFairBoundMultiple = 5
	// embFairAttempts is how many times the arm re-tries to park the wide head
	// behind a holder before recording the assertion as unexercised.
	embFairAttempts = 3
	// embAdmissionCommands is how many commands each admission burst offers:
	// more than the pool can admit plus queue, so refusals are counted rather
	// than raced.
	embAdmissionCommands = embPoolWorkers + 16
	// embAdmissionRunMultiple is how many serial runs the deadline arm's rail
	// lets a wait span before it calls the wait a collapse. A waiter sits behind
	// the pool arm's queue bound (two waiters per admission unit), then behind
	// the runs in flight, then pays for its own run; the burst's runs cost more
	// than the idle serial probe measured them at, so the shape is doubled for
	// that headroom. It is a collapse detector, not a register of the measured
	// value: `timeouts` is the tight term.
	embAdmissionRunMultiple = 2 * (2*embBudget + embBudget + 1)
)

// ---- fixture ---------------------------------------------------------------

type embFixtureConfig struct {
	Dim          int    `json:"dim"`
	MaxLength    int    `json:"max_length"`
	Pooling      string `json:"pooling"`
	Normalize    bool   `json:"normalize"`
	OutputTensor string `json:"output_tensor"`
}

type embFixtureOutput struct {
	Text      string    `json:"text"`
	Dim       int       `json:"dim"`
	SHA256    string    `json:"sha256"`
	Embedding []float32 `json:"embedding"`
}

// embFixture is the committed fixture: the corpus, the pins, and the model the
// pins were taken against.
type embFixture struct {
	Model     string             `json:"model"`
	ModelSHA  string             `json:"model_sha256"`
	Tokenizer string             `json:"tokenizer"`
	Config    embFixtureConfig   `json:"config"`
	Outputs   []embFixtureOutput `json:"outputs"`
	// Texts comes from texts.json; expected.json carries no copy of it, so the
	// two files cannot disagree about the corpus.
	Texts []string `json:"texts"`
}

func embFixturePath(name string) string { return "../../testdata/plain-emb/" + name }

func embReadFile(t *testing.T, path string) []byte {
	t.Helper()
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("reading %s: %v (the gate fails rather than skips on a missing fixture)", path, err)
	}
	return raw
}

// embFixtureDoc is the fixture's declared shape: the config the pins were taken
// with, and the model and tokenizer the gate drives. It is the single source of
// truth for all three, so a pin cannot be taken with one layout and asserted
// with another.
func embFixtureDoc() embFixtureConfig {
	return embFixtureConfig{
		Dim:          embFixtureDim,
		MaxLength:    embFixtureMaxLength,
		Pooling:      "mean",
		Normalize:    true,
		OutputTensor: "last_hidden_state",
	}
}

const (
	embFixtureModel     = "model.onnx"
	embFixtureTokenizer = "../../testdata/laya/tokenizer/tokenizer.json"
)

// embFixtureLoad reads the corpus and the pins, and fails loudly when the two
// disagree or when the model is not the one the pins were taken against: a
// fixture that drifted must not be silently accepted.
func embFixtureLoad(t *testing.T) *embFixture {
	t.Helper()
	f := &embFixture{}
	if err := json.Unmarshal(embReadFile(t, embFixturePath("texts.json")), f); err != nil {
		t.Fatalf("testdata/plain-emb/texts.json: %v", err)
	}
	if len(f.Texts) == 0 {
		t.Fatal("testdata/plain-emb/texts.json holds no texts")
	}
	if _, err := os.Stat(embFixturePath("expected.json")); err != nil && os.Getenv("EMB_PLAIN_EMB_UPDATE") != "" {
		// Regenerating from nothing: the corpus and the gate's own declaration
		// are the whole fixture until the run writes the pins.
		f.Model, f.Tokenizer, f.Config = embFixtureModel, embFixtureTokenizer, embFixtureDoc()
		return f
	}
	if err := json.Unmarshal(embReadFile(t, embFixturePath("expected.json")), f); err != nil {
		t.Fatalf("testdata/plain-emb/expected.json: %v (regenerate: EMB_BENCH_SMOKE=1 EMB_PLAIN_EMB_UPDATE=1 go test ./internal/server/ -run PlainEMB)", err)
	}
	if len(f.Outputs) != len(f.Texts) {
		t.Fatalf("fixture mismatch: %d texts, %d pinned outputs; regenerate expected.json", len(f.Texts), len(f.Outputs))
	}
	for i, out := range f.Outputs {
		if out.Text != f.Texts[i] {
			t.Fatalf("fixture mismatch at %d: pinned %q, corpus %q; regenerate expected.json", i, out.Text, f.Texts[i])
		}
		if out.Dim != f.Config.Dim || len(out.Embedding) != out.Dim || len(out.SHA256) != 64 {
			t.Fatalf("fixture mismatch at %d: dim=%d, %d pinned component(s); regenerate expected.json", i, out.Dim, len(out.Embedding))
		}
	}
	if f.Model != embFixtureModel || f.Tokenizer != embFixtureTokenizer {
		t.Fatalf("expected.json names model %q and tokenizer %q; this gate drives %q and %q",
			f.Model, f.Tokenizer, embFixtureModel, embFixtureTokenizer)
	}
	if f.Config != embFixtureDoc() {
		t.Fatalf("the pins were taken with %+v; this gate drives %+v", f.Config, embFixtureDoc())
	}
	// From here on the declared shape is the truth: the pins were taken with it,
	// so the rest of the gate reads one config rather than two.
	f.Config = embFixtureDoc()
	sum := sha256.Sum256(embReadFile(t, embFixturePath(f.Model)))
	if got := hex.EncodeToString(sum[:]); got != f.ModelSHA {
		t.Fatalf("testdata/plain-emb/%s is %s, the pins were taken against %s; regenerate expected.json", f.Model, got, f.ModelSHA)
	}
	return f
}

// embWritePins writes expected.json from a real run, which is the fixture's
// regeneration path.
func embWritePins(t *testing.T, fixture *embFixture, pins []embFixtureOutput) {
	t.Helper()
	sum := sha256.Sum256(embReadFile(t, embFixturePath(fixture.Model)))
	doc := struct {
		Model     string             `json:"model"`
		ModelSHA  string             `json:"model_sha256"`
		Tokenizer string             `json:"tokenizer"`
		Config    embFixtureConfig   `json:"config"`
		Outputs   []embFixtureOutput `json:"outputs"`
	}{
		Model:     fixture.Model,
		ModelSHA:  hex.EncodeToString(sum[:]),
		Tokenizer: fixture.Tokenizer,
		Config:    fixture.Config,
		Outputs:   pins,
	}
	raw, err := json.MarshalIndent(doc, "", "  ")
	if err != nil {
		t.Fatalf("encoding expected.json: %v", err)
	}
	if err := os.WriteFile(embFixturePath("expected.json"), append(raw, '\n'), 0o644); err != nil {
		t.Fatalf("writing expected.json: %v", err)
	}
}

// ---- assertions ------------------------------------------------------------

type embOutcome struct {
	name   string
	result string
	detail string
}

// embAssertions collects every assertion's outcome and prints the summary line
// the sweep and the CI job read. A failure is reported twice: as the
// `assertion <name> failed` line `verify-gate.sh` greps for, and as the
// `failed=<assertion>` line the CI step greps for.
type embAssertions struct {
	outcomes []embOutcome
}

func (g *embAssertions) check(t *testing.T, name string, ok bool, format string, args ...any) bool {
	t.Helper()
	result, detail := "pass", fmt.Sprintf(format, args...)
	if !ok {
		result = "fail"
		t.Errorf("emb-smoke: assertion %s failed: %s", name, detail)
	}
	g.outcomes = append(g.outcomes, embOutcome{name: name, result: result, detail: detail})
	return ok
}

// skip records an assertion the run could not exercise, with the property that
// prevented it: never a pass.
func (g *embAssertions) skip(name, property string) {
	g.outcomes = append(g.outcomes, embOutcome{name: name, result: "skipped", detail: property})
}

func (g *embAssertions) report(t *testing.T) {
	t.Helper()
	var pass, fail, skipped int
	var failed []string
	for _, o := range g.outcomes {
		t.Logf("emb-smoke assertion %-26s %-7s %s", o.name, o.result, o.detail)
		switch o.result {
		case "pass":
			pass++
		case "fail":
			fail++
			failed = append(failed, o.name)
		default:
			// A skip stays a skip, naming the host property that prevented the
			// assertion: it is never a pass, and never dropped from the counts
			// (so an assertion that did not run cannot hide in this arm's total).
			skipped++
		}
	}
	summary := fmt.Sprintf("emb-smoke: arm=%s assertions=%d pass=%d fail=%d skipped=%d",
		embArmName, len(g.outcomes), pass, fail, skipped)
	t.Logf("%s", summary)
	// Also on stderr, unprefixed: the summary is the machine-readable contract
	// the sweep and the CI job read, and it must not depend on -v being passed.
	fmt.Fprintln(os.Stderr, summary)
	if len(failed) > 0 {
		t.Logf("emb-smoke: failed=%s", strings.Join(failed, ","))
		fmt.Fprintf(os.Stderr, "emb-smoke: failed=%s\n", strings.Join(failed, ","))
		t.Fatalf("emb-smoke: plain-EMB gate: %d assertion(s) failed", fail)
	}
}

// ---- RESP ------------------------------------------------------------------

// embValue is one parsed RESP value. Arrays keep their elements so the gate can
// read the field/value pairs EMB.INFO and EMB.STATS answer with.
type embValue struct {
	kind  byte
	text  string
	elems []embValue
}

// embParse reads one complete RESP value from the front of raw, reporting how
// many bytes it used. It returns ok=false while the value is still incomplete.
func embParse(raw []byte) (embValue, int, bool) {
	if len(raw) == 0 {
		return embValue{}, 0, false
	}
	end := bytes.Index(raw, []byte("\r\n"))
	if end < 0 {
		return embValue{}, 0, false
	}
	line, head := raw[:end], end+2
	switch raw[0] {
	case '+', '-', ':':
		return embValue{kind: raw[0], text: string(raw[1:end])}, head, true
	case '$':
		n, err := strconv.Atoi(string(line[1:]))
		if err != nil {
			return embValue{}, 0, false
		}
		if n < 0 {
			return embValue{kind: '$'}, head, true
		}
		if len(raw) < head+n+2 {
			return embValue{}, 0, false
		}
		return embValue{kind: '$', text: string(raw[head : head+n])}, head + n + 2, true
	case '*':
		n, err := strconv.Atoi(string(line[1:]))
		if err != nil || n < 0 {
			return embValue{}, 0, false
		}
		value := embValue{kind: '*', elems: make([]embValue, 0, n)}
		offset := head
		for range n {
			element, used, ok := embParse(raw[offset:])
			if !ok {
				return embValue{}, 0, false
			}
			value.elems = append(value.elems, element)
			offset += used
		}
		return value, offset, true
	}
	return embValue{}, 0, false
}

// embReply is one command's answer: err for a server error (a busy refusal is an
// expected outcome under load, not a test failure), status for a simple string,
// and bulk for a bulk reply's payload or an array's elements.
type embReply struct {
	err    string
	status string
	bulk   []string
}

func (r embReply) busy() bool { return strings.Contains(r.err, "busy") }

// embAsk sends one command over its own connection and reads one reply. It
// returns errors instead of failing the test, so it is safe from a test
// goroutine.
func embAsk(addr string, deadline time.Time, args ...string) (embReply, error) {
	conn, err := net.Dial("tcp", addr)
	if err != nil {
		return embReply{}, err
	}
	defer func() { _ = conn.Close() }()
	return embAskConn(conn, deadline, args...)
}

func embAskConn(conn net.Conn, deadline time.Time, args ...string) (embReply, error) {
	if err := conn.SetDeadline(deadline); err != nil {
		return embReply{}, err
	}
	if _, err := conn.Write(respCommand(args...)); err != nil {
		return embReply{}, err
	}
	raw := make([]byte, 0, 1<<16)
	tmp := make([]byte, 64*1024)
	for {
		if len(raw) > 0 {
			if value, used, ok := embParse(raw); ok {
				if used != len(raw) {
					return embReply{}, fmt.Errorf("trailing bytes after a complete reply")
				}
				return embValueReply(value), nil
			}
		}
		n, err := conn.Read(tmp)
		if n > 0 {
			raw = append(raw, tmp[:n]...)
		}
		if err != nil {
			return embReply{}, err
		}
	}
}

func embValueReply(value embValue) embReply {
	switch value.kind {
	case '-':
		return embReply{err: value.text}
	case '+':
		return embReply{status: value.text}
	case '$':
		return embReply{bulk: []string{value.text}}
	case '*':
		out := embReply{bulk: make([]string, 0, len(value.elems))}
		for _, element := range value.elems {
			out.bulk = append(out.bulk, element.text)
		}
		return out
	}
	return embReply{}
}

// embStatus sends a command and returns its simple-string status.
func embStatus(addr string, deadline time.Time, args ...string) (string, error) {
	reply, err := embAsk(addr, deadline, args...)
	if err != nil {
		return "", err
	}
	return reply.status, nil
}

// embPairs reads a flat field/value array reply (EMB.INFO, EMB.STATS).
func embPairs(t *testing.T, addr string, args ...string) map[string]string {
	t.Helper()
	reply, err := embAsk(addr, time.Now().Add(5*time.Second), args...)
	if err != nil {
		t.Fatalf("%v: %v", args, err)
	}
	if reply.err != "" {
		t.Fatalf("%v: %s", args, reply.err)
	}
	if len(reply.bulk)%2 != 0 {
		t.Fatalf("%v: reply holds %d element(s), not field/value pairs", args, len(reply.bulk))
	}
	out := make(map[string]string, len(reply.bulk)/2)
	for i := 0; i+1 < len(reply.bulk); i += 2 {
		out[reply.bulk[i]] = reply.bulk[i+1]
	}
	return out
}

// embPairInt reads one integer field out of a pair reply.
func embPairInt(t *testing.T, addr string, field string, args ...string) int {
	t.Helper()
	pairs := embPairs(t, addr, args...)
	raw, ok := pairs[field]
	if !ok {
		t.Fatalf("%v does not report %s", args, field)
	}
	n, err := strconv.Atoi(strings.TrimSpace(raw))
	if err != nil {
		t.Fatalf("%v reports %s=%q, not an integer", args, field, raw)
	}
	return n
}

// ---- log capture -----------------------------------------------------------

// embLogTee forwards the server's log to stderr while keeping the bytes, so the
// gate can assert on what the boot path logged without hiding it from a
// debugging run.
type embLogTee struct {
	mu  sync.Mutex
	out io.Writer
	buf bytes.Buffer
}

func (w *embLogTee) Write(p []byte) (int, error) {
	w.mu.Lock()
	w.buf.Write(p)
	w.mu.Unlock()
	return w.out.Write(p)
}

// countAfter counts the lines holding match in the log written after mark.
func (w *embLogTee) countAfter(mark int, match string) int {
	w.mu.Lock()
	defer w.mu.Unlock()
	body := w.buf.String()
	if mark > len(body) {
		return 0
	}
	count := 0
	for _, line := range strings.Split(body[mark:], "\n") {
		if strings.Contains(line, match) {
			count++
		}
	}
	return count
}

var embLog = struct {
	sync.Mutex
	tee *embLogTee
}{}

func embLogStart(t *testing.T) {
	t.Helper()
	embLog.Lock()
	defer embLog.Unlock()
	if embLog.tee != nil {
		return
	}
	tee := &embLogTee{out: os.Stderr}
	embLog.tee = tee
	log.SetOutput(tee)
	t.Cleanup(func() {
		embLog.Lock()
		defer embLog.Unlock()
		log.SetOutput(os.Stderr)
		embLog.tee = nil
	})
}

func embLogMark() int {
	embLog.Lock()
	defer embLog.Unlock()
	if embLog.tee == nil {
		return 0
	}
	return embLog.tee.buf.Len()
}

func embLogCountAfter(mark int, match string) int {
	embLog.Lock()
	tee := embLog.tee
	embLog.Unlock()
	if tee == nil {
		return 0
	}
	return tee.countAfter(mark, match)
}

// ---- server arms -----------------------------------------------------------

// embArm is one in-process server arm of the gate.
type embArm struct {
	addr string
	srv  *Server
	// loadedAt marks the log once the model was preloaded: every line after it
	// was written while requests were being served.
	loadedAt int
}

type embArmOptions struct {
	// model is the fixture model, registered as embModelName.
	model config.ModelConfig
	// extra registers further models beside it (the fairness arm needs two
	// models of different admission widths on one budget); nil for the
	// single-model arms.
	extra     map[string]config.ModelConfig
	cache     string
	admission time.Duration
	budget    int
	calibrate *bool
	// lazy starts the model without a preload and leaves the server not-ready,
	// so the first request drives ResolveReady's off-request-path load (the
	// arm that exercises the calibration-on-request-path regression). The zero
	// value preloads and reports ready, as every other arm expects.
	lazy bool
}

// embStart starts an in-process server over the fixture, with the model
// preloaded exactly as cmd/emb does: the pool (and any calibration) exists
// before the server accepts a request.
func embStart(t *testing.T, opts embArmOptions) *embArm {
	t.Helper()
	models := map[string]config.ModelConfig{embModelName: opts.model}
	for name, model := range opts.extra {
		models[name] = model
	}
	cfg := &config.Config{
		CPUBudget: strconv.Itoa(opts.budget),
		Calibrate: opts.calibrate,
		Models:    models,
	}
	reg, err := registry.NewWithConfig(cfg)
	if err != nil {
		t.Fatalf("resolving the CPU plan: %v", err)
	}
	for _, name := range embSortedNames(models) {
		entry, err := registry.LoadModelDeferred(models[name], name)
		if err != nil {
			t.Fatalf("loading %q: %v", name, err)
		}
		reg.Add(name, entry)
		if !opts.lazy {
			if err := entry.Preload(); err != nil {
				t.Fatalf("preloading %q: %v", name, err)
			}
		}
	}
	reg.SetModelCount(len(models))
	addr := getFreeAddr()
	srv := New(addr, reg, "", opts.cache, nil, WithAdmissionTimeout(opts.admission))
	// A lazy arm leaves the server not-ready until a request drives the load
	// through ResolveReady, exactly as cmd/emb does when LayoutsDecided is false.
	if !opts.lazy {
		srv.SetReady()
	}
	go srv.ListenAndServe()
	t.Cleanup(func() {
		_ = srv.Close()
		_ = reg.Close()
	})

	arm := &embArm{addr: addr, srv: srv, loadedAt: embLogMark()}
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if status, err := embStatus(addr, time.Now().Add(time.Second), "PING"); err == nil && status == "PONG" {
			return arm
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("the plain-emb arm never answered PING")
	return nil
}

// embModelConfig is the fixture model's layout. batching is the coalescing
// window (nil disables batching, which is what makes the admission gate
// observable); workers and threads are explicit only where an arm needs them.
func embModelConfig(preload bool, batching *int, maxBatch, workers, threads int) config.ModelConfig {
	return config.ModelConfig{
		ONNX:           embFixturePath(embFixtureModel),
		Tokenizer:      embFixtureTokenizer,
		Dim:            embFixtureDoc().Dim,
		MaxLength:      embFixtureDoc().MaxLength,
		Pooling:        embFixtureDoc().Pooling,
		Normalize:      embFixtureDoc().Normalize,
		OutputTensor:   embFixtureDoc().OutputTensor,
		Preload:        preload,
		Workers:        workers,
		IntraOpThreads: threads,
		Batching:       config.BatchingConfig{Timeout: batching, MaxBatch: maxBatch},
	}
}

// ---- requests and measurements ---------------------------------------------

// embEmbed embeds texts in one command and returns the raw embedding bytes per
// requested text.
func embEmbed(t *testing.T, addr string, texts ...string) [][]byte {
	t.Helper()
	args := append([]string{"EMB", embModelName}, texts...)
	reply, err := embAsk(addr, time.Now().Add(60*time.Second), args...)
	if err != nil {
		t.Fatalf("EMB %d text(s): %v", len(texts), err)
	}
	if reply.err != "" {
		t.Fatalf("EMB %d text(s): server error %s", len(texts), reply.err)
	}
	if len(reply.bulk) != len(texts) {
		t.Fatalf("EMB %d text(s): %d embedding(s) in the reply", len(texts), len(reply.bulk))
	}
	out := make([][]byte, len(reply.bulk))
	for i, payload := range reply.bulk {
		out[i] = []byte(payload)
	}
	return out
}

// embDigest pins a reply: the sha256 of its little-endian float32 bytes, plus
// the decoded vector so a mismatch is diagnosable.
func embDigest(payload []byte) (string, []float32, error) {
	if len(payload)%4 != 0 {
		return "", nil, fmt.Errorf("payload of %d bytes is not float32-aligned", len(payload))
	}
	sum := sha256.Sum256(payload)
	values := make([]float32, len(payload)/4)
	for i := range values {
		values[i] = math.Float32frombits(binary.LittleEndian.Uint32(payload[i*4:]))
	}
	return hex.EncodeToString(sum[:]), values, nil
}

// embCloseWithin compares two vectors within a stated tolerance. The digest is
// the pin; this exists because ONNX Runtime picks kernels per host, so the same
// graph can differ in its last bits across architectures while meaning the same
// thing. A difference beyond the tolerance is a real change.
func embCloseWithin(got, want []float32, tol float32) bool {
	if len(got) != len(want) {
		return false
	}
	for i := range got {
		if math.Abs(float64(got[i]-want[i])) > float64(tol) {
			return false
		}
	}
	return true
}

type embBurst struct {
	latencies []time.Duration
	completed int
	refused   int
	failed    int
}

func (b embBurst) p99() time.Duration { return embPercentile(b.latencies, 0.99) }
func (b embBurst) max() time.Duration { return embPercentile(b.latencies, 1) }

// embPercentile returns the p-th percentile of the samples (nearest rank).
func embPercentile(samples []time.Duration, p float64) time.Duration {
	if len(samples) == 0 {
		return 0
	}
	sorted := append([]time.Duration(nil), samples...)
	sort.Slice(sorted, func(i, j int) bool { return sorted[i] < sorted[j] })
	idx := int(math.Ceil(p*float64(len(sorted)))) - 1
	if idx < 0 {
		idx = 0
	}
	if idx >= len(sorted) {
		idx = len(sorted) - 1
	}
	return sorted[idx]
}

// embFire sends every command concurrently, each on its own connection, and
// returns the per-command latency. disconnect names the one command whose
// client walks away before reading its reply (-1 for none). A busy answer counts
// as a refusal: under overload a bounded refusal is the contract.
func embFire(addr string, commands [][]string, timeout time.Duration, disconnect int) embBurst {
	start := make(chan struct{})
	results := make(chan embBurst, len(commands))
	var wg sync.WaitGroup
	for i, command := range commands {
		wg.Add(1)
		go func(i int, command []string) {
			defer wg.Done()
			conn, err := net.Dial("tcp", addr)
			if err != nil {
				results <- embBurst{failed: 1}
				return
			}
			defer func() { _ = conn.Close() }()
			<-start
			begin := time.Now()
			reply, err := embAskConn(conn, time.Now().Add(timeout), command...)
			if disconnect == i {
				// Walked away mid-command: the server must finish or refuse
				// without leaving the work behind.
				return
			}
			out := embBurst{latencies: []time.Duration{time.Since(begin)}}
			switch {
			case err != nil:
				out.failed = 1
			case reply.busy():
				out.refused = 1
			case len(reply.bulk) > 0 && reply.bulk[0] != "":
				out.completed = 1
			default:
				out.failed = 1
			}
			results <- out
		}(i, command)
	}
	close(start)
	wg.Wait()
	close(results)
	total := embBurst{}
	for r := range results {
		total.latencies = append(total.latencies, r.latencies...)
		total.completed += r.completed
		total.refused += r.refused
		total.failed += r.failed
	}
	return total
}

// embSortedNames sorts a model map's keys, so an arm starts its models in a
// fixed order (the boot log and the CPU plan are then comparable run to run).
func embSortedNames(models map[string]config.ModelConfig) []string {
	names := make([]string, 0, len(models))
	for name := range models {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

// embLaunch sends one command on its own connection and returns a handle that
// waits for its reply. It is how the fairness arm keeps a run in flight while it
// offers the next two: the holder must still hold when the wide head parks, and
// the probe must arrive while the head is parked.
func embLaunch(addr string, timeout time.Duration, args ...string) (wait func() (embReply, time.Duration, error)) {
	type result struct {
		reply   embReply
		latency time.Duration
		err     error
	}
	ch := make(chan result, 1)
	go func() {
		conn, err := net.Dial("tcp", addr)
		if err != nil {
			ch <- result{err: err}
			return
		}
		defer func() { _ = conn.Close() }()
		begin := time.Now()
		reply, err := embAskConn(conn, time.Now().Add(timeout), args...)
		ch <- result{reply: reply, latency: time.Since(begin), err: err}
	}()
	return func() (embReply, time.Duration, error) {
		r := <-ch
		return r.reply, r.latency, r.err
	}
}

// embWatch polls one pair field until stopped and reports the maximum it saw,
// so a gauge that spikes and falls back is still observed.
func embWatch(addr string, command []string, field string, interval time.Duration) (stop func() int) {
	var max atomic.Int64
	done := make(chan struct{})
	var once sync.Once
	go func() {
		conn, err := net.Dial("tcp", addr)
		if err != nil {
			return
		}
		defer func() { _ = conn.Close() }()
		for {
			select {
			case <-done:
				return
			case <-time.After(interval):
			}
			reply, err := embAskConn(conn, time.Now().Add(2*time.Second), command...)
			if err != nil || len(reply.bulk)%2 != 0 {
				continue
			}
			for i := 0; i+1 < len(reply.bulk); i += 2 {
				if reply.bulk[i] != field {
					continue
				}
				if n, err := strconv.Atoi(strings.TrimSpace(reply.bulk[i+1])); err == nil && int64(n) > max.Load() {
					max.Store(int64(n))
				}
			}
		}
	}()
	return func() int {
		once.Do(func() { close(done) })
		return int(max.Load())
	}
}

// embHealthProbe answers PING and EMB.READY on one connection until stopped,
// counting the probes that were answered as expected.
func embHealthProbe(addr string, interval time.Duration) (stop func() (answered, failed int)) {
	var answered, failed atomic.Int64
	done := make(chan struct{})
	var once sync.Once
	go func() {
		conn, err := net.Dial("tcp", addr)
		if err != nil {
			failed.Add(1)
			return
		}
		defer func() { _ = conn.Close() }()
		for {
			select {
			case <-done:
				return
			case <-time.After(interval):
			}
			ok := true
			for _, probe := range [][]string{{"PING"}, {"EMB.READY"}} {
				reply, err := embAskConn(conn, time.Now().Add(2*time.Second), probe...)
				if err != nil || reply.status == "" {
					ok = false
				}
			}
			if ok {
				answered.Add(1)
			} else {
				failed.Add(1)
			}
		}
	}()
	return func() (int, int) {
		once.Do(func() { close(done) })
		return int(answered.Load()), int(failed.Load())
	}
}

// embSnapshot is the process state the drain and admission assertions compare
// across phases.
type embSnapshot struct {
	requests    int
	refusals    int
	timeouts    int
	waits       int
	waiters     int
	active      int
	activeToken int
	runs        int
	health      map[string]string
}

func embTake(t *testing.T, addr string) embSnapshot {
	t.Helper()
	stats := embPairs(t, addr, "EMB.STATS")
	info := embPairs(t, addr, "EMB.INFO", embModelName)
	at := func(pairs map[string]string, field string) int {
		n, err := strconv.Atoi(strings.TrimSpace(pairs[field]))
		if err != nil {
			t.Fatalf("%s is not reported or not an integer: %q", field, pairs[field])
		}
		return n
	}
	return embSnapshot{
		requests:    at(stats, "total_requests"),
		refusals:    at(stats, "admission_refusals"),
		timeouts:    at(stats, "cpu_admission_timeouts"),
		waits:       at(stats, "cpu_admission_wait_us"),
		waiters:     at(stats, "cpu_admission_waiters"),
		active:      at(stats, "active_requests"),
		activeToken: at(stats, "cpu_active_tokens"),
		runs:        at(info, "embed_runs"),
	}
}

// ---- the gate --------------------------------------------------------------

// TestPlainEMBSmoke is the tier-2 plain-EMB gate: serial, then a burst, then
// recovery, over the real in-process server. It is enabled by EMB_BENCH_SMOKE=1.
func TestPlainEMBSmoke(t *testing.T) {
	smokeEnabled(t)
	fixture := embFixtureLoad(t)
	embLogStart(t)
	gate := &embAssertions{}

	plain := embStart(t, embArmOptions{
		model:     embModelConfig(true, embBatchingWindow(), 8, 0, 0),
		cache:     "32MB",
		admission: config.DefaultAdmissionTimeout,
		budget:    embBudget,
	})
	// The health probe runs across every load phase, so "answers throughout"
	// covers serial, burst and drain rather than a single sampled instant.
	stopHealth := embHealthProbe(plain.addr, 5*time.Millisecond)
	pins, digests := embSerialPhase(t, gate, plain, fixture)
	if os.Getenv("EMB_PLAIN_EMB_UPDATE") != "" {
		stopHealth()
		embWritePins(t, fixture, pins)
		t.Logf("emb-smoke: regenerated testdata/plain-emb/expected.json (%d pins)", len(pins))
		return
	}
	embCacheIdle(t, gate, plain, fixture)
	embBurstPhase(t, gate, plain, fixture)
	embDrainPhase(t, gate, plain, fixture, digests)
	embHealthPhase(t, gate, plain, stopHealth)
	embCalibrationPhase(t, gate, plain, fixture)
	embLazyStartPhase(t, gate, fixture)

	serial, serialWait := embQueueBound(t, gate, fixture)
	embAdmissionBounded(t, gate, fixture, serial, serialWait)
	embLayoutConcurrency(t, gate, fixture)
	embFairness(t, gate, fixture)

	gate.report(t)
}

// embBatchingWindow is the fixture model's batching window: batching on, which
// is what the `plain` arm drives (the pool then coalesces a request's texts into
// one batched run).
func embBatchingWindow() *int {
	window := 2
	return &window
}

// embSerialPhase embeds the corpus once, pinning dim and the reply digest for
// every text. The serial phase is the reference the burst is measured against,
// and it must not have waited for admission: one batcher session under a budget
// of two never queues.
func embSerialPhase(t *testing.T, gate *embAssertions, arm *embArm, fixture *embFixture) ([]embFixtureOutput, []string) {
	t.Helper()
	pins := make([]embFixtureOutput, 0, len(fixture.Texts))
	digests := make([]string, 0, len(fixture.Texts))
	var failures []string
	var exact int
	var toleranceTexts []int
	for i, text := range fixture.Texts {
		begin := time.Now()
		payloads := embEmbed(t, arm.addr, text)
		elapsed := time.Since(begin)
		payload := payloads[0]
		digest, values, err := embDigest(payload)
		if err != nil {
			failures = append(failures, fmt.Sprintf("text %d: %v", i, err))
			continue
		}
		pins = append(pins, embFixtureOutput{Text: text, Dim: len(payload) / 4, SHA256: digest, Embedding: values})
		digests = append(digests, digest)
		if i >= len(fixture.Outputs) {
			continue
		}
		want := fixture.Outputs[i]
		t.Logf("emb-smoke: text %d (%.24q): dim %d, digest %s, first %v, took %s",
			i, text, len(payload)/4, digest[:12], values[:3], elapsed)
		switch {
		case len(payload) != fixture.Config.Dim*4:
			failures = append(failures, fmt.Sprintf("text %d: dim %d, pinned %d", i, len(payload)/4, fixture.Config.Dim))
		case digest == want.SHA256:
			exact++
		case embCloseWithin(values, want.Embedding, 1e-5):
			// The digest is the pin; a vector this close means a host whose ONNX
			// Runtime kernel keeps different low bits. Record which text took the
			// looser path, so a run that did is distinguishable in the log from
			// one where every digest matched exactly.
			toleranceTexts = append(toleranceTexts, i)
			t.Logf("emb-smoke: text %d (%.24q) digest-mismatch-tolerance: digest %s vs pinned %s, every component within 1e-5",
				i, text, digest[:12], want.SHA256[:12])
		default:
			failures = append(failures, fmt.Sprintf("text %d (%.24q): digest %s (pinned %s), first %v (pinned %v)",
				i, text, digest[:12], want.SHA256[:12], values[:3], want.Embedding[:3]))
		}
	}
	if len(fixture.Outputs) == 0 {
		gate.skip("plain-emb-pinned", "regenerating expected.json from this run")
		return pins, digests
	}
	tolerance := "all digests exact"
	if len(toleranceTexts) > 0 {
		tolerance = fmt.Sprintf("digest-mismatch-tolerance on text(s) %v (within 1e-5 per component)",
			trimInts(toleranceTexts))
	}
	gate.check(t, "plain-emb-pinned", len(failures) == 0,
		"%d/%d text(s) match at dim %d (%d exact digest(s), %s): %s",
		len(fixture.Texts)-len(failures), len(fixture.Texts), fixture.Config.Dim, exact, tolerance,
		strings.Join(failures, "; "))
	return pins, digests
}

// trimInts renders a small index list for a log line.
func trimInts(values []int) string {
	parts := make([]string, 0, len(values))
	for _, v := range values {
		parts = append(parts, strconv.Itoa(v))
	}
	return "[" + strings.Join(parts, " ") + "]"
}

// embCacheIdle asserts the reply cache really short-circuits inference: a second
// pass over the same corpus is served from the cache and does not advance the
// model's run counter, because a cache hit is not traffic.
func embCacheIdle(t *testing.T, gate *embAssertions, arm *embArm, fixture *embFixture) {
	t.Helper()
	before := embTake(t, arm.addr)
	start := embPairs(t, arm.addr, "EMB.INFO", embModelName)
	for _, text := range fixture.Texts {
		payloads := embEmbed(t, arm.addr, text)
		if _, _, err := embDigest(payloads[0]); err != nil {
			t.Fatalf("re-embedding a cached text: %v", err)
		}
	}
	end := embPairs(t, arm.addr, "EMB.INFO", embModelName)
	at := func(pairs map[string]string, field string) int {
		n, _ := strconv.Atoi(strings.TrimSpace(pairs[field]))
		return n
	}
	hits := at(end, "cache_hits") - at(start, "cache_hits")
	runs := at(end, "embed_runs") - at(start, "embed_runs")
	snapshot := embTake(t, arm.addr)
	gate.check(t, "cache-idle",
		hits == len(fixture.Texts) && runs == 0 && snapshot.runs == before.runs,
		"%d cache hit(s) over %d text(s) advanced the run counter by %d (a hit must not run inference)",
		hits, len(fixture.Texts), runs)
}

// embBurstPhase drives the concurrent burst and asserts the stated p99 bound and
// that health commands keep answering while it runs.
func embBurstPhase(t *testing.T, gate *embAssertions, arm *embArm, fixture *embFixture) {
	t.Helper()
	commands := embBurstCommands(fixture)
	begin := time.Now()
	burst := embFire(arm.addr, commands, 60*time.Second, -1)
	duration := time.Since(begin)

	gate.check(t, "plain-emb-latency",
		burst.completed == len(commands) && burst.p99() <= embBurstP99Bound,
		"burst of %d command(s) in %s: %d completed, %d refused, %d failed, p99 %s (bound %s), max %s",
		len(commands), duration, burst.completed, burst.refused, burst.failed, burst.p99(), embBurstP99Bound, burst.max())
}

// embHealthPhase asserts the liveness contract: PING and EMB.READY answered on
// every probe while the model was under load and while it drained, which is what
// keeps an overloaded node observable instead of merely unavailable.
func embHealthPhase(t *testing.T, gate *embAssertions, arm *embArm, stop func() (int, int)) {
	t.Helper()
	answered, unanswered := stop()
	gate.check(t, "health-answers", unanswered == 0 && answered >= 20,
		"PING+EMB.READY answered %d time(s) across the serial, burst and drain phases, %d probe(s) unanswered", answered, unanswered)
}

// embDrainPhase covers the drain contract: the gauges fall back after the
// burst's clients are gone, the counters freeze once the work is finished, a
// command whose client walked away leaves nothing running, and the model still
// answers with the pinned vectors.
func embDrainPhase(t *testing.T, gate *embAssertions, arm *embArm, fixture *embFixture, digests []string) {
	t.Helper()
	drained := false
	deadline := time.Now().Add(embGaugeDeadline)
	for time.Now().Before(deadline) {
		snapshot := embTake(t, arm.addr)
		if snapshot.active == 0 && snapshot.activeToken == 0 && snapshot.waiters == 0 {
			drained = true
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	settle := embTake(t, arm.addr)
	time.Sleep(embDrainQuiet)
	later := embTake(t, arm.addr)
	frozen := settle.requests == later.requests && settle.refusals == later.refusals && settle.runs == later.runs

	// A client that walks away mid-command must not leave work behind either.
	embFire(arm.addr, embBurstCommands(fixture)[:1], 30*time.Second, 0)
	live := embTake(t, arm.addr)
	woundDown := false
	deadline = time.Now().Add(embGaugeDeadline)
	for time.Now().Before(deadline) {
		snapshot := embTake(t, arm.addr)
		if snapshot.active == 0 && snapshot.activeToken == 0 && snapshot.waiters == 0 {
			woundDown = true
			break
		}
		time.Sleep(20 * time.Millisecond)
	}

	// The model still answers, byte for byte, after all of it.
	payloads := embEmbed(t, arm.addr, fixture.Texts[0])
	digest, _, err := embDigest(payloads[0])
	pinOK := err == nil && digest == digests[0]

	gate.check(t, "drain-after-disconnect", drained && frozen && woundDown && live.requests >= settle.requests && pinOK,
		"gauges drained=%t, counters frozen=%t (requests %d->%d, refusals %d->%d, runs %d->%d), a disconnected client's work wound down=%t, pinned reply afterwards=%t",
		drained, frozen, settle.requests, later.requests, settle.refusals, later.refusals, settle.runs, later.runs, woundDown, pinOK)
}

// embCalibrationPhase asserts the layout was decided while the model loaded and
// that serving requests does not (re)enter calibration. The log evidence is what
// makes a regression that moves calibration onto the request path visible: such
// a calibration logs its own line, and the request that pays for it blows the
// first-call bound.
func embCalibrationPhase(t *testing.T, gate *embAssertions, arm *embArm, fixture *embFixture) {
	t.Helper()
	atLoad := embLogCountAfter(0, "layout workers=")
	afterLoad := embLogCountAfter(arm.loadedAt, "layout workers=")
	before := embPairs(t, arm.addr, "EMB.INFO", embModelName)
	workers := embPairInt(t, arm.addr, "workers", "EMB.INFO", embModelName)
	threads := embPairInt(t, arm.addr, "intra_op_threads", "EMB.INFO", embModelName)
	decided := before["calibration_source"] != "" && before["calibration_source"] != "none"
	begin := time.Now()
	_ = embEmbed(t, arm.addr, fixture.Texts[0]+" alpha")
	firstCall := time.Since(begin)
	after := embPairs(t, arm.addr, "EMB.INFO", embModelName)
	gate.check(t, "calibration-off-request-path",
		atLoad > 0 && afterLoad == 0 && decided && workers > 0 && threads > 0 &&
			after["calibration_source"] == before["calibration_source"] && firstCall <= embFirstCallBound,
		"layout line(s) at load=%d, after load=%d, source at load=%q (unchanged after a request: %t), workers=%d intra_op_threads=%d, first request %s (bound %s)",
		atLoad, afterLoad, before["calibration_source"], after["calibration_source"] == before["calibration_source"],
		workers, threads, firstCall, embFirstCallBound)
}

// embLazyStartPhase covers the lazy path: a model configured without preload is
// not ready at boot, so the first request drives ResolveReady's off-request-path
// load. It asserts the request is answered not-ready inside the first-call bound
// (the load runs in the background, not in the request), and that the model then
// becomes ready and serves. This is the arm that reaches the load path a
// regression moves onto the request — the other arms preload, so they cannot see
// it.
func embLazyStartPhase(t *testing.T, gate *embAssertions, fixture *embFixture) {
	t.Helper()
	arm := embStart(t, embArmOptions{
		model:     embModelConfig(false, embBatchingWindow(), 8, 0, 0),
		admission: config.DefaultAdmissionTimeout,
		budget:    embBudget,
		lazy:      true,
	})
	// A lazy model must not be advertised ready until its layout is decided.
	early, earlyErr := embStatus(arm.addr, time.Now().Add(2*time.Second), "EMB.READY")
	notReadyAtBoot := earlyErr == nil && early != "OK"

	begin := time.Now()
	reply, err := embAsk(arm.addr, time.Now().Add(30*time.Second), "EMB", embModelName, fixture.Texts[0])
	firstCall := time.Since(begin)
	// Clean: the request is refused explicitly inside the bound. A regression
	// that loads on the request path either blocks past the bound or answers with
	// embeddings because it paid for the load inline.
	answered := err == nil && reply.err != "" && strings.Contains(reply.err, "not ready") && firstCall <= embFirstCallBound

	// The async load the request kicked off must finish: poll until the model
	// answers embeddings. embEmbed fatals on a reply error, so it is called from
	// a closure that recovers — a still-initializing "not ready" reply is the
	// transition being observed, not a failure.
	served := false
	deadline := time.Now().Add(15 * time.Second)
	for time.Now().Before(deadline) {
		if embServed(arm.addr, fixture.Texts[0]) {
			served = true
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	gate.check(t, "calibration-off-request-path",
		notReadyAtBoot && answered && served,
		"lazy model: not-ready at boot=%t, first request %s answered %q inside the %s bound=%t, served after load=%t",
		notReadyAtBoot, firstCall, reply.err, embFirstCallBound, answered, served)
}

// embServed reports whether the model answers a non-empty embedding for text,
// without the test fataling on a not-ready reply: it is the transition the lazy
// arm observes while the background load finishes.
func embServed(addr, text string) bool {
	reply, err := embAsk(addr, time.Now().Add(10*time.Second), "EMB", embModelName, text)
	return err == nil && reply.err == "" && len(reply.bulk) == 1 && reply.bulk[0] != ""
}

// embQueueBound starts the over-provisioned pool arm and asserts the admission
// queue is bounded: past its bound requests are refused and counted rather than
// queued, the queue depth never exceeds the bound, only a request that actually
// queued reports a wait, and every command is accounted for. It returns the
// measured serial request cost and the serial admission wait.
func embQueueBound(t *testing.T, gate *embAssertions, fixture *embFixture) (time.Duration, int) {
	t.Helper()
	off := 0
	arm := embStart(t, embArmOptions{
		model:     embModelConfig(true, &off, 0, embPoolWorkers, 1),
		admission: config.DefaultAdmissionTimeout,
		budget:    embBudget,
	})
	text := fixture.Texts[len(fixture.Texts)-1]
	samples := make([]time.Duration, 0, embSerialSamples)
	for range embSerialSamples {
		begin := time.Now()
		_ = embEmbed(t, arm.addr, text)
		samples = append(samples, time.Since(begin))
	}
	serial := embPercentile(samples, 0.5)
	serialSnapshot := embTake(t, arm.addr)
	t.Logf("emb-smoke: pool arm serial request %s (samples %v)", serial, samples)

	stopWaiters := embWatch(arm.addr, []string{"EMB.STATS"}, "cpu_admission_waiters", 2*time.Millisecond)
	burst := embFire(arm.addr, embSingleTextCommands(fixture, embAdmissionCommands), 60*time.Second, -1)
	maxWaiters := stopWaiters()
	after := embTake(t, arm.addr)

	refusals := after.refusals - serialSnapshot.refusals
	waiters := maxWaiters - serialSnapshot.waiters
	// The registry's own bound: two waiters per admission unit.
	bound := embBudget * 2
	accounted := burst.completed + burst.refused + burst.failed
	gate.check(t, "queue-bounded",
		refusals > 0 && waiters > 0 && maxWaiters <= bound && accounted == embAdmissionCommands,
		"%d concurrent command(s): %d refusal(s) counted, queue depth peaked at %d (bound %d), %d/%d accounted for, p99 %s",
		embAdmissionCommands, refusals, maxWaiters, bound, accounted, embAdmissionCommands, burst.p99())
	return serial, serialSnapshot.waits
}

// embAdmissionBounded asserts a wait that cannot be granted inside the caller's
// bound ends as a counted refusal instead of running without limit. The bound is
// the measured serial request cost divided by three: a caller that has to queue
// waits at least one run, so the margin is the same order of magnitude on any
// host. serialWait is the pool arm's serial admission wait, which must be zero
// for work that never queued.
func embAdmissionBounded(t *testing.T, gate *embAssertions, fixture *embFixture, serial time.Duration, serialWait int) {
	t.Helper()
	off := 0
	admission := serial / 3
	if admission < time.Millisecond {
		admission = time.Millisecond
	}
	arm := embStart(t, embArmOptions{
		model:     embModelConfig(true, &off, 0, embPoolWorkers, 1),
		admission: admission,
		budget:    embBudget,
	})
	text := fixture.Texts[len(fixture.Texts)-1]
	_ = embEmbed(t, arm.addr, text)
	before := embTake(t, arm.addr)

	burst := embFire(arm.addr, embSingleTextCommands(fixture, embAdmissionCommands), 60*time.Second, -1)
	after := embTake(t, arm.addr)
	timeouts := after.timeouts - before.timeouts
	refusals := after.refusals - before.refusals
	waits := after.waits - before.waits
	// timeouts > 0 is the deadline detector: an unbounded wait never ends, so it
	// is never counted. The limit is the loose rail on how long a wait may
	// outlive its deadline — the runs it queued behind — not a tight bound on a
	// queue that is, by design, several deep.
	limit := admission + embAdmissionRunMultiple*serial

	gate.check(t, "admission-bounded",
		timeouts > 0 && refusals > 0 && burst.max() <= limit && burst.failed == 0,
		"admission bound %s: %d wait(s) ended without a grant, %d refusal(s) counted, max %s (limit %s), serial %s",
		admission, timeouts, refusals, burst.max(), limit, serial)
	gate.check(t, "admission-wait-reported",
		serialWait == 0 && waits > 0,
		"serial phase reported %d us of admission wait for work that never queued; the burst reported %d us (p99 %s, max %s, serial %s)",
		serialWait, waits, burst.p99(), burst.max(), serial)
}

// embLayoutConcurrency asserts the CPU policy's layout leaves room for two
// concurrent runs at a budget of two rather than one run holding the whole
// budget: two sessions of half the share, and both halves actually used when two
// requests arrive at once.
func embLayoutConcurrency(t *testing.T, gate *embAssertions, fixture *embFixture) {
	t.Helper()
	off := 0
	calibrate := false
	arm := embStart(t, embArmOptions{
		model:     embModelConfig(true, &off, 0, 0, 0),
		admission: config.DefaultAdmissionTimeout,
		budget:    embBudget,
		calibrate: &calibrate,
	})
	workers := embPairInt(t, arm.addr, "workers", "EMB.INFO", embModelName)
	threads := embPairInt(t, arm.addr, "intra_op_threads", "EMB.INFO", embModelName)
	share := embPairInt(t, arm.addr, "cpu_thread_share", "EMB.INFO", embModelName)

	// Sample well under a fixture run (~1–5 ms) and fire enough commands that the
	// runs actually overlap: with only two, the connection setup skew can exceed
	// a run, so the peak reads 1 even on a correct layout.
	stopPeak := embWatch(arm.addr, []string{"EMB.STATS"}, "cpu_active_tokens", 100*time.Microsecond)
	commands := embSingleTextCommands(fixture, 8)
	burst := embFire(arm.addr, commands, 30*time.Second, -1)
	peak := stopPeak()

	// The behavioural term is peak >= 2, not peak == budget: a single whole-budget
	// session (the defect) also reaches the budget, so the structural
	// workers >= 2 is what a pinned single-session layout breaks, and peak >= 2
	// states that two runs overlapped without restating policyPoolLayout.
	gate.check(t, "layout-concurrency",
		workers >= 2 && workers*threads <= embBudget && share > 0 && peak >= 2 && burst.completed == len(commands),
		"layout reports workers=%d intra_op_threads=%d share=%d at a budget of %d; %d concurrent runs reached %d active token(s), %d/%d completed",
		workers, threads, share, embBudget, len(commands), peak, burst.completed, len(commands))
}

// embFairness asserts the gate is work-conserving: a narrow waiter is admitted
// into idle capacity while an earlier, wider waiter is still blocked. Two models
// share one budget with explicit widths — "wide" at the whole budget, the fixture
// model at one thread — so the shapes are what the property is about rather than
// a timing artifact of one model.
//
// The arm parks the wide head behind a narrow run that holds one unit, then
// offers a narrow probe. Borrowing idle capacity serves the probe immediately;
// a gate that only ever admits the head makes it wait out the holder and the
// wide run, which is orders of magnitude more than its own measured serial cost.
func embFairness(t *testing.T, gate *embAssertions, fixture *embFixture) {
	t.Helper()
	off := 0
	calibrate := false
	const wideModel = "wide"
	arm := embStart(t, embArmOptions{
		// Two sessions of one thread, so the holder cannot block the probe in the
		// pool: the probe's wait is the gate's or nothing.
		model:     embModelConfig(true, &off, 0, 2, 1),
		extra:     map[string]config.ModelConfig{wideModel: embWideModelConfig(&off)},
		budget:    embFairBudget,
		calibrate: &calibrate,
	})
	short := fixture.Texts[0]
	long := fixture.Texts[len(fixture.Texts)-1]

	samples := make([]time.Duration, 0, embSerialSamples)
	for range embSerialSamples {
		begin := time.Now()
		_ = embEmbed(t, arm.addr, short)
		samples = append(samples, time.Since(begin))
	}
	serial := embPercentile(samples, 0.5)
	bound := embFairBoundMultiple * serial

	detail := "the wide head never blocked (the holder had already finished before the wide run parked)"
	for attempt := range embFairAttempts {
		// Each attempt widens the window between the launches: on a slow host the
		// holder is longer, but on a fast one the sleeps are what keep the head
		// parked while the probe arrives.
		gap := time.Duration(attempt+1) * 500 * time.Microsecond
		before := embTake(t, arm.addr)
		holder := embLaunch(arm.addr, 30*time.Second, "EMB", embModelName, long)
		time.Sleep(gap)
		wide := embLaunch(arm.addr, 30*time.Second, "EMB", wideModel, short)
		time.Sleep(gap)
		begin := time.Now()
		probe, err := embAsk(arm.addr, time.Now().Add(30*time.Second), "EMB", embModelName, short)
		probeLatency := time.Since(begin)
		_, holderLatency, holderErr := holder()
		wideReply, wideLatency, wideErr := wide()
		after := embTake(t, arm.addr)
		wideWait := after.waits - before.waits
		if err != nil || holderErr != nil || wideErr != nil || probe.err != "" || wideReply.err != "" {
			detail = fmt.Sprintf("attempt %d: holder %v, wide %v, probe %v (probe %v)", attempt, holderErr, wideErr, probe.err, err)
			continue
		}
		detail = fmt.Sprintf("attempt %d: the wide head never blocked; holder %s, wide %s, probe %s",
			attempt, holderLatency, wideLatency, probeLatency)
		if wideWait <= 0 {
			continue
		}
		// The head is parked: the probe's latency is the property under test.
		gate.check(t, "admission-fair",
			len(probe.bulk) == 1 && probeLatency <= bound,
			"wide (width %d of a budget of %d) waited %d us at the gate; the narrow probe was served in %s, within %s of its %s serial latency (holder %s, wide %s)",
			embFairWideThreads, embFairBudget, wideWait, probeLatency, bound, serial, holderLatency, wideLatency)
		return
	}
	gate.skip("admission-fair", detail)
}

// embWideModelConfig is the fairness arm's wide model: one session whose width
// fills the whole budget, so its runs are admitted alone and can be parked
// behind a narrower holder.
func embWideModelConfig(batching *int) config.ModelConfig {
	return embModelConfig(true, batching, 0, 1, embFairWideThreads)
}

// embBurstCommands builds the batch-on burst: each client sends several uncached
// texts in one command, so the burst exercises the coalescing path rather than
// the reply cache.
func embBurstCommands(fixture *embFixture) [][]string {
	commands := make([][]string, 0, embBurstClients)
	for i := range embBurstClients {
		texts := make([]string, 0, embBurstTextsPerClient)
		for j := range embBurstTextsPerClient {
			// The medium corpus text with a deterministic variant word: distinct
			// texts are cache misses, which is what makes the burst run inference.
			texts = append(texts, fixture.Texts[5]+" "+embVariants[(i+j)%len(embVariants)])
		}
		commands = append(commands, append([]string{"EMB", embModelName}, texts...))
	}
	return commands
}

// embSingleTextCommands builds one long-text command per client, which is the
// shape the admission gate sees: one run per command, all of them offered at
// once.
func embSingleTextCommands(fixture *embFixture, n int) [][]string {
	long := fixture.Texts[len(fixture.Texts)-1]
	commands := make([][]string, 0, n)
	for i := range n {
		commands = append(commands, []string{"EMB", embModelName, long + " " + embVariants[i%len(embVariants)]})
	}
	return commands
}

// embVariants are words the fixture tokenizer knows, used to make burst texts
// distinct (and therefore cache misses) without randomness.
var embVariants = []string{"alpha", "beta", "gamma", "delta", "mid", "same", "label", "number"}
