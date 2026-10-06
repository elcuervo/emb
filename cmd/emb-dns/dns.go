package main

import (
	"errors"
	"fmt"
	"log"
	"net"
	"strings"

	"github.com/miekg/dns"

	"github.com/elcuervo/emb/internal/emoji"
)

// udpPayloadSize is the payload the zone advertises through EDNS0. It is the
// usual conservative ceiling for a path that may meet a tunnel, and it is far
// more than the three records a query answers with.
const udpPayloadSize = 1232

// usage is the apex's TXT record: the zone's only documentation, reachable
// without knowing anything about the service.
const usage = "dns.emb.is - every name under this zone is a query, answered as TXT. " +
	"Try: dig +short TXT i.lost.my.job.dns.emb.is | the readable form: https://dns.emb.is/i.lost.my.job"

// nameKind is where a question sits relative to the zone.
type nameKind int

const (
	outsideZone nameKind = iota
	apexName
	queryName
)

// classify splits a question name into the labels beneath the apex, and says
// whether the name is the apex, a query, or none of the zone's business.
func (c Config) classify(name string) ([]string, nameKind) {
	name = strings.ToLower(strings.TrimSuffix(name, "."))
	zone := strings.ToLower(c.Zone)
	switch {
	case name == zone:
		return nil, apexName
	case strings.HasSuffix(name, "."+zone):
		return splitLabels(strings.TrimSuffix(name, "."+zone)), queryName
	default:
		return nil, outsideZone
	}
}

// splitLabels splits a presentation-format name into its labels, as raw bytes.
//
// miekg/dns hands a name over escaped: every byte outside printable ASCII comes
// back as \DDD, so an emoji question arrives as the ten characters
// `\240\159\166\136` rather than as the four bytes it carried. The wire, the
// vocabulary, and the model all need the bytes, so the escapes are undone here
// — and the split happens on unescaped dots, so a `\.` inside a label is data
// rather than a boundary.
func splitLabels(name string) []string {
	var (
		labels  []string
		current strings.Builder
	)
	flush := func() {
		labels = append(labels, current.String())
		current.Reset()
	}
	for i := 0; i < len(name); i++ {
		switch c := name[i]; c {
		case '\\':
			if i+3 < len(name) && isDigit(name[i+1]) && isDigit(name[i+2]) && isDigit(name[i+3]) {
				current.WriteByte(byte((name[i+1]-'0')*100 + (name[i+2]-'0')*10 + (name[i+3] - '0')))
				i += 3
				continue
			}
			if i+1 < len(name) {
				i++
				current.WriteByte(name[i])
			}
		case '.':
			flush()
		default:
			current.WriteByte(c)
		}
	}
	if current.Len() > 0 {
		flush()
	}
	return labels
}

func isDigit(c byte) bool { return c >= '0' && c <= '9' }

// DNSHandler answers the zone. It holds no model: a query it can answer is one
// the service ranked.
type DNSHandler struct {
	svc *Service
}

// NewDNSHandler serves the zone from a service.
func NewDNSHandler(svc *Service) *DNSHandler { return &DNSHandler{svc: svc} }

// ServeDNS implements dns.Handler.
func (h *DNSHandler) ServeDNS(w dns.ResponseWriter, req *dns.Msg) {
	msg := new(dns.Msg)
	msg.SetReply(req)
	msg.Authoritative = true

	if len(req.Question) != 1 {
		msg.Rcode = dns.RcodeFormatError
		_ = w.WriteMsg(msg)
		return
	}
	question := req.Question[0]
	if opt := req.IsEdns0(); opt != nil {
		msg.SetEdns0(udpPayloadSize, false)
	}

	labels, kind := h.svc.cfg.classify(question.Name)
	switch kind {
	case outsideZone:
		// Authoritative for one zone and nothing else: this is also what keeps
		// the service from being usable as an open resolver.
		msg.Rcode = dns.RcodeRefused
	case apexName:
		h.apex(msg, question)
	default:
		h.query(msg, question, labels, sourceIP(w))
	}

	h.writeUDP(w, req, msg)
}

// apex answers the zone's own name: its usage, its addresses, and its
// authority records. Everything under it is a query instead.
func (h *DNSHandler) apex(msg *dns.Msg, question dns.Question) {
	cfg := h.svc.cfg
	name := dns.Fqdn(cfg.Zone)
	switch question.Qtype {
	case dns.TypeSOA:
		msg.Answer = append(msg.Answer, h.soa())
	case dns.TypeNS:
		msg.Answer = append(msg.Answer, &dns.NS{
			Hdr: dns.RR_Header{Name: name, Rrtype: dns.TypeNS, Class: dns.ClassINET, Ttl: uint32(cfg.TTL)},
			Ns:  dns.Fqdn(cfg.Nameserver),
		})
	case dns.TypeTXT, dns.TypeANY:
		msg.Answer = append(msg.Answer, &dns.TXT{
			Hdr: dns.RR_Header{Name: name, Rrtype: dns.TypeTXT, Class: dns.ClassINET, Ttl: uint32(cfg.TTL)},
			Txt: []string{usage},
		})
	case dns.TypeA:
		address := net.ParseIP(cfg.ApexAddress)
		if address == nil || address.To4() == nil {
			msg.Ns = append(msg.Ns, h.soa())
			return
		}
		msg.Answer = append(msg.Answer, &dns.A{
			Hdr: dns.RR_Header{Name: name, Rrtype: dns.TypeA, Class: dns.ClassINET, Ttl: uint32(cfg.TTL)},
			A:   address.To4(),
		})
	case dns.TypeAAAA:
		address := net.ParseIP(cfg.ApexAddressV6)
		if address == nil || address.To4() != nil || address.To16() == nil {
			msg.Ns = append(msg.Ns, h.soa())
			return
		}
		msg.Answer = append(msg.Answer, &dns.AAAA{
			Hdr:  dns.RR_Header{Name: name, Rrtype: dns.TypeAAAA, Class: dns.ClassINET, Ttl: uint32(cfg.TTL)},
			AAAA: address.To16(),
		})
	default:
		// NODATA: the name exists, this type does not, and the SOA in the
		// authority section is what lets a resolver cache that.
		msg.Ns = append(msg.Ns, h.soa())
	}
}

// query ranks a name and answers with one TXT record per result. Only the
// asked-for payload type is ever answered.
func (h *DNSHandler) query(msg *dns.Msg, question dns.Question, labels []string, source string) {
	switch question.Qtype {
	case dns.TypeTXT, dns.TypeANY:
	default:
		// The record carries the glyph and nothing else. Everything the
		// service knows beyond it is on the HTTP routes, not smuggled into a
		// type the caller did not ask for.
		msg.Ns = append(msg.Ns, h.soa())
		return
	}

	outcome, err := h.svc.Answer(labels, source)
	switch {
	case errors.Is(err, ErrNotReady):
		msg.Rcode = dns.RcodeServerFailure
		return
	case errors.Is(err, ErrRateLimited):
		msg.Rcode = dns.RcodeRefused
		return
	case errors.Is(err, emoji.ErrEmpty), errors.Is(err, emoji.ErrNoTerms), errors.Is(err, emoji.ErrTooLong):
		// A name the grammar cannot read does not exist, and the short SOA
		// minimum is what keeps a typo from being cached as permanent.
		msg.Rcode = dns.RcodeNameError
		msg.Ns = append(msg.Ns, h.soa())
		return
	case err != nil:
		log.Printf("zone: query %s failed: %v", outcome.Name, err)
		msg.Rcode = dns.RcodeServerFailure
		return
	}

	if outcome.Phrase != "" {
		// The joke, and the answer to a sentence: the whole ranking read as one
		// pictogram, before the alternatives it came from.
		msg.Answer = append(msg.Answer, &dns.TXT{
			Hdr: dns.RR_Header{
				Name:   question.Name,
				Rrtype: dns.TypeTXT,
				Class:  dns.ClassINET,
				Ttl:    uint32(h.svc.cfg.TTL),
			},
			Txt: []string{outcome.Phrase},
		})
	}
	for _, result := range outcome.Results {
		msg.Answer = append(msg.Answer, &dns.TXT{
			Hdr: dns.RR_Header{
				Name:   question.Name,
				Rrtype: dns.TypeTXT,
				Class:  dns.ClassINET,
				Ttl:    uint32(h.svc.cfg.TTL),
			},
			Txt: []string{result.Entry.Glyph},
		})
	}
}

// soa is the zone's authority record, carrying the negative TTL as its
// minimum so a refusal expires as quickly as the configuration says.
func (h *DNSHandler) soa() *dns.SOA {
	cfg := h.svc.cfg
	return &dns.SOA{
		Hdr:     dns.RR_Header{Name: dns.Fqdn(cfg.Zone), Rrtype: dns.TypeSOA, Class: dns.ClassINET, Ttl: uint32(cfg.NegativeTTL)},
		Ns:      dns.Fqdn(cfg.Nameserver),
		Mbox:    dns.Fqdn(cfg.Mailbox),
		Serial:  1,
		Refresh: 3600,
		Retry:   600,
		Expire:  86400,
		Minttl:  uint32(cfg.NegativeTTL),
	}
}

// writeUDP sends the reply, truncating it to the payload the caller can take.
// A TXT answer of a few short records never reaches the bound, but a resolver
// that advertised a small buffer would otherwise receive a message it must
// discard.
func (h *DNSHandler) writeUDP(w dns.ResponseWriter, req *dns.Msg, msg *dns.Msg) {
	size := dns.MinMsgSize
	if opt := req.IsEdns0(); opt != nil {
		size = int(opt.UDPSize())
	}
	if _, isTCP := w.RemoteAddr().(*net.TCPAddr); isTCP {
		size = dns.MaxMsgSize
	}
	if msg.Len() > size {
		msg.Truncate(size)
	}
	if err := w.WriteMsg(msg); err != nil {
		log.Printf("zone: write reply: %v", err)
	}
}

// hostOf is the address a request came from without its port, which is the unit
// the rate limit counts. It is never logged unless the operator asks for it.
func hostOf(address string) string {
	host, _, err := net.SplitHostPort(address)
	if err != nil {
		return address
	}
	return host
}

// sourceIP is the address a DNS query came from, for the same purpose.
func sourceIP(w dns.ResponseWriter) string {
	addr := w.RemoteAddr()
	if addr == nil {
		return "unknown"
	}
	return hostOf(addr.String())
}

// escapeTXT renders a record's data the way every resolver's presentation layer
// does: non-printable and non-ASCII bytes become \DDD, which is exactly why the
// zone needs an HTTP surface to show the emoji at all.
func escapeTXT(value string) string {
	var b strings.Builder
	b.WriteByte('"')
	for i := 0; i < len(value); i++ {
		c := value[i]
		switch {
		case c == '"' || c == '\\':
			b.WriteByte('\\')
			b.WriteByte(c)
		case c < 0x20 || c >= 0x7f:
			fmt.Fprintf(&b, "\\%03d", c)
		default:
			b.WriteByte(c)
		}
	}
	b.WriteByte('"')
	return b.String()
}
