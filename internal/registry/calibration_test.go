package registry

import "testing"

func TestCalibrationCandidatesRespectProcessBudget(t *testing.T) {
	for _, candidate := range calibrationCandidates(5, 4, 0, 0, false) {
		if candidate[0] < 1 || candidate[1] < 1 || candidate[0]*candidate[1] > 5 {
			t.Fatalf("invalid candidate %v for budget 5", candidate)
		}
	}
	got := calibrationCandidates(4, 4, 2, 0, true)
	if len(got) != 1 || got[0] != [2]int{1, 2} {
		t.Fatalf("batched calibration with explicit width should only test one session at width 2, got %v", got)
	}
}
