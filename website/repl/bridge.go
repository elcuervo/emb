// Command repl is the sandbox bridge: the only public surface in front of an
// emb server that listens on loopback. It accepts a fixed, read-only command
// surface over one HTTP endpoint, forwards it to emb on a single negotiated
// connection, and returns structured replies that preserve every RESP kind.
package main

import (
	"embed"
	"encoding/json"
	"errors"
	"io"
	"log"
	"net"
	"net/http"
	"net/url"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/elcuervo/emb/internal/resp"
)

// terminalFS holds the one client module, the standalone terminal page, and
// the live dashboard page. The module is served to both terminal surfaces, so
// a reply cannot be rendered two ways.
//
//go:embed terminal.js index.html stats.html
var terminalFS embed.FS

// execRequest is the one command path: the argv a client would send to emb,
// and the protocol version to carry it on. There is no per-visitor state, so
// nothing is lost between requests.
type execRequest struct {
	Args  []string `json:"args"`
	Proto int      `json:"proto"`
}

// Bridge is the one public surface. It holds the single upstream connection
// (RESP is not multiplexed, so one connection carries one in-flight command),
// the allowlist's preset manifest, and the spend bounds.
type Bridge struct {
	upstream string
	presets  presets
	limits   Limits
	origins  map[string]bool
	client   *resp.Client
	lim      *limiter
	mu       sync.Mutex // serializes the upstream connection

	// trustProxy reads X-Forwarded-For for the per-client rate-limit key.
	// Fly-Client-IP is always trusted (the Fly proxy sets it and is the only
	// path to the app); this gates the header any other client can forge.
	trustProxy bool

	now    func() time.Time
	ready  atomic.Bool
	everOK atomic.Bool

	stats *statsService
}

// bridgeError is a legible failure the bridge itself produces, carrying the
// code that distinguishes it from a command's own error reply.
type bridgeError struct {
	code string
	text string
}

func (e *bridgeError) Error() string { return e.text }

// NewBridge wires a bridge to one upstream emb address. presets is the digest
// manifest the server preloaded; origins is the CORS allowlist (the site and
// its preview aliases); trustProxy makes X-Forwarded-For the client key.
func NewBridge(upstream string, p presets, limits Limits, origins []string, trustProxy bool) *Bridge {
	allowed := make(map[string]bool, len(origins))
	for _, o := range origins {
		if o = strings.TrimSpace(o); o != "" {
			allowed[o] = true
		}
	}
	return &Bridge{
		upstream:   upstream,
		presets:    p,
		limits:     limits,
		origins:    allowed,
		client:     resp.NewClient(upstream, "", false),
		lim:        newLimiter(limits),
		now:        time.Now,
		stats:      newStatsService(),
		trustProxy: trustProxy,
	}
}

// Handler returns the HTTP surface: the health and readiness probes, the one
// command path, the standalone terminal, and the client module both surfaces
// load.
func (b *Bridge) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/api/health", b.handleHealth)
	mux.HandleFunc("/api/ready", b.handleReady)
	mux.HandleFunc("/api/presets", b.handlePresets)
	mux.HandleFunc("/api/exec", b.handleExec)
	mux.HandleFunc("/api/stats", b.handleStatsStream)
	mux.HandleFunc("/stats", b.handleStats)
	mux.HandleFunc("/terminal.js", b.handleTerminalJS)
	mux.HandleFunc("/", b.handleTerminal)
	return mux
}

func (b *Bridge) handleTerminalJS(w http.ResponseWriter, r *http.Request) {
	b.serveAsset(w, "terminal.js", "text/javascript; charset=utf-8", "public, max-age=300")
}

func (b *Bridge) handleTerminal(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" {
		http.NotFound(w, r)
		return
	}
	// The page is never cached: it names the module, and a heuristically
	// cached page is how a stale console keeps running after a deploy.
	b.serveAsset(w, "index.html", "text/html; charset=utf-8", "no-cache")
}

// serveAsset writes one embedded file.
func (b *Bridge) serveAsset(w http.ResponseWriter, name, contentType, cacheControl string) {
	data, err := terminalFS.ReadFile(name)
	if err != nil {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}
	w.Header().Set("Content-Type", contentType)
	w.Header().Set("Cache-Control", cacheControl)
	w.Header().Set("X-Content-Type-Options", "nosniff")
	_, _ = w.Write(data)
}

func (b *Bridge) handleHealth(w http.ResponseWriter, r *http.Request) {
	if !b.allowCORS(w, r) {
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (b *Bridge) handleReady(w http.ResponseWriter, r *http.Request) {
	if !b.allowCORS(w, r) {
		return
	}
	ok := b.probe()
	state := "starting"
	switch {
	case ok:
		state = "ready"
	case b.everOK.Load():
		state = "offline"
	}
	writeJSON(w, http.StatusOK, map[string]any{"ready": ok, "state": state, "upstream": b.upstream})
}

// handlePresets publishes the digests the sandbox will accept EMB.EVSHA for, so
// a client can call a preset without sending Lua. It is derived from the same
// config the server preloaded, never transcribed.
func (b *Bridge) handlePresets(w http.ResponseWriter, r *http.Request) {
	if !b.allowCORS(w, r) {
		return
	}
	list := make([]presetInfo, 0)
	for model, digests := range b.presets {
		for sha, name := range digests {
			list = append(list, presetInfo{Model: model, Name: name, SHA: sha})
		}
	}
	sort.Slice(list, func(i, j int) bool {
		if list[i].Model != list[j].Model {
			return list[i].Model < list[j].Model
		}
		return list[i].Name < list[j].Name
	})
	writeJSON(w, http.StatusOK, map[string]any{"presets": list})
}

type presetInfo struct {
	Model string `json:"model"`
	Name  string `json:"name"`
	SHA   string `json:"sha"`
}

// handleExec is the one command path: validate the whole argv, apply the spend
// bounds, run the command on the negotiated connection, and return one
// discriminated envelope.
func (b *Bridge) handleExec(w http.ResponseWriter, r *http.Request) {
	if !b.allowCORS(w, r) {
		return
	}
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, errorEnvelope(codeRefused, "commands are sent with POST"))
		return
	}
	var req execRequest
	if err := json.NewDecoder(io.LimitReader(r.Body, 1<<20)).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, errorEnvelope(codeRefused, "malformed request: "+err.Error()))
		return
	}
	env, status := b.Execute(req.Args, req.Proto, b.clientKey(r))
	writeJSON(w, status, env)
}

// Execute runs one command through the whole contract and returns the reply
// envelope plus the HTTP status that matches it. It is the seam the HTTP
// handler and the tests share.
func (b *Bridge) Execute(args []string, proto int, client string) (Envelope, int) {
	work, err := b.lim.chargeShape(args)
	if err != nil {
		return capacityEnvelope(err), statusFor(codeCapacity)
	}
	if err := validateArgv(args, b.presets); err != nil {
		return errorEnvelope(codeRefused, err.Error()), http.StatusOK
	}
	if proto != 3 {
		proto = 2
	}
	// Charging the work happens before the work does, so the ceiling bounds
	// total CPU rather than merely reporting it afterwards.
	if err := b.lim.rate(client, b.now()); err != nil {
		return capacityEnvelope(err), statusFor(codeCapacity)
	}
	if err := b.lim.charge(work, b.now()); err != nil {
		return capacityEnvelope(err), statusFor(codeCapacity)
	}
	release, err := b.lim.acquireSlot()
	if err != nil {
		return capacityEnvelope(err), statusFor(codeCapacity)
	}
	defer release()

	rep, elapsed, err := b.roundTrip(args, proto)
	if err != nil {
		var be *bridgeError
		if errors.As(err, &be) {
			return errorEnvelope(be.code, be.text), statusFor(be.code)
		}
		return errorEnvelope(codeUnavailable, err.Error()), statusFor(codeUnavailable)
	}
	env := toEnvelope(rep, isBlobEmbedding(args))
	env.ElapsedUs = elapsed.Microseconds()
	return env, http.StatusOK
}

// roundTrip runs one command on the serialized connection, negotiating the
// requested version on that same connection so the reply really is the
// server's encoding. It also returns the server's answer time, measured here
// rather than by the client, so a distant reader sees what the server cost and
// not what the network did. A connection lost while idle is retried once:
// every command on this surface is read-only, so a retry cannot run anything
// twice that matters.
func (b *Bridge) roundTrip(args []string, proto int) (resp.Reply, time.Duration, error) {
	b.mu.Lock()
	defer b.mu.Unlock()

	rep, elapsed, err := b.send(args, proto)
	if err == nil {
		return rep, elapsed, nil
	}
	var be *bridgeError
	if errors.As(err, &be) && be.code == codeUnavailable {
		if rep, elapsed, retryErr := b.send(args, proto); retryErr == nil {
			return rep, elapsed, nil
		}
	}
	return rep, elapsed, err
}

// send performs one connection + negotiation + command round trip. The elapsed
// time it returns brackets the command itself — the write through the reply
// read, after negotiation — so the value is the server's answer time, not the
// connection's setup or the client's round trip.
func (b *Bridge) send(args []string, proto int) (resp.Reply, time.Duration, error) {
	if _, err := b.client.EnsureConn(); err != nil {
		if b.everOK.Load() {
			return resp.Reply{}, 0, &bridgeError{codeUnavailable, "the sandbox server is unavailable; the machine is being replaced"}
		}
		return resp.Reply{}, 0, &bridgeError{codeStarting, "the sandbox is starting; retry in a moment"}
	}
	b.client.SetTimeout(b.limits.Timeout)
	_ = b.client.SetDeadline(b.now().Add(b.limits.Timeout))
	if err := b.client.Hello(proto); err != nil {
		_ = b.client.Close()
		return resp.Reply{}, 0, &bridgeError{codeUnavailable, "could not negotiate the protocol version: " + err.Error()}
	}
	_ = b.client.SetDeadline(b.now().Add(b.limits.Timeout))
	started := b.now()
	if err := b.client.WriteArgv(args...); err != nil {
		_ = b.client.Close()
		return resp.Reply{}, 0, &bridgeError{codeUnavailable, "lost the connection to the sandbox server"}
	}
	if err := b.client.Flush(); err != nil {
		return resp.Reply{}, 0, &bridgeError{codeUnavailable, "lost the connection to the sandbox server"}
	}
	rep, err := b.client.ReadReply()
	if err != nil {
		var ne net.Error
		if errors.As(err, &ne) && ne.Timeout() {
			return resp.Reply{}, 0, &bridgeError{codeTimeout, "the sandbox server did not answer before the deadline"}
		}
		return resp.Reply{}, 0, &bridgeError{codeUnavailable, "lost the connection to the sandbox server"}
	}
	b.ready.Store(true)
	b.everOK.Store(true)
	return rep, b.now().Sub(started), nil
}

// probe answers whether the upstream server is answering, without consuming a
// spend bound (it is the readiness check, not a command).
func (b *Bridge) probe() bool {
	if !b.mu.TryLock() {
		// A command is in flight, so the connection is demonstrably alive.
		return b.everOK.Load()
	}
	defer b.mu.Unlock()
	if _, err := b.client.EnsureConn(); err != nil {
		b.ready.Store(false)
		return false
	}
	b.client.SetTimeout(2 * time.Second)
	_ = b.client.SetDeadline(time.Now().Add(2 * time.Second))
	if err := b.client.WriteArgv("PING"); err != nil {
		_ = b.client.Close()
		b.ready.Store(false)
		return false
	}
	if err := b.client.Flush(); err != nil {
		b.ready.Store(false)
		return false
	}
	if _, err := b.client.ReadReply(); err != nil {
		b.ready.Store(false)
		return false
	}
	b.ready.Store(true)
	b.everOK.Store(true)
	return true
}

// allowCORS applies the fixed origin allowlist. CORS is not a security
// boundary — the whole surface is public — it only keeps a stray page from
// driving the sandbox. The sandbox's own origin is always allowed, because the
// standalone terminal is served from here and the browser sends Origin even on
// that same-origin POST. It returns false when it has already answered the
// request (a refused origin, or a completed preflight).
func (b *Bridge) allowCORS(w http.ResponseWriter, r *http.Request) bool {
	origin := r.Header.Get("Origin")
	if origin != "" && !b.origins[origin] && !sameHostOrigin(origin, r) {
		writeJSON(w, http.StatusForbidden, errorEnvelope(codeRefused, "origin not allowed"))
		return false
	}
	if origin != "" {
		w.Header().Set("Access-Control-Allow-Origin", origin)
		w.Header().Set("Vary", "Origin")
		w.Header().Set("Access-Control-Allow-Methods", "POST, GET, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type")
		w.Header().Set("Access-Control-Max-Age", "600")
	}
	if r.Method == http.MethodOptions {
		w.WriteHeader(http.StatusNoContent)
		return false
	}
	return true
}

// sameHostOrigin reports whether origin names the same host as the request,
// ignoring the port. The local dev loop (`just website-dev`) serves the page and
// the bridge from one machine on two ports, so the page's Origin can never equal
// the bridge's own origin; this keeps that loop working at whichever address the
// browser used — `localhost`, `127.0.0.1`, or a LAN IP. A different host is still
// refused.
func sameHostOrigin(origin string, r *http.Request) bool {
	u, err := url.Parse(origin)
	if err != nil || u.Hostname() == "" {
		return false
	}
	host := r.Host
	if h, _, err := net.SplitHostPort(r.Host); err == nil {
		host = h
	}
	return strings.EqualFold(u.Hostname(), host)
}

// capacityEnvelope turns a spend-bound refusal into the "capacity" reply kind,
// which the client distinguishes from a command's own error.
func capacityEnvelope(err error) Envelope {
	var be *boundError
	if errors.As(err, &be) {
		return errorEnvelope(be.Code(), be.text)
	}
	return errorEnvelope(codeCapacity, err.Error())
}

// statusFor maps a reply to the HTTP status that carries it: a command result
// (including a refusal by the allowlist) is 200 with the envelope; the
// unavailable conditions get their own statuses.
func statusFor(code string) int {
	switch code {
	case codeStarting:
		return http.StatusServiceUnavailable
	case codeCapacity:
		return http.StatusTooManyRequests
	case codeTimeout:
		return http.StatusGatewayTimeout
	case codeUnavailable:
		return http.StatusBadGateway
	default:
		return http.StatusOK
	}
}

// clientKey identifies the caller for the per-client bucket. The Fly proxy
// sets Fly-Client-IP and is the only path to the deployed app, so it is always
// trusted. X-Forwarded-For is client-supplied, so it is read only when the
// operator declares a proxy trusted; otherwise the peer address — which a
// client cannot choose — is the key.
func (b *Bridge) clientKey(r *http.Request) string {
	if fwd := r.Header.Get("Fly-Client-IP"); fwd != "" {
		return fwd
	}
	if b.trustProxy {
		if fwd := r.Header.Get("X-Forwarded-For"); fwd != "" {
			if i := strings.IndexByte(fwd, ','); i >= 0 {
				return strings.TrimSpace(fwd[:i])
			}
			return strings.TrimSpace(fwd)
		}
	}
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}

// isBlobEmbedding reports whether a command asks for a binary embedding
// (EMB/EMB.MULTI without the VALUES keyword), whose bulk payloads are bytes
// rather than text.
func isBlobEmbedding(args []string) bool {
	if len(args) == 0 {
		return false
	}
	switch strings.ToLower(args[0]) {
	case "emb":
		return len(args) < 3 || !isFormat(args[2])
	case "emb.multi":
		return len(args) < 2 || !isFormat(args[1])
	default:
		return false
	}
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(v); err != nil {
		log.Printf("repl: writing reply: %v", err)
	}
}
