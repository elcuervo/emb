## Why

`Emb::Client` uses a `Mutex` with double-checked locking for lazy pool initialization. Under CRuby's GIL, `||=` is atomic for this pattern. The mutex adds unnecessary complexity — 4 extra lines, an instance variable, and a synchronized block that serve no practical purpose.

## What Changes

- Remove `@mutex` and all `Mutex` usage from `gems/emb/lib/emb/client.rb`
- Simplify `pool` method to `@pool ||= default_pool`
- Simplify `setup` method to direct assignment

## Impact

| File | Change |
|------|--------|
| `gems/emb/lib/emb/client.rb` | Remove mutex, simplify lazy init to `||=` |
