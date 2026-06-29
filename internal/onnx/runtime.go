package onnx

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"runtime"

	ort "github.com/yalue/onnxruntime_go"
)

type RuntimeSession struct {
	session      *ort.DynamicAdvancedSession
	dim          int
	hasTokenType bool
}

func NewRuntimeSession(modelPath string, inputNames, outputNames []string, dim int) (*RuntimeSession, error) {
	opts, err := ort.NewSessionOptions()
	if err != nil {
		return nil, fmt.Errorf("creating session options: %w", err)
	}
	defer opts.Destroy()

	opts.SetIntraOpNumThreads(1)
	opts.SetInterOpNumThreads(2)
	opts.SetGraphOptimizationLevel(ort.GraphOptimizationLevelEnableAll)
	opts.SetCpuMemArena(true)
	opts.SetMemPattern(true)
	opts.SetExecutionMode(ort.ExecutionModeParallel)
	opts.SetLogSeverityLevel(ort.LoggingLevelFatal)

	session, err := ort.NewDynamicAdvancedSession(modelPath, inputNames, outputNames, opts)
	if err != nil {
		return nil, fmt.Errorf("creating session: %w", err)
	}

	hasTokenType := false
	for _, name := range inputNames {
		if name == "token_type_ids" {
			hasTokenType = true
			break
		}
	}

	return &RuntimeSession{
		session:      session,
		dim:          dim,
		hasTokenType: hasTokenType,
	}, nil
}

func (s *RuntimeSession) Run(inputIDs, attnMask []int64, batchSize, seqLen, dim int) ([]float32, error) {
	inputTensor, err := ort.NewTensor(ort.NewShape(int64(batchSize), int64(seqLen)), inputIDs)
	if err != nil {
		return nil, fmt.Errorf("creating input_ids tensor: %w", err)
	}
	defer inputTensor.Destroy()

	attnTensor, err := ort.NewTensor(ort.NewShape(int64(batchSize), int64(seqLen)), attnMask)
	if err != nil {
		return nil, fmt.Errorf("creating attention_mask tensor: %w", err)
	}
	defer attnTensor.Destroy()

	inputs := []ort.Value{inputTensor, attnTensor}
	if s.hasTokenType {
		ttTensor, err := ort.NewTensor(ort.NewShape(int64(batchSize), int64(seqLen)), make([]int64, batchSize*seqLen))
		if err != nil {
			return nil, fmt.Errorf("creating token_type_ids tensor: %w", err)
		}
		defer ttTensor.Destroy()
		inputs = append(inputs, ttTensor)
	}

	outputTensor, err := ort.NewEmptyTensor[float32](ort.NewShape(int64(batchSize), int64(seqLen), int64(dim)))
	if err != nil {
		return nil, fmt.Errorf("creating output tensor: %w", err)
	}
	defer outputTensor.Destroy()

	outputs := []ort.Value{outputTensor}

	if err := s.session.Run(inputs, outputs); err != nil {
		return nil, fmt.Errorf("onnx run: %w", err)
	}

	data := outputTensor.GetData()
	result := make([]float32, batchSize*seqLen*dim)
	copy(result, data)
	return result, nil
}

func (s *RuntimeSession) Close() error {
	return s.session.Destroy()
}

func InitEnvironment(libPath string) error {
	if libPath != "" {
		ort.SetSharedLibraryPath(libPath)
		return ort.InitializeEnvironment()
	}
	if runtime.GOOS == "darwin" {
		for _, lib := range []string{"onnxruntime.dylib", "libonnxruntime.dylib"} {
			ort.SetSharedLibraryPath(lib)
			err := ort.InitializeEnvironment()
			if err == nil {
				return nil
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
		if len(o.Dimensions) == 3 {
			return int(o.Dimensions[2]), nil
		}
	}
	return 0, fmt.Errorf("could not infer dim from %q outputs", modelPath)
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
