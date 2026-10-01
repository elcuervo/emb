package script

import (
	"testing"
	"time"

	"github.com/elcuervo/emb/internal/onnx"
)

// slowEchoRun takes measurable time so the returned inference duration is
// reliably positive, then echoes each input under "<name>_out" like echoRun.
func slowEchoRun(inputs []onnx.NamedTensor) (map[string]onnx.NamedTensor, error) {
	time.Sleep(2 * time.Millisecond)
	return echoRun(inputs)
}

// TestEmbRunReturnsInferenceDuration pins the second return value: the model
// call's own duration, alongside the unchanged tensor map. A script that assigns
// only the first value (see TestPackedOutputRoundTrip) must keep working.
func TestEmbRunReturnsInferenceDuration(t *testing.T) {
	got := evalInt(t, `
local out, ms = emb.run({x = {shape = {1}, data = {7}}})
return (ms > 0 and out.x_out.data[1] == 7) and 1 or 0`, Hosts{Run: slowEchoRun})
	if got != 1 {
		t.Fatalf("emb.run second return = %d, want 1 (a positive duration and the unchanged output)", got)
	}
}

// TestEmbRunBatchReturnsInferenceDuration pins one duration for the whole batch
// call, not one per row.
func TestEmbRunBatchReturnsInferenceDuration(t *testing.T) {
	got := evalInt(t, `
local items, ms = emb.run_batch({{x = {shape = {1}, data = {1}}}, {x = {shape = {1}, data = {2}}}})
return (ms > 0 and items[2].x_out.data[1] == 2) and 1 or 0`, Hosts{Run: slowEchoRun})
	if got != 1 {
		t.Fatalf("emb.run_batch second return = %d, want 1", got)
	}
}
