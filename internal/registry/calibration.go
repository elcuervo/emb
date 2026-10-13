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
	// Limit names what cut the candidate set below the CPU ceiling ("memory"),
	// or is empty. Ceiling is the CPU ceiling candidates were bounded by; both
	// are filled in by the caller that owns those limits.
	Limit   string
	Ceiling int
	Elapsed time.Duration
}

type calibrationInput struct {
	ids, mask []int64
}

// calibrationInputs builds the short/typical/long fixtures. Sizes track real
// embedding traffic (a phrase, a sentence, a paragraph) rather than saturating
// max_length: a max-length fixture costs several times a typical request and
// spends the whole deadline before a second candidate can be measured.
func calibrationInputs(tok tokenizer.Tokenizer, maxLen int) ([]calibrationInput, error) {
	texts := []string{
		"a short calibration request",
		strings.Repeat("representative embedding calibration text ", 8),
		strings.Repeat("longer calibration text with varied words and punctuation, ", 16),
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

// calibrationNow and calibrationCPUTimes are seams so model-free tests can
// drive the clock and CPU accounting deterministically.
var (
	calibrationNow      = time.Now
	calibrationCPUTimes = ProcessCPUTimes
)

// calibrationDeadline bounds the whole trial; a slow in-flight Run may exceed
// it (temporary sessions close only after calls return).
const calibrationDeadline = 4 * time.Second

// calibrationTieTolerance is the relative run-to-run noise band: a candidate
// must beat the incumbent by more than this fraction to displace it, and a
// candidate inside the band only wins when it uses fewer resources.
const calibrationTieTolerance = 0.05

// calibrationBatchRows and calibrationBatchTokens bound a batching pool's
// trial runs. A batcher coalesces concurrent requests, and intra-op width that
// barely helps one row is worth several threads for a coalesced batch, so
// scoring candidates on single rows under-provisions a batched model. Each
// fixture is tiled to as many identical rows as fit calibrationBatchTokens, up
// to calibrationBatchRows, so the long fixture does not multiply trial cost.
const (
	calibrationBatchRows   = 4
	calibrationBatchTokens = 256
)

// calibrationCandidates screens the thread-width/session-count pairs a model
// may be calibrated at: deduplicated, inside the process CPU budget (the width
// × session product may not exceed budget), and capped by maxWorkers, the
// process memory headroom (see memoryPlanner). A dimension the operator set
// explicitly is fixed to that value so calibration never replaces it. When the
// batcher owns embedding inference (batched) only one session can run
// batches, so session counts stay at 1 and only the width is screened. Joint
// multi-model trials are deliberately not attempted here; the shared admission
// budget bounds the concurrent product.
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

// calibrationRounds measures every candidate this many times and keeps each
// candidate's best score. A candidate's run-to-run spread on a shared host is
// larger than the difference between neighbouring layouts, so one round alone
// decides as much by luck as by measurement.
const calibrationRounds = 2

func calibrateLayout(factory func(int) (onnx.Session, error), tok tokenizer.Tokenizer, dim, maxLen, budget, maxWorkers, configuredThreads, configuredWorkers int, batched bool, batch int) calibratedLayout {
	fallback := calibratedLayout{Workers: 1, Threads: max(1, configuredThreads), Source: "fallback", Reason: "no_valid_candidate"}
	if budget < 1 || maxLen < 1 {
		fallback.Reason = "invalid_limits"
		return fallback
	}
	if batch < 1 {
		batch = 1
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
	if len(candidates) == 0 {
		// Every candidate costs more than the process budget — reachable with an
		// explicit width above it. Keep the explicit width; there is nothing
		// to try.
		fallback.Reason = "over_budget"
		return fallback
	}
	if len(candidates) == 1 {
		// Nothing to choose between (an explicit dimension, or a one-slot
		// budget): skip the trials entirely.
		return calibratedLayout{Workers: candidates[0][0], Threads: candidates[0][1], Source: "calibrated", Reason: "single_candidate"}
	}
	if len(candidates) > 12 {
		candidates = candidates[:12]
	}
	// Keep the serial baseline first, then spend the remaining deadline from the
	// largest layout down: if the deadline truncates the search, the layout with
	// the most resources has still been measured rather than only the cheapest.
	candidates = calibrationOrder(candidates)
	started := calibrationNow()
	deadline := started.Add(calibrationDeadline)
	var reference [][]float32
	var referenceP95 time.Duration
	bestScore := -1.0
	best := fallback
	scores := make(map[[2]int]float64, len(candidates))
	truncated := false
	var candidateCost time.Duration
search:
	for round := 0; round < calibrationRounds; round++ {
		for i, candidate := range candidates {
			now := calibrationNow()
			// Predict, do not just react: a candidate started with less than the
			// last one's cost left would run past the deadline and burn the whole
			// budget on a single layout.
			if now.After(deadline) || (candidateCost > 0 && now.Add(candidateCost).After(deadline)) {
				truncated = true
				break search
			}
			result, candidateRef, ok := runCalibrationCandidate(factory, inputs, dim, candidate[0], candidate[1], batch)
			candidateCost = result.Total
			if i == 0 && round == 0 {
				if !ok {
					return finishCalibration(best, bestScore, fallback, "baseline_failed", started)
				}
				// The lowest-resource candidate is the serial baseline every other
				// candidate is judged against (correctness and the p95 guard).
				reference, referenceP95 = candidateRef, result.P95
			} else if !ok {
				fallback.Reason = "candidate_failed"
				continue
			}
			if result.CPUUsec == 0 {
				fallback.Reason = "cpu_accounting_unavailable"
				continue
			}
			if referenceP95 > 0 && result.P95 > 2*referenceP95 {
				// A slow tail also rejects noisy trials: a candidate several times
				// more variable than the baseline is not selected even if it looks
				// faster on average.
				fallback.Reason = "p95_regression"
				continue
			}
			if !calibrationOutputsMatch(reference, candidateRef) {
				continue
			}
			key := [2]int{candidate[0], candidate[1]}
			if score := float64(result.Rows) / float64(result.Elapsed); score > scores[key] {
				scores[key] = score
			}
		}
	}

	reason := "no_valid_candidate"
	if truncated {
		reason = "calibration_deadline"
	}
	best, bestScore = chooseCalibratedLayout(scores, candidates, batched, fallback)
	return finishCalibration(best, bestScore, fallback, reason, started)
}

// chooseCalibratedLayout picks a measured layout from its scores: the highest
// score outside the noise band, and inside the band the wider layout for a
// batching pool (its trial serves a smaller batch than the coalescing pool, so
// the measurement understates threads) or the lower-resource layout otherwise.
// It returns the fallback and a negative score when nothing was measured. The
// incumbent's score moves with it, so a later candidate is compared against the
// layout it would replace rather than against the serial baseline.
func chooseCalibratedLayout(scores map[[2]int]float64, candidates [][2]int, batched bool, fallback calibratedLayout) (calibratedLayout, float64) {
	bestScore, best := -1.0, fallback
	for _, candidate := range candidates {
		score, measured := scores[[2]int{candidate[0], candidate[1]}]
		if !measured {
			continue
		}
		switch {
		case bestScore < 0, score > bestScore*(1+calibrationTieTolerance):
			bestScore = score
			best = calibratedLayout{Workers: candidate[0], Threads: candidate[1], Source: "calibrated"}
		case score > bestScore*(1-calibrationTieTolerance):
			if batched && candidate[1] > best.Threads {
				bestScore = score
				best = calibratedLayout{Workers: candidate[0], Threads: candidate[1], Source: "calibrated"}
			} else if !batched && candidate[0]*candidate[1] < best.Workers*best.Threads {
				bestScore = score
				best.Workers, best.Threads, best.Source = candidate[0], candidate[1], "calibrated"
			}
		}
	}
	return best, bestScore
}

// finishCalibration returns the best validated candidate, or the safe
// fallback carrying the most specific recorded reason when nothing was
// validated in time.
func finishCalibration(best calibratedLayout, bestScore float64, fallback calibratedLayout, reason string, started time.Time) calibratedLayout {
	elapsed := calibrationNow().Sub(started)
	if bestScore >= 0 {
		best.Elapsed = elapsed
		return best
	}
	if fallback.Reason == "" || fallback.Reason == "no_valid_candidate" {
		fallback.Reason = reason
	}
	fallback.Elapsed = elapsed
	return fallback
}

type calibrationResult struct {
	Rows    int
	CPUUsec uint64
	P95     time.Duration
	// Elapsed is the timed inference section and drives scoring; Total is the
	// candidate's whole cost, session construction through close, and drives
	// deadline prediction.
	Elapsed time.Duration
	Total   time.Duration
}

// calibrationOrder keeps the serial baseline (candidates[0]) first and sorts
// the rest by total resource descending.
func calibrationOrder(candidates [][2]int) [][2]int {
	ordered := make([][2]int, len(candidates))
	copy(ordered, candidates)
	sort.SliceStable(ordered[1:], func(i, j int) bool {
		a, b := ordered[1+i], ordered[1+j]
		return a[0]*a[1] > b[0]*b[1]
	})
	return ordered
}

// calibrationPrepared is one fixture tiled to the candidate's batch width.
type calibrationPrepared struct {
	ids, mask []int64
	seqLen    int
	batch     int
}

// fixtureBatch sizes a fixture's trial batch: as many identical rows as fit the
// token budget, capped at maxRows. A long fixture stays close to one row.
func fixtureBatch(seqLen, maxRows int) int {
	if seqLen <= 0 || maxRows <= 1 {
		return 1
	}
	rows := calibrationBatchTokens / seqLen
	if rows < 1 {
		rows = 1
	}
	if rows > maxRows {
		rows = maxRows
	}
	return rows
}

// prepareInputs tiles each fixture to its per-length batch. The same fixture
// always yields the same batch, so candidate outputs stay comparable.
func prepareInputs(inputs []calibrationInput, maxRows int) []calibrationPrepared {
	prepared := make([]calibrationPrepared, len(inputs))
	for i, input := range inputs {
		batch := fixtureBatch(len(input.ids), maxRows)
		if batch <= 1 {
			prepared[i] = calibrationPrepared{ids: input.ids, mask: input.mask, seqLen: len(input.ids), batch: 1}
			continue
		}
		ids := make([]int64, 0, len(input.ids)*batch)
		mask := make([]int64, 0, len(input.mask)*batch)
		for range batch {
			ids = append(ids, input.ids...)
			mask = append(mask, input.mask...)
		}
		prepared[i] = calibrationPrepared{ids: ids, mask: mask, seqLen: len(input.ids), batch: batch}
	}
	return prepared
}

// runCalibrationCandidate measures one candidate and stamps its total cost,
// from before the first session is created until every temporary session is
// closed.
func runCalibrationCandidate(factory func(int) (onnx.Session, error), inputs []calibrationInput, dim, workers, threads, maxRows int) (calibrationResult, [][]float32, bool) {
	begin := calibrationNow()
	result, reference, ok := trialCalibrationCandidate(factory, inputs, dim, workers, threads, maxRows)
	result.Total = calibrationNow().Sub(begin)
	return result, reference, ok
}

func trialCalibrationCandidate(factory func(int) (onnx.Session, error), inputs []calibrationInput, dim, workers, threads, maxRows int) (calibrationResult, [][]float32, bool) {
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

	prepared := prepareInputs(inputs, maxRows)
	// The reference pass doubles as sessions[0]'s warmup; the loop below warms
	// only the remaining sessions, so a candidate pays one warmup pass, not two.
	reference := make([][]float32, len(prepared))
	for i, input := range prepared {
		out, err := sessions[0].Run(input.ids, input.mask, input.batch, input.seqLen, dim)
		if err != nil || len(out) == 0 {
			return calibrationResult{}, nil, false
		}
		reference[i] = append([]float32(nil), out...)
	}
	for _, sess := range sessions[1:] {
		for _, input := range prepared {
			if _, err := sess.Run(input.ids, input.mask, input.batch, input.seqLen, dim); err != nil {
				return calibrationResult{}, nil, false
			}
		}
	}

	userBefore, sysBefore := calibrationCPUTimes()
	started := time.Now()
	var mu sync.Mutex
	var latencies []time.Duration
	var rows int64
	var wg sync.WaitGroup
	for _, sess := range sessions {
		wg.Add(1)
		go func(sess onnx.Session) {
			defer wg.Done()
			for repeat := 0; repeat < 3; repeat++ {
				for _, input := range prepared {
					start := time.Now()
					out, err := sess.Run(input.ids, input.mask, input.batch, input.seqLen, dim)
					elapsed := time.Since(start)
					if err != nil || !calibrationOutputValid(out) {
						return
					}
					mu.Lock()
					latencies = append(latencies, elapsed)
					rows += int64(input.batch)
					mu.Unlock()
				}
			}
		}(sess)
	}
	wg.Wait()
	userAfter, sysAfter := calibrationCPUTimes()
	if len(latencies) != workers*len(prepared)*3 {
		return calibrationResult{}, nil, false
	}
	sort.Slice(latencies, func(i, j int) bool { return latencies[i] < latencies[j] })
	return calibrationResult{
		Rows:    int(rows),
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
