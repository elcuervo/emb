package emoji

import (
	"errors"
	"math"
	"testing"
)

func threeEntries(t *testing.T) Vocab {
	t.Helper()
	v, err := LoadBytes(fixture(`{"glyph":"🔴","slug":"right","name":"right","description":"right"},{"glyph":"🔵","slug":"up","name":"up","description":"up"},{"glyph":"🟢","slug":"diagonal","name":"diagonal","description":"diagonal"}`))
	if err != nil {
		t.Fatalf("LoadBytes: %v", err)
	}
	return v
}

func TestNewIndexRejectsMismatchedVectors(t *testing.T) {
	v := threeEntries(t)
	if _, err := NewIndex(v, 0, nil); err == nil {
		t.Fatal("NewIndex accepted a zero dimension")
	}
	if _, err := NewIndex(v, 2, make([]float32, 4)); err == nil {
		t.Fatal("NewIndex accepted a matrix that does not hold one row per entry")
	}
	if _, err := NewIndex(v, 2, make([]float32, 6)); err != nil {
		t.Fatalf("NewIndex rejected a well-formed matrix: %v", err)
	}
}

func newTestIndex(t *testing.T, matrix []float32) *Index {
	t.Helper()
	ix, err := NewIndex(threeEntries(t), 2, matrix)
	if err != nil {
		t.Fatalf("NewIndex: %v", err)
	}
	return ix
}

func TestRankOrdersBySimilarity(t *testing.T) {
	// Rows are deliberately unnormalized: the index normalizes them once, so a
	// row's length must not change its rank.
	ix := newTestIndex(t, []float32{
		10, 0, // right
		0, 1, // up
		1, 1, // diagonal
	})
	got, err := ix.Rank([]float32{1, 0}, 3)
	if err != nil {
		t.Fatalf("Rank: %v", err)
	}
	want := []string{"right", "diagonal", "up"}
	if len(got) != len(want) {
		t.Fatalf("Rank returned %d results, want %d", len(got), len(want))
	}
	for i, slug := range want {
		if got[i].Entry.Slug != slug {
			t.Fatalf("result %d is %q, want %q (scores %v)", i, got[i].Entry.Slug, slug, scores(got))
		}
	}
	if math.Abs(got[0].Score-1) > 1e-6 {
		t.Fatalf("the best result scored %v, want 1", got[0].Score)
	}
	if math.Abs(got[2].Score) > 1e-6 {
		t.Fatalf("the orthogonal result scored %v, want 0", got[2].Score)
	}
}

func TestRankKeepsVocabularyOrderOnTies(t *testing.T) {
	ix := newTestIndex(t, []float32{
		1, 0, // right
		1, 0, // up — the same direction, so the same score
		0, 1, // diagonal
	})
	for attempt := range 3 {
		got, err := ix.Rank([]float32{1, 0}, 2)
		if err != nil {
			t.Fatalf("Rank: %v", err)
		}
		if got[0].Entry.Slug != "right" || got[1].Entry.Slug != "up" {
			t.Fatalf("attempt %d ordered tied results as %v, want vocabulary order", attempt, scores(got))
		}
	}
}

func TestRankTruncatesAndBounds(t *testing.T) {
	ix := newTestIndex(t, []float32{1, 0, 0, 1, 1, 1})
	got, err := ix.Rank([]float32{1, 0}, 9)
	if err != nil {
		t.Fatalf("Rank: %v", err)
	}
	if len(got) != 3 {
		t.Fatalf("asking for more results than exist returned %d, want 3", len(got))
	}
	if got, err := ix.Rank([]float32{1, 0}, 0); err != nil || got != nil {
		t.Fatalf("a non-positive k returned %v, %v; want no results and no error", got, err)
	}
	if _, err := ix.Rank([]float32{1, 0, 0}, 1); err == nil {
		t.Fatal("Rank accepted a query of the wrong dimension")
	}
}

func TestRankOfAZeroVector(t *testing.T) {
	ix := newTestIndex(t, []float32{1, 0, 0, 1, 1, 1})
	got, err := ix.Rank([]float32{0, 0}, 3)
	if err != nil {
		t.Fatalf("Rank: %v", err)
	}
	for _, r := range got {
		if math.IsNaN(r.Score) {
			t.Fatalf("a zero query produced a NaN score for %q", r.Entry.Slug)
		}
	}
}

func TestRankOfAnEmptyIndex(t *testing.T) {
	v, err := LoadBytes(fixture(``))
	if err != nil {
		t.Fatalf("LoadBytes: %v", err)
	}
	ix, err := NewIndex(v, 2, nil)
	if err != nil {
		t.Fatalf("NewIndex: %v", err)
	}
	got, err := ix.Rank([]float32{1, 0}, 3)
	if err != nil || got != nil {
		t.Fatalf("ranking an empty vocabulary gave %v, %v; want nothing and no error", got, err)
	}
}

func scores(results []Result) []float64 {
	out := make([]float64, len(results))
	for i, r := range results {
		out[i] = r.Score
	}
	return out
}

func TestRankJointPrefersAnEntryCloseToEveryTerm(t *testing.T) {
	ix := newTestIndex(t, []float32{
		1, 0, // right
		0, 1, // up
		1, 1, // diagonal
	})
	// Two orthogonal terms: each of the operands matches one term and not the
	// other, so the entry between them is the best joint answer.
	got, err := ix.RankJoint([][]float32{{1, 0}, {0, 1}}, nil, 3)
	if err != nil {
		t.Fatalf("RankJoint: %v", err)
	}
	if got[0].Entry.Slug != "diagonal" {
		t.Fatalf("the best joint entry is %q, want diagonal (scores %v)", got[0].Entry.Slug, scores(got))
	}
	want := math.Sqrt(2) / 2
	for i, leg := range got[0].Legs {
		if math.Abs(leg-want) > 1e-6 {
			t.Fatalf("leg %d is %v, want %v", i, leg, want)
		}
	}
	if math.Abs(got[0].Score-want*want) > 1e-6 {
		t.Fatalf("the joint score is %v, want %v", got[0].Score, want*want)
	}
}

func TestRankJointExcludesTheEntriesTheQueryNamed(t *testing.T) {
	ix := newTestIndex(t, []float32{
		1, 0, // right
		0, 1, // up
		1, 1, // diagonal
	})
	// `right*right` names one entry twice; it must not answer with itself.
	got, err := ix.RankJoint([][]float32{{1, 0}, {1, 0}}, map[int]struct{}{0: {}}, 3)
	if err != nil {
		t.Fatalf("RankJoint: %v", err)
	}
	if got[0].Entry.Slug == "right" {
		t.Fatalf("the excluded entry answered: %v", scores(got))
	}
}

func TestRankJointKeepsVocabularyOrderOnTies(t *testing.T) {
	ix := newTestIndex(t, []float32{
		1, 0, // right
		1, 0, // up — the same direction, so the same score
		0, 1, // diagonal
	})
	for attempt := range 3 {
		got, err := ix.RankJoint([][]float32{{1, 0}, {1, 0}}, nil, 2)
		if err != nil {
			t.Fatalf("RankJoint: %v", err)
		}
		if got[0].Entry.Slug != "right" || got[1].Entry.Slug != "up" {
			t.Fatalf("attempt %d ordered tied results as %v, want vocabulary order", attempt, scores(got))
		}
	}
}

func TestRankJointOfAZeroTerm(t *testing.T) {
	ix := newTestIndex(t, []float32{1, 0, 0, 1, 1, 1})
	got, err := ix.RankJoint([][]float32{{0, 0}, {1, 0}}, nil, 3)
	if err != nil {
		t.Fatalf("RankJoint: %v", err)
	}
	for _, r := range got {
		if math.IsNaN(r.Score) {
			t.Fatalf("a zero term produced a NaN score for %q", r.Entry.Slug)
		}
	}
}

func TestRankJointBoundsAndRefuses(t *testing.T) {
	ix := newTestIndex(t, []float32{1, 0, 0, 1, 1, 1})
	if got, err := ix.RankJoint([][]float32{{1, 0}, {0, 1}}, nil, 0); err != nil || got != nil {
		t.Fatalf("a non-positive k returned %v, %v; want no results and no error", got, err)
	}
	if got, err := ix.RankJoint([][]float32{{1, 0}, {0, 1}}, nil, 9); err != nil || len(got) != 3 {
		t.Fatalf("asking for more results than exist gave %d results, %v; want 3", len(got), err)
	}
	if _, err := ix.RankJoint(nil, nil, 1); !errors.Is(err, ErrNoTerms) {
		t.Fatalf("a conjunction of no terms gave %v, want %v", err, ErrNoTerms)
	}
	if _, err := ix.RankJoint([][]float32{{1, 0, 0}}, nil, 1); err == nil {
		t.Fatal("RankJoint accepted a term of the wrong dimension")
	}
}

func TestRankJointOnAnEmptyIndex(t *testing.T) {
	v, err := LoadBytes(fixture(``))
	if err != nil {
		t.Fatalf("LoadBytes: %v", err)
	}
	ix, err := NewIndex(v, 2, nil)
	if err != nil {
		t.Fatalf("NewIndex: %v", err)
	}
	got, err := ix.RankJoint([][]float32{{1, 0}, {0, 1}}, nil, 3)
	if err != nil || got != nil {
		t.Fatalf("ranking an empty vocabulary gave %v, %v; want nothing and no error", got, err)
	}
}

func TestRankLeavesNoLegs(t *testing.T) {
	ix := newTestIndex(t, []float32{1, 0, 0, 1, 1, 1})
	got, err := ix.Rank([]float32{1, 0}, 1)
	if err != nil {
		t.Fatalf("Rank: %v", err)
	}
	if got[0].Legs != nil {
		t.Fatalf("a single-vector rank reported legs: %v", got[0].Legs)
	}
}
