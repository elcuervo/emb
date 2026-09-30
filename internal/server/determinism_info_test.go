package server

import (
	"strings"
	"testing"
	"time"

	"github.com/elcuervo/emb/internal/onnx"
	"github.com/elcuervo/emb/internal/pipeline"
	"github.com/elcuervo/emb/internal/registry"
)

// serveDeterminismInfo starts a server with three models exposing the three
// probe verdicts: "ok" (probe passed, batching on), "deg" (probe failed,
// degraded to worker pool), "off" (batching disabled, untested). The pools
// use the existing mockSession (batch-invariant); the verdict fields on the
// entries are what the tests exercise.
func serveDeterminismInfo(t *testing.T) string {
	t.Helper()
	reg := registry.New()

	add := func(name string, timeoutMS int, verdict, reason string) {
		pool, err := pipeline.NewPool(
			func() (onnx.Session, error) { return &mockSession{}, nil },
			mockTokenizer{}, 2, 4, 128, true, "mean", timeoutMS, 32, 0, 0,
		)
		if err != nil {
			t.Fatal(err)
		}
		reg.Add(name, &registry.ModelEntry{
			Pool:                   pool,
			Dim:                    4,
			Name:                   name,
			BatchDeterminism:       verdict,
			BatchDeterminismReason: reason,
		})
	}
	add("ok", 5, "passed", "passed")
	add("deg", 0, "failed", "dql_batch_dependence")
	add("off", 0, "untested", "untested")

	addr := getFreeAddr()
	srv := New(addr, reg, "", "", nil)
	go srv.ListenAndServe()
	t.Cleanup(func() { srv.Close() })
	time.Sleep(50 * time.Millisecond)
	return addr
}

func TestINFOSurfacesBatchDeterminism(t *testing.T) {
	addr := serveDeterminismInfo(t)
	conn := dial(t, addr)
	defer conn.Close()

	cases := []struct {
		model                   string
		wantVerdict, wantReason string
		wantEffectiveTimeoutMS  int
	}{
		{"ok", "passed", "passed", 5},
		{"deg", "failed", "dql_batch_dependence", 0},
		{"off", "untested", "untested", 0},
	}
	for _, tc := range cases {
		conn.Write(respCommand("EMB.INFO", tc.model))
		fields := statsFields(t, parseRESP(t, readRESP(t, conn)))
		gotVerdict := bulkOf(t, fields["batch_determinism"])
		gotReason := bulkOf(t, fields["batch_determinism_reason"])
		gotTimeout := intOf(t, fields["batching_timeout_ms"])
		if gotVerdict != tc.wantVerdict || gotReason != tc.wantReason {
			t.Errorf("%s: batch_determinism=%q reason=%q, want %q/%q",
				tc.model, gotVerdict, gotReason, tc.wantVerdict, tc.wantReason)
		}
		if gotTimeout != tc.wantEffectiveTimeoutMS {
			t.Errorf("%s: batching_timeout_ms=%d, want %d (effective timeout after gating)",
				tc.model, gotTimeout, tc.wantEffectiveTimeoutMS)
		}
	}
}

func TestSTATSPerModelIncludesDeterminism(t *testing.T) {
	addr := serveDeterminismInfo(t)
	conn := dial(t, addr)
	defer conn.Close()

	conn.Write(respCommand("EMB.STATS"))
	body := readRESP(t, conn)
	for _, want := range []string{"det=passed", "det=failed", "det=untested"} {
		if !strings.Contains(body, want) {
			t.Errorf("EMB.STATS missing %q in:\n%s", want, body)
		}
	}
}
