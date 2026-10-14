package embstress

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestGeneratorIsDeterministic(t *testing.T) {
	a := NewGenerator(42, 0, 512)
	b := NewGenerator(42, 0, 512)
	for i := range 200 {
		if x, y := a.Next(), b.Next(); x != y {
			t.Fatalf("step %d differs between two runs of the same seed:\n%q\n%q", i, x, y)
		}
	}
	// A different seed offers a different request set.
	c := NewGenerator(43, 0, 512)
	same := 0
	d := NewGenerator(42, 0, 512)
	for range 50 {
		if d.Next() == c.Next() {
			same++
		}
	}
	if same == 50 {
		t.Fatal("a different seed offered the same texts")
	}
	// Two workers of one run must not offer identical texts in lockstep.
	w0, w1 := NewGenerator(42, 0, 512), NewGenerator(42, 1, 512)
	if w0.Next() == w1.Next() {
		t.Fatal("two workers of one run offered the same first text")
	}
}

func TestGeneratorLengthsFollowTheConfiguredMaximum(t *testing.T) {
	for _, maxLength := range []int{8, 128, 512, 4096} {
		g := NewGenerator(7, 0, maxLength)
		seen := map[int]bool{}
		for range 64 {
			text := g.Next()
			n := len(strings.Fields(text))
			if n < 1 {
				t.Fatalf("max_length %d: generated an empty text", maxLength)
			}
			if n > maxLength {
				t.Fatalf("max_length %d: generated %d words: %q", maxLength, n, text)
			}
			seen[n] = true
		}
		if len(seen) < 2 {
			t.Errorf("max_length %d: every text had the same length (%v), no classes", maxLength, seen)
		}
	}
}

func TestGeneratorLengthClassesCoverTheMaximum(t *testing.T) {
	// The longest class is the configured maximum itself, so the run offers the
	// lengths the model was sized for.
	g := NewGenerator(1, 0, 512)
	longest := 0
	for range 64 {
		if n := len(strings.Fields(g.Next())); n > longest {
			longest = n
		}
	}
	if longest != 512 {
		t.Fatalf("longest text = %d words, want the configured maximum 512", longest)
	}

	// A configuration that states no maximum falls back to the server's own.
	g = NewGenerator(1, 0, 0)
	longest = 0
	for range 64 {
		if n := len(strings.Fields(g.Next())); n > longest {
			longest = n
		}
	}
	if longest != 512 {
		t.Fatalf("longest text with no configured maximum = %d words, want 512", longest)
	}

	// A tiny maximum still yields at least one word per class.
	g = NewGenerator(1, 0, 2)
	for range 16 {
		if text := g.Next(); len(strings.Fields(text)) < 1 {
			t.Fatalf("a tiny maximum produced an empty text: %q", text)
		}
	}
}

func TestCorpusReplacesGeneration(t *testing.T) {
	corpus := []string{"the first text", "the second text"}
	g := NewCorpusGenerator(corpus)
	for i := range 6 {
		if got, want := g.Next(), corpus[i%len(corpus)]; got != want {
			t.Fatalf("step %d = %q, want %q", i, got, want)
		}
	}
	// Two corpus generators walk the same cycle.
	a, b := NewCorpusGenerator(corpus), NewCorpusGenerator(corpus)
	for range 4 {
		if a.Next() != b.Next() {
			t.Fatal("two corpus generators diverged")
		}
	}
}

func TestLoadCorpus(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "corpus.txt")
	if err := os.WriteFile(path, []byte("first line\n\n  second line  \nthird\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	got, err := LoadCorpus(path)
	if err != nil {
		t.Fatalf("LoadCorpus: %v", err)
	}
	want := []string{"first line", "second line", "third"}
	if len(got) != len(want) {
		t.Fatalf("corpus = %q, want %q", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("corpus[%d] = %q, want %q", i, got[i], want[i])
		}
	}

	if _, err := LoadCorpus(filepath.Join(dir, "missing.txt")); err == nil {
		t.Error("a missing corpus file did not fail")
	}
	empty := filepath.Join(dir, "empty.txt")
	if err := os.WriteFile(empty, []byte("\n\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadCorpus(empty); err == nil {
		t.Error("an empty corpus did not fail")
	}
}
