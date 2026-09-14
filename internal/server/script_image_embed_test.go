package server

import (
	"encoding/binary"
	"image/color"
	"math"
	"strconv"
	"strings"
	"testing"

	"github.com/elcuervo/emb/internal/onnx"
	"github.com/elcuervo/emb/internal/registry"
)

// injectImageBranch attaches a deterministic fake image branch to an
// embeddable script model so cross-modal scripts can be exercised without a
// vision model. Vectors land in the same [dim] space as the text embeddings.
func injectImageBranch(t *testing.T, srv *Server, model string, dim int) {
	t.Helper()
	entry, err := srv.reg.Resolve(model)
	if err != nil {
		t.Fatal(err)
	}
	entry.ImageRes = &registry.ImageResources{
		Sessions:     []onnx.NamedSession{&fakeImageSession{dim: dim}},
		Plan:         testImagePlan(4),
		OutputTensor: "image_embeds",
		Pooling:      "none",
		Dim:          dim,
	}
}

// numOf decodes a numeric script reply: an integer reply or a RESP2 decimal
// bulk string.
func numOf(t *testing.T, tok respToken) float64 {
	t.Helper()
	switch tok.kind {
	case "int":
		return float64(tok.val.(int))
	case "bulk":
		f, err := strconv.ParseFloat(tok.val.(string), 64)
		if err != nil {
			t.Fatalf("numeric reply %q: %v", tok.val.(string), err)
		}
		return f
	}
	t.Fatalf("expected a numeric reply, got %s", tok.kind)
	return 0
}

func floatsOf(t *testing.T, packed string) []float32 {
	t.Helper()
	if len(packed)%4 != 0 {
		t.Fatalf("packed vector length %d not a multiple of 4", len(packed))
	}
	out := make([]float32, len(packed)/4)
	for i := range out {
		out[i] = math.Float32frombits(binary.LittleEndian.Uint32([]byte(packed)[i*4:]))
	}
	return out
}

func cosineOf(t *testing.T, a, b []float32) float64 {
	t.Helper()
	if len(a) != len(b) {
		t.Fatalf("cosine operands differ in length: %d vs %d", len(a), len(b))
	}
	var dot, na, nb float64
	for i := range a {
		x, y := float64(a[i]), float64(b[i])
		dot += x * y
		na += x * x
		nb += y * y
	}
	return dot / (math.Sqrt(na) * math.Sqrt(nb))
}

// TestEmbImageEmbedMatchesEMBIMG verifies emb.image.embed returns the same
// vector as the EMB.IMG command for the same bytes.
func TestEmbImageEmbedMatchesEMBIMG(t *testing.T) {
	addr, _, _ := serveImage(t, "")
	png := solidImagePNG(t, color.White)

	want := bulkOf(t, redisCmd(t, addr, "EMB.IMG", "imgA", png))
	got := bulkOf(t, redisCmd(t, addr, "EMB.EVAL", "imgA",
		`return emb.math.float32_bytes(emb.image.embed(KEYS[1]))`, "1", png))
	if got != want {
		t.Fatalf("emb.image.embed differs from EMB.IMG (%d vs %d bytes)", len(got), len(want))
	}
}

// TestEmbImageEmbedRejectsURLs verifies remote references are refused.
func TestEmbImageEmbedRejectsURLs(t *testing.T) {
	addr, _, _ := serveImage(t, "")
	got := errorOf(t, redisCmd(t, addr, "EMB.EVAL", "imgA",
		`return emb.image.embed("https://example.com/cat.png")`, "1", "x"))
	if !strings.Contains(got, "URL") {
		t.Fatalf("emb.image.embed URL error = %q, want a URL rejection", got)
	}
}

// TestCrossModalSimilarity verifies a script can score a text against an image
// through emb.embed + emb.image.embed + emb.similarity, and that the value
// matches the cosine of the two vectors fetched through the command surface.
func TestCrossModalSimilarity(t *testing.T) {
	addr, srv := serveScriptTest(t, "")
	injectImageBranch(t, srv, "test", 384)
	png := solidImagePNG(t, color.White)
	const text = "a photo of a cat"

	textVec := bulkOf(t, redisCmd(t, addr, "EMB", "test", text))
	imgVec := bulkOf(t, redisCmd(t, addr, "EMB.IMG", "test", png))
	if len(textVec) == 0 || len(imgVec) == 0 {
		t.Fatalf("reference vectors empty: text=%d image=%d bytes", len(textVec), len(imgVec))
	}
	want := cosineOf(t, floatsOf(t, textVec), floatsOf(t, imgVec))

	script := `local t = emb.embed(KEYS[1])
local i = emb.image.embed(ARGV[1])
return emb.similarity(t, i)`
	got := numOf(t, redisCmd(t, addr, "EMB.EVAL", "test", script, "1", text, png))
	if math.Abs(got-want) > 1e-6 {
		t.Fatalf("cross-modal similarity = %v, want %v (from EMB/EMB.IMG vectors)", got, want)
	}
}
