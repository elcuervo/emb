package registry

import (
	"encoding/json"
	"fmt"
	"log"
	"math"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"

	"github.com/elcuervo/emb/internal/config"
	"github.com/elcuervo/emb/internal/imageproc"
	"github.com/elcuervo/emb/internal/onnx"
	"github.com/elcuervo/emb/internal/pipeline"
)

// imagePlanResult memoizes the outcome of resolving a model's preprocessing
// plan without opening sessions.
type imagePlanResult struct {
	plan imageproc.Plan
	err  error
}

// ImageResources bundles what an image embedding request needs: a pool of
// named-tensor sessions (round-robin, so concurrent EMB.IMG requests do not
// serialize) and the immutable preprocessing plan resolved at load.
type ImageResources struct {
	// Sessions is the named-tensor session pool (round-robin). It is exported so
	// tests can inject a fake; callers should use Session().
	Sessions []onnx.NamedSession
	next     atomic.Uint64

	// Plan is the resolved, immutable preprocessing plan.
	Plan imageproc.Plan
	// OutputTensor is the graph output the image embedding is read from.
	OutputTensor string
	// Pooling is the model's pooling mode ("none", "cls", or "mean").
	Pooling string
	// Normalize is the model's L2-normalization setting.
	Normalize bool
	// Dim is the embedding dimension shared with the text branch.
	Dim int
}

// Session returns the next named-tensor session for an image run, round-robin
// across the pool. Each session serializes its own runs.
func (r *ImageResources) Session() onnx.NamedSession {
	i := r.next.Add(1) - 1
	return r.Sessions[i%uint64(len(r.Sessions))]
}

// Embed runs one batched inference over the preprocessed image tensors and
// returns one little-endian float32 embedding per image, in order. Exactly one
// session run executes regardless of the batch size.
func (r *ImageResources) Embed(tensors [][]float32) ([][]byte, error) {
	n := len(tensors)
	if n == 0 {
		return nil, nil
	}
	elem := r.Plan.ElementCount()
	batch := make([]float32, n*elem)
	for i, t := range tensors {
		if len(t) != elem {
			return nil, fmt.Errorf("image %d: tensor has %d elements, want %d", i, len(t), elem)
		}
		copy(batch[i*elem:], t)
	}
	shape := []int64{int64(n), 3, int64(r.Plan.Size), int64(r.Plan.Size)}
	outs, err := r.Session().RunNamed([]onnx.NamedTensor{{
		Name:  r.Plan.Input,
		Shape: shape,
		DType: onnx.TensorFloat32,
		Float: batch,
	}})
	if err != nil {
		return nil, err
	}
	out, ok := outs[r.OutputTensor]
	if !ok {
		return nil, fmt.Errorf("output tensor %q missing from graph result", r.OutputTensor)
	}
	return r.pool(out, n)
}

func (r *ImageResources) pool(out onnx.NamedTensor, batch int) ([][]byte, error) {
	rank := len(out.Shape)
	if rank < 2 {
		return nil, fmt.Errorf("image output %q has rank %d, want >= 2", r.OutputTensor, rank)
	}
	dim := int(out.Shape[rank-1])
	if r.Dim > 0 && dim != r.Dim {
		return nil, fmt.Errorf("image output %q dim %d does not match model dim %d", r.OutputTensor, dim, r.Dim)
	}
	switch strings.ToLower(r.Pooling) {
	case "none", "":
		return pipeline.ExtractPrePooled(out.Float, batch, dim, r.Normalize), nil
	case "cls":
		if rank < 3 {
			return pipeline.ExtractPrePooled(out.Float, batch, dim, r.Normalize), nil
		}
		seq := int(out.Shape[rank-2])
		return pipeline.ExtractCLS(out.Float, batch, dim, seq, r.Normalize), nil
	default: // mean
		if rank < 3 {
			return pipeline.ExtractPrePooled(out.Float, batch, dim, r.Normalize), nil
		}
		seq := int(out.Shape[rank-2])
		masks := make([]int64, batch*seq)
		for i := range masks {
			masks[i] = 1
		}
		return pipeline.MeanPoolAndNormalize(out.Float, masks, dim, seq, batch, r.Normalize), nil
	}
}

// HasImageSurface reports whether the model can serve the image surface — an
// image: block in its config, or a test-injected ImageRes — without opening
// any session. It is the gate for binding emb.image.* into the sandbox.
func (e *ModelEntry) HasImageSurface() bool {
	return e.cfg.Image != nil || e.ImageRes != nil
}

// ImagePlan resolves and validates the model's immutable preprocessing plan
// without opening any inference session. It is memoized, so emb.image.info and
// emb.image.preprocess can report/use configuration while the image session
// pool stays unopened. A test-injected ImageRes (no image: block) contributes
// its pre-built plan.
func (e *ModelEntry) ImagePlan() (imageproc.Plan, error) {
	e.imagePlanOnce.Do(func() {
		if e.cfg.Image == nil {
			// Text-only model; ImageRes is only ever set by test injection
			// before the server starts serving.
			if e.ImageRes != nil {
				e.imagePlanRes = &imagePlanResult{plan: e.ImageRes.Plan}
			} else {
				e.imagePlanRes = &imagePlanResult{}
			}
			return
		}
		plan, err := resolveImagePlan(e.cfg, e.Name)
		e.imagePlanRes = &imagePlanResult{plan: plan, err: err}
	})
	return e.imagePlanRes.plan, e.imagePlanRes.err
}

// ImageResources lazily opens the image named-session pool and resolves the
// preprocessing plan. It returns (nil, nil) for a model without an image
// block, so callers use a nil result to mean "this model is text-only".
func (e *ModelEntry) ImageResources() (*ImageResources, error) {
	if e.cfg.Image == nil {
		// Text-only model; ImageRes is only ever set by test injection, before
		// the server starts serving.
		return e.ImageRes, nil
	}
	e.imageOnce.Do(func() {
		if e.ImageRes == nil {
			e.ImageRes, e.imageResErr = e.openImageResources()
		}
	})
	return e.ImageRes, e.imageResErr
}

func (e *ModelEntry) openImageResources() (*ImageResources, error) {
	cfg := e.cfg
	plan, err := e.ImagePlan()
	if err != nil {
		return nil, err
	}
	imageOutput := cfg.Image.Output
	if imageOutput == "" {
		imageOutput = cfg.OutputTensor
	}
	if err := validateImagePairing(cfg, e.Name, imageOutput); err != nil {
		return nil, err
	}

	inputNames, err := onnx.GetInputNames(cfg.ONNX)
	if err != nil {
		return nil, fmt.Errorf("reading input names for %q: %w", e.Name, err)
	}
	outInfo, err := onnx.GetOutputInfo(cfg.ONNX)
	if err != nil {
		return nil, fmt.Errorf("reading output info for %q: %w", e.Name, err)
	}
	if _, ok := outInfo[imageOutput]; !ok && imageOutput != "" {
		return nil, fmt.Errorf("model %q: image output tensor %q not found in graph", e.Name, imageOutput)
	}
	outputNames := make([]string, 0, len(outInfo))
	for n := range outInfo {
		outputNames = append(outputNames, n)
	}

	modelData, err := os.ReadFile(cfg.ONNX)
	if err != nil {
		return nil, fmt.Errorf("reading model file for %q: %w", e.Name, err)
	}

	intraThreads := cfg.IntraOpThreads
	if intraThreads <= 0 {
		intraThreads = defaultIntraOpThreads()
	}
	execMode := onnx.ExecModeSequential
	if cfg.ExecutionMode == "parallel" {
		execMode = onnx.ExecModeParallel
	}

	numSessions := autoTuneWorkers(cfg.ONNX, 0)
	if numSessions < 1 {
		numSessions = 1
	}
	sessions := make([]onnx.NamedSession, 0, numSessions)
	for i := 0; i < numSessions; i++ {
		sess, err := newNamedSession(
			modelData, inputNames, outputNames, intraThreads, cfg.InterOpThreads, execMode,
		)
		if err != nil {
			for _, opened := range sessions {
				_ = opened.Close()
			}
			return nil, fmt.Errorf("creating image session %d for %q: %w", i, e.Name, err)
		}
		sessions = append(sessions, sess)
	}

	e.imageSessions.Store(int64(len(sessions)))
	return &ImageResources{
		Sessions:     sessions,
		Plan:         plan,
		OutputTensor: imageOutput,
		Pooling:      cfg.Pooling,
		Normalize:    cfg.Normalize,
		Dim:          cfg.Dim,
	}, nil
}

// checkImagePairing is the pure dual-encoder rule: when the same model serves
// text and images, their embedding dimensions must agree, or text-to-image
// similarity is meaningless. It returns a descriptive error naming both
// dimensions.
func checkImagePairing(name, imageOutput string, textDim, imageDim int) error {
	if textDim <= 0 || imageDim <= 0 || textDim == imageDim {
		return nil
	}
	return fmt.Errorf("model %q: dual-encoder pairing violated: text dim %d and image output %q dim %d disagree; text-to-image retrieval would be meaningless",
		name, textDim, imageOutput, imageDim)
}

func validateImagePairing(cfg config.ModelConfig, name, imageOutput string) error {
	if imageOutput == "" || cfg.Dim <= 0 {
		return nil
	}
	info, err := onnx.GetOutputInfo(cfg.ONNX)
	if err != nil {
		return fmt.Errorf("model %q: reading image output info: %w", name, err)
	}
	oi, ok := info[imageOutput]
	if !ok {
		return fmt.Errorf("model %q: image output tensor %q not found in graph", name, imageOutput)
	}
	return checkImagePairing(name, imageOutput, cfg.Dim, int(oi.Dim))
}

// preprocessorConfig is the subset of HuggingFace preprocessor_config.json the
// image autoconfiguration reads.
type preprocessorConfig struct {
	DoCenterCrop  *bool           `json:"do_center_crop"`
	Size          json.RawMessage `json:"size"`
	CropSize      json.RawMessage `json:"crop_size"`
	Resample      json.RawMessage `json:"resample"`
	RescaleFactor *float64        `json:"rescale_factor"`
	ImageMean     []float64       `json:"image_mean"`
	ImageStd      []float64       `json:"image_std"`
}

// loadPreprocessorConfig finds and parses preprocessor_config.json next to the
// model or one directory up (HF repos often keep it at the repo root while the
// ONNX file lives in onnx/). A missing file returns (nil, nil).
func loadPreprocessorConfig(modelPath string) (*preprocessorConfig, error) {
	dir := filepath.Dir(modelPath)
	for _, cand := range []string{
		filepath.Join(dir, "preprocessor_config.json"),
		filepath.Join(filepath.Dir(dir), "preprocessor_config.json"),
	} {
		data, err := os.ReadFile(cand)
		if err != nil {
			continue
		}
		var pc preprocessorConfig
		if err := json.Unmarshal(data, &pc); err != nil {
			return nil, fmt.Errorf("parsing %s: %w", cand, err)
		}
		return &pc, nil
	}
	return nil, nil
}

// preprocessorSize extracts a square target size from a preprocessor size or
// crop_size value, which may be an integer, {"shortest_edge": n}, or
// {"height": h, "width": w}.
func preprocessorSize(raw json.RawMessage) (int, bool) {
	if len(raw) == 0 {
		return 0, false
	}
	var n int
	if err := json.Unmarshal(raw, &n); err == nil && n > 0 {
		return n, true
	}
	var obj struct {
		ShortestEdge int `json:"shortest_edge"`
		Height       int `json:"height"`
		Width        int `json:"width"`
	}
	if err := json.Unmarshal(raw, &obj); err == nil {
		if obj.Height > 0 && obj.Height == obj.Width {
			return obj.Height, true
		}
		if obj.ShortestEdge > 0 {
			return obj.ShortestEdge, true
		}
		if obj.Height > 0 && obj.Width == 0 {
			return obj.Height, true
		}
	}
	return 0, false
}

// preprocessorResample maps a PIL resample constant (or a name) to a Resample.
func preprocessorResample(raw json.RawMessage) (imageproc.Resample, bool) {
	if len(raw) == 0 {
		return 0, false
	}
	var n int
	if err := json.Unmarshal(raw, &n); err == nil {
		switch n {
		case 0:
			return imageproc.ResampleNearest, true
		case 1:
			return imageproc.ResampleLanczos, true
		case 2:
			return imageproc.ResampleBilinear, true
		case 3:
			return imageproc.ResampleBicubic, true
		}
		return 0, false
	}
	var s string
	if err := json.Unmarshal(raw, &s); err == nil {
		if r, err := imageproc.ParseResample(strings.ToLower(s)); err == nil {
			return r, true
		}
	}
	return 0, false
}

// detectImageInput scans the graph inputs for a 4D image tensor (rank 4 with 3
// channels) and returns its name plus a static square spatial size when known.
func detectImageInput(modelPath string) (name string, size int, found bool) {
	if _, err := os.Stat(modelPath); err != nil {
		return "", 0, false
	}
	infos, err := onnx.GetInputInfo(modelPath)
	if err != nil {
		return "", 0, false
	}
	return detectImageInputFromInfos(infos)
}

// detectImageInputFromInfos is the pure detection rule behind detectImageInput,
// split out so it can be tested without an ONNX environment: prefer the
// conventional pixel_values name, then any 3-channel rank-4 input, then any
// rank-4 input.
func detectImageInputFromInfos(infos []onnx.InputInfo) (name string, size int, found bool) {
	for _, in := range infos {
		if in.Name == "pixel_values" && in.Rank == 4 {
			return in.Name, staticSquareSize(in.Dimensions), true
		}
	}
	for _, in := range infos {
		if in.Rank != 4 || len(in.Dimensions) != 4 {
			continue
		}
		if in.Dimensions[1] == 3 || in.Dimensions[1] == -1 {
			return in.Name, staticSquareSize(in.Dimensions), true
		}
	}
	for _, in := range infos {
		if in.Rank == 4 {
			return in.Name, staticSquareSize(in.Dimensions), true
		}
	}
	return "", 0, false
}

func staticSquareSize(dims []int64) int {
	if len(dims) != 4 {
		return 0
	}
	h, w := dims[2], dims[3]
	if h > 0 && h == w {
		return int(h)
	}
	if h > 0 && w <= 0 {
		return int(h)
	}
	if w > 0 && h <= 0 {
		return int(w)
	}
	return 0
}

// resolveImagePlan produces the immutable preprocessing plan for a model:
// explicit configuration wins, then preprocessor_config.json, then the ONNX
// graph, then documented defaults. A size that cannot be determined fails load
// with a descriptive error rather than guessing.
func resolveImagePlan(cfg config.ModelConfig, name string) (imageproc.Plan, error) {
	ic := cfg.Image
	if ic == nil {
		return imageproc.Plan{}, nil
	}
	crop, err := imageproc.ParseCrop(ic.Crop)
	if err != nil {
		return imageproc.Plan{}, fmt.Errorf("model %q: %w", name, err)
	}
	resample, err := imageproc.ParseResample(ic.Resample)
	if err != nil {
		return imageproc.Plan{}, fmt.Errorf("model %q: %w", name, err)
	}
	resampleSet := ic.Resample != ""
	cropSet := ic.Crop != ""

	plan := imageproc.Plan{
		Input:    ic.Input,
		Size:     ic.Size,
		Crop:     crop,
		Resample: resample,
		Rescale:  1.0 / 255.0,
		Mean:     [3]float64{0.5, 0.5, 0.5},
		Std:      [3]float64{0.5, 0.5, 0.5},
	}

	pp, err := loadPreprocessorConfig(cfg.ONNX)
	if err != nil {
		return imageproc.Plan{}, fmt.Errorf("model %q: %w", name, err)
	}
	if pp != nil {
		if !cropSet && pp.DoCenterCrop != nil {
			if *pp.DoCenterCrop {
				plan.Crop = imageproc.CropCenter
			} else {
				plan.Crop = imageproc.CropNone
			}
		}
		if !resampleSet {
			if rs, ok := preprocessorResample(pp.Resample); ok {
				plan.Resample = rs
			}
		}
		if ic.Rescale == nil && pp.RescaleFactor != nil {
			plan.Rescale = *pp.RescaleFactor
		}
		if len(ic.Mean) == 0 && len(pp.ImageMean) == 3 {
			copy(plan.Mean[:], pp.ImageMean)
		}
		if len(ic.Std) == 0 && len(pp.ImageStd) == 3 {
			copy(plan.Std[:], pp.ImageStd)
		}
		if plan.Size == 0 {
			if sz, ok := preprocessorSize(pp.CropSize); ok {
				plan.Size = sz
			} else if sz, ok := preprocessorSize(pp.Size); ok {
				plan.Size = sz
			}
		}
	}
	// Explicit mean/std/rescale override anything detected.
	if len(ic.Mean) == 3 {
		copy(plan.Mean[:], ic.Mean)
	}
	if len(ic.Std) == 3 {
		copy(plan.Std[:], ic.Std)
	}
	if ic.Rescale != nil {
		plan.Rescale = *ic.Rescale
	}

	// Graph detection fills an unset image input and size.
	if plan.Input == "" || plan.Size == 0 {
		detName, detSize, found := detectImageInput(cfg.ONNX)
		if found {
			if plan.Input == "" {
				plan.Input = detName
			}
			if plan.Size == 0 {
				plan.Size = detSize
			}
		}
	}
	if plan.Input == "" {
		plan.Input = "pixel_values"
	}
	if plan.Size <= 0 {
		return imageproc.Plan{}, fmt.Errorf("model %q: image.size could not be determined from config, preprocessor_config.json, or the ONNX graph; set image.size explicitly", name)
	}
	if err := validateImagePlan(name, plan); err != nil {
		return imageproc.Plan{}, err
	}
	return plan, nil
}

// validateImagePlan rejects a fully resolved plan whose numeric fields would
// produce non-finite tensors. Explicit configuration is checked in
// config.validateImageConfig, but rescale/mean/std copied from
// preprocessor_config.json (or the built-in defaults) reach imageproc.Plan
// directly, so the resolved values are re-checked at this boundary.
func validateImagePlan(name string, plan imageproc.Plan) error {
	if math.IsNaN(plan.Rescale) || math.IsInf(plan.Rescale, 0) || plan.Rescale < 0 {
		return fmt.Errorf("model %q: image rescale must be finite and non-negative, got %v", name, plan.Rescale)
	}
	for i, m := range plan.Mean {
		if math.IsNaN(m) || math.IsInf(m, 0) {
			return fmt.Errorf("model %q: image mean channel %d must be finite, got %v", name, i, m)
		}
	}
	for i, s := range plan.Std {
		if math.IsNaN(s) || math.IsInf(s, 0) || s <= 0 {
			return fmt.Errorf("model %q: image std channel %d must be finite and positive, got %v", name, i, s)
		}
	}
	return nil
}

// LogImagePlan reports the resolved plan at load for operator visibility.
func logImagePlan(name string, res *ImageResources) {
	if res == nil {
		return
	}
	log.Printf("  %s: image input=%q size=%d crop=%s resample=%s output=%q dim=%d sessions=%d",
		name, res.Plan.Input, res.Plan.Size, res.Plan.Crop, res.Plan.Resample,
		res.OutputTensor, res.Dim, len(res.Sessions))
}

// closeImageSessions releases the pool (used by tests and Registry.Close).
func (e *ModelEntry) closeImageSessions() {
	if e.ImageRes == nil {
		return
	}
	for _, sess := range e.ImageRes.Sessions {
		_ = sess.Close()
	}
}
