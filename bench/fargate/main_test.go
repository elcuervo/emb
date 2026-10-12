package main

// Unit tests for the evidence bundle and its comparability rules. They run with
// no Docker, no model download and no network: the host contract is exercised
// through synthetic reports, and the diff through fixture bundles written to a
// temporary directory.

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"strconv"
	"strings"
	"testing"

	"github.com/elcuervo/emb/bench/hostcheck"
)

// ---- fixtures ----

// qualifiedReport is the validation host contract satisfied: the shape every
// test that needs a usable host starts from. It is synthetic on purpose — the
// tests describe the contract, they do not measure the machine running them.
func qualifiedReport() hostcheck.Report {
	r := hostcheck.Report{
		OS:            "linux",
		Arch:          "arm64",
		PresentedCPUs: 4,
		Allowance: hostcheck.AllowanceView{
			CPUs: 4, Source: "/sys/fs/cgroup/cpu.max", Quota: true, Scheduler: 4, Affinity: 4,
		},
		Memory:       hostcheck.MemoryView{LimitBytes: 8 << 30, HostBytes: 16 << 30, Source: "cgroup"},
		ServerBudget: 4,
		Steal:        hostcheck.StealView{Measurable: true, Percent: 0.10, Verdict: hostcheck.VerdictOK},
		Throttle:     hostcheck.ThrottleView{Measurable: true, Ratio: 0.01, Periods: 100, Throttled: 1, Verdict: hostcheck.VerdictOK},
		Thresholds:   hostcheck.DefaultThresholds(),
	}
	r.Qualify()
	return r
}

// unqualifiedReport is the development laptop: not the deployment platform, and
// neither steal nor throttle measurable.
func unqualifiedReport() hostcheck.Report {
	r := qualifiedReport()
	r.OS = "darwin"
	r.Steal = hostcheck.StealView{Verdict: hostcheck.VerdictUnmeasured}
	r.Throttle = hostcheck.ThrottleView{Verdict: hostcheck.VerdictUnmeasured}
	r.Qualify()
	return r
}

func testHost() hostBundle { return hostBundleFrom(qualifiedReport()) }

// otherHost is a second, different measurement host (more presented CPUs).
func otherHost() hostBundle {
	r := qualifiedReport()
	r.PresentedCPUs, r.Allowance.CPUs, r.ServerBudget = 8, 8, 8
	return hostBundleFrom(r)
}

func testModel() modelInputs {
	return modelInputs{
		Name:      "minilm",
		ONNX:      fileDigest{Path: "models/minilm/model.onnx", Bytes: 90_000_000, SHA256: strings.Repeat("ab", 32)},
		Tokenizer: fileDigest{Path: "models/minilm/tokenizer.json", Bytes: 700_000, SHA256: strings.Repeat("cd", 32)},
	}
}

func testBundle(host hostBundle, cells ...benchCell) *result {
	return &result{
		Revision: "0123abc",
		Created:  "2026-10-12T00:00:00Z",
		Host:     host,
		Model:    testModel(),
		Config: harnessConfig{
			Platform: "linux/arm64", Model: "minilm", MaxLength: 512,
			CPUs: []int{1, 2}, Clients: []int{1}, Pipelines: []int{1},
			CountPerCell: 500, ReqGate: 0.05, P50Gate: 0.10,
		},
		Cells: cells,
		Assertions: []assertion{
			{Name: assertHostContract, Outcome: outcomePass, Detail: "qualified linux/arm64"},
			{Name: assertCellsComplete, Outcome: outcomePass, Detail: "2 of 2 cells measured"},
		},
	}
}

func testCell(reqs, p50 float64) benchCell {
	return benchCell{
		Tier: 1, Workload: "fixed-length", Clients: 1, Pipeline: 1,
		Reqs: reqs, P50: p50, P90: p50 * 2, P99: p50 * 4,
	}
}

func writeBundleFile(t *testing.T, dir, name string, res *result) string {
	t.Helper()
	data, err := json.MarshalIndent(res, "", "  ")
	if err != nil {
		t.Fatalf("marshal fixture: %v", err)
	}
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatalf("write fixture: %v", err)
	}
	return path
}

// captureStdout runs fn with stdout redirected and returns what it printed: the
// diff and the refusal both report by printing, so the tests read the report.
func captureStdout(t *testing.T, fn func()) string {
	t.Helper()
	old := os.Stdout
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("pipe: %v", err)
	}
	os.Stdout = w
	done := make(chan string, 1)
	go func() {
		b, _ := io.ReadAll(r)
		done <- string(b)
	}()
	fn()
	if err := w.Close(); err != nil {
		t.Fatalf("close pipe: %v", err)
	}
	os.Stdout = old
	return <-done
}

func mustHavePath(t *testing.T, doc map[string]any, path string) {
	t.Helper()
	var cur any = doc
	for _, part := range strings.Split(path, ".") {
		switch node := cur.(type) {
		case map[string]any:
			v, ok := node[part]
			if !ok {
				t.Errorf("bundle JSON has no %s", path)
				return
			}
			cur = v
		case []any:
			i, err := strconv.Atoi(part)
			if err != nil || i < 0 || i >= len(node) {
				t.Errorf("bundle JSON has no %s", path)
				return
			}
			cur = node[i]
		default:
			t.Errorf("bundle JSON has no %s (stopped at %T)", path, cur)
			return
		}
	}
}

// ---- bundle schema ----

func TestBundleRoundTrips(t *testing.T) {
	pad := 0.83
	want := testBundle(testHost(),
		testCell(239.35, 1.367),
		benchCell{Tier: 2, Workload: "mixed-length", Clients: 8, Pipeline: 1, Reqs: 120.5, P50: 8.2, P90: 20.1, P99: 61.4, PadEff: &pad},
	)

	data, err := json.MarshalIndent(want, "", "  ")
	if err != nil {
		t.Fatalf("marshal bundle: %v", err)
	}
	var got result
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatalf("unmarshal bundle: %v", err)
	}
	if !reflect.DeepEqual(*want, got) {
		t.Fatalf("bundle did not round-trip:\n got %#v\nwant %#v", got, *want)
	}
}

func TestBundleNamesEveryFieldTheDiffReads(t *testing.T) {
	pad := 0.5
	b := testBundle(testHost(),
		benchCell{Tier: 1, Workload: "mixed-length", Clients: 1, Pipeline: 1, Reqs: 120, P50: 8, P90: 20, P99: 60, PadEff: &pad},
		testCell(239.35, 1.367),
	)
	raw, err := json.Marshal(b)
	if err != nil {
		t.Fatalf("marshal bundle: %v", err)
	}
	var doc map[string]any
	if err := json.Unmarshal(raw, &doc); err != nil {
		t.Fatalf("unmarshal bundle: %v", err)
	}

	// The fields the diff reads, named individually: a refusal and a verdict
	// both depend on them.
	for _, path := range []string{
		"host.identity", "host.os", "host.arch",
		"cells.0.tier", "cells.0.workload", "cells.0.clients", "cells.0.pipeline",
		"cells.0.req_s", "cells.0.p50_ms", "cells.0.p90_ms", "cells.0.p99_ms",
		"cells.0.padding_efficiency",
	} {
		mustHavePath(t, doc, path)
	}

	// The rest of the recorded provenance.
	for _, path := range []string{
		"revision", "created",
		"host.presented_cpus", "host.server_budget", "host.memory_limit_bytes",
		"host.allowance.cpus", "host.allowance.source", "host.allowance.quota",
		"host.steal_verdict", "host.throttle_verdict", "host.gold",
		"model.name", "model.onnx.path", "model.onnx.bytes", "model.onnx.sha256",
		"model.tokenizer.path", "model.tokenizer.bytes", "model.tokenizer.sha256",
		"config.platform", "config.model", "config.max_length",
		"config.cpus", "config.clients", "config.pipelines", "config.count_per_cell",
		"config.req_gate", "config.p50_gate",
		"assertions.0.name", "assertions.0.outcome", "assertions.0.detail",
	} {
		mustHavePath(t, doc, path)
	}
}

func TestHostBundleCarriesTheFingerprintAndItsVerdict(t *testing.T) {
	q := qualifiedReport()
	for _, h := range []hostBundle{hostBundleFrom(q), testHost()} {
		if h.Identity == "" {
			t.Fatal("a recorded host must carry an identity digest")
		}
		if h.Identity != h.Fingerprint.Identity() {
			t.Errorf("identity = %q, want the fingerprint digest %q", h.Identity, h.Fingerprint.Identity())
		}
		if !h.Gold {
			t.Error("a qualified host must be recorded gold")
		}
		if h.OS != "linux" || h.Arch != "arm64" || h.Allowance.CPUs != 4 || h.Allowance.Source == "" {
			t.Errorf("fingerprint must name os/arch/allowance and its source: %#v", h.Fingerprint)
		}
		if h.PresentedCPUs == 0 || h.ServerBudget == 0 || h.MemoryLimit == 0 {
			t.Errorf("fingerprint must name the CPU and memory shape: %#v", h.Fingerprint)
		}
	}

	u := unqualifiedReport()
	h := hostBundleFrom(u)
	if h.Gold {
		t.Error("an unqualified host must not be recorded gold")
	}
	if h.FailedProperty != u.FailedProperty || h.FailedProperty == "" {
		t.Errorf("failed property = %q, want %q", h.FailedProperty, u.FailedProperty)
	}
	if len(h.FailedProperties) != len(u.FailedProperties) {
		t.Errorf("failed properties = %v, want %v", h.FailedProperties, u.FailedProperties)
	}
}

// ---- assertions ----

func TestHostContractAssertion(t *testing.T) {
	pass := hostContractAssertion(qualifiedReport())
	if pass.Name != assertHostContract || pass.Outcome != outcomePass {
		t.Fatalf("qualified host: assertion = %#v, want pass", pass)
	}

	u := unqualifiedReport()
	fail := hostContractAssertion(u)
	if fail.Name != assertHostContract || fail.Outcome != outcomeFail {
		t.Fatalf("unqualified host: assertion = %#v, want fail", fail)
	}
	if !strings.Contains(fail.Detail, u.FailedProperty) {
		t.Errorf("fail detail %q must name the failed property %q", fail.Detail, u.FailedProperty)
	}
}

func TestCellsCompleteNeverRecordsAnUnfinishedRunAsAPass(t *testing.T) {
	if a := cellsCompleteAssertion(28, 28, nil); a.Name != assertCellsComplete || a.Outcome != outcomePass {
		t.Errorf("complete matrix: assertion = %#v, want pass", a)
	}

	short := cellsCompleteAssertion(28, 14, nil)
	if short.Outcome != outcomeInvalid {
		t.Errorf("incomplete matrix: outcome = %q, want %q", short.Outcome, outcomeInvalid)
	}
	if !strings.Contains(short.Detail, "14 of 28") {
		t.Errorf("invalid detail %q must name the shortfall", short.Detail)
	}

	broken := cellsCompleteAssertion(28, 0, errors.New("docker run: no such image"))
	if broken.Outcome != outcomeInvalid {
		t.Errorf("aborted run: outcome = %q, want %q", broken.Outcome, outcomeInvalid)
	}
	if !strings.Contains(broken.Detail, "no such image") {
		t.Errorf("invalid detail %q must name the cause", broken.Detail)
	}
	for _, a := range []assertion{short, broken} {
		if a.Outcome == outcomePass {
			t.Fatalf("an unmet precondition was recorded as a pass: %#v", a)
		}
	}
}

func TestCheckAssertionsRejectsAnOutcomeOutsideTheSchema(t *testing.T) {
	for _, o := range []string{outcomePass, outcomeFail, outcomeInvalid, outcomeSkipped} {
		if err := checkAssertions([]assertion{{Name: "x", Outcome: o}}); err != nil {
			t.Errorf("outcome %q rejected: %v", o, err)
		}
	}
	if err := checkAssertions([]assertion{{Name: "x", Outcome: "green"}}); err == nil {
		t.Error("an outcome outside pass|fail|invalid|skipped must not be recorded")
	}
}

func TestExpectedCells(t *testing.T) {
	o := &options{cpus: []int{1, 2}, clients: []int{1, 8}, pipelines: []int{1, 8}}
	// per tier: 3 * pipelines * clients + clients = 3*2*2 + 2 = 14
	if got := expectedCells(o); got != 28 {
		t.Errorf("expectedCells = %d, want 28", got)
	}
	single := &options{cpus: []int{1}, clients: []int{1}, pipelines: []int{1}}
	if got := expectedCells(single); got != 4 {
		t.Errorf("expectedCells = %d, want 4", got)
	}
}

func TestModelDigestsHashTheMeasuredFiles(t *testing.T) {
	dir := t.TempDir()
	onnx, tok := []byte("onnx weights"), []byte(`{"tokenizer":true}`)
	if err := os.WriteFile(filepath.Join(dir, "model.onnx"), onnx, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "tokenizer.json"), tok, 0o600); err != nil {
		t.Fatal(err)
	}

	got, err := modelDigests(&options{modelDir: dir, model: "minilm"})
	if err != nil {
		t.Fatalf("modelDigests: %v", err)
	}
	wantONNX := sha256.Sum256(onnx)
	wantTok := sha256.Sum256(tok)
	if got.Name != "minilm" {
		t.Errorf("name = %q, want minilm", got.Name)
	}
	if got.ONNX.Bytes != int64(len(onnx)) || got.ONNX.SHA256 != hex.EncodeToString(wantONNX[:]) {
		t.Errorf("onnx digest = %#v, want %d bytes / %s", got.ONNX, len(onnx), hex.EncodeToString(wantONNX[:]))
	}
	if got.Tokenizer.Bytes != int64(len(tok)) || got.Tokenizer.SHA256 != hex.EncodeToString(wantTok[:]) {
		t.Errorf("tokenizer digest = %#v, want %d bytes / %s", got.Tokenizer, len(tok), hex.EncodeToString(wantTok[:]))
	}
	if got.ONNX.Path != filepath.Join(dir, "model.onnx") {
		t.Errorf("onnx path = %q, want the file that was hashed", got.ONNX.Path)
	}

	if _, err := modelDigests(&options{modelDir: filepath.Join(dir, "missing")}); err == nil {
		t.Error("a missing model must fail the run, not produce an empty digest")
	}
}

// ---- baseline refusal ----

func TestBaselineRefusalNamesTheFailedProperty(t *testing.T) {
	q, u := qualifiedReport(), unqualifiedReport()
	cases := []struct {
		name    string
		mode    string
		report  hostcheck.Report
		wantErr bool
	}{
		{"baseline on a qualified host", "baseline", q, false},
		{"baseline on an unqualified host", "baseline", u, true},
		{"run on an unqualified host", "run", u, false},
		{"run on a qualified host", "run", q, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := baselineRefusal(tc.mode, tc.report)
			if !tc.wantErr {
				if err != nil {
					t.Fatalf("baselineRefusal(%q) = %v, want nil", tc.mode, err)
				}
				return
			}
			if err == nil {
				t.Fatal("expected a refusal")
			}
			if !strings.Contains(err.Error(), tc.report.FailedProperty) {
				t.Errorf("refusal %q must name the failed property %q", err, tc.report.FailedProperty)
			}
		})
	}
}

func TestUnqualifiedBaselineRefusesBeforeWritingAnything(t *testing.T) {
	out := t.TempDir()
	o := &options{mode: "baseline", outDir: out}
	unqualified := unqualifiedReport()

	// Where resultPath would persist a baseline if the run were allowed.
	target := filepath.Join(findBaselineDir(t), "baseline."+gitSha()+".json")
	existed := func() bool {
		_, err := os.Stat(target)
		return err == nil
	}
	wasThere := existed()

	var err error
	printed := captureStdout(t, func() { err = runMain(o, unqualified) })
	if err == nil {
		t.Fatal("runMain captured a baseline on an unqualified host")
	}
	if !strings.Contains(printed, "verdict: "+hostcheck.Unqualified) {
		t.Errorf("the refusal must print the verdict, got:\n%s", printed)
	}
	if !strings.Contains(printed, unqualified.FailedProperty) {
		t.Errorf("the refusal must print the failed property, got:\n%s", printed)
	}

	entries, rerr := os.ReadDir(out)
	if rerr != nil {
		t.Fatalf("read out dir: %v", rerr)
	}
	if len(entries) != 0 {
		t.Fatalf("a refused baseline wrote %d entries to %s", len(entries), out)
	}
	if existed() != wasThere {
		t.Errorf("a refused baseline created or removed %s", target)
	}
}

// findBaselineDir is where resultPath would persist a baseline.
func findBaselineDir(t *testing.T) string {
	t.Helper()
	root, err := findRepoRoot()
	if err != nil {
		t.Fatalf("findRepoRoot: %v", err)
	}
	return filepath.Join(root, "bench", "fargate")
}

// ---- diff refusal ----

func TestDiffRefusesDifferingFingerprintsAndPrintsBoth(t *testing.T) {
	dir := t.TempDir()
	before := testBundle(testHost(), testCell(239.35, 1.367))
	after := testBundle(otherHost(), testCell(239.35, 1.367))
	if before.Host.Identity == after.Host.Identity {
		t.Fatal("fixture hosts must differ")
	}
	pb := writeBundleFile(t, dir, "before.json", before)
	pa := writeBundleFile(t, dir, "after.json", after)

	var ok bool
	var err error
	printed := captureStdout(t, func() { ok, err = diffBundles(pb, pa, 0.05, 0.10) })
	if err == nil {
		t.Fatal("the diff compared two bundles from different hosts")
	}
	if ok {
		t.Error("a refused comparison must not report a cell verdict")
	}
	for _, want := range []string{
		before.Host.Identity, after.Host.Identity, // both digests
		pb, pa, // which file was which
		"presented CPUs  4", "presented CPUs  8", // both readable shapes
		"linux/arm64", "identity",
	} {
		if !strings.Contains(printed, want) {
			t.Errorf("refusal did not print %q:\n%s", want, printed)
		}
	}
}

func TestDiffRefusesALegacyBundleWithoutAFingerprint(t *testing.T) {
	dir := t.TempDir()
	after := testBundle(testHost(), testCell(239.35, 1.367))
	pa := writeBundleFile(t, dir, "after.json", after)

	// The committed baseline schema, verbatim in shape: host {os, arch, gold},
	// no identity, top-level count_per_cell. It predates fingerprints, so it
	// cannot be compared with anything.
	legacy := `{
  "sha": "7b16edc",
  "created": "2026-09-01T01:55:29Z",
  "platform": "linux/arm64",
  "host": {"os": "darwin", "arch": "arm64", "gold": false},
  "count_per_cell": 500,
  "cells": [{"tier": 1, "workload": "fixed-length", "clients": 1, "pipeline": 1,
             "req_s": 239.35, "p50_ms": 1.367, "p90_ms": 1.871, "p99_ms": 70.783}]
}`
	pb := filepath.Join(dir, "legacy.json")
	if err := os.WriteFile(pb, []byte(legacy), 0o600); err != nil {
		t.Fatal(err)
	}

	var ok bool
	var err error
	printed := captureStdout(t, func() { ok, err = diffBundles(pb, pa, 0.05, 0.10) })
	if err == nil {
		t.Fatal("a bundle with no fingerprint was compared")
	}
	if ok {
		t.Error("a refused comparison must not report a cell verdict")
	}
	if !strings.Contains(err.Error(), "no host fingerprint") {
		t.Errorf("refusal = %q, want it to name the missing fingerprint", err)
	}
	if !strings.Contains(printed, pb) || !strings.Contains(printed, `identity        ""`) {
		t.Errorf("refusal must print both fingerprints, including the empty one:\n%s", printed)
	}

	// A legacy bundle against another legacy bundle is refused too.
	pb2 := filepath.Join(dir, "legacy2.json")
	if err := os.WriteFile(pb2, []byte(legacy), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := diffBundles(pb, pb2, 0.05, 0.10); err == nil {
		t.Error("two legacy bundles must not be compared")
	}
}

func TestDiffRefusesTheCommittedLegacyBaselines(t *testing.T) {
	for _, legacy := range []string{"baseline.7b16edc.json", "baseline.42cf74a.json"} {
		t.Run(legacy, func(t *testing.T) {
			if _, err := os.Stat(legacy); err != nil {
				t.Skipf("%s is not committed here: %v", legacy, err)
			}
			after := testBundle(testHost(), testCell(239.35, 1.367))
			pa := writeBundleFile(t, t.TempDir(), "after.json", after)

			var err error
			printed := captureStdout(t, func() { _, err = diffBundles(legacy, pa, 0.05, 0.10) })
			if err == nil {
				t.Fatalf("%s (no fingerprint) was compared against a fingerprinted bundle", legacy)
			}
			if !strings.Contains(err.Error(), "no host fingerprint") {
				t.Errorf("refusal %q must say the legacy bundle carries no fingerprint", err)
			}
			if !strings.Contains(printed, legacy) || !strings.Contains(printed, `identity        ""`) {
				t.Errorf("refusal must print both fingerprints, including the empty one:\n%s", printed)
			}
		})
	}
}

func TestDiffAcceptsMatchingFingerprints(t *testing.T) {
	dir := t.TempDir()
	before := testBundle(testHost(), testCell(239.35, 1.367))
	// The identity covers the stable host shape only: a steal or throttle
	// verdict that followed the load on the host must not split a comparable
	// pair.
	after := testBundle(testHost(), testCell(236.0, 1.40))
	after.Host.StealVerdict = hostcheck.VerdictHigh
	if before.Host.Identity != after.Host.Identity {
		t.Fatal("the same host shape must produce the same identity")
	}
	pb := writeBundleFile(t, dir, "before.json", before)
	pa := writeBundleFile(t, dir, "after.json", after)

	var ok bool
	var err error
	printed := captureStdout(t, func() { ok, err = diffBundles(pb, pa, 0.05, 0.10) })
	if err != nil {
		t.Fatalf("matching fingerprints were refused: %v", err)
	}
	if !ok {
		t.Errorf("a within-tolerance pair must pass:\n%s", printed)
	}
}

// ---- diff tolerances ----

func TestDiffTolerancesPerCell(t *testing.T) {
	cases := []struct {
		name   string
		before benchCell
		after  benchCell
		want   bool
	}{
		{"req/s just inside -5%", testCell(239.35, 1.367), testCell(239.35*0.951, 1.367), true},
		{"req/s just outside -5%", testCell(239.35, 1.367), testCell(239.35*0.949, 1.367), false},
		{"p50 just inside +10%", testCell(239.35, 1.367), testCell(239.35, 1.367*1.099), true},
		{"p50 just outside +10%", testCell(239.35, 1.367), testCell(239.35, 1.367*1.101), false},
		{"saturated cell gates p50 only", testCell(6000, 0.4), testCell(3000, 0.4*1.09), true},
		{"saturated cell still gates p50", testCell(6000, 0.4), testCell(6000, 0.4*1.11), false},
		{"an improvement passes", testCell(239.35, 1.367), testCell(300, 1.0), true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			before := testBundle(testHost(), tc.before)
			after := testBundle(testHost(), tc.after)
			var got bool
			captureStdout(t, func() { got = diffRun(before, after, 0.05, 0.10) })
			if got != tc.want {
				t.Errorf("diffRun = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestDiffFailsOnACellMissingFromTheCandidate(t *testing.T) {
	before := testBundle(testHost(), testCell(239.35, 1.367),
		benchCell{Tier: 1, Workload: "unique-text", Clients: 1, Pipeline: 1, Reqs: 194.78, P50: 1.647})
	after := testBundle(testHost(), testCell(239.35, 1.367))

	var got bool
	captureStdout(t, func() { got = diffRun(before, after, 0.05, 0.10) })
	if got {
		t.Error("a cell missing from the candidate must fail the diff")
	}
}
