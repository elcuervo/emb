package server

import (
	"image/color"
	"net"
	"reflect"
	"strconv"
	"sync"
	"testing"
	"time"

	"github.com/elcuervo/emb/internal/config"
)

// The tests in this file are the regression guards for the pre-0.4.0 hardening
// change: they fail against the pre-fix code and are cheap to run without a
// model. TestConfigCapsRace is meaningful under `go test -race`.

// TestConfigGetCacheEchoesSet covers the stale-getter bug: CONFIG SET cache
// resized the budget but CONFIG GET kept reporting the boot value.
func TestConfigGetCacheEchoesSet(t *testing.T) {
	addr, srv := serveTestWithCacheOptions(t, "64mb")
	if tok := redisCmd(t, addr, "CONFIG", "SET", "cache", "128mb"); tok.kind != "status" || tok.val != "OK" {
		t.Fatalf("CONFIG SET cache = %#v", tok)
	}
	elems := arrayOf(t, redisCmd(t, addr, "CONFIG", "GET", "cache"))
	if len(elems) != 2 || elems[0].val != "cache" || elems[1].val != "128mb" {
		t.Fatalf("CONFIG GET cache after SET = %#v, want [cache 128mb]", elems)
	}
	if got := srv.cache.Stats().MaxBytes; got != 128_000_000 {
		t.Fatalf("effective cache budget = %d, want 128000000", got)
	}
}

// TestCacheSetStoresOwnedCopy covers the sub-slice aliasing bug: the cache used
// to retain the caller's slice, so mutating the source changed the cached value.
func TestCacheSetStoresOwnedCopy(t *testing.T) {
	c := NewCache(1 << 20)
	src := []byte{1, 2, 3, 4}
	c.Set("txt:m:x", src)
	src[0] = 99 // mutate the caller's slice after storing

	got, ok := c.Get("txt:m:x")
	if !ok {
		t.Fatal("cache miss after Set")
	}
	if len(got) != 4 || got[0] != 1 || got[3] != 4 {
		t.Fatalf("cached value aliased the source: %v", got)
	}
}

// TestCacheSetRowDoesNotAliasBatchBuffer is the retention shape of the same bug:
// a row sliced from a batch-wide buffer must not keep the batch alive or be
// changed when the batch is reused.
func TestCacheSetRowDoesNotAliasBatchBuffer(t *testing.T) {
	c := NewCache(1 << 20)
	batch := make([]byte, 5*4)
	row := batch[4:8]
	row[0] = 7
	c.Set("txt:m:row", row)

	for i := range batch {
		batch[i] = 255 // reuse/overwrite the whole batch buffer
	}

	got, ok := c.Get("txt:m:row")
	if !ok {
		t.Fatal("cache miss after Set")
	}
	if len(got) != 4 || got[0] != 7 {
		t.Fatalf("cached row reflects the reused batch buffer: %v", got)
	}
}

// drainReply reads one complete RESP value without touching *testing.T, so it
// is safe to call from the helper goroutines of TestConfigCapsRace.
func drainReply(c net.Conn) {
	buf := make([]byte, 0, 4096)
	tmp := make([]byte, 4096)
	for {
		n, err := c.Read(tmp)
		if err != nil {
			return
		}
		buf = append(buf, tmp[:n]...)
		if consumed, ok := respValueLen(buf); ok && consumed == len(buf) {
			return
		}
	}
}

// TestConfigCapsRace exercises CONFIG SET of the runtime caps concurrently with
// EMB requests. It asserts nothing on its own; under `go test -race` it fails if
// the cap reads and writes are unsynchronized (the pre-fix behavior).
func TestConfigCapsRace(t *testing.T) {
	addr := serveTestWithCache(t, "auto")

	stop := make(chan struct{})
	var wg sync.WaitGroup

	for g := 0; g < 2; g++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			c, err := net.Dial("tcp", addr)
			if err != nil {
				return
			}
			defer c.Close()
			for i := 0; ; i++ {
				select {
				case <-stop:
					return
				default:
				}
				// Cycle every runtime-settable cap so the race covers each setter.
				settings := [][2]string{
					{"max_texts", strconv.Itoa(100 + i%50)},
					{"max_pairs", strconv.Itoa(100 + i%50)},
					{"max_images", strconv.Itoa(100 + i%50)},
					{"max_image_bytes", strconv.Itoa(1_000_000 + i)},
					{"max_image_pixels", strconv.Itoa(1_000_000 + i)},
					{"max_command_bytes", strconv.Itoa(60_000_000 + i)},
				}
				s := settings[i%len(settings)]
				_, _ = c.Write(respCommand("CONFIG", "SET", s[0], s[1]))
				drainReply(c)
			}
		}()
	}

	for g := 0; g < 3; g++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			c, err := net.Dial("tcp", addr)
			if err != nil {
				return
			}
			defer c.Close()
			requests := [][]string{
				{"EMB", "test", "a", "b", "c"},
				{"EMB.MULTI", "test", "a", "test", "b"},
				{"EMB.IMGMULTI", "test", "a", "test", "b"},
			}
			for i := 0; ; i++ {
				select {
				case <-stop:
					return
				default:
				}
				_, _ = c.Write(respCommand(requests[i%len(requests)]...))
				drainReply(c)
			}
		}()
	}

	time.Sleep(400 * time.Millisecond)
	close(stop)
	wg.Wait()
}

// TestScriptReplyCacheScopedByArity reproduces the collision between a
// single-text and a multi-text evaluation of the same script on the same text.
// Before the fix, the two-text evaluation overwrote the single-text entry and
// the following single-text call replayed the wrong shape/value.
func TestScriptReplyCacheScopedByArity(t *testing.T) {
	addr, _ := serveScriptModel(t, config.ModelConfig{}, "auto")
	const script = `local out = {} for i=1,#KEYS do out[i] = #KEYS end return out`

	one := redisCmd(t, addr, "EMB.EVAL", "test", script, "1", "x")
	_ = redisCmd(t, addr, "EMB.EVAL", "test", script, "2", "x", "y")
	again := redisCmd(t, addr, "EMB.EVAL", "test", script, "1", "x")

	if !reflect.DeepEqual(one, again) {
		t.Fatalf("single-text reply changed after a multi-text evaluation:\n first %#v\n again %#v", one, again)
	}
	if elems := arrayOf(t, again); len(elems) != 1 || elems[0].val != 1 {
		t.Fatalf("single-text reply = %#v, want a one-element array carrying 1", again)
	}
}

// TestScriptReplyCacheBypassesDuplicateKeys covers the residual cache hazard:
// one per-text key cannot carry two element replies, so an evaluation with a
// repeated text must not be cached (else the second identical request replays
// the last element for both positions).
func TestScriptReplyCacheBypassesDuplicateKeys(t *testing.T) {
	addr, _ := serveScriptModel(t, config.ModelConfig{}, "auto")
	// Position-dependent: element i = i. KEYS=[x, x] must return [1, 2] every time.
	const script = `local out = {} for i=1,#KEYS do out[i] = i end return out`

	first := redisCmd(t, addr, "EMB.EVAL", "test", script, "2", "x", "x")
	again := redisCmd(t, addr, "EMB.EVAL", "test", script, "2", "x", "x")

	if !reflect.DeepEqual(first, again) {
		t.Fatalf("duplicate-KEYS reply changed across identical requests: %#v vs %#v", first, again)
	}
	if elems := arrayOf(t, again); len(elems) != 2 || elems[0].val != 1 || elems[1].val != 2 {
		t.Fatalf("duplicate-KEYS reply = %#v, want [1, 2]", again)
	}
}

// TestImageCapsRace is the image-path counterpart of TestConfigCapsRace: it
// drives EMB.IMG/EMB.IMGMULTI (which read max_image_bytes/max_image_pixels in
// embedImages) while those caps are changed. Meaningful under `-race`.
func TestImageCapsRace(t *testing.T) {
	addr, _, _ := serveImage(t, "auto")
	png := solidImagePNG(t, color.White)

	stop := make(chan struct{})
	var wg sync.WaitGroup

	wg.Add(1)
	go func() {
		defer wg.Done()
		c, err := net.Dial("tcp", addr)
		if err != nil {
			return
		}
		defer c.Close()
		for i := 0; ; i++ {
			select {
			case <-stop:
				return
			default:
			}
			settings := [][2]string{
				{"max_image_bytes", strconv.Itoa(len(png) + i%8)},
				{"max_image_pixels", strconv.Itoa(16 + i%8)},
				{"max_images", strconv.Itoa(4 + i%4)},
			}
			s := settings[i%len(settings)]
			_, _ = c.Write(respCommand("CONFIG", "SET", s[0], s[1]))
			drainReply(c)
		}
	}()

	for g := 0; g < 2; g++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			c, err := net.Dial("tcp", addr)
			if err != nil {
				return
			}
			defer c.Close()
			for i := 0; ; i++ {
				select {
				case <-stop:
					return
				default:
				}
				if i%2 == 0 {
					_, _ = c.Write(respCommand("EMB.IMG", "imgA", png))
				} else {
					_, _ = c.Write(respCommand("EMB.IMGMULTI", "imgA", png, "imgB", png))
				}
				drainReply(c)
			}
		}()
	}

	time.Sleep(400 * time.Millisecond)
	close(stop)
	wg.Wait()
}
