package script

import (
	"encoding/json"
	"flag"
	"os"
	"path/filepath"
	"sort"
	"testing"

	lua "github.com/yuin/gopher-lua"

	"github.com/elcuervo/emb/internal/onnx"
	"github.com/elcuervo/emb/internal/tokenizer"
)

// updateGLiNER regenerates testdata/gliner_golden.json from the real model.
// go test ./internal/script/ -run TestGLiNER -update
var updateGLiNER = flag.Bool("update", false, "regenerate gliner golden fixtures")

const (
	glinerModel     = "../../models/gliner2/model_int8.onnx"
	glinerTokenizer = "../../models/gliner2/tokenizer.json"
	glinerScript    = "../../examples/scripts/gliner2.lua"
	glinerGolden    = "testdata/gliner_golden.json"
)

type glinerCase struct {
	Text   string              `json:"text"`
	Labels []string            `json:"labels"`
	Golden map[string][]string `json:"entities"`
}

func loadGLiNER(t *testing.T) (session onnx.NamedSession, tok tokenizer.PretokenizedTokenizer, src string) {
	t.Helper()
	for _, f := range []string{glinerModel, glinerTokenizer} {
		if _, err := os.Stat(f); err != nil {
			t.Skipf("gliner testbed model not present: %v (run: just download-gliner-model)", err)
		}
	}
	if err := onnx.InitEnvironment(""); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = onnx.DestroyEnvironment() })

	data, err := os.ReadFile(glinerModel)
	if err != nil {
		t.Fatal(err)
	}
	inputNames, err := onnx.GetInputNames(glinerModel)
	if err != nil {
		t.Fatal(err)
	}
	outInfo, err := onnx.GetOutputInfo(glinerModel)
	if err != nil {
		t.Fatal(err)
	}
	outputNames := make([]string, 0, len(outInfo))
	for n := range outInfo {
		outputNames = append(outputNames, n)
	}
	sort.Strings(outputNames)

	sess, err := onnx.NewNamedRuntimeSessionFromBytes(data, inputNames, outputNames, 1, 2, onnx.ExecModeSequential)
	if err != nil {
		t.Fatalf("opening gliner session: %v", err)
	}
	t.Cleanup(func() { _ = sess.Close() })

	rt, err := tokenizer.NewTokenizer(glinerTokenizer, false)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = rt.Close() })
	tok, ok := any(rt).(tokenizer.PretokenizedTokenizer)
	if !ok {
		t.Fatalf("gliner tokenizer lacks pretokenized encoding")
	}

	raw, err := os.ReadFile(glinerScript)
	if err != nil {
		t.Fatal(err)
	}
	return sess, tok, string(raw)
}

func glinerHosts(sess onnx.NamedSession, tok tokenizer.PretokenizedTokenizer) Hosts {
	return Hosts{
		Run: func(inputs []onnx.NamedTensor) (map[string]onnx.NamedTensor, error) {
			return sess.RunNamed(inputs)
		},
		EncodePretokenized: tok.EncodePretokenized,
	}
}

// extractEntities reads the script's returned hash {LABEL = {texts...}}.
func extractEntities(v lua.LValue) map[string][]string {
	out := map[string][]string{}
	if t, ok := v.(*lua.LTable); ok {
		t.ForEach(func(k, val lua.LValue) {
			if _, ok := k.(lua.LString); !ok {
				return
			}
			var list []string
			if lt, ok := val.(*lua.LTable); ok {
				for i := 1; i <= lt.Len(); i++ {
					if s, ok := lt.RawGetInt(i).(lua.LString); ok {
						list = append(list, string(s))
					}
				}
			}
			out[k.String()] = list
		})
	}
	return out
}

func runGLiNERCase(t *testing.T, sess onnx.NamedSession, tok tokenizer.PretokenizedTokenizer, src string, c glinerCase) map[string][]string {
	t.Helper()
	v, err := EvalWithHosts(src, []string{c.Text}, c.Labels, glinerHosts(sess, tok), EvalOptions{})
	if err != nil {
		t.Fatalf("eval %q: %v", c.Text, err)
	}
	return extractEntities(v)
}

func glinerCases() []glinerCase {
	return []glinerCase{
		{Text: "Apple CEO Tim Cook announced iPhone 15.", Labels: []string{"PERSON", "ORG", "PRODUCT"}},
		{Text: "Stripe hired Mary in Dublin.", Labels: []string{"PERSON", "ORG"}},
		{Text: "Google launched the Pixel 9 in Mountain View.", Labels: []string{"PRODUCT", "LOCATION"}},
		{Text: "Elon founded SpaceX in Hawthorne.", Labels: []string{"FEATURE", "CURRENCY"}},
	}
}

func TestGLiNERGolden(t *testing.T) {
	sess, tok, src := loadGLiNER(t)
	cases := glinerCases()

	// Generate fresh results and compare (or write) the golden file.
	var fresh []glinerCase
	for _, c := range cases {
		entities := runGLiNERCase(t, sess, tok, src, c)
		c.Golden = entities
		fresh = append(fresh, c)
	}

	// Reference assertions from specs/script-eval (independent of golden file).
	apple := fresh[0].Golden
	if len(apple["PERSON"]) != 1 || apple["PERSON"][0] != "Tim Cook" {
		t.Fatalf("PERSON extraction wrong: %v", apple["PERSON"])
	}
	if len(apple["ORG"]) != 1 || apple["ORG"][0] != "Apple" {
		t.Fatalf("ORG extraction wrong: %v", apple["ORG"])
	}
	if len(apple["PRODUCT"]) != 1 || apple["PRODUCT"][0] != "iPhone 15" {
		t.Fatalf("PRODUCT extraction wrong: %v", apple["PRODUCT"])
	}

	if *updateGLiNER {
		data, err := json.MarshalIndent(fresh, "", "  ")
		if err != nil {
			t.Fatal(err)
		}
		if err := os.MkdirAll(filepath.Dir(glinerGolden), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(glinerGolden, append(data, '\n'), 0o644); err != nil {
			t.Fatal(err)
		}
		t.Logf("wrote %s", glinerGolden)
		return
	}

	data, err := os.ReadFile(glinerGolden)
	if err != nil {
		t.Fatalf("golden file missing (run with -update): %v", err)
	}
	var golden []glinerCase
	if err := json.Unmarshal(data, &golden); err != nil {
		t.Fatal(err)
	}
	if len(golden) != len(fresh) {
		t.Fatalf("golden has %d cases, fresh has %d", len(golden), len(fresh))
	}
	for i, g := range golden {
		if g.Text != fresh[i].Text {
			t.Fatalf("case %d text mismatch", i)
		}
		got := fresh[i].Golden
		for label, want := range g.Golden {
			gotList := got[label]
			if len(gotList) != len(want) {
				t.Fatalf("case %q label %s: got %v want %v", g.Text, label, gotList, want)
			}
			for j := range want {
				if gotList[j] != want[j] {
					t.Fatalf("case %q label %s[%d]: got %q want %q", g.Text, label, j, gotList[j], want[j])
				}
			}
		}
	}
}
