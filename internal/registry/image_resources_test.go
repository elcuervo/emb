package registry

import (
	"errors"
	"os"
	"runtime"
	"sync/atomic"
	"testing"

	"github.com/elcuervo/emb/internal/config"
	"github.com/elcuervo/emb/internal/onnx"
)

func imageFixtureConfig() config.ModelConfig {
	return config.ModelConfig{
		Image: &config.ImageConfig{
			Input:  "pixel_values",
			Size:   4,
			Output: "last_hidden_state",
			Std:    []float64{1, 1, 1},
		},
	}
}

// TestOpenImageResourcesLifecycle covers the real construction path (with an
// injected session seam): the pool is published, its footprint matches the
// opened sessions, and every opened session is closed exactly once.
func TestOpenImageResourcesLifecycle(t *testing.T) {
	orig := newNamedSession
	t.Cleanup(func() { newNamedSession = orig })

	var created, closed atomic.Int64
	newNamedSession = func([]byte, []string, []string, int, int, int) (onnx.NamedSession, error) {
		created.Add(1)
		return &countingSession{closes: &closed}, nil
	}

	reg, entry := loadModelFixture(t, imageFixtureConfig())
	res, err := entry.ImageResources()
	if err != nil {
		t.Fatalf("ImageResources: %v", err)
	}
	if res == nil || len(res.Sessions) == 0 {
		t.Fatalf("ImageResources returned %+v, want a session pool", res)
	}
	if got := entry.ImageFootprint(); got != created.Load() || got == 0 {
		t.Fatalf("footprint = %d, created = %d, want equal and > 0", got, created.Load())
	}
	if res.OutputTensor != "last_hidden_state" || res.Dim != 384 {
		t.Fatalf("resources = {output:%q dim:%d}, want {last_hidden_state 384}", res.OutputTensor, res.Dim)
	}

	if err := reg.Close(); err != nil {
		t.Fatal(err)
	}
	if got := closed.Load(); got != created.Load() {
		t.Fatalf("closed = %d, want every opened session closed (%d)", got, created.Load())
	}
}

// TestOpenImageResourcesRollsBackOnFailure verifies a construction failure
// partway through the pool closes the sessions already opened and publishes no
// footprint.
func TestOpenImageResourcesRollsBackOnFailure(t *testing.T) {
	// Force more than one session so the failure lands after a session was
	// already opened, regardless of the host's core count.
	prev := runtime.GOMAXPROCS(4)
	t.Cleanup(func() { runtime.GOMAXPROCS(prev) })

	orig := newNamedSession
	t.Cleanup(func() { newNamedSession = orig })

	var created, closed atomic.Int64
	injected := errors.New("injected image session failure")
	newNamedSession = func([]byte, []string, []string, int, int, int) (onnx.NamedSession, error) {
		if created.Add(1) == 2 {
			return nil, injected
		}
		return &countingSession{closes: &closed}, nil
	}

	_, entry := loadModelFixture(t, imageFixtureConfig())
	if _, err := entry.ImageResources(); !errors.Is(err, injected) {
		t.Fatalf("ImageResources error = %v, want the injected failure", err)
	}
	if got := created.Load(); got != 2 {
		t.Fatalf("sessions attempted = %d, want 2", got)
	}
	if got := closed.Load(); got != 1 {
		t.Fatalf("sessions closed after failure = %d, want 1", got)
	}
	if got := entry.ImageFootprint(); got != 0 {
		t.Fatalf("footprint after failed construction = %d, want 0", got)
	}
}

// TestValidateImagePairing covers the dual-encoder dimension check: matching
// dims pass, a mismatch and a missing output tensor fail, and the early-return
// cases skip validation.
func TestValidateImagePairing(t *testing.T) {
	const model = "../../models/minilm/model.onnx"
	if _, err := os.Stat(model); err != nil {
		t.Skipf("test model not present: %v (run: just download-model)", err)
	}
	requireORT(t)

	if err := validateImagePairing(config.ModelConfig{ONNX: model, Dim: 384}, "test", "last_hidden_state"); err != nil {
		t.Errorf("matching dimensions: %v", err)
	}
	if err := validateImagePairing(config.ModelConfig{ONNX: model, Dim: 5}, "test", "last_hidden_state"); err == nil {
		t.Error("expected a mismatch error for dim 5 vs the graph's 384")
	}
	if err := validateImagePairing(config.ModelConfig{ONNX: model, Dim: 384}, "test", "nope"); err == nil {
		t.Error("expected a missing-output-tensor error")
	}
	if err := validateImagePairing(config.ModelConfig{ONNX: model, Dim: 384}, "test", ""); err != nil {
		t.Errorf("empty output tensor should skip validation: %v", err)
	}
	if err := validateImagePairing(config.ModelConfig{ONNX: model}, "test", "last_hidden_state"); err != nil {
		t.Errorf("non-positive dim should skip validation: %v", err)
	}
}
