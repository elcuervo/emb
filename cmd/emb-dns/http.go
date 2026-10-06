package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"strings"

	"github.com/elcuervo/emb/internal/emoji"
)

// HTTPHandler carries what the DNS record cannot: the vocabulary name and the
// score beside each glyph, the escaped record data so a reader can see why
// `dig` prints bytes, and the service's own health, metadata, and counters.
type HTTPHandler struct {
	svc *Service
}

// NewHTTPHandler serves the readable, health, metadata, and statistics routes.
func NewHTTPHandler(svc *Service) http.Handler { return &HTTPHandler{svc: svc} }

func (h *HTTPHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if !h.cors(w, r) {
		return
	}
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		// Nothing here changes anything, so nothing else is answered.
		w.Header().Set("Allow", "GET, HEAD, OPTIONS")
		http.Error(w, "only GET is served", http.StatusMethodNotAllowed)
		return
	}
	if r.URL.Path == "/healthz" {
		h.health(w)
		return
	}
	if r.URL.Path == "/meta" {
		writeJSON(w, h.svc.Meta())
		return
	}
	if r.URL.Path == "/stats" {
		writeJSON(w, h.svc.stats.Snapshot())
		return
	}
	h.read(w, r)
}

// cors answers the preflight and, for the site's own origins, allows the read.
// Every other origin gets no allowance, so a stranger's page cannot read the
// zone's answers from a browser.
func (h *HTTPHandler) cors(w http.ResponseWriter, r *http.Request) bool {
	origin := r.Header.Get("Origin")
	if origin == "" {
		return true
	}
	w.Header().Add("Vary", "Origin")
	allowed := false
	for _, candidate := range h.svc.cfg.Origins {
		if strings.EqualFold(strings.TrimSuffix(candidate, "/"), strings.TrimSuffix(origin, "/")) {
			allowed = true
			break
		}
	}
	if !allowed {
		if r.Method == http.MethodOptions {
			http.Error(w, "origin not allowed", http.StatusForbidden)
			return false
		}
		return true
	}
	w.Header().Set("Access-Control-Allow-Origin", origin)
	w.Header().Set("Access-Control-Allow-Methods", "GET, HEAD")
	w.Header().Set("Access-Control-Allow-Headers", "Accept, Content-Type")
	if r.Method == http.MethodOptions {
		w.WriteHeader(http.StatusNoContent)
		return false
	}
	return true
}

func (h *HTTPHandler) health(w http.ResponseWriter) {
	ready := h.svc.Ready()
	status := http.StatusOK
	if !ready {
		status = http.StatusServiceUnavailable
	}
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]any{"ready": ready, "zone": h.svc.cfg.FQDN()})
}

// read answers a query name over HTTP. The name is the path (or `?q=` for
// JSON), and it is read exactly as a DNS name would be: the same labels, the
// same grammar, the same refusals.
func (h *HTTPHandler) read(w http.ResponseWriter, r *http.Request) {
	asJSON := r.URL.Query().Has("q")
	name := strings.Trim(r.URL.Query().Get("q"), ".")
	if name == "" {
		name = strings.Trim(strings.TrimPrefix(r.URL.Path, "/"), ".")
	}
	if name == "" {
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		_, _ = fmt.Fprintln(w, usage)
		return
	}
	// A dot in a name is a word break, exactly as it is on the wire.
	labels := strings.Split(name, ".")
	zone := strings.ToLower(strings.TrimSuffix(h.svc.cfg.Zone, "."))
	lower := strings.ToLower(name)
	if lower == zone {
		labels = nil
	} else if strings.HasSuffix(lower, "."+zone) {
		// Asked with the zone, the way a `dig` line spells it.
		labels = labels[:len(labels)-len(strings.Split(zone, "."))]
	}

	outcome, err := h.svc.Answer(labels, hostOf(r.RemoteAddr))
	if err != nil {
		h.refuse(w, outcome.Name, err)
		return
	}
	if asJSON {
		writeJSON(w, h.document(outcome))
		return
	}
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	_, _ = fmt.Fprintf(w, "# %s\t%d\tIN\tTXT\n", outcome.Name, h.svc.cfg.TTL)
	if outcome.Phrase != "" {
		_, _ = fmt.Fprintf(w, "%s\t%s\t%s\t\n", escapeTXT(outcome.Phrase), outcome.Phrase, "the sentence")
	}
	for _, result := range outcome.Results {
		_, _ = fmt.Fprintf(w, "%s\t%s\t%s\t%.3f\n",
			escapeTXT(result.Entry.Glyph), result.Entry.Glyph, result.Entry.Name, result.Score)
	}
}

// document is the JSON form: the same ranking, with the record data the DNS
// answer carries and the name and score it does not.
func (h *HTTPHandler) document(outcome Outcome) map[string]any {
	records := make([]map[string]any, 0, len(outcome.Results))
	for _, result := range outcome.Results {
		records = append(records, map[string]any{
			// escaped is the record data as any resolver's tooling renders it.
			"escaped": escapeTXT(result.Entry.Glyph),
			// glyph is the same bytes, as a reader sees them.
			"glyph": result.Entry.Glyph,
			"name":  result.Entry.Name,
			"slug":  result.Entry.Slug,
			"score": result.Score,
		})
	}
	document := map[string]any{
		"name":    outcome.Name,
		"type":    "TXT",
		"ttl":     h.svc.cfg.TTL,
		"model":   h.svc.cfg.Model,
		"records": records,
	}
	if outcome.Phrase != "" {
		document["sentence"] = map[string]any{
			"escaped": escapeTXT(outcome.Phrase),
			"glyphs":  outcome.Phrase,
		}
	}
	return document
}

// refuse maps the service's errors onto HTTP. A name the grammar cannot read is
// the caller's mistake; a service that is not ready or an upstream that failed
// is not.
func (h *HTTPHandler) refuse(w http.ResponseWriter, name string, err error) {
	status := http.StatusBadGateway
	switch {
	case errors.Is(err, emoji.ErrEmpty), errors.Is(err, emoji.ErrNoTerms), errors.Is(err, emoji.ErrTooLong):
		status = http.StatusBadRequest
	case errors.Is(err, ErrNotReady):
		status = http.StatusServiceUnavailable
	case errors.Is(err, ErrRateLimited):
		status = http.StatusTooManyRequests
	default:
		log.Printf("zone: http query %s failed: %v", name, err)
	}
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.WriteHeader(status)
	_, _ = fmt.Fprintf(w, "%s\t%s\n", name, err)
}

func writeJSON(w http.ResponseWriter, value any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	encoder := json.NewEncoder(w)
	encoder.SetEscapeHTML(false)
	if err := encoder.Encode(value); err != nil {
		log.Printf("zone: write json: %v", err)
	}
}
