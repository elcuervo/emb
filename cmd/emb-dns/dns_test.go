package main

import (
	"net"
	"strings"
	"testing"

	"github.com/miekg/dns"

	"github.com/elcuervo/emb/internal/emoji"
)

// recorder is a dns.ResponseWriter that keeps the reply instead of sending it,
// so a zone rule can be asserted without a socket. The socket itself is
// exercised separately (TestSocketsAnswerTheZone).
type recorder struct {
	msg  *dns.Msg
	from string
	netw string
	fail error
}

func newRecorder(from string) *recorder { return &recorder{from: from, netw: "udp"} }

func (r *recorder) WriteMsg(m *dns.Msg) error   { r.msg = m; return r.fail }
func (r *recorder) Write(b []byte) (int, error) { return len(b), r.fail }
func (r *recorder) LocalAddr() net.Addr         { return &net.UDPAddr{IP: net.ParseIP("192.0.2.53"), Port: 53} }
func (r *recorder) RemoteAddr() net.Addr {
	return &net.UDPAddr{IP: net.ParseIP(r.from), Port: 5354}
}
func (r *recorder) TsigStatus() error   { return nil }
func (r *recorder) TsigTimersOnly(bool) {}
func (r *recorder) Hijack()             {}
func (r *recorder) Close() error        { return nil }

// ask builds a query for a name and type and returns the zone's reply.
func ask(t *testing.T, service *Service, name string, qtype uint16) *dns.Msg {
	t.Helper()
	req := new(dns.Msg)
	req.SetQuestion(dns.Fqdn(name), qtype)
	req.SetEdns0(udpPayloadSize, false)
	w := newRecorder("192.0.2.1")
	NewDNSHandler(service).ServeDNS(w, req)
	if w.msg == nil {
		t.Fatalf("the zone answered nothing for %s", name)
	}
	return w.msg
}

func txtStrings(t *testing.T, msg *dns.Msg) []string {
	t.Helper()
	var out []string
	for _, rr := range msg.Answer {
		record, ok := rr.(*dns.TXT)
		if !ok {
			t.Fatalf("answer carries a %T, want TXT", rr)
		}
		out = append(out, record.Txt...)
	}
	return out
}

func TestQueryAnswersOneGlyphPerRecord(t *testing.T) {
	service, _ := newTestService(t, nil)
	msg := ask(t, service, "shark.dns.emb.is", dns.TypeTXT)

	if msg.Rcode != dns.RcodeSuccess {
		t.Fatalf("rcode is %s, want NOERROR", dns.RcodeToString[msg.Rcode])
	}
	if !msg.Authoritative {
		t.Fatal("the zone answered without claiming authority")
	}
	got := txtStrings(t, msg)
	if len(got) != 3 {
		t.Fatalf("answered with %d records, want top_k=3: %v", len(got), got)
	}
	if got[0] != "🦈" {
		t.Fatalf("the first record is %q, want the top-ranked glyph", got[0])
	}
	for _, rr := range msg.Answer {
		if rr.Header().Name != "shark.dns.emb.is." {
			t.Fatalf("a record is owned by %q, not the queried name", rr.Header().Name)
		}
		if tt := rr.Header().Ttl; tt != uint32(service.cfg.TTL) {
			t.Fatalf("a record carries TTL %d, want %d", tt, service.cfg.TTL)
		}
		if record := rr.(*dns.TXT); len(record.Txt) != 1 {
			t.Fatalf("a record carries %d strings (%v), want only its glyph", len(record.Txt), record.Txt)
		}
	}
}

func TestSentencesRankWithoutASlug(t *testing.T) {
	service, fake := newTestService(t, nil)
	fake.composed = []float32{0, 0, 1}
	msg := ask(t, service, "my.mom.is.in.the.hospital.dns.emb.is", dns.TypeTXT)
	got := txtStrings(t, msg)
	// A sentence answers with a sentence first, then the alternatives it read.
	if len(got) != 4 || got[0] != strings.Join(got[1:], "") {
		t.Fatalf("a sentence answered %v, want the sentence then its three results", got)
	}
	if terms := fake.lastQuery().Terms; len(terms) != 1 || terms[0].Text != "my mom is in the hospital" {
		t.Fatalf("the sentence reached the model as %+v", terms)
	}
}

func TestASentenceIsAnsweredWithASentence(t *testing.T) {
	service, fake := newTestService(t, nil)
	fake.composed = []float32{0, 0, 1} // party popper, then loudly crying face, then shark

	sentence := ask(t, service, "my.mom.is.in.the.hospital.dns.emb.is", dns.TypeTXT)
	got := txtStrings(t, sentence)
	if len(got) != 4 {
		t.Fatalf("a sentence answered %d records, want the sentence and three results: %v", len(got), got)
	}
	if got[0] != strings.Join(got[1:], "") {
		t.Fatalf("the sentence record is %q, want the ranked records joined: %v", got[0], got[1:])
	}
	if len(got[0]) == 0 || got[1] != "🎉" {
		t.Fatalf("the ranked records after the sentence are %v, want the top result first", got[1:])
	}
	if first := sentence.Answer[0].(*dns.TXT); len(first.Txt) != 1 {
		t.Fatalf("the sentence record carries %d strings, want one", len(first.Txt))
	}

	for _, name := range []string{"shark.dns.emb.is", "🦈.dns.emb.is", "shark-fish+bird.dns.emb.is"} {
		records := txtStrings(t, ask(t, service, name, dns.TypeTXT))
		if len(records) != 3 {
			t.Fatalf("%s answered %d records (%v), want no sentence record", name, len(records), records)
		}
	}
}

func TestASentenceOfOneIsOneGlyph(t *testing.T) {
	service, _ := newTestService(t, func(cfg *Config) { cfg.TopK = 1 })
	got := txtStrings(t, ask(t, service, "i.am.so.tired.dns.emb.is", dns.TypeTXT))
	if len(got) != 2 || got[0] != got[1] {
		t.Fatalf("a sentence with one result answered %v, want the sentence and the result", got)
	}
}

func TestApexCarriesItsOwnRecords(t *testing.T) {
	service, _ := newTestService(t, func(cfg *Config) {
		cfg.ApexAddress = "203.0.113.7"
		cfg.ApexAddressV6 = "2001:db8::7"
	})

	txt := ask(t, service, "dns.emb.is", dns.TypeTXT)
	if got := txtStrings(t, txt); len(got) != 1 || !strings.Contains(got[0], "dns.emb.is") {
		t.Fatalf("the apex TXT is %v, want the zone's usage", got)
	}

	soa := ask(t, service, "dns.emb.is", dns.TypeSOA)
	if len(soa.Answer) != 1 {
		t.Fatalf("the apex SOA has %d records, want 1", len(soa.Answer))
	}
	record, ok := soa.Answer[0].(*dns.SOA)
	if !ok {
		t.Fatalf("the apex SOA is a %T", soa.Answer[0])
	}
	if record.Minttl != uint32(service.cfg.NegativeTTL) {
		t.Fatalf("the SOA minimum is %d, want the negative TTL %d", record.Minttl, service.cfg.NegativeTTL)
	}
	if strings.HasSuffix(record.Ns, ".dns.emb.is.") {
		t.Fatalf("the nameserver %q lives inside the zone, where it would be a query", record.Ns)
	}

	a := ask(t, service, "dns.emb.is", dns.TypeA)
	if len(a.Answer) != 1 || a.Answer[0].(*dns.A).A.String() != "203.0.113.7" {
		t.Fatalf("the apex A is %v", a.Answer)
	}

	aaaa := ask(t, service, "dns.emb.is", dns.TypeAAAA)
	if len(aaaa.Answer) != 1 || aaaa.Answer[0].(*dns.AAAA).AAAA.String() != "2001:db8::7" {
		t.Fatalf("the apex AAAA is %v", aaaa.Answer)
	}

	other := ask(t, service, "dns.emb.is", dns.TypeMX)
	if len(other.Answer) != 0 || len(other.Ns) != 1 {
		t.Fatalf("an unserved type at the apex answered %v with %v in authority", other.Answer, other.Ns)
	}
}

func TestOnlyTheAskedTypeIsAnswered(t *testing.T) {
	service, _ := newTestService(t, nil)
	msg := ask(t, service, "shark.dns.emb.is", dns.TypeA)
	if len(msg.Answer) != 0 {
		t.Fatalf("an A question was answered with %v", msg.Answer)
	}
	if msg.Rcode != dns.RcodeSuccess {
		t.Fatalf("NODATA came back as %s, want NOERROR", dns.RcodeToString[msg.Rcode])
	}
	if len(msg.Ns) != 1 {
		t.Fatalf("NODATA carried %d authority records, want the SOA", len(msg.Ns))
	}
	if soa, ok := msg.Ns[0].(*dns.SOA); !ok || soa.Hdr.Ttl != uint32(service.cfg.NegativeTTL) {
		t.Fatalf("the negative TTL is not the configured one: %v", msg.Ns[0])
	}

	any := ask(t, service, "shark.dns.emb.is", dns.TypeANY)
	if got := txtStrings(t, any); len(got) != 3 {
		t.Fatalf("ANY answered %v, want the ranked records", got)
	}
}

func TestUnparseableNamesDoNotExist(t *testing.T) {
	service, _ := newTestService(t, nil)
	for _, name := range []string{"-.dns.emb.is", strings.Repeat("a", emoji.MaxLabelBytes+1) + ".dns.emb.is"} {
		msg := ask(t, service, name, dns.TypeTXT)
		if msg.Rcode != dns.RcodeNameError {
			t.Fatalf("%s answered %s, want NXDOMAIN", name, dns.RcodeToString[msg.Rcode])
		}
		soa, ok := msg.Ns[0].(*dns.SOA)
		if !ok || soa.Minttl != uint32(service.cfg.NegativeTTL) {
			t.Fatalf("%s carried %v in authority, want a short negative SOA", name, msg.Ns)
		}
	}
}

func TestOutsideTheZoneIsRefused(t *testing.T) {
	service, fake := newTestService(t, nil)
	for _, name := range []string{"example.com", "emb.is", "notdns.emb.is.example"} {
		msg := ask(t, service, name, dns.TypeTXT)
		if msg.Rcode != dns.RcodeRefused {
			t.Fatalf("%s answered %s, want REFUSED", name, dns.RcodeToString[msg.Rcode])
		}
	}
	if len(fake.queries) != 0 {
		t.Fatal("a name outside the zone reached the model")
	}
}

func TestAQueryBeforeReadinessFailsLoudly(t *testing.T) {
	vocab, err := emoji.LoadBytes([]byte(fixtureVocab))
	if err != nil {
		t.Fatalf("LoadBytes: %v", err)
	}
	service := NewService(testConfig(), vocab, newFakeEmbedder())
	msg := ask(t, service, "shark.dns.emb.is", dns.TypeTXT)
	if msg.Rcode != dns.RcodeServerFailure {
		t.Fatalf("a query before readiness answered %s, want SERVFAIL", dns.RcodeToString[msg.Rcode])
	}
	if len(msg.Answer) != 0 {
		t.Fatalf("a query before readiness was answered with %v", msg.Answer)
	}
}

func TestAnUpstreamFailureIsNotANegativeAnswer(t *testing.T) {
	service, fake := newTestService(t, nil)
	fake.compErr = errTestUpstream
	msg := ask(t, service, "shark.dns.emb.is", dns.TypeTXT)
	if msg.Rcode != dns.RcodeServerFailure {
		t.Fatalf("an upstream failure answered %s, want SERVFAIL", dns.RcodeToString[msg.Rcode])
	}
	if len(msg.Answer) != 0 {
		t.Fatalf("an upstream failure was answered with %v", msg.Answer)
	}
}

func TestRateLimitedQueriesAreRefused(t *testing.T) {
	service, _ := newTestService(t, func(cfg *Config) { cfg.RateLimit = 1 })
	first := ask(t, service, "shark.dns.emb.is", dns.TypeTXT)
	if first.Rcode != dns.RcodeSuccess {
		t.Fatalf("the first query answered %s", dns.RcodeToString[first.Rcode])
	}
	second := ask(t, service, "shark.dns.emb.is", dns.TypeTXT)
	if second.Rcode != dns.RcodeRefused {
		t.Fatalf("a query past the burst answered %s, want REFUSED", dns.RcodeToString[second.Rcode])
	}
	if len(second.Answer) != 0 {
		t.Fatalf("a rate-limited query was answered with %v", second.Answer)
	}
	if service.stats.RateLimited.Load() != 1 {
		t.Fatalf("rate_limited counter is %d, want 1", service.stats.RateLimited.Load())
	}
}

func TestTheEscapedRecordCarriesTheGlyph(t *testing.T) {
	service, _ := newTestService(t, nil)
	msg := ask(t, service, "shark.dns.emb.is", dns.TypeTXT)
	glyph := txtStrings(t, msg)[0]
	// What a resolver's tooling prints for the record the zone just sent.
	rendered := "shark.dns.emb.is.\t300\tIN\tTXT\t" + escapeTXT(glyph)
	if strings.Contains(rendered, glyph) {
		t.Fatalf("a resolver's rendering would show the raw glyph: %q", rendered)
	}
	if decoded := unescapeBytes(t, unescapeRecordData(t, rendered)); decoded != glyph {
		t.Fatalf("the escaped record decoded to %q, want %q", decoded, glyph)
	}
}

func TestWriteUDPTruncatesRatherThanOverflows(t *testing.T) {
	service, _ := newTestService(t, nil)
	req := new(dns.Msg)
	req.SetQuestion("shark.dns.emb.is.", dns.TypeTXT)
	msg := new(dns.Msg)
	msg.SetReply(req)
	// One record the answer would never carry: long enough that a 512-byte
	// reply cannot hold it.
	msg.Answer = append(msg.Answer, &dns.TXT{
		Hdr: dns.RR_Header{Name: "shark.dns.emb.is.", Rrtype: dns.TypeTXT, Class: dns.ClassINET, Ttl: 1},
		Txt: []string{strings.Repeat("x", 600)},
	})
	w := newRecorder("192.0.2.1")
	NewDNSHandler(service).writeUDP(w, req, msg)
	if w.msg.Len() > dns.MinMsgSize {
		t.Fatalf("a reply was written at %d bytes, past the 512-byte bound, with TC=%v", w.msg.Len(), w.msg.Truncated)
	}
	if !w.msg.Truncated {
		t.Fatal("an oversized reply was cut without telling the resolver")
	}
}

// unescapeRecordData pulls the quoted data field out of a rendered record.
func unescapeRecordData(t *testing.T, rendered string) string {
	t.Helper()
	start := strings.Index(rendered, "\tTXT\t")
	if start < 0 {
		t.Fatalf("%q carries no TXT data", rendered)
	}
	return strings.Trim(rendered[start+len("\tTXT\t"):], `"`)
}

// unescapeBytes reads a presentation-format string back into the bytes it
// stands for: the inverse of what a resolver's tooling does when it prints a
// name or a record's data.
func unescapeBytes(t *testing.T, field string) string {
	t.Helper()
	var out strings.Builder
	for i := 0; i < len(field); i++ {
		if field[i] != '\\' {
			out.WriteByte(field[i])
			continue
		}
		if i+3 >= len(field) {
			t.Fatalf("%q ends in a partial escape", field)
		}
		value := 0
		for _, digit := range field[i+1 : i+4] {
			if digit < '0' || digit > '9' {
				t.Fatalf("%q carries a malformed escape", field)
			}
			value = value*10 + int(digit-'0')
		}
		out.WriteByte(byte(value))
		i += 3
	}
	return out.String()
}

func TestAConjunctionAnswersOneGlyphPerRecord(t *testing.T) {
	service, _ := newTestService(t, nil)
	msg := ask(t, service, "🐟*🐟.dns.emb.is", dns.TypeTXT)
	if msg.Rcode != dns.RcodeSuccess {
		t.Fatalf("rcode is %s, want NOERROR", dns.RcodeToString[msg.Rcode])
	}
	got := txtStrings(t, msg)
	// The fixture has four entries and the query names one of them, so three
	// candidates remain and the named entry is not among them.
	if len(got) != 3 {
		t.Fatalf("answered with %d records, want the 3 candidates: %v", len(got), got)
	}
	if got[0] != "🦈" {
		t.Fatalf("the first record is %q, want the best joint glyph (not the term)", got[0])
	}
	for _, rr := range msg.Answer {
		if record := rr.(*dns.TXT); len(record.Txt) != 1 {
			t.Fatalf("a conjunction's record carries %d strings (%v), want only its glyph", len(record.Txt), record.Txt)
		}
	}
	// A conjunction is not a sentence, so no record carries a joined phrase.
	for _, text := range got {
		if len([]rune(text)) > 1 {
			t.Fatalf("a record carries the joined phrase %q", text)
		}
	}
}

func TestAMixedNameDoesNotExist(t *testing.T) {
	service, _ := newTestService(t, nil)
	msg := ask(t, service, "🦈*🐟+🎉.dns.emb.is", dns.TypeTXT)
	if msg.Rcode != dns.RcodeNameError {
		t.Fatalf("rcode is %s, want NXDOMAIN", dns.RcodeToString[msg.Rcode])
	}
	if len(msg.Answer) != 0 {
		t.Fatalf("a mixed name was answered with %v", txtStrings(t, msg))
	}
}
