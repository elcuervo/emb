# Clients

The response is raw little-endian float32 bytes by default, so any Redis
client works. To avoid decoding raw floats, ask for `VALUES` (decimal
values); under `HELLO 3` the values come back as typed doubles. See
[Commands](commands.md) for both reply formats.

**Ruby (raw RESP2):**

```ruby
require "redis_client"

redis = RedisClient.new(port: 6379)
raw = redis.call("EMB", "minilm", "hello world")
emb = raw.unpack("e*")

# decimal reply (no binary unpack needed):
envelope = redis.call("EMB", "minilm", "VALUES", "hello world")
# => ["dtype", "FLOAT", "shape", [1, 384], "values", ["-0.1974...", ...]]
```

**Python (RESP3, typed doubles):**

```python
import redis
r = redis.Redis(port=6379, protocol=3)  # sends HELLO 3
env = r.execute_command("EMB", "minilm", "VALUES", "hello world")
# env => {b"dtype": b"FLOAT", b"shape": [1, 384], b"values": [-0.1974...]}
```

**Python (binary reply):**

```python
import struct
raw = redis.execute_command("EMB", "minilm", "hello world")
emb = list(struct.unpack(f"<{len(raw)//4}f", raw))
```

**Go:**

```go
var vec []float32
binary.Read(bytes.NewReader(raw), binary.LittleEndian, &vec)
```

## Ruby gem

The [`emb`](../gems/emb/README.md) gem adds connection pooling, a proxy, and
multi-model support with automatic float32 decoding:

```ruby
require "emb"

Emb[:minilm]["hello world"]
# => [0.0123, -0.0456, 0.0789, ...]
```

Images use the same proxy and float32 decode; pass the encoded file bytes
unchanged (they are sent as an `ASCII-8BIT` RESP bulk):

```ruby
Emb[:clip].image(File.binread("cat.jpg"))
# => [0.0123, -0.0456, ...]

# several images: one vector per requested slot, nil for a failed/truncated one
Emb[:clip].image(File.binread("cat.jpg"), File.binread("dog.png"))
# => [[0.01, ...], [0.07, ...]]
```

Gem list:

- [`emb`](https://rubygems.org/gems/emb) — client library ([README](../gems/emb/README.md))
- [`emb-server`](https://rubygems.org/gems/emb-server) — precompiled server binary ([README](../gems/emb-server/README.md))
