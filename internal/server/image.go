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

// imageCachePrefix namespaces image entries in the shared cache. The full key
// is img:<model>:<sha256(bytes)>, so changed bytes are always a different key
// (never stale) and model-scoped flush still works.
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

func imageCacheKey(model string, data []byte) string {
	sum := sha256.Sum256(data)
	return imageCachePrefix + model + ":" + hex.EncodeToString(sum[:])
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

// handleIMG implements EMB.IMG <model> [BLOB|VALUES] <bytes> [<bytes>...].
// Each argument is the raw encoded content of an image; the reply has one slot
// per argument (null for a failed/truncated one) under BLOB, or a single
// dtype/shape/values envelope over the processed images under VALUES.
func (s *Server) handleIMG(conn redcon.Conn, cmd redcon.Command) {
	if len(cmd.Args) < 3 {
		conn.WriteError("ERR wrong number of arguments for 'EMB.IMG' command")
		return
	}

	started := time.Now()
	modelName := string(cmd.Args[1])
	// Reply-format keyword sits at position 2 (right after the model) and is
	// only recognized when at least one image argument follows.
	format, hasFormat := parseFormatArg(cmd.Args[1:], 1)
	imgArgs := cmd.Args[2:]
	if hasFormat {
		imgArgs = cmd.Args[3:]
	}
	total := len(imgArgs)

	failed := false
	defer func() {
		s.monitor.Add(MonitorEvent{
			AtUs:      started.UnixMicro(),
			Model:     modelName,
			Texts:     total,
			LatencyUs: time.Since(started).Microseconds(),
			Err:       failed,
		})
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
	s.imageRequests.Add(1)

	// Truncate oversized commands: only the first maxImages are decoded and
	// inferred; the overflow slots stay null.
	n := total
	if limit := s.maxImages.Load(); limit > 0 && int64(n) > limit {
		s.truncatedImages.Add(int64(n) - limit)
		n = int(limit)
	}
	images := imgArgs[:n]

	results, errs := s.embedImages(modelName, res, images)
	for _, e := range errs {
		if e != nil {
			failed = true
			break
		}
	}
	writeIMGResult(conn, format, results, n, total, res.Dim)
}

// writeIMGResult emits the EMB.IMG reply: a single bulk for one image, a
// total-length array with nulls for failures/overflow under BLOB, or one
// [m, dim] VALUES envelope over the processed images under VALUES.
func writeIMGResult(conn redcon.Conn, format replyFormat, results [][]byte, n, total, dim int) {
	if format == formatVALUES {
		ok := make([][]byte, 0, len(results))
		for _, r := range results {
			if r != nil {
				ok = append(ok, r)
			}
		}
		writeValuesEmbReply(conn, ok, dim)
		return
	}
	if total == 1 && n == 1 {
		if results[0] == nil {
			conn.WriteNull()
			return
		}
		conn.WriteBulk(results[0])
		return
	}
	conn.WriteArray(total)
	for _, r := range results {
		if r == nil {
			conn.WriteNull()
		} else {
			conn.WriteBulk(r)
		}
	}
	for i := n; i < total; i++ {
		conn.WriteNull()
	}
}

// embedImages resolves raw image bytes to embeddings for one model: cache
// hits are returned directly, misses are preprocessed (in parallel) and run in
// bounded batched inferences. results[i] is nil when images[i] failed and errs[i]
// carries its error. A single bad image never fails the others.
//
// Work is chunked by a fixed tensor budget: an image expands to exactly
// 3*size*size float32 values regardless of its encoded size, so a command
// carrying thousands of tiny valid PNGs would otherwise hold gigabytes of
// decoded tensors (plus one equally large contiguous batch) before any inference
// runs. Chunking keeps peak memory proportional to the budget, not to the number
// of images.
func (s *Server) embedImages(modelName string, res *registry.ImageResources, images [][]byte) ([][]byte, []error) {
	results := make([][]byte, len(images))
	errs := make([]error, len(images))
	if len(images) == 0 {
		return results, errs
	}

	misses := make([]int, 0, len(images))
	for i, data := range images {
		if s.cache != nil {
			if emb, ok := s.cache.Get(imageCacheKey(modelName, data)); ok {
				results[i] = emb
				continue
			}
		}
		misses = append(misses, i)
	}
	if len(misses) == 0 {
		return results, errs
	}

	// Per-request plan copy carries the server's live byte/pixel caps without
	// mutating the shared immutable plan.
	plan := res.Plan
	plan.MaxBytes = s.maxImageBytes.Load()
	plan.MaxPixels = s.maxImagePixels.Load()

	// Bound each chunk by the image count and float32-element budget.
	chunkSize := min(len(misses), maxImageBatchImages)
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

		embeddings, err := res.Embed(tensors)
		if err != nil {
			for _, slot := range slots {
				errs[slot] = err
			}
			continue
		}
		for k, slot := range slots {
			results[slot] = embeddings[k]
			if s.cache != nil {
				s.cache.Set(imageCacheKey(modelName, images[slot]), embeddings[k])
			}
		}
	}
	return results, errs
}

// handleIMGMULTI implements EMB.IMGMULTI [BLOB|VALUES] <model> <bytes> ... with
// alternating model/image pairs and MGET-style per-pair nulls.
func (s *Server) handleIMGMULTI(conn redcon.Conn, cmd redcon.Command) {
	pairs := cmd.Args[1:]
	format, hasFormat := parseFormatArg(cmd.Args[1:], 0)
	if hasFormat {
		pairs = cmd.Args[2:]
	}
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
	results := make([][]byte, n)

	fanOut := s.multiPairFanOut(n)
	jobs := make(chan int)
	var wg sync.WaitGroup
	for w := 0; w < fanOut; w++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for idx := range jobs {
				s.processIMGMultiPair(pairs, results, idx)
			}
		}()
	}
	for i := range n {
		jobs <- i
	}
	close(jobs)
	wg.Wait()

	writeIMGMULTIResult(conn, s, format, pairs, results, n, total)
}

func (s *Server) processIMGMultiPair(pairs [][]byte, results [][]byte, idx int) {
	model := string(pairs[idx*2])
	data := pairs[idx*2+1]

	started := time.Now()
	s.imageRequests.Add(1)
	failed := false
	defer func() {
		s.monitor.Add(MonitorEvent{
			AtUs:      started.UnixMicro(),
			Model:     model,
			Texts:     1,
			LatencyUs: time.Since(started).Microseconds(),
			Err:       failed,
		})
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
	embeddings, errs := s.embedImages(model, res, [][]byte{data})
	if len(errs) == 1 && errs[0] != nil {
		failed = true
		return
	}
	if len(embeddings) == 1 {
		results[idx] = embeddings[0]
	}
}

// writeIMGMULTIResult mirrors writeMultiResult for image pairs: bulk/null slots
// under BLOB, one model-tagged VALUES envelope per successful pair under VALUES.
func writeIMGMULTIResult(conn redcon.Conn, s *Server, format replyFormat, pairs [][]byte, results [][]byte, n, total int) {
	conn.WriteArray(total)
	if format == formatVALUES {
		for i, r := range results {
			if r == nil {
				conn.WriteNull()
				continue
			}
			model := string(pairs[i*2])
			dim := 0
			if entry, err := s.reg.Resolve(model); err == nil {
				if res, _ := entry.ImageResources(); res != nil {
					dim = res.Dim
				}
			}
			if dim <= 0 {
				conn.WriteNull()
				continue
			}
			writePairs(conn, 4)
			conn.WriteBulkString("model")
			conn.WriteBulkString(model)
			conn.WriteBulkString("dtype")
			conn.WriteBulkString("FLOAT")
			conn.WriteBulkString("shape")
			conn.WriteArray(2)
			conn.WriteInt(1)
			conn.WriteInt(dim)
			conn.WriteBulkString("values")
			writeValuesArray(conn, [][]byte{r}, dim)
		}
		for i := n; i < total; i++ {
			conn.WriteNull()
		}
		return
	}
	for _, r := range results {
		if r == nil {
			conn.WriteNull()
		} else {
			conn.WriteBulk(r)
		}
	}
	for i := n; i < total; i++ {
		conn.WriteNull()
	}
}
