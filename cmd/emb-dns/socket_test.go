package main

import (
	"context"
	"testing"
	"time"

	"github.com/miekg/dns"
)

func glyphRequest(t *testing.T, name string) *dns.Msg {
	t.Helper()
	req := new(dns.Msg)
	req.SetQuestion(dns.Fqdn(name+".dns.emb.is"), dns.TypeTXT)
	return req
}

// TestSocketsAnswerTheZone is the end-to-end proof that the listeners work:
// a real UDP socket and a real TCP socket, bound the way the service binds
// them, answering a real query. It needs no model: the embedder is the fixture.
func TestSocketsAnswerTheZone(t *testing.T) {
	service, fake := newTestService(t, nil)
	cfg := testConfig()
	cfg.ListenUDP = "127.0.0.1:0"
	cfg.ListenTCP = "127.0.0.1:0"

	servers, err := bindDNS(cfg, NewDNSHandler(service))
	if err != nil {
		t.Fatalf("bindDNS: %v", err)
	}
	if len(servers) != 2 {
		t.Fatalf("bound %d servers, want a UDP and a TCP listener", len(servers))
	}
	defer closeServers(context.Background(), servers)
	for _, server := range servers {
		go func() { _ = server.ActivateAndServe() }()
	}

	for _, server := range servers {
		network := "tcp"
		if server.PacketConn != nil {
			network = "udp"
		}
		t.Run(network, func(t *testing.T) {
			client := &dns.Client{Net: network, Timeout: 2 * time.Second}
			addr := listenerAddr(server)

			req := new(dns.Msg)
			req.SetQuestion("shark.dns.emb.is.", dns.TypeTXT)
			req.SetEdns0(udpPayloadSize, false)

			var reply *dns.Msg
			deadline := time.Now().Add(5 * time.Second)
			for {
				reply, _, err = client.Exchange(req, addr)
				if err == nil || time.Now().After(deadline) {
					break
				}
				time.Sleep(10 * time.Millisecond)
			}
			if err != nil {
				t.Fatalf("exchange over %s: %v", network, err)
			}
			if reply.Rcode != dns.RcodeSuccess {
				t.Fatalf("%s answered %s", network, dns.RcodeToString[reply.Rcode])
			}
			got := txtStrings(t, reply)
			if len(got) != 3 {
				t.Fatalf("%s answered %d records, want 3: %v", network, len(got), got)
			}
			// A resolver's own tooling escapes the record data on the way in,
			// exactly as `dig` prints it, so the glyph is recovered from the
			// bytes the zone put on the wire.
			if first := unescapeBytes(t, got[0]); first != "🦈" {
				t.Fatalf("%s answered %v, whose first record is %q", network, got, first)
			}
			if reply.IsEdns0() == nil {
				t.Fatalf("%s answered without the OPT record the query advertised", network)
			}

			// A glyph name has to survive the wire, where the library hands
			// it over escaped. The zone can only spell 🦈 out as `shark` if the
			// four bytes arrived intact, and only a real socket proves that.
			if _, _, err := client.Exchange(glyphRequest(t, "🦈-fish+bird"), addr); err != nil {
				t.Fatalf("glyph exchange over %s: %v", network, err)
			}
			terms := fake.lastQuery().Terms
			if len(terms) != 3 || terms[0].Text != "shark" || terms[1].Text != "fish" || terms[2].Text != "bird" {
				t.Fatalf("%s: a glyph name reached the model as %+v", network, terms)
			}
		})
	}
}
