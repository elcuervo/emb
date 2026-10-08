package telemetry

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"sync"
	"sync/atomic"
	"time"
)

// Sender POSTs one encoded OTLP payload. The default sender speaks OTLP/HTTP;
// tests inject a fake.
type Sender interface {
	Send(ctx context.Context, payload []byte, headers map[string]string) error
}

// Exporter periodically snapshots the server and POSTs OTLP metrics. One
// export runs at a time; a tick that arrives while the previous send is still
// in flight is skipped rather than queued.
type Exporter struct {
	cfg      Config
	resource Resource
	snapshot func() Snapshot
	sender   Sender
	now      func() time.Time
	logf     func(format string, args ...any)

	mu      sync.Mutex
	prev    *Snapshot
	prevAt  time.Time
	failing bool

	started  atomic.Bool
	inFlight atomic.Bool
	stop     chan struct{}
	done     chan struct{}
	stopOnce sync.Once
}

// Option configures an Exporter.
type Option func(*Exporter)

// WithSender replaces the HTTP transport (tests).
func WithSender(s Sender) Option {
	return func(e *Exporter) { e.sender = s }
}

// WithLogger replaces the failure logger (tests).
func WithLogger(f func(format string, args ...any)) Option {
	return func(e *Exporter) { e.logf = f }
}

// New returns an Exporter for cfg. It is inert until Start is called, and
// Start does nothing when the configuration is disabled or has no endpoint.
func New(cfg Config, snapshot func() Snapshot, opts ...Option) *Exporter {
	e := &Exporter{
		cfg:      cfg,
		resource: defaultResource(cfg),
		snapshot: snapshot,
		now:      time.Now,
		logf:     log.Printf,
		stop:     make(chan struct{}),
		done:     make(chan struct{}),
	}
	for _, o := range opts {
		o(e)
	}
	if e.sender == nil {
		e.sender = &httpSender{
			client:   &http.Client{Timeout: sendTimeout},
			endpoint: metricsURL(cfg.Endpoint),
		}
	}
	return e
}

// Start launches the export loop. It is a no-op when disabled or already
// started, so callers can call it unconditionally.
func (e *Exporter) Start(context.Context) {
	if !e.cfg.Enabled() {
		return
	}
	if !e.started.CompareAndSwap(false, true) {
		return
	}
	//nolint:gosec // the export loop is server-scoped; its ticks must outlive any request context.
	go e.loop()
}

func (e *Exporter) loop() {
	defer close(e.done)
	t := time.NewTicker(e.cfg.IntervalOrDefault())
	defer t.Stop()
	for {
		select {
		case <-t.C:
			e.tick(context.Background())
		case <-e.stop:
			return
		}
	}
}

// tick runs one export unless the previous one is still in flight.
func (e *Exporter) tick(ctx context.Context) {
	if !e.inFlight.CompareAndSwap(false, true) {
		return
	}
	defer e.inFlight.Store(false)
	e.export(ctx)
}

func (e *Exporter) export(ctx context.Context) {
	now := e.now()
	cur := e.snapshot()

	e.mu.Lock()
	prev := e.prev
	prevAt := e.prevAt
	e.prev = &cur
	e.prevAt = now
	e.mu.Unlock()

	var startNano uint64
	if prev != nil {
		startNano = uint64(prevAt.UnixNano())
	}
	payload, err := encodeRequest(cur, prev, startNano, uint64(now.UnixNano()), e.resource)
	if err != nil {
		e.report(err)
		return
	}

	sendCtx, cancel := context.WithTimeout(ctx, sendTimeout)
	defer cancel()
	if err := e.sender.Send(sendCtx, payload, e.cfg.Headers); err != nil {
		e.report(err)
		return
	}
	e.recover()
}

// report logs the first failure of a failing streak; a later healthy export
// logs the recovery. Repeated failures stay quiet.
func (e *Exporter) report(err error) {
	e.mu.Lock()
	first := !e.failing
	e.failing = true
	e.mu.Unlock()
	if first {
		e.logf("telemetry export failed: %v", err)
	}
}

func (e *Exporter) recover() {
	e.mu.Lock()
	was := e.failing
	e.failing = false
	e.mu.Unlock()
	if was {
		e.logf("telemetry export recovered")
	}
}

// Shutdown stops the loop and performs one final best-effort export bounded by
// ctx. A failed flush is logged and ignored; it never changes the exit status.
func (e *Exporter) Shutdown(ctx context.Context) {
	e.stopOnce.Do(func() { close(e.stop) })
	if e.started.Load() {
		select {
		case <-e.done:
		case <-ctx.Done():
		}
	}
	if !e.cfg.Enabled() {
		return
	}
	e.export(ctx)
}

// Close stops the loop without a final export.
func (e *Exporter) Close() {
	e.stopOnce.Do(func() { close(e.stop) })
	if e.started.Load() {
		<-e.done
	}
}

// httpSender is the default OTLP/HTTP transport.
type httpSender struct {
	client   *http.Client
	endpoint string
}

func (h *httpSender) Send(ctx context.Context, payload []byte, headers map[string]string) error {
	if h.endpoint == "" {
		return errors.New("telemetry: no endpoint configured")
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, h.endpoint, bytes.NewReader(payload))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", "emb-telemetry")
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	resp, err := h.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 4096))
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("telemetry endpoint returned %s", resp.Status)
	}
	return nil
}

// metricsURL resolves the OTLP/HTTP metrics path: a bare endpoint gets the
// standard "/v1/metrics" appended; an endpoint that already carries a path
// (Datadog's direct intake, a custom collector route) is used as-is.
func metricsURL(endpoint string) string {
	if endpoint == "" {
		return ""
	}
	u, err := url.Parse(endpoint)
	if err != nil {
		return endpoint
	}
	if u.Path == "" || u.Path == "/" {
		u.Path = "/v1/metrics"
	}
	return u.String()
}
