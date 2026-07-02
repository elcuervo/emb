# Design

## emb Ruby gem — forward redis-client options

### Approach

Accept `**rest` and merge with defaults. Only `pool` stays as a named kwarg.

```ruby
DEFAULTS = { host: 'localhost', port: 6379, pool: 5 }.freeze

def initialize(pool: DEFAULTS[:pool], **redis_options)
  url = extract_url!(redis_options)
  redis_options[:host] ||= DEFAULTS[:host] unless url
  redis_options[:port] ||= DEFAULTS[:port] unless url
  redis_options[:protocol] ||= 2
  redis_options[:reconnect_attempts] ||= 3

  @pool = ConnectionPool.new(size: pool) do
    RedisClient.new(url: url, **redis_options)
  end
end

private

def extract_url!(opts)
  url = opts.delete(:url)
  url.nil? ? ENV.fetch('EMB_URL', nil) : url
end
```

### What this unlocks

Users can now pass any redis-client option:

```ruby
Emb.setup(
  url: "redis://localhost:6379",
  pool: 10,
  connect_timeout: 2,
  read_timeout: 10,
  ssl: true,
  ssl_params: { verify_mode: OpenSSL::SSL::VERIFY_PEER },
  driver: :hiredis,
  inherit_socket: true
)
```

### Defaults (overridable)

- `protocol: 2` — emb speaks RESP2 via redcon. Default ensures safe behavior. Users can override but RESP3 will fail at the server level (redcon doesn't support HELLO).
- `reconnect_attempts: 3` — default for resilience; fully overridable.

### Module-level forwarding

`Emb.setup(**kwargs)` and `Emb.new(**kwargs)` pass everything through, so forwarded options work at both levels.

---

## emb Go server — TLS support

### Approach

Add `tls_cert` and `tls_key` fields to config. When both are non-empty, load the `tls.Config` and pass it to the server. The server stores a `*tls.Config` (nil = plain TCP).

```yaml
listen: ":6379"
tls_cert: /etc/emb/cert.pem
tls_key:  /etc/emb/key.pem
```

CLI equivalent:

```
emb -listen :6379 -tls-cert /etc/emb/cert.pem -tls-key /etc/emb/key.pem
```

### Server change

```go
type Server struct {
    // ... existing fields
    tlsConfig *tls.Config
}

func New(addr string, reg *registry.Registry, password string, cacheConfig string, tlsConfig *tls.Config) *Server

func (s *Server) ListenAndServe() error {
    var ln net.Listener
    var err error
    if s.tlsConfig != nil {
        ln, err = tls.Listen("tcp", s.addr, s.tlsConfig)
    } else {
        ln, err = net.Listen("tcp", s.addr)
    }
    // ... rest identical
}
```

Redcon's `Serve(net.Listener)` accepts both `net.Listener` and `tls.Listener` — no changes to the library.

### Config parsing

```go
type Config struct {
    Listen   string `yaml:"listen"`
    Password string `yaml:"password"`
    TLSCert  string `yaml:"tls_cert"`
    TLSKey   string `yaml:"tls_key"`
    Cache    string `yaml:"cache"`
    Models   map[string]ModelConfig `yaml:"models"`
}
```

Flags: `-tls-cert` and `-tls-key`.

### Edge cases

| Scenario | Behavior |
|---|---|
| Both cert and key set | TLS listener, log "listening with TLS on ..." |
| Neither set | Plain TCP, identical to today |
| Only cert set (no key) | Error at startup: "tls_cert requires tls_key" |
| Cert/key unreadable | Error at startup from `tls.LoadX509KeyPair` |
| Cert/key paths relative | Resolved relative to working directory |
| Client connects via plain TCP to TLS port | Fails at client side (TLS handshake) |
| Client connects via TLS to plain TCP port | Fails at client side |

### Self-signed certs

No built-in cert generation. Users who need self-signed certs for development can use
`openssl req -x509 ...` or `mkcert` externally. The server only needs valid PEM files.
