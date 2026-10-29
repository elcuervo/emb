package registry

import (
	"encoding/json"
	"fmt"
	"log"
	"math"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync/atomic"

	"github.com/elcuervo/emb/internal/config"
	"github.com/elcuervo/emb/internal/imageproc"
	"github.com/elcuervo/emb/internal/onnx"
	"github.com/elcuervo/emb/internal/pipeline"
)

// ImageOutput is one named product of a model's image branch, resolved at load:
// which graph tensor it reads, how its rows are reduced, and the shape the
// reply advertises.
type ImageOutput struct {
	// Name is the reply key clients read.
	Name string
	// Tensor is the graph output tensor it is read from.
	Tensor string
	// Select is the reduction: "none", "cls", "mean", "tokens", or "map" (a
	// 2-D tensor emitted flat, never normalized).
	Select string
	// Start is the first row of a "tokens" output (1 CLS + register tokens for
	// a DINOv3-family export).
	Start int
	// Grid is the patch edge of a "tokens" output; Rows is grid*grid. Both are
	// 0 for pooled and map outputs.
	Grid int
	// Rows is how many vectors one image yields for this output, or a map's
	// height.
	Rows int
	// Dim is the output tensor's last dimension: a row's width for tokens, a
	// map's width for map.
	Dim int
}

// imagePreset is one family profile: the preprocessing constants and token
// layout a model inherits when neither its config nor the export's metadata
// states them.
type imagePreset struct {
	Mean     [3]float64
	Std      [3]float64
	Rescale  float64
	Resample imageproc.Resample
	Crop     imageproc.CropMode
	// LayoutFromModel makes an unset token grid come from the model's own
	// config.json (size / patch_size) and an unset start from its register
	// count.
	LayoutFromModel bool
}

// imagePresets are the named family profiles image.preset may select. A new
// model family is a row here plus documentation, never a code branch.
var imagePresets = map[string]imagePreset{
	"clip": {
		Mean:     [3]float64{0.48145466, 0.4578275, 0.40821073},
		Std:      [3]float64{0.26862954, 0.26130258, 0.27577711},
		Rescale:  1.0 / 255.0,
		Resample: imageproc.ResampleBicubic,
		Crop:     imageproc.CropCenter,
	},
	"siglip": {
		Mean:     [3]float64{0.5, 0.5, 0.5},
		Std:      [3]float64{0.5, 0.5, 0.5},
		Rescale:  1.0 / 255.0,
		Resample: imageproc.ResampleBicubic,
		Crop:     imageproc.CropNone,
	},
	"imagenet": {
		Mean:     [3]float64{0.485, 0.456, 0.406},
		Std:      [3]float64{0.229, 0.224, 0.225},
		Rescale:  1.0 / 255.0,
		Resample: imageproc.ResampleBicubic,
		Crop:     imageproc.CropNone,
	},
	"dinov3": {
		Mean:            [3]float64{0.485, 0.456, 0.406},
		Std:             [3]float64{0.229, 0.224, 0.225},
		Rescale:         1.0 / 255.0,
		Resample:        imageproc.ResampleBilinear,
		Crop:            imageproc.CropNone,
		LayoutFromModel: true,
	},
}

// imagePresetNames lists the valid presets for an error message, sorted so the
// message is stable.
func imagePresetNames() string {
	return "clip, siglip, imagenet, dinov3"
}

// modelImageMetadata is the layout subset of a model's config.json that an
// export publishes about itself.
type modelImageMetadata struct {
	PatchSize         int `json:"patch_size"`
	NumRegisterTokens int `json:"num_register_tokens"`
}

// loadModelImageMetadata reads config.json beside the graph or one directory up
// (HF repos often keep it at the repo root while the ONNX file lives in onnx/).
// A missing file returns the zero metadata.
func loadModelImageMetadata(modelPath string) modelImageMetadata {
	dir := filepath.Dir(modelPath)
	for _, cand := range []string{
		filepath.Join(dir, "config.json"),
		filepath.Join(filepath.Dir(dir), "config.json"),
	} {
		data, err := os.ReadFile(cand)
		if err != nil {
			continue
		}
		var md modelImageMetadata
		if err := json.Unmarshal(data, &md); err != nil {
			continue
		}
		return md
	}
	return modelImageMetadata{}
}

// imagePlanSources records which source supplied each resolved preprocessing
// field, so a wrong family default is visible in the boot log instead of silent.
type imagePlanSources struct {
	Input    string
	Size     string
	Crop     string
	Resample string
	Rescale  string
	Mean     string
	Std      string
}

// imagePlanResult memoizes the outcome of resolving a model's preprocessing
// plan and its named outputs without opening sessions.
type imagePlanResult struct {
	plan    imageproc.Plan
	outputs []ImageOutput
	sources imagePlanSources
	err     error
}

// ImageResources bundles what an image embedding request needs: a pool of
// named-tensor sessions (round-robin, so concurrent EMB.IMG requests do not
// serialize), the immutable preprocessing plan, and the model's named outputs.
type ImageResources struct {
	// Sessions is the named-tensor session pool. It is exported so tests can
	// inject a fake; callers should use RunNamed (or Session for a specific
	// instance).
	Sessions []onnx.NamedSession
	// idle hands out whichever session is free first, so concurrent EMB.IMG
	// requests never queue behind a busy session while another is idle.
	idle chan onnx.NamedSession
	next atomic.Uint64

	// Plan is the resolved, immutable preprocessing plan.
	Plan imageproc.Plan
	// Outputs is the resolved ordered list of this model's image products.
	Outputs []ImageOutput
	// Normalize is the model's L2-normalization setting.
	Normalize bool
	// BatchCap is the largest batch the graph's image input accepts: a positive
	// statically declared batch dimension, or 0 when the axis is symbolic (an
	// unbounded batch).
	BatchCap int
}

// Session returns the next named-tensor session for an image run, round-robin
// across the pool. Each session serializes its own runs.
func (r *ImageResources) Session() onnx.NamedSession {
	i := r.next.Add(1) - 1
	return r.Sessions[i%uint64(len(r.Sessions))]
}

// RunNamed runs an inference on the first session that is free. A
// test-injected pool without an idle channel (idle == nil) falls back to
// round-robin.
func (r *ImageResources) RunNamed(inputs []onnx.NamedTensor) (map[string]onnx.NamedTensor, error) {
	if r.idle == nil {
		return r.Session().RunNamed(inputs)
	}
	sess := <-r.idle
	out, err := sess.RunNamed(inputs)
	r.idle <- sess
	return out, err
}

// Embed runs one batched inference over the preprocessed image tensors and
// returns, per image, one float32 blob per requested output name. Exactly one
// session run executes regardless of the batch size or how many outputs are
// requested.
func (r *ImageResources) Embed(tensors [][]float32, outs []ImageOutput) ([]map[string][]byte, error) {
	n := len(tensors)
	if n == 0 {
		return nil, nil
	}
	if len(outs) == 0 {
		return nil, fmt.Errorf("image request selected no outputs")
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
	outsByTensor, err := r.RunNamed([]onnx.NamedTensor{{
		Name:  r.Plan.Input,
		Shape: shape,
		DType: onnx.TensorFloat32,
		Float: batch,
	}})
	if err != nil {
		return nil, err
	}
	results := make([]map[string][]byte, n)
	for _, o := range outs {
		tensor, ok := outsByTensor[o.Tensor]
		if !ok {
			return nil, fmt.Errorf("output tensor %q missing from graph result", o.Tensor)
		}
		blobs, err := r.extract(tensor, n, o)
		if err != nil {
			return nil, err
		}
		for i := range results {
			if results[i] == nil {
				results[i] = make(map[string][]byte, len(outs))
			}
			results[i][o.Name] = blobs[i]
		}
	}
	return results, nil
}

// extract reduces one graph output to one float32 blob per image, refusing a
// tensor whose shape contradicts the configured output before any row is read:
// a wrong batch would read rows that are not there, and a wrong row count would
// return a silently shifted grid.
func (r *ImageResources) extract(out onnx.NamedTensor, batch int, o ImageOutput) ([][]byte, error) {
	rank := len(out.Shape)
	if rank < 2 {
		return nil, fmt.Errorf("image output %q has rank %d, want >= 2", o.Name, rank)
	}
	if got := int(out.Shape[0]); got != batch {
		return nil, fmt.Errorf("image output %q has batch dimension %d, want %d", o.Name, got, batch)
	}
	dim := int(out.Shape[rank-1])
	if o.Dim > 0 && dim != o.Dim {
		return nil, fmt.Errorf("image output %q dim %d does not match the configured dim %d", o.Name, dim, o.Dim)
	}
	seq := 0
	if rank >= 3 {
		seq = int(out.Shape[rank-2])
	}
	switch o.Select {
	case "tokens":
		if rank < 3 {
			return nil, fmt.Errorf("image output %q selects tokens from a rank-%d tensor", o.Name, rank)
		}
		if got := seq - o.Start; got != o.Rows {
			return nil, fmt.Errorf("image output %q has %d rows after start=%d, want %d (grid %dx%d)",
				o.Name, got, o.Start, o.Rows, o.Grid, o.Grid)
		}
		return pipeline.ExtractTokenRows(out.Float, batch, dim, seq, o.Start, r.Normalize), nil
	case "cls":
		if rank < 3 {
			return nil, fmt.Errorf("image output %q selects cls from a rank-%d tensor", o.Name, rank)
		}
		return pipeline.ExtractCLS(out.Float, batch, dim, seq, r.Normalize), nil
	case "mean":
		if rank < 3 {
			return nil, fmt.Errorf("image output %q selects mean from a rank-%d tensor", o.Name, rank)
		}
		masks := make([]int64, batch*seq)
		for i := range masks {
			masks[i] = 1
		}
		return pipeline.MeanPoolAndNormalize(out.Float, masks, dim, seq, batch, r.Normalize), nil
	case "map":
		var rows, cols int
		switch rank {
		case 3:
			rows, cols = int(out.Shape[1]), int(out.Shape[2])
		case 4:
			if ch := int(out.Shape[1]); ch != 1 {
				return nil, fmt.Errorf("image output %q maps %d channels, want 1", o.Name, ch)
			}
			rows, cols = int(out.Shape[2]), int(out.Shape[3])
		default:
			return nil, fmt.Errorf("image output %q selects map from a rank-%d tensor", o.Name, rank)
		}
		if rows != o.Rows || cols != o.Dim {
			return nil, fmt.Errorf("image output %q is %d×%d, want %d×%d (image.size)", o.Name, rows, cols, o.Rows, o.Dim)
		}
		return pipeline.ExtractMap(out.Float, batch, o.Rows*o.Dim), nil
	default: // "none": the graph already pooled the rows
		return pipeline.ExtractPrePooled(out.Float, batch, dim, r.Normalize), nil
	}
}

// HasImageSurface reports whether the model can serve the image surface — an
// image: block in its config, or a test-injected ImageRes — without opening
// any session. It is the gate for binding emb.image.* into the sandbox.
func (e *ModelEntry) HasImageSurface() bool {
	return e.cfg.Image != nil || e.ImageRes != nil
}

// resolveImage once resolves and validates the model's immutable preprocessing
// plan and named outputs, without opening any inference session. A
// test-injected ImageRes (no image: block) contributes its pre-built plan and
// outputs.
func (e *ModelEntry) resolveImage() *imagePlanResult {
	e.imagePlanOnce.Do(func() {
		if e.cfg.Image == nil {
			// Text-only model; ImageRes is only ever set by test injection
			// before the server starts serving.
			if e.ImageRes != nil {
				e.imagePlanRes = &imagePlanResult{plan: e.ImageRes.Plan, outputs: e.ImageRes.Outputs}
			} else {
				e.imagePlanRes = &imagePlanResult{}
			}
			return
		}
		plan, outputs, sources, err := resolveImageResources(e.cfg, e.Name)
		e.imagePlanRes = &imagePlanResult{plan: plan, outputs: outputs, sources: sources, err: err}
	})
	return e.imagePlanRes
}

// ImagePlan resolves and validates the model's immutable preprocessing plan
// without opening any inference session. It is memoized, so emb.image.info and
// emb.image.preprocess can report/use configuration while the image session
// pool stays unopened.
func (e *ModelEntry) ImagePlan() (imageproc.Plan, error) {
	res := e.resolveImage()
	return res.plan, res.err
}

// ImageOutputs reports the model's resolved image outputs without opening any
// inference session. It is what EMB.INFO and emb.image.info list.
func (e *ModelEntry) ImageOutputs() ([]ImageOutput, error) {
	res := e.resolveImage()
	return res.outputs, res.err
}

// ImageResources lazily opens the image named-session pool. It returns
// (nil, nil) for a model without an image block, so callers use a nil result to
// mean "this model is text-only".
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
	// An explicit workers value bounds the image pool exactly as it bounds the
	// embedding pool; unset keeps the RAM-derived auto-tune.
	numSessions := cfg.Workers
	if numSessions <= 0 {
		numSessions = autoTuneWorkers(cfg.ONNX, 0)
	}
	if e.cpuPolicy {
		numSessions = 1
	}
	if tuned := int(e.tunedWorkers.Load()); e.cpuPolicy && cfg.Workers <= 0 && tuned > 0 {
		numSessions = tuned
	}
	if numSessions < 1 {
		numSessions = 1
	}
	if err := e.resolveImage().err; err != nil {
		return nil, err
	}
	return e.buildImageResources(numSessions)
}

// buildImageResources opens numSessions named-tensor sessions over the resolved
// plan and outputs. Sessions are created here and nowhere else, so the
// production pool and the single-session test helper cannot drift.
func (e *ModelEntry) buildImageResources(numSessions int) (*ImageResources, error) {
	cfg := e.cfg
	resolved := e.resolveImage()
	if resolved.err != nil {
		return nil, resolved.err
	}
	plan, outputs := resolved.plan, resolved.outputs

	inputInfos, err := onnx.GetInputInfo(cfg.ONNX)
	if err != nil {
		return nil, fmt.Errorf("reading input info for %q: %w", e.Name, err)
	}
	inputNames := make([]string, len(inputInfos))
	for i, in := range inputInfos {
		inputNames[i] = in.Name
	}
	outInfo, err := onnx.GetOutputInfo(cfg.ONNX)
	if err != nil {
		return nil, fmt.Errorf("reading output info for %q: %w", e.Name, err)
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
		if e.cpuPolicy {
			intraThreads = e.cpuThreads
		} else {
			intraThreads = defaultIntraOpThreads()
		}
	}
	if tuned := int(e.tunedThreads.Load()); e.cpuPolicy && cfg.IntraOpThreads <= 0 && tuned > 0 {
		intraThreads = tuned
	}
	execMode := onnx.ExecModeSequential
	if cfg.ExecutionMode == "parallel" {
		execMode = onnx.ExecModeParallel
	}

	sessions := make([]onnx.NamedSession, 0, numSessions)
	for i := 0; i < numSessions; i++ {
		spinning := cfg.AllowsSpinning()
		if e.cpuPolicy && cfg.AllowSpinning == nil {
			spinning = false
		}
		sess, err := newNamedSession(
			modelData, cfg.ONNX, inputNames, outputNames, intraThreads, cfg.InterOpThreads, execMode, spinning,
		)
		if err != nil {
			for _, opened := range sessions {
				_ = opened.Close()
			}
			return nil, fmt.Errorf("creating image session %d for %q: %w", i, e.Name, err)
		}
		if e.cpuGate != nil {
			sess = &budgetedNamedSession{NamedSession: sess, gate: e.cpuGate, width: intraThreads}
		}
		sessions = append(sessions, sess)
	}

	e.imageSessions.Store(int64(len(sessions)))
	idle := make(chan onnx.NamedSession, len(sessions))
	for _, sess := range sessions {
		idle <- sess
	}
	return &ImageResources{
		Sessions:  sessions,
		idle:      idle,
		Plan:      plan,
		Outputs:   outputs,
		Normalize: cfg.Normalize,
		BatchCap:  staticBatchCap(inputInfos, plan.Input),
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

// staticBatchCap returns the batch dimension a graph declares statically for
// input, or 0 when the axis is symbolic. It is what makes a batch-1 export (for
// example a static DINOv3 ONNX) servable: every run carries exactly the batch
// the graph accepts.
func staticBatchCap(infos []onnx.InputInfo, input string) int {
	for _, in := range infos {
		if in.Name != input || len(in.Dimensions) == 0 {
			continue
		}
		if batch := in.Dimensions[0]; batch > 0 {
			return int(batch)
		}
		return 0
	}
	return 0
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

// resolveImageResources produces the immutable preprocessing plan and the named
// outputs for a model. Preprocessing fields follow a fixed authority order —
// explicit config, then the export's preprocessor_config.json, then the named
// preset, then the ONNX graph, then the built-in default — and an unresolvable
// size fails the load with a descriptive error rather than guessing.
func resolveImageResources(cfg config.ModelConfig, name string) (imageproc.Plan, []ImageOutput, imagePlanSources, error) {
	plan, src, err := resolveImagePlan(cfg, name)
	if err != nil {
		return imageproc.Plan{}, nil, src, err
	}
	preset, presetSet, err := imagePresetFor(cfg, name)
	if err != nil {
		return imageproc.Plan{}, nil, src, err
	}
	outInfo, err := onnx.GetOutputInfo(cfg.ONNX)
	if err != nil {
		return imageproc.Plan{}, nil, src, fmt.Errorf("model %q: reading output info: %w", name, err)
	}
	outputs, err := resolveImageOutputs(cfg, name, plan.Size, preset, presetSet, outInfo, loadModelImageMetadata(cfg.ONNX))
	if err != nil {
		return imageproc.Plan{}, nil, src, err
	}
	return plan, outputs, src, nil
}

// imagePresetFor resolves the named family profile, if the config declares one.
func imagePresetFor(cfg config.ModelConfig, name string) (imagePreset, bool, error) {
	if cfg.Image == nil || cfg.Image.Preset == "" {
		return imagePreset{}, false, nil
	}
	p, ok := imagePresets[cfg.Image.Preset]
	if !ok {
		return imagePreset{}, false, fmt.Errorf("model %q: unknown image.preset %q (want one of %s)", name, cfg.Image.Preset, imagePresetNames())
	}
	return p, true, nil
}

// resolveImagePlan resolves only the preprocessing half, so callers that need
// no graph metadata (and tests over preprocessor_config.json alone) do not pay
// for ONNX introspection.
func resolveImagePlan(cfg config.ModelConfig, name string) (imageproc.Plan, imagePlanSources, error) {
	var src imagePlanSources
	ic := cfg.Image
	if ic == nil {
		return imageproc.Plan{}, src, nil
	}

	preset, presetSet, err := imagePresetFor(cfg, name)
	if err != nil {
		return imageproc.Plan{}, src, err
	}

	crop, err := imageproc.ParseCrop(ic.Crop)
	if err != nil {
		return imageproc.Plan{}, src, fmt.Errorf("model %q: %w", name, err)
	}
	resample, err := imageproc.ParseResample(ic.Resample)
	if err != nil {
		return imageproc.Plan{}, src, fmt.Errorf("model %q: %w", name, err)
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
	src.Crop, src.Resample, src.Rescale, src.Mean, src.Std = "default", "default", "default", "default", "default"
	if presetSet {
		label := "preset:" + ic.Preset
		if !cropSet {
			plan.Crop = preset.Crop
			src.Crop = label
		}
		if !resampleSet {
			plan.Resample = preset.Resample
			src.Resample = label
		}
		if ic.Rescale == nil {
			plan.Rescale = preset.Rescale
			src.Rescale = label
		}
		if len(ic.Mean) == 0 {
			plan.Mean = preset.Mean
			src.Mean = label
		}
		if len(ic.Std) == 0 {
			plan.Std = preset.Std
			src.Std = label
		}
	}
	if cropSet {
		src.Crop = "explicit"
	}
	if resampleSet {
		src.Resample = "explicit"
	}

	pp, err := loadPreprocessorConfig(cfg.ONNX)
	if err != nil {
		return imageproc.Plan{}, src, fmt.Errorf("model %q: %w", name, err)
	}
	if pp != nil {
		const label = "preprocessor_config.json"
		if !cropSet && pp.DoCenterCrop != nil {
			if *pp.DoCenterCrop {
				plan.Crop = imageproc.CropCenter
			} else {
				plan.Crop = imageproc.CropNone
			}
			src.Crop = label
		}
		if !resampleSet {
			if rs, ok := preprocessorResample(pp.Resample); ok {
				plan.Resample = rs
				src.Resample = label
			}
		}
		if ic.Rescale == nil && pp.RescaleFactor != nil {
			plan.Rescale = *pp.RescaleFactor
			src.Rescale = label
		}
		if len(ic.Mean) == 0 && len(pp.ImageMean) == 3 {
			copy(plan.Mean[:], pp.ImageMean)
			src.Mean = label
		}
		if len(ic.Std) == 0 && len(pp.ImageStd) == 3 {
			copy(plan.Std[:], pp.ImageStd)
			src.Std = label
		}
		if plan.Size == 0 {
			if sz, ok := preprocessorSize(pp.CropSize); ok {
				plan.Size = sz
				src.Size = label
			} else if sz, ok := preprocessorSize(pp.Size); ok {
				plan.Size = sz
				src.Size = label
			}
		}
	}
	// Explicit mean/std/rescale override everything detected.
	if len(ic.Mean) == 3 {
		copy(plan.Mean[:], ic.Mean)
		src.Mean = "explicit"
	}
	if len(ic.Std) == 3 {
		copy(plan.Std[:], ic.Std)
		src.Std = "explicit"
	}
	if ic.Rescale != nil {
		plan.Rescale = *ic.Rescale
		src.Rescale = "explicit"
	}

	// Graph detection fills an unset image input and size.
	if plan.Input == "" {
		detName, detSize, found := detectImageInput(cfg.ONNX)
		if found {
			plan.Input = detName
			src.Input = "graph"
			if plan.Size == 0 && detSize > 0 {
				plan.Size = detSize
				src.Size = "graph"
			}
		}
	} else {
		src.Input = "explicit"
	}
	if plan.Input == "" {
		plan.Input = "pixel_values"
		src.Input = "default"
	}
	if ic.Size > 0 {
		src.Size = "explicit"
	}
	if plan.Size <= 0 {
		return imageproc.Plan{}, src, fmt.Errorf("model %q: image.size could not be determined from config, preprocessor_config.json, or the ONNX graph; set image.size explicitly", name)
	}
	if err := validateImagePlan(name, plan); err != nil {
		return imageproc.Plan{}, src, err
	}
	return plan, src, nil
}

// resolveImageOutputs resolves and validates each configured output against the
// graph and the model's own layout metadata. Layout comes from the output
// config first, then the model's config.json (patch_size, num_register_tokens),
// then the preset.
func resolveImageOutputs(cfg config.ModelConfig, name string, size int, preset imagePreset, presetSet bool, outInfo map[string]onnx.OutputInfo, md modelImageMetadata) ([]ImageOutput, error) {
	ic := cfg.Image
	if len(ic.Outputs) == 0 {
		return nil, fmt.Errorf("model %q: image.outputs must declare at least one named output", name)
	}
	available := make([]string, 0, len(outInfo))
	for n := range outInfo {
		available = append(available, n)
	}
	slices.Sort(available)

	outputs := make([]ImageOutput, 0, len(ic.Outputs))
	seen := make(map[string]struct{}, len(ic.Outputs))
	for _, oc := range ic.Outputs {
		if _, dup := seen[oc.Name]; dup {
			return nil, fmt.Errorf("model %q: duplicate image output name %q", name, oc.Name)
		}
		seen[oc.Name] = struct{}{}

		oi, ok := outInfo[oc.Tensor]
		if !ok {
			return nil, fmt.Errorf("model %q: image output %q: tensor %q not found in graph (available: %s)",
				name, oc.Name, oc.Tensor, strings.Join(available, ", "))
		}

		// select defaults to the model's pooling when that pooling can mean the
		// same thing here: on a text+image model it describes the text tower, so
		// it is inherited only for a rank-3 tensor and only when it names a
		// reduction (`cls`, `mean`). A pre-pooled rank-2 row is never `none`
		// because its text twin pooled "mean". With neither set, the tensor's
		// rank decides (a pre-pooled row, or the CLS token of a sequence). tokens
		// is never inferred: it needs an explicit intent and a grid.
		select_ := oc.Select
		if select_ == "" && oi.Rank == 3 {
			if pooled := strings.ToLower(cfg.Pooling); pooled == "cls" || pooled == "mean" {
				select_ = pooled
			}
		}
		if select_ == "" {
			if oi.Rank == 2 {
				select_ = "none"
			} else {
				select_ = "cls"
			}
		}
		switch select_ {
		case "none", "cls", "mean", "tokens", "map":
		default:
			return nil, fmt.Errorf("model %q: image output %q has unknown select %q (want none|cls|mean|tokens|map)", name, oc.Name, select_)
		}

		rank := int(oi.Rank)
		if select_ == "map" {
			// A map is the tensor's own 2-D shape: [1,H,W], or [1,1,H,W] for a
			// convolutional head that keeps a channel axis.
			if rank != 3 && rank != 4 {
				return nil, fmt.Errorf("model %q: image output %q selects map from a rank-%d tensor (want [1,H,W] or [1,1,H,W])", name, oc.Name, rank)
			}
			if oc.Start != nil || oc.Grid != 0 {
				return nil, fmt.Errorf("model %q: image output %q selects map, which takes no start or grid", name, oc.Name)
			}
		} else {
			if select_ == "none" && rank != 2 {
				return nil, fmt.Errorf("model %q: image output %q selects none from a rank-%d tensor (want a pre-pooled rank-2 tensor or select cls|mean|tokens)",
					name, oc.Name, rank)
			}
			if select_ != "none" && rank != 3 {
				return nil, fmt.Errorf("model %q: image output %q selects %s from a rank-%d tensor (want a rank-3 sequence)",
					name, oc.Name, select_, rank)
			}
		}

		out := ImageOutput{
			Name:   oc.Name,
			Tensor: oc.Tensor,
			Select: select_,
			Rows:   1,
			Dim:    int(oi.Dim),
		}
		if select_ == "map" {
			// A fully-convolutional map head emits at the input edge, so the
			// square shape is the configured size; a graph whose width is
			// static must agree with it. The request re-checks the real shape
			// before a byte is read.
			out.Rows = size
			out.Dim = size
			if oi.Dim > 0 && int(oi.Dim) != size {
				return nil, fmt.Errorf("model %q: image output %q maps a width of %d, but image.size is %d", name, oc.Name, oi.Dim, size)
			}
		} else if err := checkImagePairing(name, oc.Name, cfg.Dim, out.Dim); err != nil {
			return nil, err
		}

		if select_ == "tokens" {
			if oc.Start != nil {
				out.Start = *oc.Start
			} else if md.NumRegisterTokens > 0 {
				// 1 CLS token + N register tokens before the patch grid.
				out.Start = 1 + md.NumRegisterTokens
			}
			out.Grid = oc.Grid
			if out.Grid == 0 && md.PatchSize > 0 && (!presetSet || preset.LayoutFromModel) {
				// The model declares the patch size in pixels; the grid edge is
				// how many patches fit across the configured input.
				if size%md.PatchSize != 0 {
					return nil, fmt.Errorf("model %q: image output %q: image.size %d is not a multiple of the model's patch_size %d", name, oc.Name, size, md.PatchSize)
				}
				out.Grid = size / md.PatchSize
			}
			if out.Grid <= 0 {
				return nil, fmt.Errorf("model %q: image output %q selects tokens but has no grid; set image.outputs[].grid (the model's config.json declares no patch_size)", name, oc.Name)
			}
			if size%out.Grid != 0 {
				return nil, fmt.Errorf("model %q: image output %q grid %d does not divide image.size %d", name, oc.Name, out.Grid, size)
			}
			out.Rows = out.Grid * out.Grid
		}
		outputs = append(outputs, out)
	}
	return outputs, nil
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

// logImagePlan reports the resolved plan and outputs at load, naming the source
// of every preprocessing field so a wrong family default is visible rather than
// silent.
func logImagePlan(name string, plan imageproc.Plan, outputs []ImageOutput, src imagePlanSources) {
	log.Printf("  %s: image input=%q (%s) size=%d (%s) crop=%s (%s) resample=%s (%s) rescale=%g (%s) mean=%v (%s) std=%v (%s)",
		name, plan.Input, src.Input, plan.Size, src.Size, plan.Crop, src.Crop,
		plan.Resample, src.Resample, plan.Rescale, src.Rescale, plan.Mean, src.Mean, plan.Std, src.Std)
	for _, o := range outputs {
		if o.Select == "tokens" {
			log.Printf("  %s: image output %q tensor=%q select=%s start=%d grid=%dx%d rows=%d dim=%d",
				name, o.Name, o.Tensor, o.Select, o.Start, o.Grid, o.Grid, o.Rows, o.Dim)
			continue
		}
		log.Printf("  %s: image output %q tensor=%q select=%s rows=%d dim=%d",
			name, o.Name, o.Tensor, o.Select, o.Rows, o.Dim)
	}
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
