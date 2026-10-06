package main

import (
	"flag"
	"fmt"
	"os"

	"gopkg.in/yaml.v3"
)

// Config is the zone's whole configuration. A YAML file carries it; flags
// override the handful of values an operator changes per run (where to listen,
// who to ask, which zone to serve).
type Config struct {
	// Zone is the apex the service is authoritative for. Names strictly
	// beneath it are queries; the apex itself is reserved for the zone's own
	// records.
	Zone string `yaml:"zone"`
	// ListenUDP is the address the DNS socket binds. On Fly this is
	// `fly-global-services:53`, because a UDP reply sent from any other local
	// address leaves the machine with the wrong source address.
	ListenUDP string `yaml:"listen_udp"`
	// ListenTCP is the DNS-over-TCP listener. It cannot bind the UDP address;
	// it belongs on the wildcard address.
	ListenTCP string `yaml:"listen_tcp"`
	// ListenHTTP carries the readable routes, the health route, the metadata
	// route, and the statistics route.
	ListenHTTP string `yaml:"listen_http"`
	// ApexAddress is the address the apex answers with, so the zone's own name
	// resolves to the HTTP surface beside the DNS one.
	ApexAddress string `yaml:"apex_address"`
	// ApexAddressV6 is the apex's IPv6, answered as AAAA. It exists for the
	// HTTP surface and for platform domain verification; it is deliberately not
	// a UDP endpoint, because Fly answers UDP only on the dedicated IPv4.
	ApexAddressV6 string `yaml:"apex_address_v6"`
	// Nameserver and Mailbox are the zone's authority contacts. Both are
	// deliberately outside the zone: a name inside it would be an emoji query,
	// and the zone's own infrastructure cannot be answered by a ranking.
	Nameserver string `yaml:"nameserver"`
	Mailbox    string `yaml:"mailbox"`

	// Upstream is the emb server that does the embedding, on loopback.
	Upstream string `yaml:"upstream"`
	Password string `yaml:"password"`
	Model    string `yaml:"model"`
	// Script is the Lua preset that composes a query's terms into one vector.
	Script string `yaml:"script"`
	// Vocabulary is the committed asset the zone answers from.
	Vocabulary string `yaml:"vocabulary"`

	TopK        int     `yaml:"top_k"`
	TTL         int     `yaml:"ttl"`
	NegativeTTL int     `yaml:"negative_ttl"`
	RateLimit   float64 `yaml:"rate_limit"`
	// Origins are the page origins allowed to read the HTTP routes. Only these
	// may fetch them cross-origin; every other origin is refused.
	Origins []string `yaml:"origins"`
	// LogQueries logs the name of every query. Off by default: a query name is
	// a sentence about someone's day, and the service does not need it.
	LogQueries bool `yaml:"log_queries"`
}

// Default is the configuration a bare `emb-dns` runs with: a loopback DNS
// socket on a non-privileged port, the local emb server, and the committed
// vocabulary.
func Default() Config {
	return Config{
		Zone:        "dns.emb.is",
		ListenUDP:   "127.0.0.1:5354",
		ListenTCP:   "",
		ListenHTTP:  "127.0.0.1:8080",
		Upstream:    "127.0.0.1:6379",
		Nameserver:  "ns1.emb.is",
		Mailbox:     "hostmaster.emb.is",
		Model:       "minilm",
		Script:      "scripts/emoji.lua",
		Vocabulary:  "dns/emoji-vocab.json",
		TopK:        3,
		TTL:         300,
		NegativeTTL: 60,
		RateLimit:   20,
	}
}

// Load reads the configuration from an optional YAML file. It reports whether
// -version was asked for, which main answers: a configuration parser should not
// end the process.
//
// There is one way to configure the zone — the file — because there is one
// caller that varies it (`just dns-dev`, which rewrites the listen addresses
// for a local run). A second mechanism would be a second thing to keep true.
func Load(args []string) (Config, bool, error) {
	fs := flag.NewFlagSet("emb-dns", flag.ContinueOnError)
	configPath := fs.String("config", "", "path to a YAML configuration file")
	showVersion := fs.Bool("version", false, "print the version and exit")
	if err := fs.Parse(args); err != nil {
		return Config{}, false, err
	}
	if *showVersion {
		return Config{}, true, nil
	}

	cfg := Default()
	if *configPath != "" {
		raw, err := os.ReadFile(*configPath)
		if err != nil {
			return Config{}, false, fmt.Errorf("config: %w", err)
		}
		// Decode over the defaults so a field the file omits keeps its default
		// rather than its zero value.
		if err := yaml.Unmarshal(raw, &cfg); err != nil {
			return Config{}, false, fmt.Errorf("config %s: %w", *configPath, err)
		}
	}
	if err := cfg.validate(); err != nil {
		return Config{}, false, err
	}
	return cfg, false, nil
}

func (c Config) validate() error {
	switch {
	case c.Zone == "":
		return fmt.Errorf("config: a zone is required")
	case c.ListenUDP == "" && c.ListenTCP == "":
		return fmt.Errorf("config: at least one DNS listener is required")
	case c.Nameserver == "":
		return fmt.Errorf("config: a nameserver is required")
	case c.Mailbox == "":
		return fmt.Errorf("config: an SOA mailbox is required")
	case c.ListenHTTP == "":
		return fmt.Errorf("config: an HTTP listener is required: the readable routes, health, metadata, and statistics live there")
	case c.Upstream == "":
		return fmt.Errorf("config: an upstream server is required")
	case c.Vocabulary == "":
		return fmt.Errorf("config: a vocabulary is required")
	case c.TopK <= 0:
		return fmt.Errorf("config: top_k must be positive, got %d", c.TopK)
	case c.TTL <= 0:
		return fmt.Errorf("config: ttl must be positive, got %d", c.TTL)
	case c.NegativeTTL <= 0:
		return fmt.Errorf("config: negative_ttl must be positive, got %d", c.NegativeTTL)
	}
	return nil
}

// FQDN is the zone apex as the wire spells it.
func (c Config) FQDN() string { return c.Zone + "." }
