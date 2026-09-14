package server

import (
	"crypto/sha1" //nolint:gosec // script cache identity by SHA1, Redis semantics (not a security primitive)
	"encoding/hex"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/tidwall/redcon"
	lua "github.com/yuin/gopher-lua"

	"github.com/elcuervo/emb/internal/bounded"
	"github.com/elcuervo/emb/internal/onnx"
	"github.com/elcuervo/emb/internal/registry"
	"github.com/elcuervo/emb/internal/script"
	"github.com/elcuervo/emb/internal/tokenizer"
)

// scriptCache stores script source by model and SHA1. Scripts are cached per
// model (mirroring the registry): the same SHA against two models is two
// entries, because the graph contract differs.
//
// Ownership and bound: the cache owns the source strings and is bounded per
// model by max (default 1024) via bounded.Map, which evicts the
// lexicographically smallest key. EMB.SCRIPT FLUSH drops a model's entries (or
// all of them). The compiled-bytecode cache (script.Compiler) is bounded the
// same way and flushed alongside it.
type scriptCache struct {
	by *bounded.Map[string] // model → sha → source
}

func newScriptCache(max int) *scriptCache {
	if max <= 0 {
		max = 1024
	}
	return &scriptCache{by: bounded.New[string](max)}
}

func scriptSHA(src string) string {
	//nolint:gosec // cache identity only, per Redis EVALSHA semantics
	h := sha1.Sum([]byte(src))
	return hex.EncodeToString(h[:])
}

func (c *scriptCache) Load(model, src string) (sha string, exists bool) {
	sha = scriptSHA(src)
	_, existed, _ := c.by.GetOrCreate(model, sha, func() (string, error) { return src, nil })
	return sha, existed
}

func (c *scriptCache) Get(model, sha string) (string, bool) { return c.by.Get(model, sha) }

func (c *scriptCache) Exists(model, sha string) bool {
	_, ok := c.by.Get(model, sha)
	return ok
}

func (c *scriptCache) Flush(model string) { c.by.Clear(model) }

// handleSCRIPT implements EMB.SCRIPT LOAD|EXISTS|FLUSH [<model>] [<script>|<sha...>].
func (s *Server) handleSCRIPT(conn redcon.Conn, cmd redcon.Command) {
	if s.shuttingDown.Load() {
		conn.WriteError("ERR server shutting down")
		return
	}
	args := cmd.Args[1:]
	var argStr []string
	for _, a := range args {
		argStr = append(argStr, string(a))
	}
	if len(argStr) < 1 {
		conn.WriteError("ERR wrong number of arguments for 'EMB.SCRIPT' command")
		return
	}
	sub := strings.ToLower(argStr[0])
	switch sub {
	case "load":
		s.handleScriptLoad(conn, argStr[1:])
	case "exists":
		s.handleScriptExists(conn, argStr[1:])
	case "flush":
		s.handleScriptFlush(conn, argStr[1:])
	default:
		conn.WriteError(fmt.Sprintf("ERR unknown EMB.SCRIPT subcommand '%s'", sub))
	}
}

func (s *Server) handleScriptLoad(conn redcon.Conn, args []string) {
	if len(args) != 2 {
		conn.WriteError("ERR wrong number of arguments for 'EMB.SCRIPT LOAD'")
		return
	}
	sha, err := s.PreloadScript(args[0], args[1])
	if err != nil {
		conn.WriteError(fmt.Sprintf("ERR %v", err))
		return
	}
	conn.WriteBulkString(sha)
}

func (s *Server) handleScriptExists(conn redcon.Conn, args []string) {
	if len(args) < 2 {
		conn.WriteError("ERR wrong number of arguments for 'EMB.SCRIPT EXISTS'")
		return
	}
	model := args[0]
	if _, err := s.reg.Resolve(model); err != nil {
		conn.WriteError(fmt.Sprintf("ERR %v", err))
		return
	}
	conn.WriteArray(len(args) - 1)
	for _, sha := range args[1:] {
		if s.scripts.Exists(model, sha) {
			conn.WriteInt(1)
		} else {
			conn.WriteInt(0)
		}
	}
}

func (s *Server) handleScriptFlush(conn redcon.Conn, args []string) {
	if len(args) > 1 {
		conn.WriteError("ERR wrong number of arguments for 'EMB.SCRIPT FLUSH'")
		return
	}
	model := ""
	if len(args) == 1 {
		model = args[0]
	}
	s.scripts.Flush(model)
	s.compiler.Flush(model)
	conn.WriteString("OK")
}

// handleEVAL implements EMB.EVAL <model> <script> <numtexts> <text...> <arg...>.
func (s *Server) handleEVAL(conn redcon.Conn, cmd redcon.Command) {
	args := cmdArgs(cmd)
	rest, err := splitEvalArgs(args)
	if err != nil {
		conn.WriteError(fmt.Sprintf("ERR %v", err))
		return
	}
	s.runScripted(conn, rest.model, rest.script, rest.sha, rest.texts, rest.args)
}

// handleEVSHA implements EMB.EVSHA <model> <sha> <numtexts> <text...> <arg...>.
func (s *Server) handleEVSHA(conn redcon.Conn, cmd redcon.Command) {
	args := cmdArgs(cmd)
	if len(args) < 3 {
		conn.WriteError("ERR wrong number of arguments for 'EMB.EVSHA' command")
		return
	}
	model, sha := args[0], args[1]
	src, ok := s.scripts.Get(model, sha)
	if !ok {
		conn.WriteError("ERR no such script")
		return
	}
	parseArgs := append([]string{model, src}, args[2:]...)
	rest, err := splitEvalArgs(parseArgs)
	if err != nil {
		conn.WriteError(fmt.Sprintf("ERR %v", err))
		return
	}
	rest.sha = sha
	s.runScripted(conn, rest.model, rest.script, rest.sha, rest.texts, rest.args)
}

func cmdArgs(cmd redcon.Command) []string {
	out := make([]string, len(cmd.Args)-1)
	for i, a := range cmd.Args[1:] {
		out[i] = string(a)
	}
	return out
}

type evalSplit struct {
	model  string
	script string
	sha    string
	texts  []string
	args   []string
}

// errTokenizerUnavailable is returned by the lazily-bound tokenizer hosts when
// the model's tokenizer does not provide the requested capability. It mirrors
// the message the eager binding used to produce via a nil host.
var errTokenizerUnavailable = errors.New("unavailable for this model")

// splitEvalArgs parses `<model> <script> <numtexts> <text...> <arg...>`.
func splitEvalArgs(args []string) (*evalSplit, error) {
	if len(args) < 3 {
		return nil, fmt.Errorf("wrong number of arguments for script evaluation")
	}
	model, scriptSource := args[0], args[1]
	numTexts, err := strconv.Atoi(args[2])
	if err != nil || numTexts < 1 || numTexts > 1<<30 {
		return nil, fmt.Errorf("numtexts must be a positive integer")
	}
	if len(args) < 3+numTexts {
		return nil, fmt.Errorf("not enough text arguments")
	}
	texts := make([]string, numTexts)
	for i := 0; i < numTexts; i++ {
		texts[i] = args[3+i]
	}
	rest := args[3+numTexts:]
	return &evalSplit{
		model:  model,
		script: scriptSource,
		sha:    scriptSHA(scriptSource),
		texts:  texts,
		args:   rest,
	}, nil
}

// runScripted executes a script over one or more texts and writes the reply:
// a single converted value for one text, an array of converted values for
// several. The script runs ONCE with all request texts as KEYS, so the script
// can batch its inference (emb.run_batch); for multi-text calls the script
// must return an array whose elements correspond 1:1 to the texts. When the
// server cache is enabled, a request whose texts are ALL cache hits replies
// without re-running the script; any miss triggers one evaluation over the
// full KEYS list, and each text's converted element is cached under its
// content-addressed key.
func (s *Server) runScripted(conn redcon.Conn, model, src, sha string, texts, args []string) {
	// Record the evaluation for EMB.STATS and MONITOR (bounded ring, no text
	// payloads). The clock starts before any work so the recorded latency
	// covers parsing through reply writing, mirroring EMB.
	started := time.Now()
	failed := false
	defer func() {
		latency := time.Since(started)
		s.scriptRequests.Add(1)
		s.scriptLatencyUs.Add(latency.Microseconds())
		if failed {
			s.scriptErrors.Add(1)
		}
		if entry, err := s.reg.Resolve(model); err == nil {
			entry.RecordScriptedEvaluation(failed)
		}
		s.monitor.Add(MonitorEvent{
			AtUs:      started.UnixMicro(),
			Model:     model,
			Texts:     len(texts),
			LatencyUs: latency.Microseconds(),
			Err:       failed,
		})
	}()

	if s.maxTexts > 0 && len(texts) > s.maxTexts {
		failed = true
		conn.WriteError(fmt.Sprintf("ERR too many texts: %d (max %d)", len(texts), s.maxTexts))
		return
	}

	entry, err := s.reg.Resolve(model)
	if err != nil {
		failed = true
		conn.WriteError(fmt.Sprintf("ERR %v", err))
		return
	}

	// Script resources (named-tensor sessions + the model tokenizer) are loaded
	// lazily: only the host functions that actually need them trigger the load.
	// A script that uses emb.embed alone, or that returns a constant, therefore
	// never opens a second session pool.
	var resOnce sync.Once
	var res *registry.ScriptResources
	var resErr error
	resolve := func() (*registry.ScriptResources, error) {
		resOnce.Do(func() { res, resErr = entry.ScriptResources() })
		return res, resErr
	}

	hosts := script.Hosts{
		Run: func(inputs []onnx.NamedTensor) (map[string]onnx.NamedTensor, error) {
			r, err := resolve()
			if err != nil {
				return nil, err
			}
			return r.Session().RunNamed(inputs)
		},
		EncodePretokenized: func(words []string, maxLen int) ([]int64, []int64, error) {
			r, err := resolve()
			if err != nil {
				return nil, nil, err
			}
			pT, ok := r.Tokenizer.(tokenizer.PretokenizedTokenizer)
			if !ok {
				return nil, nil, errTokenizerUnavailable
			}
			return pT.EncodePretokenized(words, maxLen)
		},
		EncodePlain: func(text string, maxLen int) ([]int64, []int64, [][2]int, error) {
			r, err := resolve()
			if err != nil {
				return nil, nil, nil, err
			}
			oT, ok := r.Tokenizer.(tokenizer.OffsetTokenizer)
			if !ok {
				return nil, nil, nil, errTokenizerUnavailable
			}
			return oT.EncodeOffsets(text, maxLen)
		},
		EncodePair: func(first, second string, maxLen int) ([]int64, []int64, [][2]int, int, error) {
			r, err := resolve()
			if err != nil {
				return nil, nil, nil, 0, err
			}
			oT, ok := r.Tokenizer.(tokenizer.OffsetTokenizer)
			if !ok {
				return nil, nil, nil, 0, errTokenizerUnavailable
			}
			return oT.EncodePairOffsets(first, second, maxLen)
		},
	}
	// emb.embed is bound only for models that can produce embeddings, so a
	// script targeting a non-embeddable graph leaves the function absent (the
	// same gating style as emb.image). The pool loads lazily on first call.
	if entry.Embeddable() {
		hosts.Embed = func(texts []string) ([][]byte, error) {
			e, err := s.reg.GetOrInit(model)
			if err != nil {
				return nil, err
			}
			if s.maxTexts > 0 && len(texts) > s.maxTexts {
				return nil, fmt.Errorf("too many texts: %d (max %d)", len(texts), s.maxTexts)
			}
			return s.embedTexts(e, model, texts)
		}
	}
	// A model with an image surface also gets emb.image.preprocess / info /
	// embed. The plan resolves lazily without opening sessions (so info and
	// preprocess allocate nothing image-related), and the image session pool is
	// opened only on the first emb.image.embed, which shares the EMB.IMG cache
	// through embedImages.
	if entry.HasImageSurface() {
		var imgOnce sync.Once
		var imgRes *registry.ImageResources
		var imgErr error
		resolveImage := func() (*registry.ImageResources, error) {
			imgOnce.Do(func() { imgRes, imgErr = entry.ImageResources() })
			return imgRes, imgErr
		}
		hosts.Image = &script.ImageHost{
			Plan: entry.ImagePlan,
			Preprocess: func(data []byte) ([]float32, error) {
				plan, err := entry.ImagePlan()
				if err != nil {
					return nil, err
				}
				// The per-request plan copy carries the server's live byte/pixel
				// caps so scripts cannot bypass them, exactly like EMB.IMG.
				plan.MaxBytes = s.maxImageBytes
				plan.MaxPixels = s.maxImagePixels
				return plan.Tensor(data)
			},
			Embed: func(images [][]byte) ([][]byte, error) {
				r, err := resolveImage()
				if err != nil {
					return nil, err
				}
				if r == nil {
					return nil, fmt.Errorf("model '%s' has no image configuration", model)
				}
				if s.maxImages > 0 && len(images) > s.maxImages {
					return nil, fmt.Errorf("too many images: %d (max %d)", len(images), s.maxImages)
				}
				results, errs := s.embedImages(model, r, images)
				for i, e := range errs {
					if e != nil {
						return nil, fmt.Errorf("image %d: %w", i, e)
					}
				}
				return results, nil
			},
		}
	}

	// Cache lookup: serve entirely from cache when every text is a hit; any
	// miss (or no cache) falls through to ONE evaluation with ALL the request
	// texts as KEYS (Redis semantics), so the script always sees the true
	// request context and returns the same shape as a cold run. Per-text
	// replies are cached under their content-addressed keys.
	replies := make([][]byte, len(texts))
	if s.cache != nil {
		allHit := true
		for i, text := range texts {
			if hit, ok := s.cache.Get(script.CacheKey(model, sha, args, text)); ok {
				replies[i] = hit
			} else {
				allHit = false
			}
		}
		if allHit {
			s.writeScriptReply(conn, replies)
			return
		}
	}

	// Evaluate the script once with all texts as KEYS. For a multi-text
	// request the script must return one value per text (in order); a single
	// text returns the value itself.
	v, err := s.compiler.Eval(model, src, texts, args, hosts, script.EvalOptions{Deadline: s.scriptDeadline})
	if err != nil {
		failed = true
		conn.WriteError(fmt.Sprintf("ERR %v", err))
		return
	}

	var values []lua.LValue
	if len(texts) == 1 {
		values = []lua.LValue{v}
	} else {
		tbl, ok := v.(*lua.LTable)
		if !ok || tbl.Len() != len(texts) {
			failed = true
			conn.WriteError(fmt.Sprintf("ERR script must return one value per text (%d texts)", len(texts)))
			return
		}
		values = make([]lua.LValue, len(texts))
		for j := 0; j < len(texts); j++ {
			values[j] = tbl.RawGetInt(j + 1)
		}
	}

	for i := range values {
		encoded, err := script.EncodeReply(values[i])
		if err != nil {
			failed = true
			conn.WriteError(fmt.Sprintf("ERR %v", err))
			return
		}
		replies[i] = encoded
		if s.cache != nil {
			s.cache.Set(script.CacheKey(model, sha, args, texts[i]), encoded)
		}
	}
	s.writeScriptReply(conn, replies)
}

// writeScriptReply emits a single converted reply for a one-text request or an
// array of per-text replies for a multi-text request.
func (s *Server) writeScriptReply(conn redcon.Conn, replies [][]byte) {
	if len(replies) == 1 {
		conn.WriteRaw(replies[0])
		return
	}
	conn.WriteArray(len(replies))
	for _, r := range replies {
		conn.WriteRaw(r)
	}
}
