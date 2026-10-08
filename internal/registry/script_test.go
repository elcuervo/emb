package registry

import (
	"testing"

	"github.com/elcuervo/emb/internal/config"
)

// scriptFixture loads the real minilm model with the given scripted config;
// skips when the model files are absent (run: just download-model).
func scriptFixture(t *testing.T, workers int, preload bool) *ModelEntry {
	t.Helper()
	initORT(t)

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
	// Close any scripted sessions/tokenizer the test opens. The shared ONNX
	// environment is initialized once for the binary (initORT) and never torn
	// down, so this only releases the model's own resources.
	t.Cleanup(func() {
		if entry.scriptRes == nil {
			return
		}
		for _, sess := range entry.scriptRes.Sessions() {
			_ = sess.Close()
		}
		if entry.scriptRes.Tokenizer != nil {
			_ = entry.scriptRes.Tokenizer.Close()
		}
	})
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

func TestScriptPreloadWarmsAtLoad(t *testing.T) {
	entry := scriptFixture(t, 1, true)
	if entry.scriptRes == nil {
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
