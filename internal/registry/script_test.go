package registry

import (
	"testing"

	"github.com/elcuervo/emb/internal/config"
	"github.com/elcuervo/emb/internal/onnx"
)

// scriptFixture loads the real minilm model with the given scripted config;
// skips when the model files are absent (run: just download-model).
func scriptFixture(t *testing.T, workers int, preload bool) *ModelEntry {
	t.Helper()
	if err := onnx.InitEnvironment(""); err != nil {
		t.Skipf("onnx runtime unavailable: %v", err)
	}
	t.Cleanup(func() { _ = onnx.DestroyEnvironment() })

	entry, err := LoadModel(config.ModelConfig{
		ONNX:          "../../models/minilm/model.onnx",
		Tokenizer:     "../../models/minilm/tokenizer.json",
		Dim:           384,
		MaxLength:     128,
		Pooling:       "mean",
		Normalize:     false,
		ScriptWorkers: workers,
		ScriptPreload: preload,
	}, "test")
	if err != nil {
		t.Skipf("test model not present: %v (run: just download-model)", err)
	}
	return entry
}

func TestScriptSessionPoolSize(t *testing.T) {
	entry := scriptFixture(t, 2, false)
	res, err := entry.ScriptResources()
	if err != nil {
		t.Fatal(err)
	}
	if got := len(res.Sessions()); got != 2 {
		t.Fatalf("expected 2 scripted sessions, got %d", got)
	}
}

func TestScriptSessionAutoTuneMinOne(t *testing.T) {
	entry := scriptFixture(t, 0, false)
	res, err := entry.ScriptResources()
	if err != nil {
		t.Fatal(err)
	}
	if got := len(res.Sessions()); got < 1 {
		t.Fatalf("expected at least 1 auto-tuned session, got %d", got)
	}
}

func TestScriptSessionRoundRobin(t *testing.T) {
	entry := scriptFixture(t, 3, false)
	res, err := entry.ScriptResources()
	if err != nil {
		t.Fatal(err)
	}
	a := res.Session()
	b := res.Session()
	c := res.Session()
	d := res.Session()
	if a == b || b == c || a == c {
		t.Fatalf("expected 3 distinct sessions, got a=%p b=%p c=%p", a, b, c)
	}
	if d != a {
		t.Fatalf("round-robin did not wrap back to the first session: d=%p a=%p", d, a)
	}
}

func TestScriptPreloadWarmsAtLoad(t *testing.T) {
	entry := scriptFixture(t, 1, true)
	if !entry.loaded.Load() && entry.scriptRes == nil {
		t.Fatal("expected script resources warmed by preload")
	}
	res, err := entry.ScriptResources()
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Sessions()) != 1 {
		t.Fatalf("expected 1 session, got %d", len(res.Sessions()))
	}
}
