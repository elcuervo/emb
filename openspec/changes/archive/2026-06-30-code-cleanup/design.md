## Context

The current client.rb uses a `Mutex` for thread-safe lazy initialization. CRuby's GIL already serializes Ruby method calls, making `||=` safe for this pattern (assignment of a reference to an instance variable). The mutex adds visual complexity with no measurable benefit.

## Goals / Non-Goals

**Goals:**
- Remove `@mutex` and all `Mutex` references
- Replace synchronized lazy init with `@pool ||= default_pool`
- Keep identical external behavior

**Non-Goals:**
- Changing `ConnectionPool` or `RedisClient` usage
- Thread-safety beyond what CRuby provides by default

## Decisions

### Decision: Replace mutex with ||=

```ruby
# Before
def pool
  @pool || @mutex.synchronize { @pool ||= default_pool }
end

# After
def pool
  @pool ||= default_pool
end
```

Under CRuby, `@pool ||= default_pool` compiles to:
1. Read `@pool`
2. If nil, call `default_pool`
3. Write `@pool`

The GIL prevents another thread from interleaving between steps 2 and 3. The worst case is two threads both seeing `nil` on step 1, both calling `default_pool`, and one result being discarded. No corruption, no crash — just one extra pool object.

## Risks / Trade-offs

- **JRuby/TruffleRuby** — no GIL, so two threads could both create pools. Rarely used in practice. The extra pool is GC'd, no data corruption.
- **Setup race** — if `setup` is called while commands are running, the old pool's connections leak. Same as before, and doesn't happen in practice.
