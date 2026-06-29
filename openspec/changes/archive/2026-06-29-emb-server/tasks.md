## 1. Project Scaffolding

- [x] 1.1 Initialize Go module and create directory structure
- [x] 1.2 Create `flake.nix` with Go, ONNX Runtime C library, and build tooling
- [x] 1.3 Create minimal config.yaml example and config type definitions

## 2. ONNX Runtime + Tokenizer Setup

- [x] 2.1 Implement ONNX environment initialization (load shared library, init environment)
- [x] 2.2 Implement model config loader (parse YAML, validate paths)
- [x] 2.3 Create tokenizer wrapper using pure-Go tokenizer that loads from tokenizer.json
- [x] 2.4 Create session factory that builds `DynamicAdvancedSession` with optimized options (IntraOpNumThreads=1, EnableAll optimization, CPU arena, mem pattern)

## 3. Worker Pool

- [x] 3.1 Implement worker struct holding session, tokenizer, pre-allocated input/output tensors
- [x] 3.2 Implement batch pad logic: pad token IDs within batch to max sequence length
- [x] 3.3 Implement mean pooling: mask-weighted average of last_hidden_state
- [x] 3.4 Implement L2 normalization
- [x] 3.5 Implement worker pool: N goroutines reading from request channel, round-robin distribution, pre-allocated tensor reuse
- [x] 3.6 Implement request struct with texts slice and response channel

## 4. Model Registry

- [x] 4.1 Implement model registry that holds all loaded models and their worker pools
- [x] 4.2 Implement model lookup by name
- [x] 4.3 Wire model loading at startup: config → registry → worker pools

## 5. REDCON Server + EMB Commands

- [x] 5.1 Initialize redcon server with command handler
- [x] 5.2 Implement `EMB <model> <text...>` handler: parse args, dispatch to model registry, write RESP response
- [x] 5.3 Implement `EMB.MODELS` handler: list model names, dims, status
- [x] 5.4 Implement `EMB.INFO <model>` handler: model metadata + request count + avg latency
- [x] 5.5 Implement `EMB.STATS` handler: uptime, total requests, models loaded
- [x] 5.6 Implement `EMB.HELP` handler: usage text
- [x] 5.7 Implement `PING` handler for Redis client compatibility
- [x] 5.8 Wire main.go: parse flags, load config, start server

## 6. Tests

- [x] 6.1 Write unit tests for mean pooling with known inputs
- [x] 6.2 Write unit tests for L2 normalization
- [x] 6.3 Write unit tests for batch pad logic
- [x] 6.4 Write unit tests for model config parsing
- [x] 6.5 Write integration test: start server, connect with RESP client, send EMB commands, verify responses

## 7. Benchmarks

- [x] 7.1 Write benchmark for mean pooling at various batch sizes
- [x] 7.2 Write benchmark for end-to-end embedding (pre-allocated tensors)
- [x] 7.3 Write benchmark for concurrent requests through worker pool
