## 1. Stats hash

- [x] 1.1 Rewrite `Client#stats`: `EMB.STATS` reply → `each_slice(2).to_h { |key, value| [key.to_sym, value] }` (symbol keys, values as decoded — no coercion); integration spec: `Emb.stats` is a Hash (not Array), `:models_loaded` is an Integer, `:per_model` is a String, `:active_requests` equals the server's value
- [x] 1.2 Confirm `Emb.stats` module delegation returns the same hash as the default client's `stats`; verify a spec asserts equality

## 2. INFO and CONFIG wrappers

- [x] 2.1 Add `Client#server_info(*sections)` (no args = all; args passed as-is to `INFO`), parsing `# Section` blocks into a nested Hash with Symbol section/key names and values as decoded; integration spec: full INFO gives `:Server`/`:Cache`/`:Keyspace`/`:Stats`/`:Clients`, `server_info(:server, :cache)` filters to two sections, `:redis_version` is a String, `:cache_hit_rate` a String
- [x] 2.2 Implement `Emb::RuntimeConfig` (`emb/runtime_config.rb`): `to_h` (all params), `[](key)` (exact → String value, glob → Hash, unknown → nil), `[]=(key, value)` (CONFIG SET, returns reply); integration specs: `to_h` has all params as Strings, `config["listen"]` scalar, `config["nope"]` → nil, `config["cache*"]` → Hash, `config["listen"] = ":9999"` raises read-only, `config["cache"] = "100MB"` raises on a cache-disabled-at-boot server
- [x] 2.3 Add `Client#config` (memoized `RuntimeConfig`) and repurpose module-level `Emb.config` to it (drop the `alias config setup`); verify a spec asserts `Emb.config` returns the same RuntimeConfig as the default client and `config["cache_file"] = v` round-trips through `config["cache_file"]` (assignment yields RHS; effect verified by read)
- [x] 2.4 Add module-level `Emb.server_info` delegate in `emb.rb`; verify it delegates to the default client (mirror `stats`)

## 3. Docs + validation

- [x] 3.1 Add a "Server info & config" section to `gems/emb/README.md`: typed stats hash example (values as decoded), `server_info` sections, `config_get`/`config_set` usage, and the BREAKING note (`stats` was an Array); verify rendered text matches the spec examples
- [x] 3.2 `bundle exec rubocop` clean in `gems/emb`
- [x] 3.3 `bundle exec rake` (server on `test-two-models.yaml`, port 16379) passes including the new specs; pre-existing `TestAsyncTokenizerOverlapsWork` Go flake noted separately if `just all` is run
- [x] 3.4 Verify against the real server: `Emb.stats` hash shape, `Emb.server_info` parse of a warm cache (`# Cache` section present with `cache_hit_rate`), `Emb.config_set(:cache_file, ...)` reflected in `Emb.config_get["cache_file"]`