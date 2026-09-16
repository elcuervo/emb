// The live dashboard view: one `emb-top` producer per sandbox, fanned out to
// every viewer as pre-rendered frames. The producer is a child process rather
// than an import, so the bridge never carries the TUI's rendering graph (and
// never triggers the terminal background query its package init performs).
package main

import (
	"bufio"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"os/exec"
	"strconv"
	"sync"
	"time"
)

// The view's fixed shape: a five-minute window, and a hard cap on how many
// viewers the endpoint will hold open. The frames are small, but the endpoint
// is public.
const (
	statsInterval       = time.Second
	statsWindow         = 300
	statsMaxSubscribers = 16
	statsFrameBuffer    = 1 << 20
)

// producerFrame is one newline-delimited frame from `emb-top -frames`.
type producerFrame struct {
	ANSI string `json:"ansi"`
}

// statsEvent is one server-sent event: either a rendered frame, or a status
// line when there is no frame to send.
type statsEvent struct {
	Kind string `json:"kind"`
	HTML string `json:"html,omitempty"`
	Text string `json:"text,omitempty"`
}

// statsHub is the fan-out: it keeps the newest event for a viewer that joins
// mid-stream and broadcasts every new one to the current subscribers.
type statsHub struct {
	mu     sync.Mutex
	max    int
	latest []byte
	subs   map[chan []byte]struct{}
}

func newStatsHub(max int) *statsHub {
	return &statsHub{max: max, subs: map[chan []byte]struct{}{}}
}

// publish stores ev as the latest event and hands it to every subscriber. A
// subscriber that has not drained its slot loses the older event, which is
// harmless: frames are complete, so the newest one is the whole state.
func (h *statsHub) publish(ev statsEvent) {
	payload, err := json.Marshal(ev)
	if err != nil {
		return
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	h.latest = payload
	for ch := range h.subs {
		select {
		case ch <- payload:
		default:
			select {
			case <-ch:
			default:
			}
			select {
			case ch <- payload:
			default:
			}
		}
	}
}

// subscribe admits a viewer, returning the newest event to send first. ok is
// false when the viewer bound is reached.
func (h *statsHub) subscribe() (ch chan []byte, latest []byte, cancel func(), ok bool) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if len(h.subs) >= h.max {
		return nil, nil, nil, false
	}
	ch = make(chan []byte, 1)
	h.subs[ch] = struct{}{}
	if h.latest != nil {
		latest = append([]byte(nil), h.latest...)
	}
	cancel = func() {
		h.mu.Lock()
		defer h.mu.Unlock()
		if _, present := h.subs[ch]; present {
			delete(h.subs, ch)
			close(ch)
		}
	}
	return ch, latest, cancel, true
}

// statsService supervises the frame producer and owns the hub.
type statsService struct {
	hub *statsHub
	// retryDelay is the first restart backoff; it doubles up to 30s.
	retryDelay time.Duration
}

func newStatsService() *statsService {
	return &statsService{hub: newStatsHub(statsMaxSubscribers), retryDelay: time.Second}
}

// start launches the supervisor. It returns immediately; the producer runs
// until ctx is cancelled, restarting with backoff if it exits.
func (s *statsService) start(ctx context.Context, producer, addr string, interval time.Duration, window int) {
	go s.loop(ctx, producer, addr, interval, window)
}

func (s *statsService) loop(ctx context.Context, producer, addr string, interval time.Duration, window int) {
	backoff := s.retryDelay
	if backoff <= 0 {
		backoff = time.Second
	}
	for ctx.Err() == nil {
		err := s.supervise(ctx, producer, addr, interval, window)
		if ctx.Err() != nil {
			return
		}
		text := "the live dashboard is unavailable"
		if err != nil {
			text += ": " + err.Error()
		}
		s.hub.publish(statsEvent{Kind: "status", Text: text})
		select {
		case <-ctx.Done():
			return
		case <-time.After(backoff):
		}
		if backoff < 30*time.Second {
			backoff *= 2
		}
	}
}

// supervise runs one producer to completion and returns why it ended.
func (s *statsService) supervise(ctx context.Context, producer, addr string, interval time.Duration, window int) error {
	//nolint:gosec // the producer path is operator configuration, not request data
	cmd := exec.CommandContext(ctx, producer,
		"-frames",
		"-addr", addr,
		"-interval", interval.String(),
		"-window", strconv.Itoa(window),
	)
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return err
	}
	cmd.Stderr = io.Discard
	if err := cmd.Start(); err != nil {
		return err
	}
	err = s.readFrames(stdout)
	_ = cmd.Wait()
	return err
}

// readFrames turns the producer's JSON lines into rendered events until the
// stream ends.
func (s *statsService) readFrames(r io.Reader) error {
	sc := bufio.NewScanner(r)
	sc.Buffer(make([]byte, 0, 64*1024), statsFrameBuffer)
	for sc.Scan() {
		var f producerFrame
		if err := json.Unmarshal(sc.Bytes(), &f); err != nil || f.ANSI == "" {
			continue
		}
		s.hub.publish(statsEvent{Kind: "frame", HTML: ansiHTML(f.ANSI)})
	}
	if err := sc.Err(); err != nil {
		return err
	}
	return io.EOF
}

// handleStats serves the live view page.
func (b *Bridge) handleStats(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/stats" {
		http.NotFound(w, r)
		return
	}
	b.serveAsset(w, "stats.html", "text/html; charset=utf-8", "no-cache")
}

// handleStatsStream is the read-only frame stream. It is GET-only and takes no
// input: there is no request the page can make that reaches the node.
func (b *Bridge) handleStatsStream(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeJSON(w, http.StatusMethodNotAllowed, errorEnvelope(codeRefused, "the live view is read-only"))
		return
	}
	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "streaming unsupported", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/event-stream; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("X-Accel-Buffering", "no")

	ch, latest, cancel, ok := b.stats.hub.subscribe()
	if !ok {
		w.WriteHeader(http.StatusOK)
		writeSSE(w, statsEvent{Kind: "status", Text: "the live view is at capacity; retry shortly"})
		flusher.Flush()
		return
	}
	defer cancel()

	w.WriteHeader(http.StatusOK)
	if latest != nil {
		writeSSE(w, decodeEvent(latest))
	} else {
		writeSSE(w, statsEvent{Kind: "status", Text: "the live dashboard is starting"})
	}
	flusher.Flush()

	for {
		select {
		case <-r.Context().Done():
			return
		case msg, ok := <-ch:
			if !ok {
				return
			}
			if _, err := w.Write([]byte("data: ")); err != nil {
				return
			}
			if _, err := w.Write(msg); err != nil {
				return
			}
			if _, err := w.Write([]byte("\n\n")); err != nil {
				return
			}
			flusher.Flush()
		}
	}
}

// writeSSE encodes and writes one event, used only on the non-streaming paths
// where a single message is sent before the stream starts.
func writeSSE(w io.Writer, ev statsEvent) {
	payload, err := json.Marshal(ev)
	if err != nil {
		return
	}
	_, _ = w.Write([]byte("data: "))
	_, _ = w.Write(payload)
	_, _ = w.Write([]byte("\n\n"))
}

// decodeEvent unmarshals a stored payload so a joining viewer's first event
// goes through the same encoder as every later one.
func decodeEvent(payload []byte) statsEvent {
	var ev statsEvent
	if err := json.Unmarshal(payload, &ev); err != nil {
		return statsEvent{Kind: "status", Text: "the live dashboard is starting"}
	}
	return ev
}
