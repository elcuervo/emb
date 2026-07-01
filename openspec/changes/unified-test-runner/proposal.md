## Why

There's no single command to validate the entire project. A developer must run Go tests, switch to `gems/emb/` for Ruby tests, and manually verify the `gems/emb-server` gem can build. A `just all` command runs everything in sequence so CI and local development can validate the full stack with one command.

## What Changes

- Add `all` recipe to `justfile` that:
  1. Runs `just test` (Go server tests)
  2. Builds the `emb` binary
  3. Starts the `emb` server with test config
  4. Runs `gems/emb` RSpec test suite
  5. Validates `gems/emb-server` gem can build
  6. Stops the server

## Capabilities

### New Capabilities
- (none — justfile recipe only)

### Modified Capabilities
- (none)

## Impact

| File | Change |
|------|--------|
| `justfile` | Add `all` recipe that orchestrates Go + Ruby tests and gem build validation |
