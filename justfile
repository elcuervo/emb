default:
    @just --list

ort_lib := `echo "${DYLD_LIBRARY_PATH:-}" | grep -o '/nix/store/[^:]*onnxruntime[^:]*/lib' | head -1`
libtokenizers_dir := "./lib/libtokenizers"
redis_benchmark := `which redis-benchmark 2>/dev/null || echo ""`
image_tag := `cat VERSION 2>/dev/null || echo "dev"`
docker_user := "elcuervo"
image_name := "{{docker_user}}/emb"

# CPU partition for benchmark runs (reference machine: 10 CPUs → 6 app / 4 bench).
# The emb server runs in the app partition (GOMAXPROCS + intra_op_threads budget;
# taskset -c on Linux) and the benchmark tooling (harness, parse-load generator,
# redis-benchmark) runs in the disjoint benchmark partition. Override per run with
# `just bench-ruby app_cpus=4 bench_cpus=6` or set inside a dev shell.
app_cpus := "6"   # CPUs reserved for the server (app partition)
bench_cpus := "4" # CPUs reserved for the benchmark tooling

# Format all Go code with golangci-lint (gofmt + goimports)
format:
    golangci-lint fmt ./...

# Lint all Go code with golangci-lint (staticcheck + govet)
lint:
    golangci-lint run ./...
    go vet ./...

# Run all tests
test:
    go test ./...

# Report production functions no entry point can reach. Names in
# deadcode-allow.txt are intentional test seams, documented there; anything
# else fails the check. Uses the pinned golang.org/x/tools tool dependency.
deadcode:
    @go tool deadcode ./... 2>/tmp/emb-deadcode.err > /tmp/emb-deadcode.out || { echo "✗ deadcode failed:"; cat /tmp/emb-deadcode.err; exit 1; }; \
    unexpected=$(grep -vF -f deadcode-allow.txt /tmp/emb-deadcode.out || true); \
    if [ -n "$unexpected" ]; then \
        echo "✗ unreachable production functions:"; \
        echo "$unexpected"; \
        exit 1; \
    fi; \
    echo "✓ no unreachable production functions outside deadcode-allow.txt"

# Per-package statement coverage plus an overall total (visibility, no gate).
cover:
    @go test -coverprofile=/tmp/emb-cover.out -covermode=atomic ./... 2>/dev/null | grep -E "coverage:" | sort
    @go tool cover -func=/tmp/emb-cover.out | tail -1

# Run all benchmarks (no baseline comparison)
bench:
    go test -bench=. -benchmem ./...

# Capture benchmark baseline
baseline:
    go test -bench=. -benchmem ./... | tee benchmark-baseline.txt

build:
    @mkdir -p bin
    CGO_ENABLED=1 CGO_LDFLAGS="-L{{libtokenizers_dir}}" go build \
        -ldflags="-X main.version={{image_tag}}" -o ./bin/emb ./cmd/emb
    CGO_ENABLED=0 go build \
        -ldflags="-X main.version={{image_tag}}" -o ./bin/emb-top ./cmd/emb-top

# Run all tests: Go server, Ruby client
all: test build
    @echo "=== Testing Ruby client ==="
    @EMB_CMD="./bin/emb -config test-two-models.yaml"; \
    pkill -f "emb -config test-two-models" 2>/dev/null || true; \
    if [ -n "{{ort_lib}}" ]; then \
        DYLD_LIBRARY_PATH="{{ort_lib}}:$DYLD_LIBRARY_PATH" $EMB_CMD >/tmp/emb-all-server.log 2>&1 & \
    else \
        $EMB_CMD >/tmp/emb-all-server.log 2>&1 & \
    fi; \
    echo $! > /tmp/emb-all.pid; \
    sleep 8
    cd gems/emb && bundle exec rake
    @echo "=== All tests passed ==="
    -kill `cat /tmp/emb-all.pid` 2>/dev/null
    rm -f /tmp/emb-all.pid /tmp/emb-all-server.log

# Build, install, and validate both gems locally
validate-gems: build
    @echo "=== Validating emb gem ==="
    cd gems/emb && gem build emb.gemspec && gem install --local emb-*.gem --no-doc && ruby -e "require 'emb'; puts \"emb gem: #{Emb::VERSION}\""
    @echo "=== Validating emb-server gem ==="
    cp bin/emb gems/emb-server/lib/emb-server/emb-binary-arm64-darwin
    cp bin/emb-top gems/emb-server/lib/emb-server/emb-top-binary-arm64-darwin
    cd gems/emb-server && gem build emb-server.gemspec && gem install --local emb-server-*.gem --no-doc 2>/dev/null
    @which emb || echo "WARNING: emb not on PATH (check GEM_HOME/bin)"
    @which emb-top || echo "WARNING: emb-top not on PATH (check GEM_HOME/bin)"
    rm gems/emb-server/lib/emb-server/emb-binary-arm64-darwin
    rm gems/emb-server/lib/emb-server/emb-top-binary-arm64-darwin
    @echo "=== Both gems valid ==="

# Build and run the server
dev: download-libtokenizers build
    @if [ -z "{{ort_lib}}" ]; then \
        echo "WARNING: onnxruntime not found in DYLD_LIBRARY_PATH."; \
        echo "Run 'nix develop' first, or set DYLD_LIBRARY_PATH manually."; \
        echo "Falling back to system library paths..."; \
        ./bin/emb -config config.yaml; \
    else \
        DYLD_LIBRARY_PATH="{{ort_lib}}:$DYLD_LIBRARY_PATH" ./bin/emb -config config.yaml; \
    fi

# Launch an IRB console with the emb gem loaded
console:
    @cd gems/emb && bundle exec rake console

# Run the kitchensink example: an end-to-end vector application that embeds a
# corpus with emb and searches it with Redis vector sets (see
# examples/kitchensink/README.md).
#
#   just kitchensink index README.md DESIGN.md
#   just kitchensink stats
#   just kitchensink stop
#
# Arguments are forwarded through the shell, so one containing spaces needs its
# own quotes:
#
#   just kitchensink search '"how does batching work"' 3
kitchensink *ARGS:
    @examples/kitchensink/run.sh {{ARGS}}

# Download libtokenizers.a for the current platform
# Uses the pre-built release from daulet/tokenizers
libtokenizers-version := `grep '^TOKENIZERS_VERSION=' .github/versions.env 2>/dev/null | cut -d= -f2 || echo "v1.27.0"`

download-libtokenizers:
    @mkdir -p {{libtokenizers_dir}}; \
    if [ -f {{libtokenizers_dir}}/libtokenizers.a ]; then \
        echo "✓ libtokenizers.a already exists"; \
        exit 0; \
    fi; \
    echo "Downloading libtokenizers.a ({{libtokenizers-version}})..." && \
    case "$(uname -s),$(uname -m)" in \
        Darwin,arm64)  ARCH="darwin-arm64" ;; \
        Darwin,x86_64) ARCH="darwin-x86_64" ;; \
        Linux,aarch64) ARCH="linux-aarch64" ;; \
        Linux,x86_64)  ARCH="linux-x86_64" ;; \
        *) echo "unsupported platform: $(uname -s)-$(uname -m)"; exit 1 ;; \
    esac && \
    curl -fsSL "https://github.com/daulet/tokenizers/releases/download/{{libtokenizers-version}}/libtokenizers.${ARCH}.tar.gz" \
      -o /tmp/libtokenizers.tar.gz && \
    tar xzf /tmp/libtokenizers.tar.gz -C {{libtokenizers_dir}} && \
    rm /tmp/libtokenizers.tar.gz && \
    echo "✓ Downloaded libtokenizers.a ($ARCH)"

# Download a model from HuggingFace
# Usage: just download-model [huggingface_repo] [output_dir]
# Fills in whatever is missing; a directory with only some of the files is
# completed rather than reported as done.
download-model repo="Xenova/all-MiniLM-L6-v2" dir="./models/minilm":
    @mkdir -p {{dir}}
    @if [ -f "{{dir}}/model.onnx" ] && [ "$(wc -c < '{{dir}}/model.onnx')" -gt 100 ] && [ -f "{{dir}}/tokenizer.json" ] && [ -f "{{dir}}/config.json" ]; then \
        echo "✓ Already exists at {{dir}}"; \
        exit 0; \
    fi; \
    if [ -f "{{dir}}/model.onnx" ] && [ "$(wc -c < '{{dir}}/model.onnx')" -gt 100 ]; then \
        echo "✓ model.onnx already exists at {{dir}}"; \
    else \
        echo "Downloading {{repo}}..."; \
        curl -sL "https://huggingface.co/{{repo}}/resolve/main/model.onnx" -o "{{dir}}/model.onnx"; \
        if [ -f "{{dir}}/model.onnx" ] && [ "$(wc -c < '{{dir}}/model.onnx')" -gt 100 ]; then \
            echo "  model.onnx (root)"; \
        else \
            curl -sL "https://huggingface.co/{{repo}}/resolve/main/onnx/model.onnx" -o "{{dir}}/model.onnx" && echo "  model.onnx (onnx/)"; \
        fi; \
    fi; \
    if [ -s "{{dir}}/tokenizer.json" ]; then echo "✓ tokenizer.json already exists"; else curl -sL "https://huggingface.co/{{repo}}/resolve/main/tokenizer.json" -o "{{dir}}/tokenizer.json" && echo "  tokenizer.json"; fi; \
    if [ -s "{{dir}}/config.json" ]; then echo "✓ config.json already exists"; else curl -sL "https://huggingface.co/{{repo}}/resolve/main/config.json" -o "{{dir}}/config.json" && echo "  config.json"; fi; \
    curl -fsSL "https://huggingface.co/{{repo}}/resolve/main/preprocessor_config.json" -o "{{dir}}/preprocessor_config.json" && echo "  preprocessor_config.json" || rm -f "{{dir}}/preprocessor_config.json"

# Download a vision export for EMB.IMG: a SigLIP2/CLIP ONNX vision model plus
# its preprocessor_config.json (mean/std/rescale/size/crop/resample).
# Usage: just download-vision-model [huggingface_repo] [output_dir]
download-vision-model repo="onnx-community/siglip2-base-patch16-224-ONNX" dir="./models/siglip2-vision":
    @mkdir -p {{dir}}
    @for f in onnx/vision_model.onnx config.json preprocessor_config.json tokenizer.json; do \
        out="{{dir}}/$(basename $f)"; \
        if [ -f "$out" ] && [ "$(wc -c < "$out")" -gt 100 ]; then echo "✓ $f (exists)"; \
        else curl -fsSL "https://huggingface.co/{{repo}}/resolve/main/$f" -o "$out" && echo "✓ $f" || { rm -f "$out"; echo "failed to download $f" >&2; exit 1; }; fi; \
    done

# Download the GLiNER2 scripted-model testbed (cuerbot/gliner2-multi-v1 int8)
# Used by the gated script-eval/GLiNER tests; ~380MB. Skips files that are
# already present and fails on HTTP errors (curl -f), mirroring download-model.
download-gliner-model:
    @mkdir -p ./models/gliner2
    @for f in model_int8.onnx tokenizer.json config.json; do \
        if [ -f "./models/gliner2/$f" ] && [ "$(wc -c < "./models/gliner2/$f")" -gt 100 ]; then \
            echo "✓ $f (exists)"; \
        else \
            curl -fsSL "https://huggingface.co/cuerbot/gliner2-multi-v1/resolve/main/$f" -o "./models/gliner2/$f" && echo "✓ $f" || { rm -f "./models/gliner2/$f"; echo "failed to download $f" >&2; exit 1; }; \
        fi; \
    done

# GLiNER extraction benchmarks against the real int8 model (Apple sentence
# material; requires: just download-gliner-model)
bench-gliner intra="4":
    @EMB_BENCH_INTRA={{intra}} go test ./internal/script/ -bench=BenchmarkGLiNERExtract -benchtime=5x -run=^$

# Scripted-inference benchmarks for the production-scripting change
# (requires: just download-model)
bench-script:
    @go test ./internal/server/ -bench="BenchmarkScript" -benchmem -run=^$ -benchtime=20x

# Enforce the scripted-inference budgets (latency parity, metal parity,
# materialization, memory, throughput scaling). Timing- and RSS-sensitive: run
# on a quiet reference machine. Set EMB_BENCH_REFERENCE=1 to also assert the
# absolute throughput-scaling target (0.85 x N); on shared hosts the recorded
# baseline ratio is the gate.
bench-budgets:
    @EMB_BENCH_BUDGETS=1 go test ./internal/server/ -run "Budget" -v -timeout 900s

# Run redis-benchmark with a single-threaded server
# Uses 1 client, 1 pipeline, 500 requests (~2s at 280 req/s)
# Requires: redis-benchmark, downloaded model at ./models/minilm
bench-redis-single: build
    @if [ "{{redis_benchmark}}" = "" ]; then echo "ERROR: redis-benchmark not found. Install: brew install redis"; exit 1; fi
    @aff() { [ "$(uname -s)" = "Linux" ] && command -v taskset >/dev/null 2>&1 && echo "taskset -c 0-$(( {{app_cpus}} - 1 ))"; }; echo "Starting server (app partition: $(aff) GOMAXPROCS=1)..."; DYLD_LIBRARY_PATH="{{ort_lib}}:$DYLD_LIBRARY_PATH" GOMAXPROCS=1 $(aff) ./bin/emb -config config.yaml & echo $! > /tmp/emb-srv.pid
    sleep 10
    @bench() { [ "$(uname -s)" = "Linux" ] && command -v taskset >/dev/null 2>&1 && echo "taskset -c {{app_cpus}}-$(( {{app_cpus}} + {{bench_cpus}} - 1 ))"; }; echo "Running: redis-benchmark (bench partition: $(bench)) -p 6379 -q -c 1 -P 1 -n 500 EMB minilm hello world"; $(bench) {{redis_benchmark}} -p 6379 -q -c 1 -P 1 -n 500 EMB minilm hello world
    -kill `cat /tmp/emb-srv.pid` 2>/dev/null
    rm -f /tmp/emb-srv.pid

# Run redis-benchmark with a multi-threaded server
# Uses 16 clients, 1 pipeline, 2000 requests (~5s at 400 req/s)
# Requires: redis-benchmark, downloaded model at ./models/minilm
bench-redis-multi: build
    @if [ "{{redis_benchmark}}" = "" ]; then echo "ERROR: redis-benchmark not found. Install: brew install redis"; exit 1; fi
    @aff() { [ "$(uname -s)" = "Linux" ] && command -v taskset >/dev/null 2>&1 && echo "taskset -c 0-$(( {{app_cpus}} - 1 ))"; }; echo "Starting server (app partition: $(aff) GOMAXPROCS=0)..."; DYLD_LIBRARY_PATH="{{ort_lib}}:$DYLD_LIBRARY_PATH" GOMAXPROCS=0 $(aff) ./bin/emb -config config.yaml & echo $! > /tmp/emb-srv.pid
    sleep 10
    @bench() { [ "$(uname -s)" = "Linux" ] && command -v taskset >/dev/null 2>&1 && echo "taskset -c {{app_cpus}}-$(( {{app_cpus}} + {{bench_cpus}} - 1 ))"; }; echo "Running: redis-benchmark (bench partition: $(bench)) -p 6379 -q -c 16 -P 1 -n 2000 EMB minilm hello world"; $(bench) {{redis_benchmark}} -p 6379 -q -c 16 -P 1 -n 2000 EMB minilm hello world
    -kill `cat /tmp/emb-srv.pid` 2>/dev/null
    rm -f /tmp/emb-srv.pid

# Run redis-benchmark with cache enabled (cache hit benchmark)
# Uses 1 client, 1 pipeline, 500 requests with same text (all cache hits after first)
bench-cache: build
    @if [ "{{redis_benchmark}}" = "" ]; then echo "ERROR: redis-benchmark not found. Install: brew install redis"; exit 1; fi
    @echo "Starting server with cache (GOMAXPROCS=0)..."
    DYLD_LIBRARY_PATH="{{ort_lib}}:$DYLD_LIBRARY_PATH" GOMAXPROCS=0 ./bin/emb -config config.yaml -cache auto & echo $! > /tmp/emb-srv.pid
    sleep 10
    @echo "Warming cache with first request..."
    @{{redis_benchmark}} -p 6379 -q -c 1 -P 1 -n 1 EMB minilm "hello world" > /dev/null 2>&1
    @echo "Running: redis-benchmark -p 6379 -q -c 1 -P 1 -n 500 EMB minilm hello world"
    {{redis_benchmark}} -p 6379 -q -c 1 -P 1 -n 500 EMB minilm "hello world"
    -kill `cat /tmp/emb-srv.pid` 2>/dev/null
    rm -f /tmp/emb-srv.pid

# Run redis-benchmark with cache enabled and specified cache size
# Usage: just bench-cache-size size="256MB"
bench-cache-size size="auto":
    @if [ "{{redis_benchmark}}" = "" ]; then echo "ERROR: redis-benchmark not found. Install: brew install redis"; exit 1; fi
    @echo "Starting server with cache size {{size}} (GOMAXPROCS=0)..."
    DYLD_LIBRARY_PATH="{{ort_lib}}:$DYLD_LIBRARY_PATH" GOMAXPROCS=0 ./bin/emb -config config.yaml -cache {{size}} & echo $! > /tmp/emb-srv.pid
    sleep 10
    @{{redis_benchmark}} -p 6379 -q -c 1 -P 1 -n 1 EMB minilm "hello world" > /dev/null 2>&1
    @echo "Running: redis-benchmark -p 6379 -q -c 1 -P 1 -n 500 EMB minilm hello world"
    {{redis_benchmark}} -p 6379 -q -c 1 -P 1 -n 500 EMB minilm "hello world"
    -kill `cat /tmp/emb-srv.pid` 2>/dev/null
    rm -f /tmp/emb-srv.pid

# Run all redis-benchmark variants (single-threaded + multi-threaded + cache)
bench-redis: bench-redis-single bench-redis-multi

# Run the Ruby client benchmark harness under a CPU partition
# (eager/lazy/pipelined/threaded + round-trip check + stability gate).
#
# The server starts in the app partition (GOMAXPROCS={{app_cpus}} + the config's
# intra_op_threads; real pinning via `taskset -c` on Linux). The harness and its
# parse-load generator run in the disjoint benchmark partition ({{bench_cpus}} CPUs).
# On macOS (no taskset) the server is bounded by GOMAXPROCS and the tooling runs
# unconstrained — see BENCHMARK.md for the partition layout.
#
# Requires: emb server model at ./models, Ruby gem deps installed
bench-ruby config="bench-cpu-partition.yaml":
    @aff() { [ "$(uname -s)" = "Linux" ] && command -v taskset >/dev/null 2>&1 && echo "taskset -c 0-$(( {{app_cpus}} - 1 ))"; }; \
    echo "Starting emb server in app partition: $(aff) GOMAXPROCS={{app_cpus}} (config={{config}})"; \
    DYLD_LIBRARY_PATH="{{ort_lib}}:$DYLD_LIBRARY_PATH" GOMAXPROCS={{app_cpus}} $(aff) ./bin/emb -config {{config}} & echo $! > /tmp/emb-bench.pid; \
    sleep 2; \
    until redis-cli -p 16379 ping >/dev/null 2>&1; do sleep 1; done; \
    bench() { [ "$(uname -s)" = "Linux" ] && command -v taskset >/dev/null 2>&1 && echo "taskset -c {{app_cpus}}-$(( {{app_cpus}} + {{bench_cpus}} - 1 ))"; }; \
    echo "Server ready — running client harness (benchmark partition: $(bench), EMB_BENCH_APP_CPUS={{app_cpus}} EMB_BENCH_BENCH_CPUS={{bench_cpus}})"; \
    (cd gems/emb && EMB_BENCH_APP_CPUS={{app_cpus}} EMB_BENCH_BENCH_CPUS={{bench_cpus}} $(bench) bundle exec ruby bench/bench.rb); \
    status=$?; \
    kill `cat /tmp/emb-bench.pid` 2>/dev/null; \
    rm -f /tmp/emb-bench.pid; \
    exit $status

# Run the Ruby client harness against TWO emb instances (url-array mechanisms:
# eager-2node distribution and batch-2node concurrent fan-out). Splits the app
# partition in half — node 0 on :16379, node 1 on :16380, each with
# GOMAXPROCS=app_cpus/2 — and keeps the bench partition for the harness.
bench-ruby-multi config="bench-cpu-partition.yaml":
    @half=$(expr {{app_cpus}} / 2); \
    [ "$half" -ge 1 ] || { echo "ERROR: app_cpus must be >= 2 for a two-node run"; exit 1; }; \
    tmp=$(mktemp -d /tmp/emb-bench-multi.XXXXXX); \
    cleanup() { kill `cat "$tmp/pid0"` `cat "$tmp/pid1"` 2>/dev/null; rm -rf "$tmp"; }; \
    trap cleanup EXIT; \
    aff0() { [ "$(uname -s)" = "Linux" ] && command -v taskset >/dev/null 2>&1 && echo "taskset -c 0-$(expr $half - 1)"; }; \
    aff1() { [ "$(uname -s)" = "Linux" ] && command -v taskset >/dev/null 2>&1 && echo "taskset -c $half-$(expr {{app_cpus}} - 1)"; }; \
    sed 's/^listen:.*/listen: ":16380"/' {{config}} > "$tmp/node2.yaml"; \
    echo "Starting emb node 0 on :16379 (GOMAXPROCS=$half $(aff0)) and node 1 on :16380 (GOMAXPROCS=$half $(aff1))"; \
    DYLD_LIBRARY_PATH="{{ort_lib}}:$DYLD_LIBRARY_PATH" GOMAXPROCS=$half $(aff0) ./bin/emb -config {{config}} & echo $! > "$tmp/pid0"; \
    DYLD_LIBRARY_PATH="{{ort_lib}}:$DYLD_LIBRARY_PATH" GOMAXPROCS=$half $(aff1) ./bin/emb -config "$tmp/node2.yaml" & echo $! > "$tmp/pid1"; \
    sleep 2; \
    deadline=$(( $(date +%s) + 60 )); \
    until redis-cli -p 16379 ping >/dev/null 2>&1 && redis-cli -p 16380 ping >/dev/null 2>&1; do \
        kill -0 "$(cat "$tmp/pid0")" 2>/dev/null && kill -0 "$(cat "$tmp/pid1")" 2>/dev/null || { echo "ERROR: an EMB node exited during startup"; exit 1; }; \
        [ "$(date +%s)" -lt "$deadline" ] || { echo "ERROR: EMB nodes did not become ready within 60 seconds"; exit 1; }; \
        sleep 1; \
    done; \
    bench() { [ "$(uname -s)" = "Linux" ] && command -v taskset >/dev/null 2>&1 && echo "taskset -c {{app_cpus}}-$(expr {{app_cpus}} + {{bench_cpus}} - 1)"; }; \
    echo "Both nodes ready — running client harness (benchmark partition: $(bench), EMB_BENCH_PORT2=16380)"; \
    (cd gems/emb && EMB_BENCH_PORT2=16380 EMB_BENCH_APP_CPUS={{app_cpus}} EMB_BENCH_BENCH_CPUS={{bench_cpus}} $(bench) bundle exec ruby bench/bench.rb); \
    status=$?; \
    exit $status

# Run all benchmarks
bench-all: bench-redis bench-cache

# Fargate-shaped benchmark harness (linux/arm64, Graviton CPU). Requires Docker
# and a model at ./models; redis-benchmark/redis-cli/ruby run from `nix develop`.
# Emits bench/fargate/out/run.<sha>.<ts>.json
bench-fargate:
    go run ./bench/fargate -mode run

# Capture the golden Fargate baseline JSON (bench/fargate/baseline.<sha>.json)
bench-fargate-baseline:
    go run ./bench/fargate -mode baseline

# Diff two result/baseline JSON files per-cell with PASS/FAIL tolerances
bench-fargate-diff before after:
    go run ./bench/fargate -mode diff -before {{before}} -after {{after}}

# Verify embeddings match Python reference (requires downloaded model)
verify-embeddings: build
    @if [ ! -f ./models/minilm/model.onnx ]; then \
        echo "ERROR: ./models/minilm/model.onnx not found — run 'just download-model' first"; \
        exit 1; \
    fi
    @echo "Generating reference embeddings..."
    @if [ ! -f reference-embeddings.json ]; then \
        python3 -m venv /tmp/emb-verify-venv; \
        . /tmp/emb-verify-venv/bin/activate; \
        pip install -q sentence-transformers torch --extra-index-url https://download.pytorch.org/whl/cpu; \
        python3 cmd/emb-verify/generate-reference.py; \
        rm -rf /tmp/emb-verify-venv; \
    else echo "✓ reference-embeddings.json exists (use 'python3 cmd/emb-verify/generate-reference.py --refresh' to regenerate)"; fi
    @echo "Starting server..."
    DYLD_LIBRARY_PATH="{{ort_lib}}:$DYLD_LIBRARY_PATH" ./bin/emb -config config.yaml & echo $! > /tmp/emb-srv.pid
    @deadline=$(( $(date +%s) + 60 )); until redis-cli -p 6379 ping >/dev/null 2>&1; do \
        kill -0 `cat /tmp/emb-srv.pid` 2>/dev/null || { echo "ERROR: emb exited during startup"; exit 1; }; \
        [ $(date +%s) -lt $deadline ] || { echo "ERROR: emb not ready within 60s"; exit 1; }; \
        sleep 1; \
    done
    @echo "Running verification..."
    CGO_ENABLED=0 go run ./cmd/emb-verify -model minilm -reference reference-embeddings.json
    -kill `cat /tmp/emb-srv.pid` 2>/dev/null
    rm -f /tmp/emb-srv.pid

# Test EMB.MULTI byte-equality against sequential EMB across two models.
# Reuses test-two-models.yaml (minilm + bge, auto-downloaded on first start on
# 127.0.0.1:16379).
# Usage: just verify-emb-multi
verify-emb-multi: build
    @echo "Starting server with two models (test-two-models.yaml)..."
    DYLD_LIBRARY_PATH="{{ort_lib}}:$DYLD_LIBRARY_PATH" ./bin/emb -config test-two-models.yaml & echo $! > /tmp/emb-srv.pid
    @deadline=$(( $(date +%s) + 180 )); until redis-cli -p 16379 ping >/dev/null 2>&1; do \
        kill -0 `cat /tmp/emb-srv.pid` 2>/dev/null || { echo "ERROR: emb exited during startup"; exit 1; }; \
        [ $(date +%s) -lt $deadline ] || { echo "ERROR: emb not ready within 180s"; exit 1; }; \
        sleep 1; \
    done
    @echo "Running EMB.MULTI verification..."
    CGO_ENABLED=0 go run ./cmd/emb-multi-verify -addr 127.0.0.1:16379 -model-a minilm -model-b bge -dim-a 384 -dim-b 384
    -kill `cat /tmp/emb-srv.pid` 2>/dev/null
    rm -f /tmp/emb-srv.pid

# Unit-test the shared verification harness with no server, model, or ONNX.
verify-harness:
    CGO_ENABLED=0 go test -count=1 ./internal/resp/... ./internal/embverify/... ./cmd/emb-verify/ ./cmd/emb-multi-verify/

# Build Docker image (native platform)
docker:
    @echo "Building {{image_name}}:{{image_tag}} for $(shell uname -m)..."
    docker buildx build \
        --load \
        --build-arg EMB_VERSION={{image_tag}} \
        -t {{image_name}}:{{image_tag}} \
        -t {{image_name}}:latest \
        .

# Build and push multi-arch Docker image to Docker Hub
docker-push:
    @echo "Building and pushing {{image_name}}:{{image_tag}} for linux/amd64,linux/arm64..."
    docker buildx build \
        --platform linux/amd64,linux/arm64 \
        --push \
        --build-arg EMB_VERSION={{image_tag}} \
        -t {{image_name}}:{{image_tag}} \
        -t {{image_name}}:latest \
        .

# Build Linux binaries using Docker builder (extract from builder stage)
build-linux archx="linux/amd64":
    @echo "Building for {{archx}} using Docker builder..."
    docker buildx build \
        --platform {{archx}} \
        --output type=local,dest=./dist/emb_linux_$(shell echo {{archx}} | tr / _) \
        .

# Tag current HEAD with the version from VERSION file
tag:
	git tag -a "v$(cat VERSION)" -m "v$(cat VERSION)"

# Release: tag and push to remote
release: tag
	git push origin "v$(cat VERSION)"

# Bump version: edit VERSION with $EDITOR, then rewrite every copy of it
# (site stamps, PRODUCT.md, both gem lockfiles) and fail if any copy is stale.
version:
	$EDITOR VERSION
	just website-version
	cd gems/emb && bundle
	cd gems/emb-server && bundle
	just website-version-check

# Serve the static product site (website/) at http://localhost:8080
website port="8080":
    python3 -m http.server {{port}} --directory website

# Serve the site AND the sandbox behind its console, for testing the live panel
# locally instead of against the deployed cli.emb.is.
#
# Three processes, one terminal:
#   * emb         on 127.0.0.1:{{upstream}}, with a generated config whose model
#                 paths point at ./models (the sandbox config's own paths are
#                 the deployment's /data/models)
#   * the bridge  on {{bind}}:{{bridge}}, reading that same config for its preset
#                 digest manifest
#   * the site    on {{bind}}:{{port}}, with the console's module origin rewritten
#                 from https://cli.emb.is to this machine's own address
#
# Both the site and the bridge bind 0.0.0.0, so a phone on the same network can
# open http://<your-lan-ip>:{{port}} and get the same live console: the module
# origin is derived per request from the address the browser used, and the
# bridge accepts that same host on its own port. emb stays on loopback.
#
# Ctrl-C stops all three. `just website` still serves the published tree
# untouched — use it for the ink probe, which must measure what ships.
#
#   just website-dev                             # site :8080, bridge :8081, emb :6379
#   just website-dev port=9000 bridge=9001 upstream=16399
#   just website-dev bind=127.0.0.1              # this machine only
website-dev port="8080" bridge="8081" upstream="6379" bind="0.0.0.0": sandbox-build
    @set -eu; \
    cfg=website/repl/.sandbox-dev.yaml; \
    sed -e 's|^listen: .*|listen: "127.0.0.1:{{upstream}}"|' \
        -e "s|/data/models|$PWD/models|" \
        website/repl/sandbox.yaml > "$cfg"; \
    echo "website-dev: emb 127.0.0.1:{{upstream}} · bridge {{bind}}:{{bridge}} · site http://localhost:{{port}} (and http://<lan-ip>:{{port}})"; \
    emb=; repl=; \
    cleanup() { \
        if [ -n "$emb" ]; then kill "$emb" 2>/dev/null || true; fi; \
        if [ -n "$repl" ]; then kill "$repl" 2>/dev/null || true; fi; \
        rm -f "$cfg"; \
    }; \
    trap cleanup EXIT INT TERM; \
    DYLD_LIBRARY_PATH="{{ort_lib}}:$DYLD_LIBRARY_PATH" ./bin/emb -config "$cfg" & emb=$!; \
    ./bin/repl -listen {{bind}}:{{bridge}} -upstream 127.0.0.1:{{upstream}} \
        -config "$cfg" -emb-top ./bin/emb-top & repl=$!; \
    python3 website/tools/dev-server.py {{port}} --bind {{bind}} --sandbox-port {{bridge}}

# One-time browser fetch for the site's checks (needs `nix develop .#website`).
# agent-browser drives Chrome for Testing; nixpkgs ships the CLI only. Set
# AGENT_BROWSER_EXECUTABLE_PATH to an existing Chromium to skip the download.
website-browser:
    agent-browser install
    agent-browser doctor --offline --quick

# Full-page screenshot of the served site (`just website` first). Pass
# viewport="390x844" for a phone-width check; agent-browser takes the viewport
# as a launch flag, so it belongs on the open.
#
# The scroll to the end and back is load-bearing, not decoration: reveals are
# driven by element position, and a full-page capture otherwise lays the
# document out without ever moving the trigger line, so everything below the
# fold is captured at opacity 0. Scrolling once fires the sweep, then the
# capture is true. See website/CORRECTIONS.md pass 7.
website-shot url="http://localhost:8080" out="/tmp/emb-site.png" viewport="":
    agent-browser open {{url}} {{ if viewport != "" { "--viewport " + viewport } else { "" } }} && agent-browser scroll to end && agent-browser scroll to top && agent-browser screenshot --full {{out}}
    @echo "wrote {{out}}"

# Assert that no text ink crosses the viewport at any supported width. Opens
# website/tools/ink-probe.html, which loads a surface in a same-origin iframe at
# 24 widths and reports PASS/FAIL. Needs a served site: `just website` first.
#
# Sweeps the landing by default. The second argument selects another surface,
# positionally (just 1.54 reads `<name>=<value>` as the argument itself, so
# named args do not work here):
#
#     just website-ink                          # the landing
#     just website-ink http://localhost:8080 docs   # the documentation surface
#     just website-ink http://localhost:8080 404    # the not-found page
#
# The wait is not optional: the probe measures all 24 widths asynchronously, and
# evaluating before `window.__inkProbe` exists reports "RUNNING…". The assertion
# is `window.__inkProbe.assert()`, defined in the probe, and it throws on FAIL so
# this recipe exits non-zero — reading the verdict text alone would exit 0 on a
# failing page.
#
# The `?cb=` on the probe and inside the probe's own iframe are both load-bearing:
# `python3 -m http.server` sends no `Cache-Control`, so Chrome caches heuristically
# and a second run can measure the previous probe or the previous landing. Both
# used a constant bust once, which reported failures at 390px and 320px against
# HTML that had already changed. See the caching trap in `website/README.md`.
#
# This is the check that `scrollWidth === clientWidth` cannot replace: the page
# frame applies `overflow-x: clip`, which removes clipped content from the
# scrollable region, so the scroll-width comparison reports success even while
# glyphs are being sliced. Twelve correction passes used it and missed a real
# defect at the design's own 1086px reference frame.
website-ink url="http://localhost:8080" target="":
    agent-browser open "{{url}}/tools/ink-probe.html?cb=$(date +%s){{ if target != "" { "&target=" + target } else { "" } }}"
    agent-browser wait --fn "window.__inkProbe" --timeout 120000
    agent-browser eval "window.__inkProbe.assert()"

# Re-vendor the asciinema player the landing plate replays the take with.
#
# The player is not packaged in nixpkgs, so its release is pinned in `flake.nix`
# as an npm tarball and copied verbatim out of it here, into two committed files
# the site links by name. The site has no build step: these bytes ship, and the
# `check` variant re-derives them without writing, so a version bump is verified
# rather than trusted.
#
# Only two files of the package are taken -- the monolithic bundle, which embeds
# the terminal emulator, so the plate needs no worker and no second request.
#
# Run inside `nix develop`, which is what exports `$ASCIIINEMA_PLAYER_TARBALL`.
website-player:
    @set -eu; \
    ver="${ASCIIINEMA_PLAYER_VERSION:?run inside 'nix develop'}"; \
    tmp=$(mktemp -d /tmp/asciinema-player.XXXXXX); \
    trap 'rm -rf "$tmp"' EXIT; \
    tar xzf "$ASCIIINEMA_PLAYER_TARBALL" -C "$tmp"; \
    cp "$tmp/package/dist/bundle/asciinema-player.min.js" "website/assets/js/asciinema-player-$ver.min.js"; \
    cp "$tmp/package/dist/bundle/asciinema-player.css" "website/assets/css/asciinema-player-$ver.css"; \
    python3 -c 'import hashlib, sys; [print(hashlib.sha256(open(p, "rb").read()).hexdigest(), p) for p in sys.argv[1:]]' \
      "website/assets/js/asciinema-player-$ver.min.js" "website/assets/css/asciinema-player-$ver.css"

# Assert the committed player is exactly the pinned release, byte for byte.
website-player-check:
    @set -eu; \
    ver="${ASCIIINEMA_PLAYER_VERSION:?run inside 'nix develop'}"; \
    tmp=$(mktemp -d /tmp/asciinema-player.XXXXXX); \
    trap 'rm -rf "$tmp"' EXIT; \
    tar xzf "$ASCIIINEMA_PLAYER_TARBALL" -C "$tmp"; \
    cmp "$tmp/package/dist/bundle/asciinema-player.min.js" "website/assets/js/asciinema-player-$ver.min.js"; \
    cmp "$tmp/package/dist/bundle/asciinema-player.css" "website/assets/css/asciinema-player-$ver.css"; \
    echo "website-player-check: the vendored player is asciinema-player $ver, byte for byte"

# Assert that the set of files this folder publishes is the set we mean.
#
# Record the site's emb-top plate and the documentation's capture from a real
# run (see website/tools/topviz/README.md and
# openspec/changes/website-emb-top-recording).
#
# Builds the binaries, starts one node on :16379 with the models in
# models.yaml, waits for EMB.READY, records the dashboard headlessly against a
# scripted load, and publishes three things from that one take: the trimmed
# take itself into the landing page's plate, replayed there as text by the
# vendored player (see `just website-player`); the dashboard's own text frame
# into the same plate as its still state -- what a reader with scripting off or
# a reduced-motion preference gets; and an animated GIF of the same run into
# docs/assets/ for docs/operations.md. The raw recording, the trims, the
# frame, the traffic log and a machine-readable sample log of the same run land
# in website/tools/topviz/runs/ -- unserved, and the only way to check what the
# artifacts show.
#
# The documented capture is rendered at font-size 12: the frame comes out 881px
# wide, near enough to the width docs/operations.md is read at that GitHub shows
# it about 1:1, and it is ~100 KiB smaller than a 14px render that would only be
# downscaled to the same apparent size.
#
# Needs the full dev shell: it builds emb from Go+CGo, drives load with
# redis-cli, and takes its recorder from the website half. Nothing else in the
# site build depends on any of it.
website-topviz: build
    @set -eu; \
    cfg=website/tools/topviz/models.yaml; \
    runs=website/tools/topviz/runs; \
    port=16379; \
    theme='111110,F3F0E8,111110,A8442A,6E8B7B,B08C4F,FF5A1F,B4736A,8C8880,F3F0E8,6B6963,C23D00,7FA37A,C9A227,6B7F8C,C9C4B8,8FA9A0,F3F0E8'; \
    for tool in redis-cli asciinema agg; do \
      command -v $tool >/dev/null 2>&1 || { echo "website-topviz: $tool not found - run inside 'nix develop'"; exit 1; }; \
    done; \
    if [ -z "{{ort_lib}}" ]; then echo "website-topviz: onnxruntime is not on the library path - run inside 'nix develop'"; exit 1; fi; \
    for onnx in $(grep -E '^[[:space:]]+onnx:' $cfg | awk '{print $2}'); do \
      if [ ! -f "$onnx" ]; then \
        echo "website-topviz: missing $onnx"; \
        echo "  fetch it with: just download-model <repo> $(dirname $onnx)"; \
        exit 1; \
      fi; \
    done; \
    if redis-cli -p $port ping >/dev/null 2>&1; then echo "website-topviz: something already answers on :$port"; exit 1; fi; \
    mkdir -p $runs; \
    tmp=$(mktemp -d /tmp/emb-topviz.XXXXXX); \
    trap 'kill $(cat $runs/node.pid) 2>/dev/null || true; rm -rf $tmp' EXIT; \
    echo "website-topviz: starting the node"; \
    ./bin/emb -config $cfg > $runs/node.log 2>&1 & echo $! > $runs/node.pid; \
    deadline=$(( $(date +%s) + 300 )); \
    until redis-cli -p $port EMB.READY 2>/dev/null | grep -q OK; do \
      kill -0 $(cat $runs/node.pid) 2>/dev/null || { echo "website-topviz: emb exited during startup"; tail -20 $runs/node.log; exit 1; }; \
      if [ $(date +%s) -ge $deadline ]; then echo "website-topviz: not ready within 300s"; tail -20 $runs/node.log; exit 1; fi; \
      sleep 1; \
    done; \
    echo "website-topviz: recording (about a minute)"; \
    ./bin/emb-top -addr 127.0.0.1:$port -once -samples 44 -interval 1s > $runs/samples.txt 2>&1 & sampler=$!; \
    EMB_TOPVIS_LOG=$runs/traffic.log asciinema record --headless --quiet --overwrite --window-size 120x32 \
      -c website/tools/topviz/run.sh $runs/raw.cast; \
    kill $sampler 2>/dev/null || true; \
    python3 website/tools/topviz/trim.py $runs/raw.cast $runs/take.cast; \
    python3 website/tools/topviz/trim.py --until-pct 65 $runs/raw.cast $runs/frame.cast; \
    asciinema convert -f txt --overwrite $runs/frame.cast $runs/frame.txt >/dev/null; \
    agg --quiet --theme "$theme" --font-size 12 --line-height 1.4 \
      --fps-cap 10 --speed 1.8 --last-frame-duration 2 $runs/take.cast $tmp/take.gif; \
    python3 website/tools/topviz/publish.py --frame $runs/frame.txt --cast $runs/take.cast --gif $tmp/take.gif \
      --samples $runs/samples.txt \
      --version "$(cat VERSION)" --models "$(grep -cE '^  [a-zA-Z0-9_-]+:' $cfg)" --addr 127.0.0.1:$port; \
    python3 website/tools/published-tree.py

# `website/` is edited and `website/` is published, so `.assetsignore` is the
# only thing between an authoring file and a public URL -- which makes this
# check load-bearing rather than a convenience. It fails both ways: a file that
# would ship without being expected, and an expected file that is absent or
# excluded. Do not delete it while tidying up; without it the boundary is a
# comment. See `website/README.md`.
website-published:
    python3 website/tools/published-tree.py

# Write VERSION into every copy of it -- the `data-emb-version` elements on both
# surfaces and the version named in PRODUCT.md -- or (`--check`) fail if any has
# drifted. Run after bumping VERSION; `just version` does this for you.
website-version:
    python3 website/tools/stamp-version.py

# Stamp the sandbox preset digests into the site, or (`--check`) fail when a
# preset byte changed under a stamped digest. `EMB.EVSHA` calls a preset by the
# SHA1 of the bytes the server preloaded, so a stale digest is a command the
# site presents as working that the sandbox answers "no such script" to.
website-presets:
    python3 website/tools/stamp-presets.py

website-presets-check:
    python3 website/tools/stamp-presets.py --check

# ── the sandbox (website/repl) ───────────────────────────────────────────
#
# A small Go bridge in front of an emb server that listens on loopback. It is
# the only public surface the sandbox has, so it lives under the site directory
# without being part of the site: `website/.assetsignore` keeps it out of the
# published tree and `published-tree.py` asserts that it did.

# Build the bridge beside the server (`bin/repl`).
sandbox-build: build
    CGO_ENABLED=0 go build -o ./bin/repl ./website/repl

# The bridge's contract tests: no ONNX, no model, no server.
sandbox-test:
    CGO_ENABLED=0 go test ./website/repl/

# Run the bridge in front of an emb instance. It reads the server's config for
# the preset digests it will accept EMB.EVSHA for, so both processes must agree
# on one config file:
#
#     just dev                                        # in another shell
#     just sandbox-run config=website/repl/sandbox.yaml
#
# The sandbox config names /data/models/...; use config.yaml or a copy with
# local model paths when running against a local server.
sandbox-run upstream="127.0.0.1:6379" config="website/repl/sandbox.yaml" port="8080" origins="https://emb.is":
    go run ./website/repl -listen 127.0.0.1:{{port}} -upstream {{upstream}} -config {{config}} -origins {{origins}} -emb-top ./bin/emb-top

# Build the sandbox image (context is the repository root: the image needs the
# Go module and the server's build inputs).
sandbox-image:
    docker buildx build --load -f website/repl/Dockerfile -t {{docker_user}}/emb-sandbox:{{image_tag}} .

# Deploy the sandbox to Fly (requires flyctl and an authenticated account).
# The app, region, volume, and hostname are declared in website/repl/fly.toml.
# Run from the repository root: `.` is the build context, and fly.toml's
# `dockerfile` is resolved against *its own* directory. A first deploy needs
# the app created, the ingress IPs allocated, and cli.emb.is added as a
# certificate — see the "Deploying" note in website/README.md.
sandbox-deploy:
    fly deploy . -c website/repl/fly.toml

# Verify the stamped versions match VERSION. This is what a CI job or pre-commit
# hook runs; the emb-top capture once shipped `v0.4.0` against a `0.4.0.pre4`
# VERSION, which is the drift this exists to stop.
website-version-check:
    python3 website/tools/stamp-version.py --check

# Clean build artifacts
clean:
    rm -rf bin/ dist/
