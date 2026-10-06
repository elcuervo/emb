package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/elcuervo/emb/internal/emoji"
)

func newTestHandler(t *testing.T, tweak func(*Config)) (*Service, http.Handler) {
	t.Helper()
	service, _ := newTestService(t, tweak)
	return service, NewHTTPHandler(service)
}

func get(t *testing.T, handler http.Handler, path string, headers map[string]string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, path, nil)
	for name, value := range headers {
		req.Header.Set(name, value)
	}
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)
	return w
}

func TestTextRouteCarriesWhatTheRecordOmits(t *testing.T) {
	_, handler := newTestHandler(t, nil)
	w := get(t, handler, "/shark.dns.emb.is", nil)
	if w.Code != http.StatusOK {
		t.Fatalf("status %d: %s", w.Code, w.Body.String())
	}
	body := w.Body.String()
	if !strings.Contains(w.Header().Get("Content-Type"), "text/plain") {
		t.Fatalf("content type is %q", w.Header().Get("Content-Type"))
	}
	if !strings.Contains(body, "# shark.dns.emb.is.") {
		t.Fatalf("the header line does not name the query: %q", body)
	}
	lines := strings.Split(strings.TrimSpace(body), "\n")
	if len(lines) != 4 {
		t.Fatalf("got %d lines, want a header and three results: %q", len(lines), body)
	}
	first := lines[1]
	for _, want := range []string{`"\240\159\166\136"`, "🦈", "shark", "1.000"} {
		if !strings.Contains(first, want) {
			t.Fatalf("the first result line %q does not carry %q", first, want)
		}
	}
}

func TestTheSentenceRidesBothReadRoutes(t *testing.T) {
	_, handler := newTestHandler(t, nil)

	body := get(t, handler, "/my.mom.is.in.the.hospital", nil).Body.String()
	lines := strings.Split(strings.TrimSpace(body), "\n")
	if len(lines) != 5 || !strings.Contains(lines[1], "the sentence") {
		t.Fatalf("the text route does not carry the sentence first: %q", body)
	}
	fields := strings.Split(lines[1], "\t")
	if decoded := unescapeBytes(t, strings.Trim(fields[0], `"`)); decoded != fields[1] {
		t.Fatalf("the escaped sentence %q decoded to %q, want %q", fields[0], decoded, fields[1])
	}
	if want := glyphsOf(lines[2:]); fields[1] != want {
		t.Fatalf("the sentence is %q, want the ranked glyphs joined (%q)", fields[1], want)
	}

	var document struct {
		Sentence struct {
			Escaped string `json:"escaped"`
			Glyphs  string `json:"glyphs"`
		} `json:"sentence"`
		Records []struct {
			Glyph string `json:"glyph"`
		} `json:"records"`
	}
	if err := json.Unmarshal(get(t, handler, "/?q=my.mom.is.in.the.hospital", nil).Body.Bytes(), &document); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if document.Sentence.Glyphs == "" {
		t.Fatal("the JSON route carries no sentence")
	}
	if decoded := unescapeBytes(t, strings.Trim(document.Sentence.Escaped, `"`)); decoded != document.Sentence.Glyphs {
		t.Fatalf("the escaped sentence %q decoded to %q, want %q", document.Sentence.Escaped, decoded, document.Sentence.Glyphs)
	}
	if len(document.Records) != 3 {
		t.Fatalf("the sentence replaced the ranked records: %d of them", len(document.Records))
	}
}

// glyphsOf reads the glyph column of the ranked lines the text route prints.
func glyphsOf(lines []string) string {
	var joined strings.Builder
	for _, line := range lines {
		if fields := strings.Split(line, "\t"); len(fields) > 1 {
			joined.WriteString(fields[1])
		}
	}
	return joined.String()
}

func TestAWordCarriesNoSentence(t *testing.T) {
	_, handler := newTestHandler(t, nil)
	body := get(t, handler, "/shark.dns.emb.is", nil).Body.String()
	if strings.Contains(body, "the sentence") {
		t.Fatalf("a single word carried a sentence: %q", body)
	}
	if strings.Contains(get(t, handler, "/?q=shark.dns.emb.is", nil).Body.String(), `"sentence"`) {
		t.Fatal("a single word carried a sentence on the JSON route")
	}
}

func TestJSONRouteCarriesTheSameRanking(t *testing.T) {
	_, handler := newTestHandler(t, nil)
	w := get(t, handler, "/?q=shark.dns.emb.is", nil)
	if w.Code != http.StatusOK {
		t.Fatalf("status %d: %s", w.Code, w.Body.String())
	}
	var document struct {
		Name    string `json:"name"`
		Type    string `json:"type"`
		TTL     int    `json:"ttl"`
		Model   string `json:"model"`
		Records []struct {
			Escaped string  `json:"escaped"`
			Glyph   string  `json:"glyph"`
			Name    string  `json:"name"`
			Slug    string  `json:"slug"`
			Score   float64 `json:"score"`
		} `json:"records"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &document); err != nil {
		t.Fatalf("decode %q: %v", w.Body.String(), err)
	}
	if document.Type != "TXT" || document.TTL != 300 || document.Model != "minilm" {
		t.Fatalf("document header is %+v", document)
	}
	if len(document.Records) != 3 {
		t.Fatalf("got %d records, want 3", len(document.Records))
	}
	first := document.Records[0]
	if first.Glyph != "🦈" || first.Slug != "shark" || first.Name != "shark" {
		t.Fatalf("the first record is %+v", first)
	}
	if decoded := unescapeBytes(t, strings.Trim(first.Escaped, `"`)); decoded != first.Glyph {
		t.Fatalf("escaped %q decoded to %q, want %q", first.Escaped, decoded, first.Glyph)
	}
	if first.Score != 1 {
		t.Fatalf("the top score is %v, want 1", first.Score)
	}
}

func TestEscapingMatchesWhatAResolverPrints(t *testing.T) {
	// 🦈 is U+1F988: f0 9f a6 88, so a resolver's presentation layer prints it
	// as three-digit escapes, which is why the readable view needs the HTTP
	// routes at all.
	if got, want := escapeTXT("🦈"), `"\240\159\166\136"`; got != want {
		t.Fatalf("escapeTXT(🦈) is %s, want %s", got, want)
	}
	if got, want := escapeTXT("no-escapes-here"), `"no-escapes-here"`; got != want {
		t.Fatalf("escapeTXT of printable text is %s, want %s", got, want)
	}
	if got := escapeTXT(`a"b\c`); got != `"a\"b\\c"` {
		t.Fatalf("quotes and backslashes are not escaped: %s", got)
	}
}

func TestRefusalsCarryTheirReason(t *testing.T) {
	_, handler := newTestHandler(t, func(cfg *Config) { cfg.RateLimit = 1 })
	if w := get(t, handler, "/shark.dns.emb.is", nil); w.Code != http.StatusOK {
		t.Fatalf("the first query answered %d", w.Code)
	}
	if w := get(t, handler, "/shark.dns.emb.is", nil); w.Code != http.StatusTooManyRequests {
		t.Fatalf("a rate-limited query answered %d, want 429", w.Code)
	}
	if w := get(t, handler, "/"+strings.Repeat("a", emoji.MaxLabelBytes+1)+".dns.emb.is", nil); w.Code != http.StatusBadRequest {
		t.Fatalf("an over-limit name answered %d, want 400", w.Code)
	}
	if w := get(t, handler, "/-.dns.emb.is", nil); w.Code != http.StatusBadRequest {
		t.Fatalf("a name with no terms answered %d, want 400", w.Code)
	}
}

func TestAnUnreadyServiceSaysSo(t *testing.T) {
	vocab, err := emoji.LoadBytes([]byte(fixtureVocab))
	if err != nil {
		t.Fatalf("LoadBytes: %v", err)
	}
	service := NewService(testConfig(), vocab, newFakeEmbedder())
	handler := NewHTTPHandler(service)

	if w := get(t, handler, "/healthz", nil); w.Code != http.StatusServiceUnavailable {
		t.Fatalf("an unready health route answered %d, want 503", w.Code)
	}
	if w := get(t, handler, "/shark.dns.emb.is", nil); w.Code != http.StatusServiceUnavailable {
		t.Fatalf("a query before readiness answered %d, want 503", w.Code)
	}

	if err := service.BuildIndex(t.Context()); err != nil {
		t.Fatalf("BuildIndex: %v", err)
	}
	w := get(t, handler, "/healthz", nil)
	if w.Code != http.StatusOK || !strings.Contains(w.Body.String(), `"ready":true`) {
		t.Fatalf("a ready health route answered %d with %s", w.Code, w.Body.String())
	}
}

func TestMetadataIsReadFromTheService(t *testing.T) {
	_, handler := newTestHandler(t, nil)
	var meta map[string]any
	if err := json.Unmarshal(get(t, handler, "/meta", nil).Body.Bytes(), &meta); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if meta["entries"] != float64(4) {
		t.Fatalf("metadata reports %v entries, want 4", meta["entries"])
	}

	// A larger vocabulary changes the reading with no page edit.
	const larger = `{"source":"test","license":"test","entries":[
 {"glyph":"🦈","slug":"shark","name":"shark","description":"shark"},
 {"glyph":"😭","slug":"loudly_crying_face","name":"loudly crying face","description":"loudly crying face"},
 {"glyph":"🎉","slug":"party_popper","name":"party popper","description":"party popper"},
 {"glyph":"🐟","slug":"fish","name":"fish","description":"fish"},
 {"glyph":"🚀","slug":"rocket","name":"rocket","description":"rocket"}]}`
	vocab, err := emoji.LoadBytes([]byte(larger))
	if err != nil {
		t.Fatalf("LoadBytes: %v", err)
	}
	bigger := NewService(testConfig(), vocab, newFakeEmbedder())
	if err := bigger.BuildIndex(t.Context()); err != nil {
		t.Fatalf("BuildIndex: %v", err)
	}
	var other map[string]any
	if err := json.Unmarshal(get(t, NewHTTPHandler(bigger), "/meta", nil).Body.Bytes(), &other); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if other["entries"] != float64(5) {
		t.Fatalf("metadata reports %v entries after the vocabulary grew, want 5", other["entries"])
	}
}

func TestStatisticsSeparateRefusalsFromFailures(t *testing.T) {
	service, fake := newTestService(t, nil)
	handler := NewHTTPHandler(service)
	get(t, handler, "/shark.dns.emb.is", nil)
	get(t, handler, "/-.dns.emb.is", nil)
	fake.compErr = errTestUpstream
	get(t, handler, "/fish.dns.emb.is", nil)
	fake.compErr = nil

	var stats StatsSnapshot
	if err := json.Unmarshal(get(t, handler, "/stats", nil).Body.Bytes(), &stats); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if stats.Served != 1 || stats.Unparseable != 1 || stats.UpstreamErrors != 1 {
		t.Fatalf("counters are %+v", stats)
	}
}

func TestCrossOriginReadsAreBoundedToTheSite(t *testing.T) {
	_, handler := newTestHandler(t, func(cfg *Config) {
		cfg.Origins = []string{"https://emb.is", "https://www.emb.is"}
	})

	allowed := get(t, handler, "/shark.dns.emb.is", map[string]string{"Origin": "https://emb.is"})
	if got := allowed.Header().Get("Access-Control-Allow-Origin"); got != "https://emb.is" {
		t.Fatalf("a site origin was not allowed: %q", got)
	}
	if got := allowed.Header().Get("Vary"); !strings.Contains(got, "Origin") {
		t.Fatalf("the reply does not vary on Origin: %q", got)
	}

	stranger := get(t, handler, "/shark.dns.emb.is", map[string]string{"Origin": "https://example.com"})
	if got := stranger.Header().Get("Access-Control-Allow-Origin"); got != "" {
		t.Fatalf("a stranger origin was allowed: %q", got)
	}

	preflight := httptest.NewRequest(http.MethodOptions, "/shark.dns.emb.is", nil)
	preflight.Header.Set("Origin", "https://emb.is")
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, preflight)
	if w.Code != http.StatusNoContent || w.Header().Get("Access-Control-Allow-Methods") == "" {
		t.Fatalf("the preflight answered %d with %v", w.Code, w.Header())
	}

	blocked := httptest.NewRequest(http.MethodOptions, "/shark.dns.emb.is", nil)
	blocked.Header.Set("Origin", "https://example.com")
	w = httptest.NewRecorder()
	handler.ServeHTTP(w, blocked)
	if w.Code != http.StatusForbidden {
		t.Fatalf("a stranger's preflight answered %d, want 403", w.Code)
	}
}

func TestNothingStateChangingIsServed(t *testing.T) {
	_, handler := newTestHandler(t, nil)
	for _, method := range []string{http.MethodPost, http.MethodPut, http.MethodDelete} {
		req := httptest.NewRequest(method, "/shark.dns.emb.is", nil)
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, req)
		if w.Code != http.StatusMethodNotAllowed {
			t.Fatalf("%s answered %d, want 405", method, w.Code)
		}
	}
}

func TestTheRootExplainsTheZone(t *testing.T) {
	_, handler := newTestHandler(t, nil)
	w := get(t, handler, "/", nil)
	if w.Code != http.StatusOK || !strings.Contains(w.Body.String(), "every name under this zone is a query") {
		t.Fatalf("the root answered %d with %q", w.Code, w.Body.String())
	}
}
