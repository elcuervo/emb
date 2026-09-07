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
	// Reject oversized sources before they enter the source cache: EVAL/EVSHA
	// enforce the same cap at evaluation time, so LOAD must not accept what a
	// later EVSHA cannot run.
	if len(src) > script.DefaultMaxScriptBytes {
		conn.WriteError(fmt.Sprintf("ERR %v", script.ErrScriptTooLarge))
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
// several. The script runs ONCE with all request texts as KEYS, so the script
// can batch its inference (emb.run_batch); for multi-text calls the script
// must return an array whose elements correspond 1:1 to the texts. When the
// server cache is enabled, a request whose texts are ALL cache hits replies
// without re-running the script; any miss triggers one evaluation over the
// full KEYS list, and each text's converted element is cached under its
// content-addressed key.
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
		conn.WriteError(fmt.Sprintf("ERR %v", err))
		return
	}

	var values []lua.LValue
	if len(texts) == 1 {
		values = []lua.LValue{v}
	} else {
		tbl, ok := v.(*lua.LTable)
		if !ok || tbl.Len() != len(texts) {
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
