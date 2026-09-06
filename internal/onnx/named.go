package onnx

import (
	"fmt"
	"sync"

	ort "github.com/yalue/onnxruntime_go"
)

// TensorType identifies the element type of a NamedTensor.
type TensorType int

const (
	// TensorInt64 is a signed 64-bit integer tensor (ONNX int64).
	TensorInt64 TensorType = iota
	// TensorFloat32 is a single-precision float tensor (ONNX float).
	TensorFloat32
)

// NamedTensor is a typed tensor with an explicit shape, addressed by name.
// Exactly one of Int64 or Float holds the data, matching DType.
type NamedTensor struct {
	Name  string
	Shape []int64
	DType TensorType
	Int64 []int64
	Float []float32
}

func (t NamedTensor) elementCount() int {
	n := 1
	for _, d := range t.Shape {
		n *= int(d)
	}
	return n
}

// NamedSession is the generic multi-tensor session contract used by the
// scripted-model path (see NamedRuntimeSession). The embedding pipeline's
// narrow Session remains separate.
type NamedSession interface {
	RunNamed(inputs []NamedTensor) (map[string]NamedTensor, error)
	Close() error
}

// NamedRuntimeSession is a generic ONNX session that runs arbitrary named
// input/output tensor sets, distinct from the pooled RuntimeSession used by
// the embedding pipeline. ORT sessions serialize their runs, so an instance
// must not run concurrently (the embedded mutex queues concurrent callers).
type NamedRuntimeSession struct {
	session    *ort.DynamicAdvancedSession
	inputNames []string
	outNames   []string
	mu         sync.Mutex
}

// NewNamedRuntimeSessionFromBytes opens a scripted-model session from ONNX
// bytes. The intra/inter-op thread and execution-mode options mirror
// NewRuntimeSessionFromBytes.
func NewNamedRuntimeSessionFromBytes(data []byte, inputNames, outputNames []string, intraOpThreads, interOpThreads int, execMode int) (*NamedRuntimeSession, error) {
	opts, err := newSessionOptions(intraOpThreads, interOpThreads, execMode)
	if err != nil {
		return nil, err
	}
	defer func() { _ = opts.Destroy() }()

	session, err := ort.NewDynamicAdvancedSessionWithONNXData(data, inputNames, outputNames, opts)
	if err != nil {
		return nil, fmt.Errorf("creating session: %w", err)
	}
	return &NamedRuntimeSession{
		session:    session,
		inputNames: inputNames,
		outNames:   outputNames,
	}, nil
}

// RunNamed runs the session with the given named inputs (matched by name
// against the graph's registered inputs, order-independent) and returns every
// registered output keyed by name. Missing or unknown inputs are errors, as
// are duplicate names.
func (s *NamedRuntimeSession) RunNamed(inputs []NamedTensor) (map[string]NamedTensor, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	byName := make(map[string]NamedTensor, len(inputs))
	for _, in := range inputs {
		if _, dup := byName[in.Name]; dup {
			return nil, fmt.Errorf("duplicate input %q", in.Name)
		}
		byName[in.Name] = in
	}

	values := make([]ort.Value, len(s.inputNames))
	for i, name := range s.inputNames {
		in, ok := byName[name]
		if !ok {
			return nil, fmt.Errorf("missing input %q", name)
		}
		v, err := namedTensorValue(in)
		if err != nil {
			return nil, err
		}
		values[i] = v
	}
	defer func() {
		for _, v := range values {
			if v != nil {
				_ = v.Destroy()
			}
		}
	}()

	// nil outputs: ORT auto-allocates them, wrapping each as a typed Tensor.
	outputs := make([]ort.Value, len(s.outNames))
	defer func() {
		for _, v := range outputs {
			if v != nil {
				_ = v.Destroy()
			}
		}
	}()

	if err := s.session.Run(values, outputs); err != nil {
		return nil, fmt.Errorf("onnx run: %w", err)
	}

	result := make(map[string]NamedTensor, len(s.outNames))
	for i, name := range s.outNames {
		t, err := namedTensorFromValue(name, outputs[i])
		if err != nil {
			return nil, err
		}
		result[name] = t
	}
	return result, nil
}

func (s *NamedRuntimeSession) Close() error {
	return s.session.Destroy()
}

func namedTensorValue(t NamedTensor) (ort.Value, error) {
	shape := ort.NewShape(t.Shape...)
	switch t.DType {
	case TensorInt64:
		if len(t.Int64) != t.elementCount() {
			return nil, fmt.Errorf("input %q: expected %d int64 elements, got %d", t.Name, t.elementCount(), len(t.Int64))
		}
		return ort.NewTensor(shape, t.Int64)
	case TensorFloat32:
		if len(t.Float) != t.elementCount() {
			return nil, fmt.Errorf("input %q: expected %d float32 elements, got %d", t.Name, t.elementCount(), len(t.Float))
		}
		return ort.NewTensor(shape, t.Float)
	default:
		return nil, fmt.Errorf("input %q: unsupported dtype %d", t.Name, t.DType)
	}
}

func namedTensorFromValue(name string, v ort.Value) (NamedTensor, error) {
	shape := v.GetShape()
	switch t := v.(type) {
	case *ort.Tensor[int64]:
		data := t.GetData()
		out := make([]int64, len(data))
		copy(out, data)
		return NamedTensor{Name: name, Shape: shape, DType: TensorInt64, Int64: out}, nil
	case *ort.Tensor[float32]:
		data := t.GetData()
		out := make([]float32, len(data))
		copy(out, data)
		return NamedTensor{Name: name, Shape: shape, DType: TensorFloat32, Float: out}, nil
	default:
		return NamedTensor{}, fmt.Errorf("output %q: unsupported tensor type %T", name, v)
	}
}
