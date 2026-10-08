package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"

	"github.com/elcuervo/emb/internal/onnx"
)

// probe loads one graph, prints its real input/output contract, then runs it on
// zeros with the dtype and shape the change assumes. A wrong name, dtype or
// shape fails the run, which is the point: the third-party X-CLIP card is not
// trusted without it.
func probe(path string, inputs []onnx.NamedTensor) error {
	infos, err := onnx.GetInputInfo(path)
	if err != nil {
		return err
	}
	fmt.Printf("\n== %s\n", path)
	for _, in := range infos {
		fmt.Printf("   in  %-16s rank=%d dims=%v\n", in.Name, in.Rank, in.Dimensions)
	}
	outs, err := onnx.GetOutputInfo(path)
	if err != nil {
		return err
	}
	outNames := make([]string, 0, len(outs))
	for name, o := range outs {
		fmt.Printf("   out %-16s rank=%d dim=%d\n", name, o.Rank, o.Dim)
		outNames = append(outNames, name)
	}
	inNames := make([]string, len(infos))
	for i, in := range infos {
		inNames[i] = in.Name
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	sess, err := onnx.NewNamedRuntimeSessionFromBytes(data, inNames, outNames, 1, 1, 0, false)
	if err != nil {
		return fmt.Errorf("session: %w", err)
	}
	defer sess.Close()
	res, err := sess.RunNamed(inputs)
	if err != nil {
		return fmt.Errorf("run: %w", err)
	}
	for name, t := range res {
		nonzero := 0
		for _, f := range t.Float {
			if f != 0 {
				nonzero++
			}
		}
		fmt.Printf("   ran %-16s shape=%v dtype=%v floats=%d nonzero=%d\n", name, t.Shape, t.DType, len(t.Float), nonzero)
	}
	return nil
}

func f32(name string, shape []int64, n int) onnx.NamedTensor {
	return onnx.NamedTensor{Name: name, Shape: shape, DType: onnx.TensorFloat32, Float: make([]float32, n)}
}

func i64(name string, shape []int64, n int) onnx.NamedTensor {
	return onnx.NamedTensor{Name: name, Shape: shape, DType: onnx.TensorInt64, Int64: make([]int64, n)}
}

func main() {
	models := flag.String("models", "models", "directory holding the clap/ and xclip/ model folders")
	flag.Parse()
	if err := onnx.InitEnvironment(""); err != nil {
		fmt.Fprintf(os.Stderr, "PROBE FAILED: onnxruntime: %v\n", err)
		os.Exit(1)
	}
	if flag.NArg() < 1 {
		fmt.Fprintln(os.Stderr, "usage: emb-probe [-models dir] <clap-audio|clap-text|xclip-video|xclip-text>")
		os.Exit(2)
	}
	var err error
	switch flag.Arg(0) {
	case "clap-audio":
		err = probe(filepath.Join(*models, "clap", "audio_model_quantized.onnx"), []onnx.NamedTensor{
			f32("input_features", []int64{1, 1, 1001, 64}, 1001*64),
		})
	case "clap-text":
		err = probe(filepath.Join(*models, "clap", "text_model_quantized.onnx"), []onnx.NamedTensor{
			i64("input_ids", []int64{1, 77}, 77),
		})
	case "xclip-video":
		err = probe(filepath.Join(*models, "xclip", "video_tower.onnx"), []onnx.NamedTensor{
			f32("pixel_values", []int64{1, 8, 3, 224, 224}, 8*3*224*224),
		})
	case "xclip-text":
		err = probe(filepath.Join(*models, "xclip", "text_tower.onnx"), []onnx.NamedTensor{
			i64("input_ids", []int64{1, 77}, 77),
			i64("attention_mask", []int64{1, 77}, 77),
		})
	default:
		fmt.Fprintln(os.Stderr, "unknown graph", flag.Arg(0))
		os.Exit(2)
	}
	if err != nil {
		fmt.Fprintf(os.Stderr, "PROBE FAILED: %v\n", err)
		os.Exit(1)
	}
}
