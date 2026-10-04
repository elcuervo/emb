package registry

import "testing"

// TestListIsNameSorted guards EMB.MODELS' determinism: the registry is a map,
// so List() must sort rather than leak Go's randomized iteration order.
func TestListIsNameSorted(t *testing.T) {
	r := New()
	for _, name := range []string{"minilm", "bge", "e5", "jina"} {
		r.Add(name, &ModelEntry{Name: name})
	}
	want := []string{"bge", "e5", "jina", "minilm"}
	// Repeat: map iteration order varies per call, so a missing sort would
	// show up as a differing order on some call.
	for call := 0; call < 20; call++ {
		got := r.List()
		if len(got) != len(want) {
			t.Fatalf("call %d: len = %d, want %d", call, len(got), len(want))
		}
		for i, e := range got {
			if e.Name != want[i] {
				t.Fatalf("call %d: order = %v, want %v", call, modelNames(got), want)
			}
		}
	}
}

func modelNames(entries []*ModelEntry) []string {
	out := make([]string, len(entries))
	for i, e := range entries {
		out[i] = e.Name
	}
	return out
}
