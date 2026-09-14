package script

import (
	"math"
	"strings"
	"testing"
)

func evalNum(t *testing.T, src string) float64 {
	t.Helper()
	v, err := EvalWithHosts(src, nil, nil, Hosts{}, EvalOptions{})
	if err != nil {
		t.Fatalf("%s: %v", src, err)
	}
	return luaNumF(v)
}

func evalErr(t *testing.T, src string) string {
	t.Helper()
	_, err := EvalWithHosts(src, nil, nil, Hosts{}, EvalOptions{})
	if err == nil {
		t.Fatalf("%s: expected error, got none", src)
	}
	return err.Error()
}

func TestSimilarityCosineDefault(t *testing.T) {
	def := evalNum(t, `return emb.similarity({1, 2, 3}, {4, 5, 6})`)
	exp := evalNum(t, `return emb.similarity({1, 2, 3}, {4, 5, 6}, "cosine")`)
	if math.Abs(def-exp) > 1e-12 {
		t.Fatalf("default %v != explicit cosine %v", def, exp)
	}
	// dot = 32, |a| = sqrt(14), |b| = sqrt(77)
	want := 32 / (math.Sqrt(14) * math.Sqrt(77))
	if math.Abs(def-want) > 1e-6 {
		t.Fatalf("cosine = %v, want %v", def, want)
	}
}

func TestSimilarityIdenticalIsOne(t *testing.T) {
	if got := evalNum(t, `return emb.similarity({0.5, -1.5, 2}, {0.5, -1.5, 2})`); math.Abs(got-1) > 1e-6 {
		t.Fatalf("cosine of identical vectors = %v, want 1", got)
	}
}

func TestSimilarityDot(t *testing.T) {
	if got := evalNum(t, `return emb.similarity({1, 2, 3}, {4, 5, 6}, "dot")`); math.Abs(got-32) > 1e-6 {
		t.Fatalf("dot = %v, want 32", got)
	}
}

func TestSimilarityMixedOperandForms(t *testing.T) {
	// One array, one packed float32 string of the same vector.
	got := evalNum(t, `return emb.similarity({1, 2, 3}, emb.math.float32_bytes({1, 2, 3}))`)
	if math.Abs(got-1) > 1e-6 {
		t.Fatalf("mixed-form cosine = %v, want 1", got)
	}
}

func TestSimilarityPackedBothOperands(t *testing.T) {
	got := evalNum(t, `return emb.similarity(emb.math.float32_bytes({1, 2, 3}), emb.math.float32_bytes({4, 5, 6}))`)
	want := 32 / (math.Sqrt(14) * math.Sqrt(77))
	if math.Abs(got-want) > 1e-6 {
		t.Fatalf("packed cosine = %v, want %v", got, want)
	}
}

func TestSimilarityZeroVectorIsZero(t *testing.T) {
	if got := evalNum(t, `return emb.similarity({0, 0, 0}, {1, 2, 3})`); got != 0 {
		t.Fatalf("cosine with a zero vector = %v, want 0", got)
	}
}

func TestSimilarityLengthMismatch(t *testing.T) {
	msg := evalErr(t, `return emb.similarity({1, 2, 3}, {1, 2})`)
	if !strings.Contains(msg, "length mismatch") || !strings.Contains(msg, "3") || !strings.Contains(msg, "2") {
		t.Fatalf("length mismatch error = %q", msg)
	}
}

func TestSimilarityEmptyOperand(t *testing.T) {
	msg := evalErr(t, `return emb.similarity({}, {1})`)
	if !strings.Contains(msg, "empty vector") {
		t.Fatalf("empty operand error = %q", msg)
	}
}

func TestSimilarityMisalignedPacked(t *testing.T) {
	msg := evalErr(t, `return emb.similarity(string.char(1, 2, 3), {1})`)
	if !strings.Contains(msg, "multiple of 4") {
		t.Fatalf("misaligned packed error = %q", msg)
	}
}

func TestSimilarityUnknownMetric(t *testing.T) {
	msg := evalErr(t, `return emb.similarity({1}, {1}, "manhattan")`)
	if !strings.Contains(msg, "unknown metric") || !strings.Contains(msg, "cosine") || !strings.Contains(msg, "dot") {
		t.Fatalf("unknown metric error = %q", msg)
	}
}

func TestDistanceL2Default(t *testing.T) {
	def := evalNum(t, `return emb.distance({0, 0}, {3, 4})`)
	if math.Abs(def-5) > 1e-6 {
		t.Fatalf("l2 = %v, want 5", def)
	}
	exp := evalNum(t, `return emb.distance({0, 0}, {3, 4}, "l2")`)
	if math.Abs(def-exp) > 1e-12 {
		t.Fatalf("default %v != explicit l2 %v", def, exp)
	}
}

func TestDistanceAndSimilarityComplement(t *testing.T) {
	sim := evalNum(t, `return emb.similarity({1, 2, 3}, {4, 5, 6}, "cosine")`)
	dist := evalNum(t, `return emb.distance({1, 2, 3}, {4, 5, 6}, "cosine")`)
	if math.Abs(sim+dist-1) > 1e-9 {
		t.Fatalf("cosine similarity (%v) + distance (%v) != 1", sim, dist)
	}
}

func TestDistanceSquaredIsSquareOfL2(t *testing.T) {
	l2 := evalNum(t, `return emb.distance({0, 0}, {3, 4})`)
	l2sq := evalNum(t, `return emb.distance({0, 0}, {3, 4}, "l2sq")`)
	if math.Abs(l2sq-l2*l2) > 1e-9 {
		t.Fatalf("l2sq %v != l2^2 %v", l2sq, l2*l2)
	}
}

func TestDistanceIdenticalIsZero(t *testing.T) {
	for _, metric := range []string{"l2", "l2sq", "cosine"} {
		src := `return emb.distance({1, -2, 3}, {1, -2, 3}, "` + metric + `")`
		if got := evalNum(t, src); math.Abs(got) > 1e-6 {
			t.Fatalf("distance(%s) of identical vectors = %v, want 0", metric, got)
		}
	}
}

func TestDistanceUnknownMetric(t *testing.T) {
	msg := evalErr(t, `return emb.distance({1}, {1}, "dot")`)
	if !strings.Contains(msg, "unknown metric") || !strings.Contains(msg, "l2") {
		t.Fatalf("unknown distance metric error = %q", msg)
	}
}
