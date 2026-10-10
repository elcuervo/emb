package server

import (
	"encoding/json"
	"fmt"
	"math"
	"os"
	"strconv"
	"testing"
	"time"

	"gopkg.in/yaml.v3"

	"github.com/elcuervo/emb/internal/config"
	"github.com/elcuervo/emb/internal/registry"
)

// This file verifies the int8 decision export against the checkpoint the preview
// site mounts: same envelope, same payloads. It needs the downloaded artifacts,
// so it skips unless EMB_LAYA_INT8_PARITY=1 (see `just verify-laya-int8`).

// sandboxLayaReal is the site's `laya-real` entry: the calibrated envelope its
// preset reads and the warm payloads the plate ships. Reading them from the
// sandbox config is what makes this verification answer with exactly the
// temperatures and questions the live plate uses.
type sandboxLayaReal struct {
	envelope     string
	questionSets []string
	states       []string
}

func sandboxLayaRealEntry(t *testing.T) sandboxLayaReal {
	t.Helper()
	raw, err := os.ReadFile("../../website/repl/sandbox.yaml")
	if err != nil {
		t.Fatal(err)
	}
	var doc struct {
		Models map[string]struct {
			// A scripts list mixes bare preset paths with mappings carrying
			// config/warm, so the mappings are picked out by shape below.
			Scripts []any `yaml:"scripts"`
		} `yaml:"models"`
	}
	if err := yaml.Unmarshal(raw, &doc); err != nil {
		t.Fatal(err)
	}
	entry, ok := doc.Models["laya-real"]
	if !ok || len(entry.Scripts) == 0 {
		t.Fatal("website/repl/sandbox.yaml has no laya-real scripts entry")
	}
	var script map[string]any
	for _, s := range entry.Scripts {
		if m, isMapping := s.(map[string]any); isMapping {
			script = m
			break
		}
	}
	if script == nil {
		t.Fatal("laya-real has no script mapping with a config envelope")
	}
	config, _ := script["config"].(map[string]any)
	if len(config) == 0 {
		t.Fatal("laya-real has no scripts config envelope")
	}
	env, err := json.Marshal(config)
	if err != nil {
		t.Fatal(err)
	}
	var out sandboxLayaReal
	out.envelope = string(env)
	warmList, _ := script["warm"].([]any)
	for _, w := range warmList {
		warm, isMapping := w.(map[string]any)
		if !isMapping {
			continue
		}
		args, _ := warm["args"].([]any)
		if len(args) == 0 {
			continue
		}
		questions, _ := args[0].(string)
		if questions == "" {
			continue
		}
		out.questionSets = append(out.questionSets, questions)
		texts, _ := warm["texts"].([]any)
		for _, text := range texts {
			if s, isString := text.(string); isString {
				out.states = append(out.states, s)
			}
		}
	}
	if len(out.questionSets) == 0 || len(out.states) == 0 {
		t.Fatal("laya-real warm payloads are missing from the sandbox config")
	}
	return out
}

// layaInt8Tolerance is the parity tolerance: the winning option must match and
// every reported probability must sit within this of the reference. Overridable
// so a run can prove the assertion is real by tightening it.
func layaInt8Tolerance(t *testing.T) float64 {
	t.Helper()
	const def = 0.05
	raw := os.Getenv("EMB_LAYA_INT8_TOL")
	if raw == "" {
		return def
	}
	v, err := strconv.ParseFloat(raw, 64)
	if err != nil {
		t.Fatalf("EMB_LAYA_INT8_TOL=%q: %v", raw, err)
	}
	return v
}

// compareLayaAnswers compares one int8 reply against its reference and returns
// the largest probability delta it saw, with the answer and option it came from.
func compareLayaAnswers(t *testing.T, label, ref, got string, tol float64) (worst float64, worstAt string, compared int) {
	t.Helper()
	var a, b map[string]any
	if err := json.Unmarshal([]byte(ref), &a); err != nil {
		t.Fatalf("[%s] reference reply is not JSON: %v", label, err)
	}
	if err := json.Unmarshal([]byte(got), &b); err != nil {
		t.Fatalf("[%s] int8 reply is not JSON: %v", label, err)
	}
	if au, bu := usageTokens(t, label, a), usageTokens(t, label, b); au != bu {
		t.Errorf("[%s] input_tokens differs: reference %d, int8 %d (same preset and tokenizer must tokenize alike)", label, au, bu)
	}
	qa, _ := a["answers"].(map[string]any)
	qb, _ := b["answers"].(map[string]any)
	if len(qa) != len(qb) || len(qa) == 0 {
		t.Fatalf("[%s] answer count differs: reference %d, int8 %d", label, len(qa), len(qb))
	}
	for id, av := range qa {
		am, ok := av.(map[string]any)
		if !ok {
			t.Fatalf("[%s] %s: reference answer is not an object", label, id)
		}
		bm, ok := qb[id].(map[string]any)
		if !ok {
			t.Errorf("[%s] %s: missing from the int8 reply", label, id)
			continue
		}
		if am["type"] != bm["type"] {
			t.Errorf("[%s] %s: type differs: %v vs %v", label, id, am["type"], bm["type"])
			continue
		}
		if want, ok := am["choice"].(string); ok {
			got, _ := bm["choice"].(string)
			if want != got {
				t.Errorf("[%s] %s: winning option differs: reference %q, int8 %q", label, id, want, got)
			}
		}
		// A `noul` answer carries no probability map — its decision is the single
		// `noul` value compared with the derived values below.
		probabilities, _ := am["probabilities"].(map[string]any)
		gotProbabilities, _ := bm["probabilities"].(map[string]any)
		for option, wantAny := range probabilities {
			want, ok := wantAny.(float64)
			if !ok {
				t.Fatalf("[%s] %s/%s: reference probability is not a number", label, id, option)
			}
			got, ok := gotProbabilities[option].(float64)
			if !ok {
				t.Errorf("[%s] %s/%s: missing probability in the int8 reply", label, id, option)
				continue
			}
			compared++
			if d := math.Abs(want - got); d > worst {
				worst, worstAt = d, fmt.Sprintf("%s %s.%s", label, id, option)
			}
			if d := math.Abs(want - got); d > tol {
				t.Errorf("[%s] %s/%s: probability delta %.4f exceeds tolerance %.4f (reference %.4f, int8 %.4f)",
					label, id, option, d, tol, want, got)
			}
		}
		// The derived values the answer contract reports: a shift here is a
		// changed decision even when every option probability stays in range.
		for _, key := range []string{"noul", "score", "confidence"} {
			wantAny, ok := am[key]
			if !ok {
				continue
			}
			want, ok := wantAny.(float64)
			if !ok {
				continue
			}
			gotV, ok := bm[key].(float64)
			if !ok {
				t.Errorf("[%s] %s: %s missing from the int8 reply", label, id, key)
				continue
			}
			if d := math.Abs(want - gotV); d > tol {
				t.Errorf("[%s] %s: %s differs by %.4f (reference %.4f, int8 %.4f)", label, id, key, d, want, gotV)
			}
		}
	}
	return worst, worstAt, compared
}

func usageTokens(t *testing.T, label string, reply map[string]any) int {
	t.Helper()
	usage, ok := reply["usage"].(map[string]any)
	if !ok {
		t.Fatalf("[%s] reply has no usage block", label)
	}
	tokens, ok := usage["input_tokens"].(float64)
	if !ok {
		t.Fatalf("[%s] reply has no input_tokens", label)
	}
	return int(tokens)
}

// TestLayaInt8Parity answers the site's own payloads with the published int8
// export and with the checkpoint the site mounts, and asserts the decisions
// survive the quantization: same winning option, probabilities within tolerance.
func TestLayaInt8Parity(t *testing.T) {
	if os.Getenv("EMB_LAYA_INT8_PARITY") != "1" {
		t.Skip("set EMB_LAYA_INT8_PARITY=1 to run (just verify-laya-int8)")
	}
	if !ortOK {
		t.Skip("onnx runtime unavailable (run inside nix develop)")
	}
	for _, path := range []string{
		"../../models/laya-typed/model.onnx",
		"../../models/laya-int8/model_int8.onnx",
	} {
		if _, err := os.Stat(path); err != nil {
			t.Skipf("%s is missing (run: just download-laya-int8; the reference is the site's checkpoint): %v", path, err)
		}
	}

	site := sandboxLayaRealEntry(t)
	tol := layaInt8Tolerance(t)

	reg := registry.New()
	// The int8 entry names model.onnx, which does not exist: `quantize: auto`
	// resolves model_int8.onnx beside it, the mount path this change adds.
	for _, m := range []struct{ name, onnx, tokenizer string }{
		{"laya-ref", "../../models/laya-typed/model.onnx", "../../models/laya-typed/tokenizer.json"},
		{"laya-int8", "../../models/laya-int8/model.onnx", "../../models/laya-int8/tokenizer.json"},
	} {
		entry, err := registry.LoadModel(config.ModelConfig{
			ONNX:      m.onnx,
			Tokenizer: m.tokenizer,
			MaxLength: 1024,
			Workers:   1,
		}, m.name)
		if err != nil {
			t.Fatalf("loading %s: %v", m.name, err)
		}
		reg.Add(m.name, entry)
	}

	addr := getFreeAddr()
	srv := New(addr, reg, "", "", nil)
	go srv.ListenAndServe()
	t.Cleanup(func() { _ = srv.Close() })
	time.Sleep(50 * time.Millisecond)

	script := layaScriptSrc(t)
	worst, worstAt, compared := 0.0, "", 0
	for _, questions := range site.questionSets {
		for _, state := range site.states {
			label := stateLabel(state)
			ref := redisCmdBig(t, addr, "EMB.EVAL", "laya-ref", script, "1", state, questions, site.envelope)
			got := redisCmdBig(t, addr, "EMB.EVAL", "laya-int8", script, "1", state, questions, site.envelope)
			w, at, n := compareLayaAnswers(t, label, ref, got, tol)
			if w > worst {
				worst, worstAt = w, at
			}
			compared += n
		}
	}
	if compared == 0 {
		t.Fatal("no probabilities were compared: the payloads carried no answers")
	}

	// The reduced graph must also answer through the digest path the sandbox
	// uses, with the envelope carried by the preloaded preset rather than
	// ARGV[2]. The graph omits `last_hidden_state`; the preset asks only for
	// `logits` and `act_logits`.
	var envelope map[string]any
	if err := json.Unmarshal([]byte(site.envelope), &envelope); err != nil {
		t.Fatal(err)
	}
	sha, err := srv.PreloadScriptConfig("laya-int8", script, envelope)
	if err != nil {
		t.Fatalf("preloading the int8 preset: %v", err)
	}
	state, questions := site.states[0], site.questionSets[0]
	reference := redisCmdBig(t, addr, "EMB.EVAL", "laya-ref", script, "1", state, questions, site.envelope)
	viaDigest := redisCmdBig(t, addr, "EMB.EVSHA", "laya-int8", sha, "1", state, questions)
	w, at, n := compareLayaAnswers(t, "EMB.EVSHA "+stateLabel(state), reference, viaDigest, tol)
	if w > worst {
		worst, worstAt = w, at
	}
	compared += n

	t.Logf("compared %d probabilities over %d payloads: worst delta %.4f (%s), tolerance %.4f",
		compared, len(site.questionSets)*len(site.states)+1, worst, worstAt, tol)
}

// stateLabel names a state by its subject, so a failure says which payload moved.
func stateLabel(state string) string {
	var s struct {
		Subject string `json:"subject"`
	}
	if err := json.Unmarshal([]byte(state), &s); err != nil || s.Subject == "" {
		return "state"
	}
	return s.Subject
}
