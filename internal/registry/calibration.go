package registry

import (
	"fmt"
	"math"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/elcuervo/emb/internal/onnx"
	"github.com/elcuervo/emb/internal/tokenizer"
)

type calibratedLayout struct {
	Workers int
	Threads int
	Source  string
	Reason  string
	Elapsed time.Duration
}

type calibrationInput struct {
	ids, mask []int64
}

func calibrationInputs(tok tokenizer.Tokenizer, maxLen int) ([]calibrationInput, error) {
	texts := []string{
		"a short calibration request",
		strings.Repeat("representative embedding calibration text ", 24),
		strings.Repeat("longer calibration text with varied words and punctuation, ", 96),
	}
	inputs := make([]calibrationInput, 0, len(texts))
	for _, text := range texts {
		ids, mask, err := tok.Encode(text, maxLen)
		if err != nil {
			return nil, err
		}
		if len(ids) == 0 || len(ids) != len(mask) {
			return nil, fmt.Errorf("tokenizer returned invalid calibration shape ids=%d mask=%d", len(ids), len(mask))
		}
		inputs = append(inputs, calibrationInput{ids: ids, mask: mask})
	}
	return inputs, nil
}

func calibrationCandidates(budget, maxWorkers, configuredThreads, configuredWorkers int, batched bool) [][2]int {
	widths := []int{1}
	if configuredThreads > 0 {
		widths = []int{configuredThreads}
	} else {
		for n := 2; n < budget; n *= 2 {
			widths = append(widths, n)
		}
		if budget > 1 {
			widths = append(widths, budget)
		}
	}
	workers := []int{1}
	if !batched {
		if configuredWorkers > 0 {
			workers = []int{configuredWorkers}
		} else {
			for n := 2; n < maxWorkers; n *= 2 {
				workers = append(workers, n)
			}
			if maxWorkers > 1 {
				workers = append(workers, maxWorkers)
			}
		}
	}
	seen := map[[2]int]bool{}
	out := make([][2]int, 0, len(widths)*len(workers))
	for _, width := range widths {
		for _, count := range workers {
			candidate := [2]int{count, width}
			if count*width > budget || seen[candidate] {
				continue
			}
			seen[candidate] = true
			out = append(out, candidate)
		}
	}
	return out
}

func calibrateLayout(factory func(int) (onnx.Session, error), tok tokenizer.Tokenizer, dim, maxLen, budget, maxWorkers, configuredThreads, configuredWorkers int, batched bool) calibratedLayout {
	fallback := calibratedLayout{Workers: 1, Threads: max(1, configuredThreads), Source: "fallback", Reason: "no_valid_candidate"}
	if budget < 1 || maxLen < 1 {
		fallback.Reason = "invalid_limits"
		return fallback
	}
	inputs, err := calibrationInputs(tok, maxLen)
	if err != nil {
		fallback.Reason = "invalid_fixture: " + err.Error()
		return fallback
	}
	if configuredThreads > 0 {
		fallback.Threads = configuredThreads
	}
	if configuredWorkers > 0 {
		fallback.Workers = configuredWorkers
	}

	candidates := calibrationCandidates(budget, maxWorkers, configuredThreads, configuredWorkers, batched)
	if len(candidates) > 12 {
		candidates = candidates[:12]
	}
	started := time.Now()
	deadline := started.Add(4 * time.Second)
	var reference [][]float32
	var referenceP95 time.Duration
	bestScore := -1.0
	best := fallback
	for _, candidate := range candidates {
		if time.Now().After(deadline) {
			fallback.Reason = "calibration_deadline"
			break
		}
		result, candidateRef, ok := runCalibrationCandidate(factory, inputs, dim, candidate[0], candidate[1])
		if candidate[0] == 1 && candidate[1] == 1 && ok {
			reference, referenceP95 = candidateRef, result.P95
		}
		if !ok || len(reference) != len(inputs) || result.P95 > 2*referenceP95 {
			continue
		}
		if !calibrationOutputsMatch(reference, candidateRef) {
			continue
		}
		if result.CPUUsec == 0 {
			fallback.Reason = "cpu_accounting_unavailable"
			continue
		}
		score := float64(result.Rows) / float64(result.Elapsed)
		if score > bestScore || (math.Abs(score-bestScore) < 1e-9 && candidate[0]*candidate[1] < best.Workers*best.Threads) {
			bestScore = score
			best = calibratedLayout{Workers: candidate[0], Threads: candidate[1], Source: "calibrated"}
		}
	}
	best.Elapsed = time.Since(started)
	if bestScore < 0 {
		fallback.Elapsed = best.Elapsed
		return fallback
	}
	return best
}

type calibrationResult struct {
	Rows    int
	CPUUsec uint64
	P95     time.Duration
	Elapsed time.Duration
}

func runCalibrationCandidate(factory func(int) (onnx.Session, error), inputs []calibrationInput, dim, workers, threads int) (calibrationResult, [][]float32, bool) {
	sessions := make([]onnx.Session, 0, workers)
	for range workers {
		sess, err := factory(threads)
		if err != nil {
			for _, opened := range sessions {
				_ = opened.Close()
			}
			return calibrationResult{}, nil, false
		}
		sessions = append(sessions, sess)
	}
	defer func() {
		for _, sess := range sessions {
			_ = sess.Close()
		}
	}()

	reference := make([][]float32, len(inputs))
	for i, input := range inputs {
		out, err := sessions[0].Run(input.ids, input.mask, 1, len(input.ids), dim)
		if err != nil || len(out) == 0 {
			return calibrationResult{}, nil, false
		}
		reference[i] = append([]float32(nil), out...)
	}
	for _, sess := range sessions {
		for _, input := range inputs {
			if _, err := sess.Run(input.ids, input.mask, 1, len(input.ids), dim); err != nil {
				return calibrationResult{}, nil, false
			}
		}
	}

	userBefore, sysBefore := ProcessCPUTimes()
	started := time.Now()
	var mu sync.Mutex
	var latencies []time.Duration
	var wg sync.WaitGroup
	for _, sess := range sessions {
		wg.Add(1)
		go func(sess onnx.Session) {
			defer wg.Done()
			for repeat := 0; repeat < 3; repeat++ {
				for _, input := range inputs {
					start := time.Now()
					out, err := sess.Run(input.ids, input.mask, 1, len(input.ids), dim)
					elapsed := time.Since(start)
					if err != nil || !calibrationOutputValid(out) {
						return
					}
					mu.Lock()
					latencies = append(latencies, elapsed)
					mu.Unlock()
				}
			}
		}(sess)
	}
	wg.Wait()
	userAfter, sysAfter := ProcessCPUTimes()
	if len(latencies) != workers*len(inputs)*3 {
		return calibrationResult{}, nil, false
	}
	sort.Slice(latencies, func(i, j int) bool { return latencies[i] < latencies[j] })
	return calibrationResult{
		Rows:    len(latencies),
		CPUUsec: (userAfter - userBefore) + (sysAfter - sysBefore),
		P95:     latencies[(len(latencies)*95-1)/100],
		Elapsed: time.Since(started),
	}, reference, true
}

func calibrationOutputValid(out []float32) bool {
	if len(out) == 0 {
		return false
	}
	for _, value := range out {
		if math.IsNaN(float64(value)) || math.IsInf(float64(value), 0) {
			return false
		}
	}
	return true
}

func calibrationOutputsMatch(reference, candidate [][]float32) bool {
	if len(reference) != len(candidate) {
		return false
	}
	for i := range reference {
		if len(reference[i]) != len(candidate[i]) {
			return false
		}
		for j, want := range reference[i] {
			if math.Abs(float64(want-candidate[i][j])) > 1e-3 {
				return false
			}
		}
	}
	return true
}
