package server

import (
	"crypto/sha1" //nolint:gosec // script cache identity by SHA1, Redis semantics (not a security primitive)
	"encoding/hex"
	"fmt"
	"strings"
	"sync"

	"github.com/tidwall/redcon"
	lua "github.com/yuin/gopher-lua"

	"github.com/elcuervo/emb/internal/onnx"
	"github.com/elcuervo/emb/internal/script"
	"github.com/elcuervo/emb/internal/tokenizer"
)

// scriptCache stores script source by model and SHA1. Scripts are cached per
// model (mirroring the registry): the same SHA against two models is two
// entries, because the graph contract differs.
type scriptCache struct {
	mu  sync.RWMutex
	by  map[string]map[string]string // model → sha → source
	max int
}

func newScriptCache(max int) *scriptCache {
	if max <= 0 {
		max = 1024
	}
	return &scriptCache{by: make(map[string]map[string]string), max: max}
}

func scriptSHA(src string) string {
	//nolint:gosec // cache identity only, per Redis EVALSHA semantics
	h := sha1.Sum([]byte(src))
	return hex.EncodeToString(h[:])
}

func (c *scriptCache) Load(model, src string) (sha string, exists bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	perModel := c.by[model]
	if perModel != nil && len(perModel) >= c.max {
		// Evict the oldest entry (maps iterate without order; a stable choice
		// is the lexicographically smallest key, which is arbitrary but
		// deterministic).
		oldest := ""
		for k := range perModel {
			if oldest == "" || k < oldest {
				oldest = k
			}
		}
		if oldest != "" {
			delete(perModel, oldest)
		}
	}
	if perModel == nil {
		perModel = make(map[string]string)
		c.by[model] = perModel
	}
	sha = scriptSHA(src)
	if _, ok := perModel[sha]; ok {
		return sha, true
	}
	perModel[sha] = src
	return sha, false
}

func (c *scriptCache) Get(model, sha string) (string, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	perModel := c.by[model]
	if perModel == nil {
		return "", false
	}
	src, ok := perModel[sha]
	return src, ok
}

func (c *scriptCache) Exists(model, sha string) bool {
	c.mu.RLock()
	defer c.mu.RUnlock()
	perModel := c.by[model]
	if perModel == nil {
		return false
	}
	_, ok := perModel[sha]
	return ok
}

func (c *scriptCache) Flush(model string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if model == "" {
		c.by = make(map[string]map[string]string)
		return
	}
	delete(c.by, model)
}

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
	model, src := args[0], args[1]
	if _, err := s.reg.Resolve(model); err != nil {
		conn.WriteError(fmt.Sprintf("ERR %v", err))
		return
	}
	if err := script.Compile(src); err != nil {
		conn.WriteError(fmt.Sprintf("ERR %v", err))
		return
	}
	sha, _ := s.scripts.Load(model, src)
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
	if s.shuttingDown.Load() {
		conn.WriteError("ERR server shutting down")
		return
	}
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
	if s.shuttingDown.Load() {
		conn.WriteError("ERR server shutting down")
		return
	}
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

// splitEvalArgs parses `<model> <script> <numtexts> <text...> <arg...>`.
func splitEvalArgs(args []string) (*evalSplit, error) {
	if len(args) < 3 {
		return nil, fmt.Errorf("wrong number of arguments for script evaluation")
	}
	model, scriptSource := args[0], args[1]
	numTexts, err := parseInt(args[2])
	if err != nil || numTexts < 1 {
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

func parseInt(s string) (int, error) {
	n := 0
	if s == "" {
		return 0, fmt.Errorf("empty integer")
	}
	for _, c := range s {
		if c < '0' || c > '9' {
			return 0, fmt.Errorf("not an integer: %q", s)
		}
		n = n*10 + int(c-'0')
		if n > 1<<30 {
			return 0, fmt.Errorf("integer too large")
		}
	}
	return n, nil
}

// runScripted executes a script over one or more texts and writes the reply:
// a single converted value for one text, an array of converted values for
// several. The script runs ONCE with all (uncached) texts as KEYS, so the
// script can batch its inference (emb.run_batch); for multi-text calls the
// script must return an array whose elements correspond 1:1 to the texts.
// Cache hits (when the server cache is enabled) skip the evaluation; each
// text's converted element is cached under its content-addressed key.
func (s *Server) runScripted(conn redcon.Conn, model, src, sha string, texts, args []string) {
	s.active.Add(1)
	defer s.active.Done()

	if s.maxTexts > 0 && len(texts) > s.maxTexts {
		conn.WriteError(fmt.Sprintf("ERR too many texts: %d (max %d)", len(texts), s.maxTexts))
		return
	}

	entry, err := s.reg.Resolve(model)
	if err != nil {
		conn.WriteError(fmt.Sprintf("ERR %v", err))
		return
	}
	res, err := entry.ScriptResources()
	if err != nil {
		conn.WriteError(fmt.Sprintf("ERR %v", err))
		return
	}

	hosts := script.Hosts{
		Run: func(inputs []onnx.NamedTensor) (map[string]onnx.NamedTensor, error) {
			return res.Session().RunNamed(inputs)
		},
	}
	if pT, ok := res.Tokenizer.(tokenizer.PretokenizedTokenizer); ok {
		hosts.EncodePretokenized = pT.EncodePretokenized
	}
	if oT, ok := res.Tokenizer.(tokenizer.OffsetTokenizer); ok {
		hosts.EncodePlain = oT.EncodeOffsets
		hosts.EncodePair = oT.EncodePairOffsets
	}

	// Per-text cache lookup first; only the misses reach the script.
	replies := make([][]byte, len(texts))
	missIdx := []int{}
	for i, text := range texts {
		if s.cache != nil {
			key := script.CacheKey(model, sha, args, text)
			if hit, ok := s.cache.Get(key); ok {
				replies[i] = hit
				continue
			}
		}
		missIdx = append(missIdx, i)
	}

	if len(missIdx) > 0 {
		missTexts := make([]string, len(missIdx))
		for j, idx := range missIdx {
			missTexts[j] = texts[idx]
		}

		v, err := s.compiler.Eval(model, src, missTexts, args, hosts, script.EvalOptions{Deadline: s.scriptDeadline})
		if err != nil {
			conn.WriteError(fmt.Sprintf("ERR %v", err))
			return
		}

		values := []lua.LValue{v}
		if len(missTexts) > 1 {
			tbl, ok := v.(*lua.LTable)
			if !ok || tbl.Len() != len(missTexts) {
				conn.WriteError(fmt.Sprintf("ERR script must return one value per text (%d texts)", len(missTexts)))
				return
			}
			values = make([]lua.LValue, len(missTexts))
			for j := 0; j < len(missTexts); j++ {
				values[j] = tbl.RawGetInt(j + 1)
			}
		}

		for j, idx := range missIdx {
			encoded, err := script.EncodeReply(values[j])
			if err != nil {
				conn.WriteError(fmt.Sprintf("ERR %v", err))
				return
			}
			replies[idx] = encoded
			if s.cache != nil {
				s.cache.Set(script.CacheKey(model, sha, args, texts[idx]), encoded)
			}
		}
	}

	if len(texts) == 1 {
		conn.WriteRaw(replies[0])
		return
	}
	conn.WriteArray(len(texts))
	for _, r := range replies {
		conn.WriteRaw(r)
	}
}
