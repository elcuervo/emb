package registry

import (
	"testing"

	"github.com/elcuervo/emb/internal/config"
	"github.com/elcuervo/emb/internal/onnx"
)

// TestCapacityProfiles verifies the creation-time layout each profile pins:
// spinning at session creation, the initial allowance, and whether the
// controller is active.
func TestCapacityProfiles(t *testing.T) {
	orig := newNamedSession
	t.Cleanup(func() { newNamedSession = orig })

	run := func(cfg config.ModelConfig) (spinning []bool, res *ScriptResources) {
		newNamedSession = func(_ []byte, _, _ []string, _, _, _ int, allowSpinning bool) (onnx.NamedSession, error) {
			spinning = append(spinning, allowSpinning)
			return &fakeNamedSession{}, nil
		}
		_, entry := loadModelFixture(t, cfg)
		r, err := entry.ScriptResources()
		if err != nil {
			t.Fatalf("ScriptResources: %v", err)
		}
		return spinning, r
	}

	sp, res := run(config.ModelConfig{ScriptWorkers: 2, ScriptCallersPerSession: 4})
	if len(sp) != 2 || sp[0] {
		t.Fatalf("auto spinning = %v, want off for a shared session", sp)
	}
	if res.Allowance() != 4 || !res.AutotuneActive() {
		t.Fatalf("auto allowance=%d active=%v, want 4/true", res.Allowance(), res.AutotuneActive())
	}

	sp, res = run(config.ModelConfig{ScriptWorkers: 2, Capacity: "latency"})
	if len(sp) != 2 || !sp[0] {
		t.Fatalf("latency spinning = %v, want on", sp)
	}
	if res.Allowance() != 1 || res.AutotuneActive() {
		t.Fatalf("latency allowance=%d active=%v, want 1/false", res.Allowance(), res.AutotuneActive())
	}

	sp, res = run(config.ModelConfig{ScriptWorkers: 2, Capacity: "throughput", ScriptCallersPerSession: 4})
	if len(sp) != 2 || sp[0] {
		t.Fatalf("throughput spinning = %v, want off", sp)
	}
	if res.Allowance() != 4 || !res.AutotuneActive() {
		t.Fatalf("throughput allowance=%d active=%v, want 4/true", res.Allowance(), res.AutotuneActive())
	}

	_, res = run(config.ModelConfig{ScriptWorkers: 2, ScriptCallersPerSession: 4, Autotune: "off"})
	if res.AutotuneActive() {
		t.Fatal("autotune off must disable the controller")
	}

	// A profile selects a default layout; an explicit value always wins.
	noSpin, yesSpin := false, true

	sp, res = run(config.ModelConfig{ScriptWorkers: 2, Capacity: "latency", AllowSpinning: &noSpin})
	if len(sp) != 2 || sp[0] {
		t.Fatalf("latency+allow_spinning=false spinning = %v, want off", sp)
	}
	if res.Allowance() != 1 || res.AutotuneActive() {
		t.Fatalf("latency+allow_spinning=false allowance=%d active=%v, want 1/false", res.Allowance(), res.AutotuneActive())
	}

	sp, res = run(config.ModelConfig{ScriptWorkers: 2, Capacity: "throughput", AllowSpinning: &yesSpin})
	if len(sp) != 2 || !sp[0] {
		t.Fatalf("throughput+allow_spinning=true spinning = %v, want on", sp)
	}

	_, res = run(config.ModelConfig{ScriptWorkers: 2, ScriptCallersPerSession: 4, Capacity: "throughput", Autotune: "off"})
	if res.AutotuneActive() {
		t.Fatal("capacity: throughput must not force the controller past autotune: off")
	}
	if res.Allowance() != 4 {
		t.Fatalf("throughput+autotune=off allowance = %d, want the configured cap 4", res.Allowance())
	}
}
