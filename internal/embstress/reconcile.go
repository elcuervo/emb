package embstress

import (
	"fmt"
	"sort"

	"github.com/elcuervo/emb/internal/embtop"
)

// ConfiguredModel is one model the run expects the fleet to serve, taken from
// the emb server configuration. Dim is 0 when the configuration does not state
// one, because the server derives it from the ONNX graph at load.
type ConfiguredModel struct {
	Name      string
	Dim       int
	MaxLength int // 0 when the configuration does not state one
}

// Pair is one (endpoint, model) workload the run will exercise.
type Pair struct {
	Endpoint string
	Model    string
	Dim      int // the expected dimension, 0 when the fleet could not agree on one
}

// Drift is one reconciliation failure: an endpoint that will not be exercised
// for a model, and why.
type Drift struct {
	Endpoint string
	Model    string
	Detail   string
}

// String renders one drift line.
func (d Drift) String() string {
	return fmt.Sprintf("%s: %s: %s", d.Endpoint, d.Model, d.Detail)
}

// modelStatusReady is the readiness EMB.MODELS reports for a model that can
// serve. The server's other answers are "loading", "uninitialized" and
// "failed: <reason>".
const modelStatusReady = "ready"

// Reconcile compares the configured models with what each endpoint reports and
// returns the pairs to exercise plus the drift it found. The configuration is
// the expectation and the fleet is the truth:
//
//   - a model the configuration names but an endpoint does not report drops
//     that endpoint's pair, so a node is never exercised under a name it does
//     not serve;
//   - when the configuration states a dimension, that is the expectation and a
//     different one drops the pair;
//   - when it does not — a model whose dimension is auto-detected at load —
//     the endpoints are compared against each other, so disagreement between
//     them is still caught and the majority dimension is the expectation.
func Reconcile(configured []ConfiguredModel, reported map[string][]embtop.ModelListEntry) ([]Pair, []Drift) {
	endpoints := make([]string, 0, len(reported))
	for ep := range reported {
		endpoints = append(endpoints, ep)
	}
	sort.Strings(endpoints)

	byEndpoint := make(map[string]map[string]embtop.ModelListEntry, len(reported))
	for ep, entries := range reported {
		m := make(map[string]embtop.ModelListEntry, len(entries))
		for _, e := range entries {
			m[e.Name] = e
		}
		byEndpoint[ep] = m
	}

	var pairs []Pair
	var drift []Drift
	for _, cm := range configured {
		expect := cm.Dim
		if expect == 0 {
			expect = majorityDim(endpoints, byEndpoint, cm.Name)
		}
		for _, ep := range endpoints {
			entry, ok := byEndpoint[ep][cm.Name]
			if !ok {
				drift = append(drift, Drift{Endpoint: ep, Model: cm.Name, Detail: "model not reported by the endpoint"})
				continue
			}
			// A model the endpoint has not finished loading is not a model it
			// serves yet. Exercising it would report the server's own startup
			// as load errors.
			if entry.Status != "" && entry.Status != modelStatusReady {
				drift = append(drift, Drift{Endpoint: ep, Model: cm.Name,
					Detail: fmt.Sprintf("status %q: not ready to serve", entry.Status)})
				continue
			}
			if expect > 0 && entry.Dim != expect {
				from := "the fleet"
				if cm.Dim > 0 {
					from = "the configuration"
				}
				drift = append(drift, Drift{Endpoint: ep, Model: cm.Name,
					Detail: fmt.Sprintf("dimension %d, expected %d from %s", entry.Dim, expect, from)})
				continue
			}
			pairs = append(pairs, Pair{Endpoint: ep, Model: cm.Name, Dim: entry.Dim})
		}
	}
	return pairs, drift
}

// majorityDim is the dimension the endpoints agree on for a model whose
// configuration states none: the most common reported dimension, with ties
// broken by the smaller dimension so the expectation never depends on map
// iteration order.
func majorityDim(endpoints []string, byEndpoint map[string]map[string]embtop.ModelListEntry, model string) int {
	counts := map[int]int{}
	for _, ep := range endpoints {
		if entry, ok := byEndpoint[ep][model]; ok && entry.Dim > 0 {
			counts[entry.Dim]++
		}
	}
	dims := make([]int, 0, len(counts))
	for dim := range counts {
		dims = append(dims, dim)
	}
	sort.Ints(dims)
	best, bestN := 0, 0
	for _, dim := range dims {
		if counts[dim] > bestN {
			best, bestN = dim, counts[dim]
		}
	}
	return best
}
