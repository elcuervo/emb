# Changelog

All notable changes to the `emb` gem are documented here. The format follows
[Keep a Changelog](https://keepachangelog.com/en/1.1.0/).

## [0.4.3.pre4] - unreleased

### Changed

- `lazy: :batch` now splits a resolving scope into one plain `EMB <model> <text>...`
  share per model and dispatches every share concurrently; it never sends
  `EMB.MULTI`. A two-model scope runs concurrently even when it fits in a single
  `batch_size` chunk. `lazy: :multi` still coalesces into serial `EMB`/`EMB.MULTI`.

### Added

- `eval`/`evalsha` honor the configured `lazy` mode: deferred under `:multi` and
  `:batch` (each call is its own share, concurrent in `:batch`) and eager under
  `false`. `decode:` applies when the value resolves.

### Breaking

- Under `lazy: :batch`, a share that fails terminally no longer fails the force for
  its sibling shares, and its items raise that share's `Emb::ServerError` on every
  use instead of resolving to `[]`. Healthy shares in the same scope resolve
  normally. The serial (`:multi`, single-share) path is unchanged.
- Under a deferred mode (`lazy: :multi` or `:batch`), `eval`/`evalsha` return a
  lazy value instead of sending immediately. Force the value (or use an eager
  client) to get the parsed reply. The explicit `Emb.multi { }` block API is
  unaffected.
