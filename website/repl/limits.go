package main

import (
	"math"
	"strconv"
	"strings"
	"sync"
	"time"
)

// Limits bounds what a single visitor and a flood can spend. The zero value is
// not usable; DefaultLimits supplies the shipping values and every field can be
// overridden per run.
type Limits struct {
	PerClientRate  float64       // requests/second per client
	PerClientBurst float64       // burst per client
	GlobalRate     float64       // requests/second across all clients
	GlobalBurst    float64       // global burst
	MaxConcurrent  int           // in-flight requests admitted at once
	MaxArgs        int           // argv length cap
	MaxTextBytes   int           // bytes per text
	MaxTexts       int           // texts per request
	MaxImages      int           // base64 image arguments per request
	MaxImageBytes  int           // decoded bytes per image argument
	WorkWindow     time.Duration // rolling window for the work ceiling
	WorkCeiling    int           // work units (texts) per window
	Timeout        time.Duration // per-command upstream deadline
}

// DefaultLimits is sized for one shared vCPU behind a bridge: generous enough
// that a person typing commands never notices, bounded enough that a flood
// cannot bill without limit. The absolute numbers are tuning values; the shape
// is what the specs require.
func DefaultLimits() Limits {
	return Limits{
		PerClientRate:  2,
		PerClientBurst: 10,
		GlobalRate:     40,
		GlobalBurst:    80,
		MaxConcurrent:  4,
		MaxArgs:        64,
		MaxTextBytes:   2 << 10,
		MaxTexts:       8,
		// Images are far larger than texts and far more expensive to decode, so
		// the sandbox admits two per request at a quarter of a megabyte each.
		// The server's own decode caps (`max_image_bytes`, `max_image_pixels`)
		// bound the pixel count; this bounds the wire.
		MaxImages:     2,
		MaxImageBytes: 256 << 10,
		WorkWindow:    60 * time.Second,
		WorkCeiling:   20_000,
		Timeout:       30 * time.Second,
	}
}

// boundError is a refusal produced by a spend bound rather than by the
// allowlist. Its code is always "capacity" so the client can distinguish it
// from a transport, readiness, or timeout failure.
type boundError struct {
	text string
}

func (e *boundError) Error() string { return e.text }
func (e *boundError) Code() string  { return codeCapacity }

// bucket is a token bucket refilled at rate tokens/second up to burst.
type bucket struct {
	rate   float64
	burst  float64
	tokens float64
	last   time.Time
	init   bool
}

// allow consumes one token, refilling by elapsed time first. It reports false
// while the bucket is empty.
func (b *bucket) allow(now time.Time) bool {
	if !b.init {
		b.tokens = b.burst
		b.last = now
		b.init = true
	}
	b.tokens = math.Min(b.burst, b.tokens+now.Sub(b.last).Seconds()*b.rate)
	b.last = now
	if b.tokens < 1 {
		return false
	}
	b.tokens--
	return true
}

// rolling is a bounded-window counter: `buckets` slices of fixed duration,
// summed to give the work used in the last window.
type rolling struct {
	step    time.Duration
	buckets []int
	idx     int
	total   int
	last    time.Time
}

func newRolling(window time.Duration, slices int) *rolling {
	if slices < 1 {
		slices = 1
	}
	return &rolling{step: window / time.Duration(slices), buckets: make([]int, slices)}
}

// advance clears the slices that have elapsed since the last observation.
func (r *rolling) advance(now time.Time) {
	if r.last.IsZero() {
		r.last = now
		return
	}
	steps := int(now.Sub(r.last) / r.step)
	if steps <= 0 {
		return
	}
	if steps >= len(r.buckets) {
		for i := range r.buckets {
			r.buckets[i] = 0
		}
		r.total = 0
		r.idx = 0
		r.last = now
		return
	}
	for i := 0; i < steps; i++ {
		r.idx = (r.idx + 1) % len(r.buckets)
		r.total -= r.buckets[r.idx]
		r.buckets[r.idx] = 0
	}
	r.last = r.last.Add(time.Duration(steps) * r.step)
}

func (r *rolling) used(now time.Time) int {
	r.advance(now)
	return r.total
}

func (r *rolling) add(now time.Time, n int) {
	r.advance(now)
	r.buckets[r.idx] += n
	r.total += n
}

// limiter holds the shared state all clients contend for. One machine means
// this state is genuinely global.
//
// ponytail: in-memory only, so a restart resets the ceiling — accepted for a
// sandbox. Back it with a store if the ceiling must survive a deploy.
type limiter struct {
	limits  Limits
	mu      sync.Mutex
	global  bucket
	clients map[string]*bucket
	work    *rolling
	slots   chan struct{}
}

func newLimiter(l Limits) *limiter {
	return &limiter{
		limits:  l,
		global:  bucket{rate: l.GlobalRate, burst: l.GlobalBurst},
		clients: make(map[string]*bucket),
		work:    newRolling(l.WorkWindow, 60),
		slots:   make(chan struct{}, l.MaxConcurrent),
	}
}

// clientIdle is how long a client's bucket is kept after its last request. A
// bucket idle this long has refilled to burst, so dropping it and recreating it
// later make the same decision; the map then follows concurrent clients rather
// than every address ever seen.
const clientIdle = 10 * time.Minute

// clientSweepThreshold bounds the sweep's per-request cost: idle buckets are
// only pruned once the map has grown past this many clients.
const clientSweepThreshold = 4096

// rate applies the per-client and global token buckets. The per-client refusal
// names the cooling period, so a visitor understands the wait rather than
// reading it as a failure of the command.
func (l *limiter) rate(client string, now time.Time) error {
	l.mu.Lock()
	defer l.mu.Unlock()
	if len(l.clients) > clientSweepThreshold {
		for k, b := range l.clients {
			if now.Sub(b.last) >= clientIdle {
				delete(l.clients, k)
			}
		}
	}
	b, ok := l.clients[client]
	if !ok {
		b = &bucket{rate: l.limits.PerClientRate, burst: l.limits.PerClientBurst}
		l.clients[client] = b
	}
	if !b.allow(now) {
		return &boundError{text: "rate limit exceeded for this client; wait a moment and retry"}
	}
	if !l.global.allow(now) {
		return &boundError{text: "the sandbox is serving at its global rate limit; retry shortly"}
	}
	return nil
}

// charge records work against the rolling ceiling, refusing when the window
// would exceed it. This bound holds however many clients arrive, so a flood
// cannot consume unbounded CPU.
func (l *limiter) charge(work int, now time.Time) error {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.limits.WorkCeiling > 0 && l.work.used(now)+work > l.limits.WorkCeiling {
		return &boundError{text: "the sandbox is at capacity for the current window; retry shortly"}
	}
	l.work.add(now, work)
	return nil
}

// acquireSlot takes a concurrency slot without blocking, so overload is
// answered immediately instead of queued.
func (l *limiter) acquireSlot() (func(), error) {
	if l.limits.MaxConcurrent <= 0 {
		return func() {}, nil
	}
	select {
	case l.slots <- struct{}{}:
		return func() { <-l.slots }, nil
	default:
		return nil, &boundError{text: "the sandbox is at capacity; retry shortly"}
	}
}

// chargeShape applies the per-request caps and returns the request's work cost
// in texts. Each cap states which bound was hit and by how much. `bin` names the
// arguments that carry decoded image bytes, which are bounded by MaxImageBytes
// instead of the text cap.
func (l *limiter) chargeShape(args []string, bin map[int]bool) (int, error) {
	if l.limits.MaxArgs > 0 && len(args) > l.limits.MaxArgs {
		return 0, &boundError{text: "request carries " + strconv.Itoa(len(args)) + " arguments, above the sandbox cap of " + strconv.Itoa(l.limits.MaxArgs)}
	}
	work := textsIn(args)
	if l.limits.MaxTexts > 0 && work > l.limits.MaxTexts {
		return 0, &boundError{text: "request carries " + strconv.Itoa(work) + " texts, above the sandbox cap of " + strconv.Itoa(l.limits.MaxTexts)}
	}
	if l.limits.MaxTextBytes > 0 {
		for i, a := range args {
			if bin[i] {
				continue
			}
			if len(a) > l.limits.MaxTextBytes {
				return 0, &boundError{text: "one argument is " + strconv.Itoa(len(a)) + " bytes, above the sandbox text cap of " + strconv.Itoa(l.limits.MaxTextBytes)}
			}
		}
	}
	return work, nil
}

// textsIn counts the text arguments a command carries — the cost proxy the
// work ceiling charges. Control commands cost one unit.
func textsIn(args []string) int {
	if len(args) == 0 {
		return 1
	}
	switch strings.ToLower(args[0]) {
	case "emb":
		start := 2
		if len(args) > 2 && isFormat(args[2]) {
			start = 3
		}
		if n := len(args) - start; n > 0 {
			return n
		}
	case "emb.multi":
		start := 1
		if len(args) > 1 && isFormat(args[1]) {
			start = 2
		}
		if n := (len(args) - start) / 2; n > 0 {
			return n
		}
	case "emb.evsha":
		if len(args) > 3 {
			if n, err := strconv.Atoi(args[3]); err == nil && n > 0 {
				return n
			}
		}
	}
	return 1
}
