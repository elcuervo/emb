package registry

import (
	"errors"
	"runtime"
	"sync/atomic"
	"testing"

	"github.com/elcuervo/emb/internal/config"
	"github.com/elcuervo/emb/internal/onnx"
)

func imageFixtureConfig() config.ModelConfig {
	return config.ModelConfig{
		Image: &config.ImageConfig{
			Input:   "pixel_values",
			Size:    4,
			Outputs: []config.ImageOutputConfig{{Name: "embedding", Tensor: "last_hidden_state"}},
			Std:     []float64{1, 1, 1},
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
	newNamedSession = func([]byte, string, []string, []string, int, int, int, bool) (onnx.NamedSession, error) {
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
	if len(res.Outputs) != 1 || res.Outputs[0].Tensor != "last_hidden_state" || res.Outputs[0].Dim != 384 {
		t.Fatalf("resources = %+v, want one output on last_hidden_state of dim 384", res.Outputs)
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
	newNamedSession = func([]byte, string, []string, []string, int, int, int, bool) (onnx.NamedSession, error) {
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
