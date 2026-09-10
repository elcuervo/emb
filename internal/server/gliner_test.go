package server

import (
	"os"
	"strings"
	"testing"
	"time"

	"github.com/elcuervo/emb/internal/config"
	"github.com/elcuervo/emb/internal/registry"
)

// serveGLiNER starts a server whose "gliner2" model is the real
// cuerbot/gliner2-multi-v1 int8 export. Skips when the testbed model is
// absent (run: just download-gliner-model).
func serveGLiNER(t *testing.T, cacheCfg string) (string, *Server) {
	t.Helper()
	reg := registry.New()
	entry, err := registry.LoadModel(config.ModelConfig{
		ONNX:      "../../models/gliner2/model_int8.onnx",
		Tokenizer: "../../models/gliner2/tokenizer.json",
		MaxLength: 512,
	}, "gliner2")
	if err != nil {
		t.Skipf("gliner testbed model not present: %v (run: just download-gliner-model)", err)
	}
	if !ortOK {
		t.Skip("onnx runtime unavailable (run inside nix develop)")
	}
	reg.Add("gliner2", entry)

	addr := getFreeAddr()
	srv := New(addr, reg, "", cacheCfg, nil)
	go srv.ListenAndServe()
	t.Cleanup(func() { srv.Close() })
	time.Sleep(50 * time.Millisecond)
	return addr, srv
}

func glinerScriptSrc(t *testing.T) string {
	t.Helper()
	raw, err := os.ReadFile("../../examples/scripts/gliner2.lua")
	if err != nil {
		t.Fatal(err)
	}
	return string(raw)
}

// TestGLiNERWire reproduces the specs/script-eval reference scenarios over the
// wire: LOAD once, EVSHA with dynamic labels per request, hash replies.
func TestGLiNERWire(t *testing.T) {
	addr, _ := serveGLiNER(t, "")
	c := dial(t, addr)

	sha := doCmd(t, c, "EMB.SCRIPT", "LOAD", "gliner2", glinerScriptSrc(t))
	shaVal := sha[5 : len(sha)-2]

	// Scenario: extract entities from a sentence (hash = flat field/value
	// pairs, HGETALL shape).
	resp := doCmd(t, c, "EMB.EVSHA", "gliner2", shaVal, "1",
		"Apple CEO Tim Cook announced iPhone 15.", "PERSON", "ORG", "PRODUCT")
	for _, want := range []string{"PERSON", "Tim Cook", "ORG", "Apple", "PRODUCT", "iPhone 15"} {
		if !strings.Contains(resp, want) {
			t.Fatalf("reply missing %q: %q", want, resp)
		}
	}

	// Scenario: different labels, same script — fields are exactly the
	// requested labels, values are arrays.
	resp = doCmd(t, c, "EMB.EVSHA", "gliner2", shaVal, "1",
		"Google launched the Pixel 9 in Mountain View.", "PRODUCT", "LOCATION")
	for _, want := range []string{"PRODUCT", "Pixel 9", "LOCATION", "Mountain View"} {
		if !strings.Contains(resp, want) {
			t.Fatalf("reply missing %q: %q", want, resp)
		}
	}
	if strings.Contains(resp, "PERSON") {
		t.Fatalf("reply must only contain requested label fields: %q", resp)
	}

	// Two texts → array of per-text hashes.
	resp = doCmd(t, c, "EMB.EVSHA", "gliner2", shaVal, "2",
		"Tim Cook leads Apple.", "Sundar leads Google.", "PERSON", "ORG")
	if !strings.HasPrefix(resp, "*2\r\n") {
		t.Fatalf("expected array of 2 hashes, got %q", resp)
	}
	if !strings.Contains(resp, "Tim Cook") || !strings.Contains(resp, "Sundar") || !strings.Contains(resp, "Apple") || !strings.Contains(resp, "Google") {
		t.Fatalf("multi-text reply incomplete: %q", resp)
	}
	c.Close()
}

// TestGLiNERWireCache verifies content-addressed caching over the wire:
// distinct label sets are distinct cache entries for the same text.
func TestGLiNERWireCache(t *testing.T) {
	addr, srv := serveGLiNER(t, "8mb")
	c := dial(t, addr)

	sha := doCmd(t, c, "EMB.SCRIPT", "LOAD", "gliner2", glinerScriptSrc(t))
	shaVal := sha[5 : len(sha)-2]

	const apple = "Apple CEO Tim Cook announced iPhone 15."
	first := doCmd(t, c, "EMB.EVSHA", "gliner2", shaVal, "1", apple, "PERSON", "ORG", "PRODUCT")
	second := doCmd(t, c, "EMB.EVSHA", "gliner2", shaVal, "1", apple, "PERSON", "ORG", "PRODUCT")
	if first != second {
		t.Fatalf("cached reply differs:\n%q\n%q", first, second)
	}
	stats := srv.cache.Stats()
	if stats.Hits < 1 {
		t.Fatalf("expected cache hit, stats=%+v", stats)
	}

	// Different labels → different cache key → executed (miss), reply covers
	// exactly the requested labels.
	other := doCmd(t, c, "EMB.EVSHA", "gliner2", shaVal, "1", apple, "FEATURE")
	if stats = srv.cache.Stats(); stats.Hits < 1 {
		t.Fatalf("cache stats broken: %+v", stats)
	}
	if !strings.Contains(other, "FEATURE") {
		t.Fatalf("labels arg must change the reply: %q", other)
	}
	c.Close()
}
