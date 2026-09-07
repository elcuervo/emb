package script

import (
	"os"
	"strconv"
	"strings"
	"testing"

	"github.com/elcuervo/emb/internal/onnx"
	"github.com/elcuervo/emb/internal/tokenizer"
)

// benchGLiNER loads the int8 testbed model and the example script once per
// benchmark run. Intra-op threads default to a realistic setting (the
// correctness tests pin 1 thread; production uses cores-2), overridable via
// EMB_BENCH_INTRA.
func benchGLiNER(b *testing.B) (Hosts, string) {
	b.Helper()
	if _, err := os.Stat(glinerModel); err != nil {
		b.Skipf("gliner testbed model not present: %v (run: just download-gliner-model)", err)
	}
	if err := onnx.InitEnvironment(""); err != nil {
		b.Fatal(err)
	}
	b.Cleanup(func() { _ = onnx.DestroyEnvironment() })

	data, err := os.ReadFile(glinerModel)
	if err != nil {
		b.Fatal(err)
	}
	inputNames, err := onnx.GetInputNames(glinerModel)
	if err != nil {
		b.Fatal(err)
	}
	outInfo, err := onnx.GetOutputInfo(glinerModel)
	if err != nil {
		b.Fatal(err)
	}
	outputNames := make([]string, 0, len(outInfo))
	for n := range outInfo {
		outputNames = append(outputNames, n)
	}

	intra := 4
	if v := os.Getenv("EMB_BENCH_INTRA"); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil {
			b.Fatalf("EMB_BENCH_INTRA: %v", err)
		}
		intra = n
	}
	sess, err := onnx.NewNamedRuntimeSessionFromBytes(data, inputNames, outputNames, intra, 2, onnx.ExecModeSequential)
	if err != nil {
		b.Fatalf("opening gliner session: %v", err)
	}
	b.Cleanup(func() { _ = sess.Close() })

	rt, err := tokenizer.NewTokenizer(glinerTokenizer, false)
	if err != nil {
		b.Fatal(err)
	}
	b.Cleanup(func() { _ = rt.Close() })
	tok, ok := any(rt).(tokenizer.PretokenizedTokenizer)
	if !ok {
		b.Fatal("no pretokenized tokenizer")
	}

	src, err := os.ReadFile(glinerScript)
	if err != nil {
		b.Fatal(err)
	}

	hosts := Hosts{
		Run:                sess.RunNamed,
		EncodePretokenized: tok.EncodePretokenized,
	}
	return hosts, string(src)
}

// benchTexts builds an entity-rich text of approximately `words` words by
// repeating a template sentence.
func benchTexts(words int) string {
	template := "Apple CEO Tim Cook announced the iPhone 15 in Cupertino at a keynote event for developers and investors."
	parts := []string{}
	count := 0
	for count < words {
		parts = append(parts, template)
		count += len(strings.Fields(template))
	}
	return strings.Join(parts, " ")
}

// noDecodeSource cuts the span-scan decode from the example script so the
// delta between full and build-only isolates decode cost.
func noDecodeSource(full string) string {
	idx := strings.Index(full, "-- Span scan")
	if idx < 0 {
		return full
	}
	return full[:idx] + "return { seq = seq, nlab = nlab, nlogits = #out.logits.data }\n"
}

func BenchmarkGLiNERExtract(b *testing.B) {
	hosts, src := benchGLiNER(b)

	labelSets := [][]string{
		{"PERSON"},
		{"PERSON", "ORG", "PRODUCT"},
		{"PERSON", "ORG", "PRODUCT", "LOCATION", "EVENT", "FEATURE", "CURRENCY", "DATE"},
	}
	texts := []struct {
		name string
		text string
	}{
		{"w10", benchTexts(10)},
		{"w60", benchTexts(60)},
		{"w180", benchTexts(180)},
	}

	for _, labels := range labelSets {
		for _, txt := range texts {
			name := "labels" + strconv.Itoa(len(labels)) + "-" + txt.name
			b.Run(name, func(b *testing.B) {
				b.SetBytes(int64(len(txt.text)))
				b.ResetTimer()
				for i := 0; i < b.N; i++ {
					if _, err := EvalWithHosts(src, []string{txt.text}, labels, hosts, EvalOptions{}); err != nil {
						b.Fatal(err)
					}
				}
			})
		}
	}

	// Decode-cost isolation: full vs input-construction-only on a medium text.
	midText := "Apple CEO Tim Cook announced the iPhone 15 in Cupertino, and later Google shipped the Pixel 9 from Mountain View with Sundar Pichai on stage."
	midLabels := []string{"PERSON", "ORG", "PRODUCT"}
	b.Run("decode-share-labels3-medium", func(b *testing.B) {
		b.Run("full", func(b *testing.B) {
			for i := 0; i < b.N; i++ {
				if _, err := EvalWithHosts(src, []string{midText}, midLabels, hosts, EvalOptions{}); err != nil {
					b.Fatal(err)
				}
			}
		})
		b.Run("build-only", func(b *testing.B) {
			trimmed := noDecodeSource(src)
			for i := 0; i < b.N; i++ {
				if _, err := EvalWithHosts(trimmed, []string{midText}, midLabels, hosts, EvalOptions{}); err != nil {
					b.Fatal(err)
				}
			}
		})
	})

	// Multi-text cell: 8 texts of ~20 words each, serial (8 single-text evals)
	// vs batched (ONE eval with KEYS = all 8, one emb.run_batch call).
	texts8 := make([]string, 8)
	for i := range texts8 {
		texts8[i] = benchTexts(20)
	}
	texts16 := append(append([]string{}, texts8...), texts8...)
	b.Run("multi-text-labels3", func(b *testing.B) {
		b.Run("serial8", func(b *testing.B) {
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				for _, txt := range texts8 {
					if _, err := EvalWithHosts(src, []string{txt}, midLabels, hosts, EvalOptions{}); err != nil {
						b.Fatal(err)
					}
				}
			}
		})
		b.Run("batched8", func(b *testing.B) {
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				if _, err := EvalWithHosts(src, texts8, midLabels, hosts, EvalOptions{}); err != nil {
					b.Fatal(err)
				}
			}
		})
		b.Run("serial16", func(b *testing.B) {
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				for _, txt := range texts16 {
					if _, err := EvalWithHosts(src, []string{txt}, midLabels, hosts, EvalOptions{}); err != nil {
						b.Fatal(err)
					}
				}
			}
		})
		b.Run("batched16", func(b *testing.B) {
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				if _, err := EvalWithHosts(src, texts16, midLabels, hosts, EvalOptions{}); err != nil {
					b.Fatal(err)
				}
			}
		})
	})

}
