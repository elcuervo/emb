package onnx

import (
	"fmt"
	"strconv"
	"strings"
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
//
// Ownership: the session owns its output-tensor cache (outCache) and the ORT
// session handle. Both are released by Close (idempotent); an evicted cache
// entry has its tensors destroyed rather than left to the garbage collector.
// The cache is bounded by maxCachedOutputShapes.
type NamedRuntimeSession struct {
	session    *ort.DynamicAdvancedSession
	inputNames []string
	outNames   []string
	mu         sync.Mutex
	closed     bool
	// outCache reuses output tensors across calls whose input shapes match a
	// previous call, so a hot loop (repeat evaluations of the same shape) does
	// not pay a per-call output allocation and ORT memory-pattern re-plan. It
	// is bounded and cleared on Close. When a cached buffer does not fit a new
	// call's output shape, the run is retried with auto-allocation and the
	// entry dropped, so correctness never depends on the heuristic.
	outCache map[string][]ort.Value
}

// maxCachedOutputShapes bounds the per-session output-tensor cache. Each entry
// retains one full output set for a given input-shape signature.
const maxCachedOutputShapes = 4

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

	// Reuse the output tensors allocated for an identical input-shape signature;
	// ORT writes into them directly, skipping per-call allocation.
	sig := inputShapeSignature(s.inputNames, inputs)
	var outputs []ort.Value
	cached := false
	if c, ok := s.outCache[sig]; ok && len(c) == len(s.outNames) {
		outputs, cached = c, true
	} else {
		outputs = make([]ort.Value, len(s.outNames))
	}

	if err := s.session.Run(values, outputs); err != nil {
		if cached {
			// The cached buffers did not fit this call's output shapes (the
			// signature did not capture the difference): drop them and retry
			// with ORT auto-allocation.
			s.dropOutputs(sig)
			outputs = make([]ort.Value, len(s.outNames))
			if retryErr := s.session.Run(values, outputs); retryErr != nil {
				destroyValues(outputs)
				return nil, fmt.Errorf("onnx run: %w", retryErr)
			}
			cached = false
		} else {
			destroyValues(outputs)
			return nil, fmt.Errorf("onnx run: %w", err)
		}
	}
	if !cached {
		s.cacheOutputs(sig, outputs)
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

// inputShapeSignature identifies a call by the shapes of its inputs, in the
// graph's registered input order. It is the key for the output-tensor cache.
func inputShapeSignature(names []string, inputs []NamedTensor) string {
	byName := make(map[string][]int64, len(inputs))
	for _, in := range inputs {
		byName[in.Name] = in.Shape
	}
	var b strings.Builder
	for _, n := range names {
		b.WriteString(n)
		for _, d := range byName[n] {
			b.WriteByte(',')
			b.WriteString(strconv.FormatInt(d, 10))
		}
		b.WriteByte(';')
	}
	return b.String()
}

// cacheOutputs takes ownership of a freshly allocated output set, evicting a
// deterministic entry when the cache is full.
func (s *NamedRuntimeSession) cacheOutputs(sig string, outputs []ort.Value) {
	for _, v := range outputs {
		if v == nil {
			return // auto-allocation failed to fill; nothing to cache
		}
	}
	if s.outCache == nil {
		s.outCache = make(map[string][]ort.Value, maxCachedOutputShapes)
	}
	if _, exists := s.outCache[sig]; exists {
		s.dropOutputs(sig)
	}
	if len(s.outCache) >= maxCachedOutputShapes {
		oldest := ""
		for k := range s.outCache {
			if oldest == "" || k < oldest {
				oldest = k
			}
		}
		s.dropOutputs(oldest)
	}
	s.outCache[sig] = outputs
}

// dropOutputs destroys and forgets one cached output set.
func (s *NamedRuntimeSession) dropOutputs(sig string) {
	if s.outCache == nil {
		return
	}
	destroyValues(s.outCache[sig])
	delete(s.outCache, sig)
}

// destroyValue releases one ORT value. It is a package var so the eviction and
// close paths can be tested with counting fakes without an ONNX environment.
var destroyValue = func(v ort.Value) { _ = v.Destroy() }

func destroyValues(values []ort.Value) {
	for _, v := range values {
		if v != nil {
			destroyValue(v)
		}
	}
}

// CachedOutputSets reports how many output-shape signatures the session is
// currently retaining (bounded by maxCachedOutputShapes).
func (s *NamedRuntimeSession) CachedOutputSets() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.outCache)
}

func (s *NamedRuntimeSession) Close() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return nil
	}
	s.closed = true
	for sig := range s.outCache {
		s.dropOutputs(sig)
	}
	s.outCache = nil
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
