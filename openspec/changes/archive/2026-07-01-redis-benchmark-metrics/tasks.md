## 1. Setup

- [x] 1.1 Add `redis-benchmark` check to justfile (brew install redis-benchmark or detect presence)
- [x] 1.2 Deprecate `cmd/emb-bench` with notice recommending `redis-benchmark`

## 2. Justfile Recipes

- [x] 2.1 Add `bench-redis-single` recipe: start server with GOMAXPROCS=1, run `redis-benchmark -n 100000 -q -P 512 -c 512 EMB minilm "hello world"`, stop server
- [x] 2.2 Add `bench-redis-multi` recipe: start server with GOMAXPROCS=0, run `redis-benchmark -n 1000000 -q -P 512 -c 512 EMB minilm "hello world"`, stop server
- [x] 2.3 Add `bench-redis` recipe that runs all benchmark variants

## 3. Documentation

- [x] 3.1 Create BENCHMARK.md with methodology, reproduce commands, and results following tidwall/redcon format
