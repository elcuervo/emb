package embstress

import (
	"strings"
	"testing"

	"github.com/elcuervo/emb/internal/embtop"
)

func entries(rows ...any) []embtop.ModelListEntry {
	var out []embtop.ModelListEntry
	for i := 0; i+1 < len(rows); i += 2 {
		out = append(out, embtop.ModelListEntry{
			Name:   rows[i].(string),
			Dim:    rows[i+1].(int),
			Status: "ready",
		})
	}
	return out
}

func TestReconcileCleanFleet(t *testing.T) {
	pairs, drift := Reconcile(
		[]ConfiguredModel{{Name: "minilm", Dim: 384}},
		map[string][]embtop.ModelListEntry{
			"a": entries("minilm", 384),
			"b": entries("minilm", 384),
		},
	)
	if len(drift) != 0 {
		t.Fatalf("a matching fleet reported drift: %v", drift)
	}
	if len(pairs) != 2 {
		t.Fatalf("pairs = %+v, want one per endpoint", pairs)
	}
	for _, p := range pairs {
		if p.Model != "minilm" || p.Dim != 384 {
			t.Errorf("pair = %+v, want minilm at 384", p)
		}
	}
}

func TestReconcileNamesAMissingModel(t *testing.T) {
	pairs, drift := Reconcile(
		[]ConfiguredModel{{Name: "minilm", Dim: 384}},
		map[string][]embtop.ModelListEntry{
			"a": entries("minilm", 384),
			"b": entries("bge", 384),
		},
	)
	if len(pairs) != 1 || pairs[0].Endpoint != "a" {
		t.Fatalf("pairs = %+v, want only endpoint a", pairs)
	}
	if len(drift) != 1 {
		t.Fatalf("drift = %v, want one", drift)
	}
	d := drift[0]
	if d.Endpoint != "b" || d.Model != "minilm" || !strings.Contains(d.Detail, "not reported") {
		t.Fatalf("drift = %+v, want endpoint b missing minilm", d)
	}
}

func TestReconcileNamesADimensionMismatch(t *testing.T) {
	pairs, drift := Reconcile(
		[]ConfiguredModel{{Name: "minilm", Dim: 384}},
		map[string][]embtop.ModelListEntry{
			"a": entries("minilm", 384),
			"b": entries("minilm", 768),
		},
	)
	if len(pairs) != 1 || pairs[0].Endpoint != "a" {
		t.Fatalf("pairs = %+v, want only endpoint a", pairs)
	}
	if len(drift) != 1 {
		t.Fatalf("drift = %v, want one", drift)
	}
	d := drift[0]
	if d.Endpoint != "b" || !strings.Contains(d.Detail, "768") || !strings.Contains(d.Detail, "384") {
		t.Fatalf("drift = %+v, want both dimensions named", d)
	}
}

// TestReconcileCrossEndpointAgreement covers a model whose dimension the
// configuration does not state (the server auto-detects it at load): the
// endpoints are compared against each other instead of against a missing
// expectation, so a node serving a different build is still caught.
func TestReconcileCrossEndpointAgreement(t *testing.T) {
	pairs, drift := Reconcile(
		[]ConfiguredModel{{Name: "auto"}},
		map[string][]embtop.ModelListEntry{
			"a": entries("auto", 384),
			"b": entries("auto", 384),
			"c": entries("auto", 768),
		},
	)
	if len(drift) != 1 || drift[0].Endpoint != "c" {
		t.Fatalf("drift = %v, want the disagreeing endpoint c", drift)
	}
	if !strings.Contains(drift[0].Detail, "fleet") {
		t.Errorf("drift detail does not say the expectation came from the fleet: %q", drift[0].Detail)
	}
	if len(pairs) != 2 {
		t.Fatalf("pairs = %+v, want the two agreeing endpoints", pairs)
	}
	for _, p := range pairs {
		if p.Dim != 384 {
			t.Errorf("pair %+v did not take the agreed dimension 384", p)
		}
	}
}

func TestReconcileAgreeingEndpointsWithNoStatedDim(t *testing.T) {
	pairs, drift := Reconcile(
		[]ConfiguredModel{{Name: "auto"}},
		map[string][]embtop.ModelListEntry{
			"a": entries("auto", 384),
			"b": entries("auto", 384),
		},
	)
	if len(drift) != 0 {
		t.Fatalf("agreement was reported as drift: %v", drift)
	}
	if len(pairs) != 2 {
		t.Fatalf("pairs = %+v, want both endpoints", pairs)
	}
}

func TestReconcileOutputIsStable(t *testing.T) {
	// Map iteration must not reorder the pairs, or two runs of the same fleet
	// would report differently.
	reported := map[string][]embtop.ModelListEntry{
		"c": entries("m", 8), "a": entries("m", 8), "b": entries("m", 8),
	}
	want := []string{"a", "b", "c"}
	for range 20 {
		pairs, _ := Reconcile([]ConfiguredModel{{Name: "m", Dim: 8}}, reported)
		if len(pairs) != len(want) {
			t.Fatalf("pairs = %+v", pairs)
		}
		for i, ep := range want {
			if pairs[i].Endpoint != ep {
				t.Fatalf("pairs are not in a stable order: %+v", pairs)
			}
		}
	}
}

// TestReconcileRefusesANotReadyModel guards that a run never reports a server's
// own startup as load: a model still initializing is not a model the endpoint
// serves yet.
func TestReconcileRefusesANotReadyModel(t *testing.T) {
	rows := []embtop.ModelListEntry{
		{Name: "m", Dim: 8, Status: "uninitialized"},
		{Name: "n", Dim: 8, Status: "loading"},
		{Name: "o", Dim: 8, Status: "failed: no such file"},
	}
	pairs, drift := Reconcile(
		[]ConfiguredModel{{Name: "m", Dim: 8}, {Name: "n", Dim: 8}, {Name: "o", Dim: 8}},
		map[string][]embtop.ModelListEntry{"a": rows},
	)
	if len(pairs) != 0 {
		t.Fatalf("a not-ready fleet was exercised: %+v", pairs)
	}
	if len(drift) != 3 {
		t.Fatalf("drift = %v, want every not-ready model named", drift)
	}
	for i, want := range []string{"uninitialized", "loading", "failed: no such file"} {
		if !strings.Contains(drift[i].Detail, want) {
			t.Errorf("drift %d does not name the status %q: %v", i, want, drift[i])
		}
	}
}

// A server that reports no status at all is exercised: the field is not a
// readiness gate for a build that never published one.
func TestReconcileExercisesAModelWithNoReportedStatus(t *testing.T) {
	pairs, drift := Reconcile(
		[]ConfiguredModel{{Name: "m", Dim: 8}},
		map[string][]embtop.ModelListEntry{"a": {{Name: "m", Dim: 8}}},
	)
	if len(drift) != 0 {
		t.Fatalf("drift = %v, want none", drift)
	}
	if len(pairs) != 1 {
		t.Fatalf("pairs = %+v, want one", pairs)
	}
}
