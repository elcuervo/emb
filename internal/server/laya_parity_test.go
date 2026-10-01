package server

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"os"
	"reflect"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/elcuervo/emb/internal/config"
	"github.com/elcuervo/emb/internal/registry"
)

// serveLaya starts a server with the vendored tiny Laya export (ruby-laya's
// test/fixtures/tiny), mounted like any scripted model. The model and
// tokenizer are committed under testdata, so unlike the GLiNER testbed this
// never needs a download.
func serveLaya(t *testing.T, cacheCfg string) (string, *Server) {
	t.Helper()
	if !ortOK {
		t.Skip("onnx runtime unavailable (run inside nix develop)")
	}
	reg := registry.New()
	entry, err := registry.LoadModel(config.ModelConfig{
		ONNX:      "../../testdata/laya/model.onnx",
		Tokenizer: "../../testdata/laya/tokenizer/tokenizer.json",
		MaxLength: 128,
	}, "laya-tiny")
	if err != nil {
		t.Fatalf("loading laya-tiny: %v", err)
	}
	reg.Add("laya-tiny", entry)

	addr := getFreeAddr()
	srv := New(addr, reg, "", cacheCfg, nil)
	go srv.ListenAndServe()
	t.Cleanup(func() { _ = srv.Close() })
	time.Sleep(50 * time.Millisecond)
	return addr, srv
}

func layaScriptSrc(t *testing.T) string {
	t.Helper()
	raw, err := os.ReadFile("../../scripts/laya.lua")
	if err != nil {
		t.Fatal(err)
	}
	return string(raw)
}

// layaConfigJSON merges the vendored onnx_config.json and rl_agent_config.json
// into the request-time config envelope the preset reads (ARGV[2]).
func layaConfigJSON(t *testing.T) string {
	t.Helper()
	read := func(path string) map[string]json.RawMessage {
		data, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		var m map[string]json.RawMessage
		if err := json.Unmarshal(data, &m); err != nil {
			t.Fatal(err)
		}
		return m
	}
	onnx := read("../../testdata/laya/onnx_config.json")
	rl := read("../../testdata/laya/rl_agent_config.json")
	merged := map[string]any{}
	for _, k := range []string{"max_len", "head_max_len"} {
		if raw, ok := rl[k]; ok {
			merged[k] = json.RawMessage(raw)
		}
	}
	for _, k := range []string{"min_seq", "min_markers"} {
		if raw, ok := onnx[k]; ok {
			merged[k] = json.RawMessage(raw)
		}
	}
	merged["temperature"] = json.RawMessage(rl["temperature"])
	merged["temperature_by_options"] = json.RawMessage(rl["temperature_by_options"])
	b, err := json.Marshal(merged)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

// --- ordered JSON → Python-style text (the gem's PyJSON) -------------------

// orderedValue preserves a JSON document's object key order.
type orderedValue struct {
	kind   byte // 'o' object, 'a' array, 's' string, 'v' scalar
	keys   []string
	vals   []*orderedValue
	arr    []*orderedValue
	str    string
	scalar any
}

func parseOrdered(t *testing.T, raw []byte) *orderedValue {
	t.Helper()
	dec := json.NewDecoder(strings.NewReader(string(raw)))
	v := readOrderedValue(t, dec, "")
	return v
}

func readOrderedValue(t *testing.T, dec *json.Decoder, path string) *orderedValue {
	tok, err := dec.Token()
	if err != nil {
		t.Fatalf("parsing %s: %v", path, err)
	}
	return tokenToOrdered(t, dec, tok, path)
}

func tokenToOrdered(t *testing.T, dec *json.Decoder, tok json.Token, path string) *orderedValue {
	switch tt := tok.(type) {
	case json.Delim:
		switch tt {
		case '{':
			v := &orderedValue{kind: 'o'}
			for dec.More() {
				keyTok, err := dec.Token()
				if err != nil {
					t.Fatalf("parsing %s: %v", path, err)
				}
				key := keyTok.(string)
				child := readOrderedValue(t, dec, path+"."+key)
				v.keys = append(v.keys, key)
				v.vals = append(v.vals, child)
			}
			if _, err := dec.Token(); err != nil {
				t.Fatalf("parsing %s: %v", path, err)
			}
			return v
		case '[':
			v := &orderedValue{kind: 'a'}
			for dec.More() {
				v.arr = append(v.arr, readOrderedValue(t, dec, path+"[]"))
			}
			if _, err := dec.Token(); err != nil {
				t.Fatalf("parsing %s: %v", path, err)
			}
			return v
		}
		t.Fatalf("unexpected delim %v at %s", tt, path)
	case string:
		return &orderedValue{kind: 's', str: tt}
	default:
		return &orderedValue{kind: 'v', scalar: tt}
	}
	return nil
}

// pyJSON renders the ordered tree the way Python's json.dumps(obj,
// ensure_ascii=False) does (default separators: ", " and ": "), which is the
// exact state/criterion text the checkpoints were trained on.
func pyJSON(v *orderedValue) string {
	switch v.kind {
	case 'o':
		parts := make([]string, 0, len(v.keys))
		for i, k := range v.keys {
			parts = append(parts, fmt.Sprintf("%s: %s", pyJSON(&orderedValue{kind: 's', str: k}), pyJSON(v.vals[i])))
		}
		return "{" + strings.Join(parts, ", ") + "}"
	case 'a':
		parts := make([]string, 0, len(v.arr))
		for _, e := range v.arr {
			parts = append(parts, pyJSON(e))
		}
		return "[" + strings.Join(parts, ", ") + "]"
	case 's':
		// The corpus text is ASCII; the reference keeps non-ASCII bytes as-is
		// (ensure_ascii=False), so pass through. Escapes for control chars are
		// not exercised by the vendored corpus.
		return `"` + v.str + `"`
	default:
		return fmt.Sprintf("%v", v.scalar)
	}
}

// stateText turns a corpus case's state value into the text the gem feeds the
// tokenizer: JSON strings pass through unquoted; anything else is serialized
// Python-style.
func stateText(t *testing.T, raw json.RawMessage) string {
	t.Helper()
	if len(raw) > 0 && raw[0] == '"' {
		var s string
		if err := json.Unmarshal(raw, &s); err != nil {
			t.Fatal(err)
		}
		return s
	}
	return pyJSON(parseOrdered(t, raw))
}

// --- comparison -------------------------------------------------------------

// nearEqual compares decoded JSON trees tolerantly on numbers (the corpus
// rounds to four places; ulp-level drift in the softmax computation must not
// flake the suite) and exactly on everything else.
func nearEqual(a, b any) bool {
	switch av := a.(type) {
	case float64:
		bv, ok := b.(float64)
		// The corpus rounds to four places, so a value can sit on either side
		// of a rounding boundary between inference builds (nix 1.26/arm64 vs
		// CI 1.27/amd64) and differ by one corpus unit. 1.5e-4 absorbs that
		// single-boundary flip while still flagging drift of two units or more;
		// input_tokens is compared separately and exactly.
		return ok && math.Abs(av-bv) < 1.5e-4
	case string:
		bv, ok := b.(string)
		return ok && av == bv
	case bool:
		bv, ok := b.(bool)
		return ok && av == bv
	case nil:
		return b == nil
	case []any:
		bv, ok := b.([]any)
		if !ok || len(av) != len(bv) {
			return false
		}
		for i := range av {
			if !nearEqual(av[i], bv[i]) {
				return false
			}
		}
		return true
	case map[string]any:
		bv, ok := b.(map[string]any)
		if !ok || len(av) != len(bv) {
			return false
		}
		for k, ev := range av {
			bw, ok := bv[k]
			if !ok || !nearEqual(ev, bw) {
				return false
			}
		}
		return true
	}
	return reflect.DeepEqual(a, b)
}

func TestLayaParityCorpus(t *testing.T) {
	addr, _ := serveLaya(t, "")
	c := dial(t, addr)
	defer c.Close()

	raw, err := os.ReadFile("../../testdata/laya/expected.json")
	if err != nil {
		t.Fatal(err)
	}
	var corpus struct {
		Data struct {
			Cases []struct {
				Label     string          `json:"label"`
				State     json.RawMessage `json:"state"`
				Questions json.RawMessage `json:"questions"`
				Predict   struct {
					Model   string          `json:"model"`
					Answers json.RawMessage `json:"answers"`
					Usage   json.RawMessage `json:"usage"`
				} `json:"predict"`
			} `json:"cases"`
		} `json:"data"`
	}
	if err := json.Unmarshal(raw, &corpus); err != nil {
		t.Fatal(err)
	}

	shaTok := redisCmd(t, addr, "EMB.SCRIPT", "LOAD", "laya-tiny", layaScriptSrc(t))
	if shaTok.kind != "bulk" {
		t.Fatalf("SCRIPT LOAD reply = %+v", shaTok)
	}
	sha := shaTok.val.(string)

	cfg := layaConfigJSON(t)
	for _, tc := range corpus.Data.Cases {
		t.Run(tc.Label, func(t *testing.T) {
			// tc.Questions is a deep copy of the loop variable: the raw bytes
			// are shared and never mutated, so taking the address is safe.
			q := tc.Questions
			state := stateText(t, tc.State)
			replyTok := redisCmd(t, addr, "EMB.EVSHA", "laya-tiny", sha, "1", state, string(q), cfg)
			if replyTok.kind != "bulk" {
				t.Fatalf("EVSHA reply = %+v", replyTok)
			}
			reply := replyTok.val.(string)

			var got struct {
				Answers any `json:"answers"`
				Usage   any `json:"usage"`
			}
			if err := json.Unmarshal([]byte(reply), &got); err != nil {
				t.Fatalf("parsing reply %q: %v", reply, err)
			}

			var want struct {
				Answers any `json:"answers"`
				Usage   any `json:"usage"`
			}
			if err := json.Unmarshal(tc.Predict.Answers, &want.Answers); err != nil {
				t.Fatal(err)
			}
			if err := json.Unmarshal(tc.Predict.Usage, &want.Usage); err != nil {
				t.Fatal(err)
			}
			// input_tokens must match EXACTLY: it is the strongest check that
			// the sequence construction reproduced the reference byte for
			// byte. Compare before the tolerant tree comparison, separately.
			wantUsage, ok := want.Usage.(map[string]any)
			if !ok {
				t.Fatal("corpus usage is not an object")
			}
			gotMap := got.Answers.(map[string]any)
			if !nearEqual(gotMap, want.Answers) {
				t.Fatalf("answers mismatch:\n got %s\nwant %s", mustJSON(got.Answers), mustJSON(want.Answers))
			}
			tokensEqual := false
			if gu, ok := got.Usage.(map[string]any); ok {
				if strings.EqualFold(fmt.Sprintf("%v", gu["input_tokens"]), fmt.Sprintf("%v", wantUsage["input_tokens"])) {
					tokensEqual = true
				}
			}
			if !tokensEqual {
				t.Fatalf("input_tokens mismatch: got %v want %v", got.Usage, want.Usage)
			}
		})
	}
}

func mustJSON(v any) string {
	b, err := json.Marshal(v)
	if err != nil {
		return fmt.Sprintf("%v", v)
	}
	return string(b)
}

// TestLayaValidationErrors verifies malformed questions error before any
// inference runs (the spec's answer-with-error contract).
func TestLayaValidationErrors(t *testing.T) {
	addr, _ := serveLaya(t, "")

	shaTok := redisCmd(t, addr, "EMB.SCRIPT", "LOAD", "laya-tiny", layaScriptSrc(t))
	sha := shaTok.val.(string)

	bad := []string{
		`{"q": {"type": "essay", "instructions": "x"}}`,
		`{"q": {"type": "choice", "instructions": "x", "criteria": {}}}`,
		`{"q": {"type": "noul"}}`,
	}
	for _, questions := range bad {
		tok := redisCmd(t, addr, "EMB.EVSHA", "laya-tiny", sha, "1", "state", questions)
		if tok.kind != "error" {
			t.Fatalf("questions %s: expected an error, got %+v", questions, tok)
		}
	}
}

// redisCmdBig reads a bulk reply of any size. The shared redisCmd helper reads
// one 4 KiB chunk (fine for every other reply in these tests), but an episode's
// frames are tens of kilobytes.
func redisCmdBig(t *testing.T, addr string, args ...string) string {
	t.Helper()
	c := dial(t, addr)
	defer c.Close()
	if _, err := c.Write(respCommand(args...)); err != nil {
		t.Fatal(err)
	}
	r := bufio.NewReader(c)
	header, err := r.ReadString('\n')
	if err != nil {
		t.Fatal(err)
	}
	if header[0] == '-' {
		t.Fatalf("error reply: %s", strings.TrimRight(header, "\r\n"))
	}
	if header[0] != '$' {
		t.Fatalf("unexpected bulk header %q", header)
	}
	n, err := strconv.Atoi(strings.TrimRight(header[1:], "\r\n"))
	if err != nil {
		t.Fatalf("bad bulk length %q", header)
	}
	body := make([]byte, n)
	if _, err := io.ReadFull(r, body); err != nil {
		t.Fatal(err)
	}
	return string(body)
}

// layaPreload loads a task preset the way the sandbox does and returns the
// digest EMB.EVSHA will accept.
func layaPreload(t *testing.T, srv *Server, path string) string {
	t.Helper()
	src, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var cfg map[string]any
	if err := json.Unmarshal([]byte(layaConfigJSON(t)), &cfg); err != nil {
		t.Fatal(err)
	}
	sha, err := srv.PreloadScriptConfig("laya-tiny", string(src), cfg)
	if err != nil {
		t.Fatalf("preloading %s: %v", path, err)
	}
	return sha
}

// layaEpisode sends one EMB.EVSHA and parses the reply. An episode carries every
// frame, so the reply can be large and needs the full-bulk reader.
func layaEpisode(t *testing.T, addr, sha string, request map[string]any) map[string]any {
	t.Helper()
	raw, err := json.Marshal(request)
	if err != nil {
		t.Fatal(err)
	}
	reply := redisCmdBig(t, addr, "EMB.EVSHA", "laya-tiny", sha, "1", string(raw))
	var out map[string]any
	if err := json.Unmarshal([]byte(reply), &out); err != nil {
		t.Fatalf("parsing reply %v: %v", reply, err)
	}
	return out
}

// assertLayaFrames pins the frame contract both loop presets share: one to four
// move probabilities summing to one, an executed move that differs from the
// proposed one exactly when the planner intervened, and the named readout
// probabilities in [0,1].
func assertLayaFrames(t *testing.T, frames []any, reads ...string) {
	t.Helper()
	dirs := map[string]bool{"UP": true, "DOWN": true, "LEFT": true, "RIGHT": true}
	for i, raw := range frames {
		frame, ok := raw.(map[string]any)
		if !ok {
			t.Fatalf("frame %d is not an object", i)
		}
		probs, ok := frame["probs"].(map[string]any)
		if !ok || len(probs) == 0 || len(probs) > 4 {
			t.Fatalf("frame %d: move probabilities = %v", i, frame["probs"])
		}
		sum := 0.0
		for dir, p := range probs {
			if !dirs[dir] {
				t.Fatalf("frame %d: unknown direction %q", i, dir)
			}
			sum += p.(float64)
		}
		if math.Abs(sum-1) > 1e-3 {
			t.Fatalf("frame %d: move probabilities sum to %v", i, sum)
		}
		proposed, executed := frame["proposed"].(string), frame["executed"].(string)
		if !dirs[proposed] || !dirs[executed] {
			t.Fatalf("frame %d: proposed %q executed %q", i, proposed, executed)
		}
		// The planner's contract: the executed move differs from the model's own
		// pick exactly when the planner intervened.
		if intervened := frame["intervened"].(bool); intervened != (proposed != executed) {
			t.Fatalf("frame %d: intervened=%v but proposed=%q executed=%q", i, intervened, proposed, executed)
		}
		for _, key := range reads {
			v := frame[key].(float64)
			if v < 0 || v > 1 {
				t.Fatalf("frame %d: %s = %v, want [0,1]", i, key, v)
			}
		}
	}
}

// TestLayaSnakeEpisode pins the task preset's loop: one call returns a bounded
// episode of frames, each carrying the model's probabilities and the shield's
// result, and the returned board chains into the next episode. The envelope
// rides on the model entry, so the calls carry no config argument -- the wire
// the decision plate uses.
func TestLayaSnakeEpisode(t *testing.T) {
	addr, srv := serveLaya(t, "")
	sha := layaPreload(t, srv, "../../scripts/snake.lua")
	episode := func(ticks int, board any) map[string]any {
		return layaEpisode(t, addr, sha, map[string]any{
			"ticks": ticks, "board": board, "width": 20, "height": 14, "seed": 7,
		})
	}

	first := episode(8, nil)
	frames, ok := first["frames"].([]any)
	if !ok || len(frames) != 8 {
		t.Fatalf("episode returned %v frames, want 8", first["frames"])
	}
	assertLayaFrames(t, frames, "risk", "food")
	if usage := first["usage"].(map[string]any); usage["input_tokens"].(float64) <= 0 {
		t.Fatalf("usage = %v", usage)
	}

	// The returned board chains: a one-tick episode resumes from it.
	board := first["board"].(map[string]any)
	next := episode(1, board)["board"].(map[string]any)
	if next["ticks"].(float64) != board["ticks"].(float64)+1 {
		t.Fatalf("chained board ticks = %v, want %v", next["ticks"], board["ticks"].(float64)+1)
	}

	// An unbounded request is clamped to the preset's ceiling rather than run.
	if got := len(episode(9999, nil)["frames"].([]any)); got > 200 {
		t.Fatalf("episode frames = %d, want <= 200", got)
	}

	// A board past the dimension cap is refused before it allocates.
	raw, err := json.Marshal(map[string]any{"ticks": 1, "width": 100000, "height": 100000})
	if err != nil {
		t.Fatal(err)
	}
	if tok := redisCmd(t, addr, "EMB.EVSHA", "laya-tiny", sha, "1", string(raw)); tok.kind != "error" {
		t.Fatalf("oversized board reply = %+v, want an error", tok)
	}

	// A zero-tick call returns the opening board and no frames (the plate's
	// opening render); an empty Lua table encodes as `{}`, so accept either.
	opening := episode(0, nil)
	switch f := opening["frames"].(type) {
	case nil, []any, map[string]any:
	default:
		t.Fatalf("unexpected frames type %T", f)
	}
	if _, ok := opening["board"].(map[string]any); !ok {
		t.Fatalf("zero-tick episode returned no board: %v", opening)
	}
}

// TestLayaPacmanEpisode pins the second task preset's loop. It shares the
// contract above; the preset is not snake's: the readouts are danger/clear and
// the chained state is a game, not a board.
func TestLayaPacmanEpisode(t *testing.T) {
	addr, srv := serveLaya(t, "")
	sha := layaPreload(t, srv, "../../scripts/pacman.lua")
	episode := func(ticks int, game any) map[string]any {
		return layaEpisode(t, addr, sha, map[string]any{"ticks": ticks, "game": game, "seed": 7})
	}

	first := episode(6, nil)
	frames, ok := first["frames"].([]any)
	if !ok || len(frames) != 6 {
		t.Fatalf("episode returned %v frames, want 6", first["frames"])
	}
	assertLayaFrames(t, frames, "danger", "clear")
	if usage := first["usage"].(map[string]any); usage["input_tokens"].(float64) <= 0 {
		t.Fatalf("usage = %v", usage)
	}

	// The returned game chains: a one-tick episode resumes from it.
	game := first["game"].(map[string]any)
	next := episode(1, game)["game"].(map[string]any)
	if next["ticks"].(float64) != game["ticks"].(float64)+1 {
		t.Fatalf("chained game ticks = %v, want %v", next["ticks"], game["ticks"].(float64)+1)
	}

	// An unbounded request is clamped to the preset's ceiling rather than run.
	if got := len(episode(9999, nil)["frames"].([]any)); got > 400 {
		t.Fatalf("episode frames = %d, want <= 400", got)
	}

	// A zero-tick call returns the opening game and no frames; an empty Lua table
	// encodes as `{}`, so accept either.
	opening := episode(0, nil)
	switch f := opening["frames"].(type) {
	case nil, []any, map[string]any:
	default:
		t.Fatalf("unexpected frames type %T", f)
	}
	if _, ok := opening["game"].(map[string]any); !ok {
		t.Fatalf("zero-tick episode returned no game: %v", opening)
	}
}
