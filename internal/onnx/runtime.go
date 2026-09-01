package onnx

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"slices"

	ort "github.com/yalue/onnxruntime_go"
)

// Execution-mode selectors for NewRuntimeSession(…). Sequential (the ORT
// default) is the documented choice for mostly-serial encoder graphs; the
// inter-op thread pool only exists in parallel mode, so interOpThreads is
// effectively ignored under sequential execution.
const (
	ExecModeSequential = iota
	ExecModeParallel
)

type RuntimeSession struct {
	session      *ort.DynamicAdvancedSession
	dim          int
	hasAttnMask  bool
	hasTokenType bool
	outputRank   int // 2 (pre-pooled) or 3 (sequence)

	// outTensor is reused across runs to avoid per-run output allocation and
	// the post-run copy. Sessions serialize their runs, so the returned
	// GetData() slice stays valid until the next Run.
	outTensor *ort.Tensor[float32]
	outShape  []int64
	outFlat   int
}

func NewRuntimeSession(modelPath string, inputNames, outputNames []string, dim int, outputRank int, intraOpThreads, interOpThreads int, execMode int) (*RuntimeSession, error) {
	opts, err := newSessionOptions(intraOpThreads, interOpThreads, execMode)
	if err != nil {
		return nil, err
	}
	defer func() { _ = opts.Destroy() }()

	session, err := ort.NewDynamicAdvancedSession(modelPath, inputNames, outputNames, opts)
	if err != nil {
		return nil, fmt.Errorf("creating session: %w", err)
	}

	return newRuntimeSession(session, inputNames, dim, outputRank), nil
}

func NewRuntimeSessionFromBytes(data []byte, inputNames, outputNames []string, dim int, outputRank int, intraOpThreads, interOpThreads int, execMode int) (*RuntimeSession, error) {
	opts, err := newSessionOptions(intraOpThreads, interOpThreads, execMode)
	if err != nil {
		return nil, err
	}
	defer func() { _ = opts.Destroy() }()

	session, err := ort.NewDynamicAdvancedSessionWithONNXData(data, inputNames, outputNames, opts)
	if err != nil {
		return nil, fmt.Errorf("creating session: %w", err)
	}

	return newRuntimeSession(session, inputNames, dim, outputRank), nil
}

func newSessionOptions(intraOpThreads, interOpThreads int, execMode int) (*ort.SessionOptions, error) {
	opts, err := ort.NewSessionOptions()
	if err != nil {
		return nil, fmt.Errorf("creating session options: %w", err)
	}

	if intraOpThreads <= 0 {
		intraOpThreads = 1
	}
	if interOpThreads <= 0 {
		interOpThreads = 2
	}
	_ = opts.SetIntraOpNumThreads(intraOpThreads)
	_ = opts.SetInterOpNumThreads(interOpThreads)
	_ = opts.SetGraphOptimizationLevel(ort.GraphOptimizationLevelEnableAll)
	_ = opts.SetCpuMemArena(true)
	_ = opts.SetMemPattern(true)
	if execMode == ExecModeParallel {
		// Only opt into parallel graph execution explicitly; sequential is the
		// ORT default and the documented fit for mostly-serial encoder graphs.
		_ = opts.SetExecutionMode(ort.ExecutionModeParallel)
	}
	_ = opts.SetLogSeverityLevel(ort.LoggingLevelFatal)

	return opts, nil
}

func newRuntimeSession(session *ort.DynamicAdvancedSession, inputNames []string, dim, outputRank int) *RuntimeSession {
	hasAttnMask := false
	hasTokenType := false
	for _, name := range inputNames {
		switch name {
		case "attention_mask":
			hasAttnMask = true
		case "token_type_ids":
			hasTokenType = true
		}
	}

	return &RuntimeSession{
		session:      session,
		dim:          dim,
		hasAttnMask:  hasAttnMask,
		hasTokenType: hasTokenType,
		outputRank:   outputRank,
	}
}

func (s *RuntimeSession) Run(inputIDs, attnMask []int64, batchSize, seqLen, dim int) ([]float32, error) {
	inputTensor, err := ort.NewTensor(ort.NewShape(int64(batchSize), int64(seqLen)), inputIDs)
	if err != nil {
		return nil, fmt.Errorf("creating input_ids tensor: %w", err)
	}
	defer func() { _ = inputTensor.Destroy() }()

	inputs := []ort.Value{inputTensor}
	if s.hasAttnMask {
		attnTensor, err := ort.NewTensor(ort.NewShape(int64(batchSize), int64(seqLen)), attnMask)
		if err != nil {
			return nil, fmt.Errorf("creating attention_mask tensor: %w", err)
		}
		defer func() { _ = attnTensor.Destroy() }()
		inputs = append(inputs, attnTensor)
	}
	if s.hasTokenType {
		ttTensor, err := ort.NewTensor(ort.NewShape(int64(batchSize), int64(seqLen)), make([]int64, batchSize*seqLen))
		if err != nil {
			return nil, fmt.Errorf("creating token_type_ids tensor: %w", err)
		}
		defer func() { _ = ttTensor.Destroy() }()
		inputs = append(inputs, ttTensor)
	}

	var outputShape []int64
	var flatSize int
	if s.outputRank == 2 {
		outputShape = []int64{int64(batchSize), int64(dim)}
		flatSize = batchSize * dim
	} else {
		outputShape = []int64{int64(batchSize), int64(seqLen), int64(dim)}
		flatSize = batchSize * seqLen * dim
	}

	if s.outTensor == nil || !slices.Equal(s.outShape, outputShape) || s.outFlat != flatSize {
		if s.outTensor != nil {
			_ = s.outTensor.Destroy()
		}
		tensor, err := ort.NewEmptyTensor[float32](ort.NewShape(outputShape...))
		if err != nil {
			return nil, fmt.Errorf("creating output tensor: %w", err)
		}
		s.outTensor = tensor
		s.outShape = outputShape
		s.outFlat = flatSize
	}

	outputs := []ort.Value{s.outTensor}

	if err := s.session.Run(inputs, outputs); err != nil {
		return nil, fmt.Errorf("onnx run: %w", err)
	}

	// Zero-copy: the pooled tensor's backing slice is returned directly (no
	// per-run make + copy). The caller must not retain it past pooling.
	return s.outTensor.GetData(), nil
}

func (s *RuntimeSession) Close() error {
	if s.outTensor != nil {
		_ = s.outTensor.Destroy()
		s.outTensor = nil
	}
	return s.session.Destroy()
}

func InitEnvironment(libPath string) error {
	if libPath != "" {
		ort.SetSharedLibraryPath(libPath)
		return ort.InitializeEnvironment()
	}
	if runtime.GOOS == "darwin" {
		names := []string{"onnxruntime.dylib", "libonnxruntime.dylib", "libonnxruntime.1.dylib"}
		for _, lib := range names {
			ort.SetSharedLibraryPath(lib)
			if err := ort.InitializeEnvironment(); err == nil {
				return nil
			}
		}
		if exe, err := os.Executable(); err == nil {
			dir := filepath.Dir(exe)
			for _, lib := range names {
				p := filepath.Join(dir, lib)
				ort.SetSharedLibraryPath(p)
				if err := ort.InitializeEnvironment(); err == nil {
					return nil
				}
			}
		}
	}
	if runtime.GOOS == "linux" {
		for _, lib := range []string{"onnxruntime.so", "libonnxruntime.so", "libonnxruntime.so.1"} {
			ort.SetSharedLibraryPath(lib)
			err := ort.InitializeEnvironment()
			if err == nil {
				return nil
			}
		}
	}
	return ort.InitializeEnvironment()
}

func DestroyEnvironment() error {
	return ort.DestroyEnvironment()
}

func InferDim(modelPath string) (int, error) {
	_, outputs, err := ort.GetInputOutputInfo(modelPath)
	if err != nil {
		return 0, fmt.Errorf("reading ONNX metadata from %q: %w", modelPath, err)
	}
	for _, o := range outputs {
		if len(o.Dimensions) == 2 {
			return int(o.Dimensions[1]), nil
		}
	}
	for _, o := range outputs {
		if len(o.Dimensions) == 3 {
			return int(o.Dimensions[2]), nil
		}
	}
	return 0, fmt.Errorf("could not infer dim from %q outputs", modelPath)
}

func GetInputNames(modelPath string) ([]string, error) {
	inputs, _, err := ort.GetInputOutputInfo(modelPath)
	if err != nil {
		return nil, fmt.Errorf("reading ONNX metadata from %q: %w", modelPath, err)
	}
	names := make([]string, len(inputs))
	for i, inp := range inputs {
		names[i] = inp.Name
	}
	return names, nil
}

func GetOutputInfo(modelPath string) (map[string]OutputInfo, error) {
	_, outputs, err := ort.GetInputOutputInfo(modelPath)
	if err != nil {
		return nil, fmt.Errorf("reading ONNX metadata from %q: %w", modelPath, err)
	}
	result := make(map[string]OutputInfo, len(outputs))
	for _, o := range outputs {
		rank := len(o.Dimensions)
		var dim int64
		if rank >= 2 {
			dim = o.Dimensions[rank-1]
		}
		result[o.Name] = OutputInfo{Name: o.Name, Rank: rank, Dim: dim}
	}
	return result, nil
}

func InferMaxLength(modelDir string) (int, error) {
	data, err := os.ReadFile(filepath.Join(modelDir, "config.json"))
	if err != nil {
		return 0, fmt.Errorf("reading config.json: %w", err)
	}
	var mc struct {
		MaxPositionEmbeddings int `json:"max_position_embeddings"`
	}
	if err := json.Unmarshal(data, &mc); err != nil {
		return 0, fmt.Errorf("parsing config.json: %w", err)
	}
	if mc.MaxPositionEmbeddings <= 0 {
		return 0, fmt.Errorf("max_position_embeddings not found in config.json")
	}
	return mc.MaxPositionEmbeddings, nil
}
