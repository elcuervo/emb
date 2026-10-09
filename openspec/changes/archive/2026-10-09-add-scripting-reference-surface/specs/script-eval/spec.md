# Spec Delta

## REMOVED Requirements

### Requirement: Script API version

Reason: replaced by **Script API version is the server version**, which unifies
the scripted surface onto the repository `VERSION` instead of a second 1.x host
API number. The old requirement's "stable for performance-only changes" clause is
intentionally dropped: a single release version necessarily moves on every
release.

## ADDED Requirements

### Requirement: Script API version is the server version

The server SHALL expose `emb.API_VERSION` as a string carrying the server's own
version — the repository `VERSION` value injected into the binary — so `emb` has
a single version and the scripted surface reports the server a script is talking
to rather than a second numbering scheme. It SHALL fall back to `dev` when no
build version was injected, matching `INFO`'s `emb_version`. The value SHALL be
folded into reply-cache identity so an upgrade can never serve a reply computed
under an older surface.

#### Scenario: Version is readable

- **WHEN** a script returns `emb.API_VERSION`
- **THEN** the reply is a non-empty string

#### Scenario: Version tracks the server

- **WHEN** the server was built with a version injected from `VERSION`
- **THEN** `emb.API_VERSION` equals that value and equals the `emb_version`
  reported by `INFO`

#### Scenario: Unset build version falls back

- **WHEN** the server runs without an injected build version (`go test`,
  `go run`, an unlabelled build)
- **THEN** `emb.API_VERSION` is `dev`

#### Scenario: A version change invalidates cached replies

- **WHEN** a script reply is cached under one version and the server restarts on
  a different version with the same cache snapshot
- **THEN** the request misses and recomputes

#### Scenario: The server never refuses on version grounds

- **WHEN** a script asserts a capability against `emb.API_VERSION` and the
  capability is absent
- **THEN** the script can report its own error instead of the server refusing to
  evaluate it
