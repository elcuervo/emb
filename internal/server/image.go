package server

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"runtime"
	"strings"
	"sync"
	"time"

	"github.com/tidwall/redcon"

	"github.com/elcuervo/emb/internal/registry"
)

// imageCachePrefix namespaces image entries in the shared cache. The full key is
// img:<model>:<sha256(bytes)>:<output>, so changed bytes are always a different
// key (never stale), each output of one image is addressable on its own, and
// model-scoped flush still works.
const imageCachePrefix = "img:"

// Image batch chunk bounds. Peak memory for one command is proportional to a
// chunk, not to the number of images: an image expands to exactly 3*size*size
// float32 values regardless of its encoded size, so a command carrying thousands
// of tiny valid PNGs would otherwise hold gigabytes of decoded tensors (plus one
// equally large contiguous inference batch) before any inference runs. The
// element budget bounds large image sizes; the image count bounds per-run latency
// and session scratch for small ones.
const (
	maxImageBatchImages   = 64
	maxImageBatchElements = 16 << 20 // float32 elements (~64 MiB per tensor set)
)

func imageCacheKey(model string, data []byte, output string) string {
	sum := sha256.Sum256(data)
	return imageCachePrefix + model + ":" + hex.EncodeToString(sum[:]) + ":" + output
}

// isURLArg reports whether an image argument looks like a URL (or a data: URI)
// rather than raw image bytes. emb deliberately never fetches remote data, so
// these are rejected with guidance instead of being decoded.
func isURLArg(arg []byte) bool {
	for _, prefix := range []string{"http://", "https://", "data:"} {
		if len(arg) >= len(prefix) && strings.EqualFold(string(arg[:len(prefix)]), prefix) {
			return true
		}
	}
	return false
}

// imageKeywords is what the fixed leading keywords of an image command carry:
// the reply format, the requested output names (nil = every configured output),
// and how many arguments they consumed.
type imageKeywords struct {
	format   replyFormat
	names    []string
	consumed int
}

// parseImageKeywords parses the fixed leading keywords of EMB.IMG/EMB.IMGMULTI
// starting at args[pos]: the reply format first, then `OUTPUTS <name>[,<name>...]`.
// A keyword counts only when at least one payload argument follows it, so a
// trailing `VALUES` stays an image argument and never shadows a slot.
func parseImageKeywords(args [][]byte, pos int) (imageKeywords, error) {
	var kw imageKeywords
	format, hasFormat := parseFormatArg(args, pos)
	kw.format = format
	if hasFormat {
		kw.consumed = 1
		pos++
	}
	if len(args) > pos+2 && isKeyword(args[pos], "outputs") {
		list := string(args[pos+1])
		if list == "" {
			return kw, fmt.Errorf("ERR OUTPUTS requires a comma-separated list of output names")
		}
		kw.names = strings.Split(list, ",")
		kw.consumed += 2
	}
	return kw, nil
}

// selectImageOutputs narrows a model's configured outputs to the requested
// names, keeping the configured order and dropping repeats. No names means every
// configured output; a name the model does not configure is an error naming the
// configured set. It is the one place an image command's projection is decided.
func selectImageOutputs(configured []registry.ImageOutput, names []string) ([]registry.ImageOutput, error) {
	if len(names) == 0 {
		return configured, nil
	}
	known := make(map[string]struct{}, len(configured))
	available := make([]string, 0, len(configured))
	for _, o := range configured {
		known[o.Name] = struct{}{}
		available = append(available, o.Name)
	}
	wanted := make(map[string]struct{}, len(names))
	for _, n := range names {
		if _, ok := known[n]; !ok {
			return nil, fmt.Errorf("unknown image output %q; configured outputs: %s", n, strings.Join(available, ", "))
		}
		wanted[n] = struct{}{}
	}
	selected := make([]registry.ImageOutput, 0, len(wanted))
	for _, o := range configured {
		if _, ok := wanted[o.Name]; ok {
			selected = append(selected, o)
		}
	}
	return selected, nil
}

// handleIMG implements EMB.IMG <model> [BLOB|VALUES] [OUTPUTS <name>[,...]]
// <bytes> [<bytes>...]. Each trailing argument is the raw encoded content of an
// image; the reply is one map from output name to value per image (a map of
// name/value pairs, which RESP3 renders as a map), or several maps in request
// order, with null for a failed or truncated slot.
func (s *Server) handleIMG(conn redcon.Conn, cmd redcon.Command) {
	if len(cmd.Args) < 3 {
		conn.WriteError("ERR wrong number of arguments for 'EMB.IMG' command")
		return
	}

	started := time.Now()
	modelName := string(cmd.Args[1])
	kw, err := parseImageKeywords(cmd.Args[1:], 1)
	if err != nil {
		conn.WriteError(err.Error())
		return
	}
	imgArgs := cmd.Args[2+kw.consumed:]
	total := len(imgArgs)

	failed := false
	// An image request's inference runs on its own named-tensor sessions, which
	// keep no dispatch or run accounting, so its event carries the cache-served
	// count and no phases: absent attribution, not a claim of a free run.
	var attribution requestAttribution
	defer func() {
		s.recordRequest(MonitorEvent{
			AtUs:      started.UnixMicro(),
			Model:     modelName,
			Texts:     total,
			LatencyUs: time.Since(started).Microseconds(),
			Err:       failed,
		}.attributed(attribution))
	}()

	for _, arg := range imgArgs {
		if isURLArg(arg) {
			failed = true
			conn.WriteError("ERR image argument is a URL; emb does not fetch images — fetch the image client-side and send its raw bytes")
			return
		}
	}

	entry, err := s.reg.Resolve(modelName)
	if err != nil {
		failed = true
		conn.WriteError(fmt.Sprintf("ERR %v", err))
		return
	}
	res, err := entry.ImageResources()
	if err != nil {
		failed = true
		conn.WriteError(fmt.Sprintf("ERR %v", err))
		return
	}
	if res == nil {
		failed = true
		conn.WriteError(fmt.Sprintf("ERR model '%s' has no image configuration; add an image: block to enable EMB.IMG", modelName))
		return
	}
	outputs, err := selectImageOutputs(res.Outputs, kw.names)
	if err != nil {
		failed = true
		conn.WriteError(fmt.Sprintf("ERR %v", err))
		return
	}
	s.imageRequests.Add(1)

	// Truncate oversized commands: only the first maxImages are decoded and
	// inferred; the overflow slots stay null.
	n := total
	if limit := s.maxImages.Load(); limit > 0 && int64(n) > limit {
		s.truncatedImages.Add(int64(n) - limit)
		n = int(limit)
	}
	images := imgArgs[:n]

	results, cached, errs := s.embedImages(modelName, res, outputs, images)
	attribution.cachedTexts = cached
	for _, e := range errs {
		if e != nil {
			failed = true
			break
		}
	}
	writeIMGResult(conn, kw.format, outputs, results, n, total)
}

// writeIMGResult emits the EMB.IMG reply: one keyed map for a single image, or a
// total-length array of maps (null for failures/overflow) in request order.
func writeIMGResult(conn redcon.Conn, format replyFormat, outputs []registry.ImageOutput, results []map[string][]byte, n, total int) {
	if total == 1 && n == 1 {
		if results[0] == nil {
			conn.WriteNull()
			return
		}
		writeImageMap(conn, format, outputs, results[0])
		return
	}
	conn.WriteArray(total)
	for _, r := range results {
		if r == nil {
			conn.WriteNull()
			continue
		}
		writeImageMap(conn, format, outputs, r)
	}
	for i := n; i < total; i++ {
		conn.WriteNull()
	}
}

// writeImageMap writes one image's products as a map from output name to value,
// in configured order: a bulk of raw little-endian float32 bytes under BLOB, a
// dtype/shape/values envelope (plus grid for a token output) under VALUES.
func writeImageMap(conn redcon.Conn, format replyFormat, outputs []registry.ImageOutput, values map[string][]byte) {
	writePairs(conn, len(outputs))
	for _, o := range outputs {
		conn.WriteBulkString(o.Name)
		writeImageOutputValue(conn, format, o, values[o.Name])
	}
}

// writeImageOutputValue writes one output's value: raw float32 bytes under BLOB,
// or a self-describing envelope under VALUES.
func writeImageOutputValue(conn redcon.Conn, format replyFormat, o registry.ImageOutput, blob []byte) {
	if format == formatBLOB {
		conn.WriteBulk(blob)
		return
	}
	pairs := 3
	if o.Select == "tokens" {
		pairs = 4
	}
	writePairs(conn, pairs)
	conn.WriteBulkString("dtype")
	conn.WriteBulkString("FLOAT")
	conn.WriteBulkString("shape")
	conn.WriteArray(2)
	conn.WriteInt(o.Rows)
	conn.WriteInt(o.Dim)
	conn.WriteBulkString("values")
	// One blob of rows*dim float32 values: the row width is the whole vector.
	writeValuesArray(conn, [][]byte{blob}, o.Rows*o.Dim)
	if o.Select == "tokens" {
		conn.WriteBulkString("grid")
		conn.WriteArray(2)
		conn.WriteInt(o.Grid)
		conn.WriteInt(o.Grid)
	}
}

// embedImages resolves raw image bytes to their requested outputs for one model:
// each output's cache entry is consulted and populated independently, misses are
// preprocessed (in parallel) and run in bounded batched inferences that compute
// every still-missing output from one run per chunk. results[i] is nil when
// images[i] failed and errs[i] carries its error. A single bad image never fails
// the others. The returned count is how many images the reply cache answered
// completely.
//
// Work is chunked by a fixed tensor budget: an image expands to exactly
// 3*size*size float32 values regardless of its encoded size, so a command
// carrying thousands of tiny valid PNGs would otherwise hold gigabytes of
// decoded tensors (plus one equally large contiguous batch) before any inference
// runs. Chunking keeps peak memory proportional to the budget, not to the number
// of images.
func (s *Server) embedImages(modelName string, res *registry.ImageResources, outputs []registry.ImageOutput, images [][]byte) ([]map[string][]byte, int, []error) {
	results := make([]map[string][]byte, len(images))
	errs := make([]error, len(images))
	if len(images) == 0 {
		return results, 0, errs
	}

	cached := 0
	misses := make([]int, 0, len(images))
	for i, data := range images {
		values := make(map[string][]byte, len(outputs))
		complete := true
		for _, o := range outputs {
			if s.cache != nil {
				if emb, ok := s.cache.Get(imageCacheKey(modelName, data, o.Name)); ok {
					values[o.Name] = emb
					continue
				}
			}
			complete = false
		}
		if complete {
			results[i] = values
			cached++
			continue
		}
		if len(values) > 0 {
			results[i] = values
		}
		misses = append(misses, i)
	}
	if len(misses) == 0 {
		return results, cached, errs
	}

	// Per-request plan copy carries the server's live byte/pixel caps without
	// mutating the shared immutable plan.
	plan := res.Plan
	plan.MaxBytes = s.maxImageBytes.Load()
	plan.MaxPixels = s.maxImagePixels.Load()

	// Bound each chunk by the image count, the float32-element budget, and a
	// batch axis the graph declares statically (a batch-1 export runs one image
	// per session run).
	chunkSize := min(len(misses), maxImageBatchImages)
	if limit := res.BatchCap; limit > 0 && limit < chunkSize {
		chunkSize = limit
	}
	if elem := plan.ElementCount(); elem > 0 && maxImageBatchElements/elem < chunkSize {
		chunkSize = maxImageBatchElements / elem
		if chunkSize < 1 {
			chunkSize = 1
		}
	}

	type preprocessed struct {
		slot   int
		tensor []float32
		err    error
	}
	for start := 0; start < len(misses); start += chunkSize {
		end := start + chunkSize
		if end > len(misses) {
			end = len(misses)
		}
		chunk := misses[start:end]

		// The outputs still missing for any image in this chunk: one session run
		// computes all of them. Repeats are impossible because the projection is
		// command-wide and deduped by selectImageOutputs.
		need := make([]registry.ImageOutput, 0, len(outputs))
		for _, o := range outputs {
			for _, slot := range chunk {
				if _, ok := results[slot][o.Name]; !ok {
					need = append(need, o)
					break
				}
			}
		}
		if len(need) == 0 {
			continue
		}

		out := make([]preprocessed, len(chunk))
		workers := runtime.GOMAXPROCS(0)
		if workers < 1 {
			workers = 1
		}
		if workers > len(chunk) {
			workers = len(chunk)
		}
		sem := make(chan struct{}, workers)
		var wg sync.WaitGroup
		for j, slot := range chunk {
			wg.Add(1)
			sem <- struct{}{}
			go func(j, slot int) {
				defer wg.Done()
				defer func() { <-sem }()
				t, err := plan.Tensor(images[slot])
				out[j] = preprocessed{slot: slot, tensor: t, err: err}
			}(j, slot)
		}
		wg.Wait()

		tensors := make([][]float32, 0, len(out))
		slots := make([]int, 0, len(out))
		for _, p := range out {
			if p.err != nil {
				errs[p.slot] = p.err
				continue
			}
			tensors = append(tensors, p.tensor)
			slots = append(slots, p.slot)
		}
		if len(tensors) == 0 {
			continue
		}

		computed, err := res.Embed(tensors, need)
		if err != nil {
			for _, slot := range slots {
				errs[slot] = err
			}
			continue
		}
		for k, slot := range slots {
			for _, o := range need {
				blob := computed[k][o.Name]
				if s.cache != nil {
					s.cache.Set(imageCacheKey(modelName, images[slot], o.Name), blob)
				}
			}
			if results[slot] == nil {
				results[slot] = computed[k]
				continue
			}
			for name, blob := range computed[k] {
				results[slot][name] = blob
			}
		}
	}
	return results, cached, errs
}

// handleIMGMULTI implements EMB.IMGMULTI [BLOB|VALUES] [OUTPUTS <name>[,...]]
// <model> <bytes> ... with alternating model/image pairs and MGET-style
// per-pair nulls.
func (s *Server) handleIMGMULTI(conn redcon.Conn, cmd redcon.Command) {
	kw, err := parseImageKeywords(cmd.Args, 1)
	if err != nil {
		conn.WriteError(err.Error())
		return
	}
	pairs := cmd.Args[1+kw.consumed:]
	if len(pairs) < 2 || len(pairs)%2 != 0 {
		conn.WriteError("ERR wrong number of arguments for 'EMB.IMGMULTI' command")
		return
	}

	for i := 1; i < len(pairs); i += 2 {
		if isURLArg(pairs[i]) {
			conn.WriteError("ERR image argument is a URL; emb does not fetch images — fetch the image client-side and send its raw bytes")
			return
		}
	}

	total := len(pairs) / 2
	n := total
	if limit := s.maxImages.Load(); limit > 0 && int64(n) > limit {
		s.truncatedImages.Add(int64(n) - limit)
		n = int(limit)
		pairs = pairs[:n*2]
	}
	results := make([]map[string][]byte, n)
	pairOutputs := make([][]registry.ImageOutput, n)
	names := kw.names

	fanOut := s.multiPairFanOut(n)
	jobs := make(chan int)
	var wg sync.WaitGroup
	for w := 0; w < fanOut; w++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for idx := range jobs {
				s.processIMGMultiPair(pairs, names, results, pairOutputs, idx)
			}
		}()
	}
	for i := range n {
		jobs <- i
	}
	close(jobs)
	wg.Wait()

	writeIMGMULTIResult(conn, kw.format, pairs, pairOutputs, results, n, total)
}

func (s *Server) processIMGMultiPair(pairs [][]byte, names []string, results []map[string][]byte, pairOutputs [][]registry.ImageOutput, idx int) {
	model := string(pairs[idx*2])
	data := pairs[idx*2+1]

	started := time.Now()
	s.imageRequests.Add(1)
	failed := false
	// See handleIMG: the image path reports its cache-served count only.
	var attribution requestAttribution
	defer func() {
		s.recordRequest(MonitorEvent{
			AtUs:      started.UnixMicro(),
			Model:     model,
			Texts:     1,
			LatencyUs: time.Since(started).Microseconds(),
			Err:       failed,
		}.attributed(attribution))
	}()

	entry, err := s.reg.Resolve(model)
	if err != nil {
		failed = true
		return
	}
	res, err := entry.ImageResources()
	if err != nil || res == nil {
		failed = true
		return
	}
	outputs, err := selectImageOutputs(res.Outputs, names)
	if err != nil {
		// A name this model does not configure fails just its pair: different
		// pairs may target models with different products.
		failed = true
		return
	}
	embeddings, cached, errs := s.embedImages(model, res, outputs, [][]byte{data})
	attribution.cachedTexts = cached
	if len(errs) == 1 && errs[0] != nil {
		failed = true
		return
	}
	if len(embeddings) == 1 {
		results[idx] = embeddings[0]
		pairOutputs[idx] = outputs
	}
}

// writeIMGMULTIResult mirrors writeMultiResult for image pairs: one keyed map per
// pair in argument order, null for a failed pair. Under VALUES each pair's map
// carries its model beside the outputs.
func writeIMGMULTIResult(conn redcon.Conn, format replyFormat, pairs [][]byte, pairOutputs [][]registry.ImageOutput, results []map[string][]byte, n, total int) {
	conn.WriteArray(total)
	for i, r := range results {
		if r == nil {
			conn.WriteNull()
			continue
		}
		outputs := pairOutputs[i]
		if format == formatBLOB {
			writeImageMap(conn, format, outputs, r)
			continue
		}
		writePairs(conn, len(outputs)+1)
		conn.WriteBulkString("model")
		conn.WriteBulkString(string(pairs[i*2]))
		for _, o := range outputs {
			conn.WriteBulkString(o.Name)
			writeImageOutputValue(conn, format, o, r[o.Name])
		}
	}
	for i := n; i < total; i++ {
		conn.WriteNull()
	}
}
