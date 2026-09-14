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
download-model repo="Xenova/all-MiniLM-L6-v2" dir="./models/minilm":
    @mkdir -p {{dir}}
    @if [ -f "{{dir}}/model.onnx" ]; then \
        echo "✓ Already exists at {{dir}}"; \
        exit 0; \
    fi; \
    echo "Downloading {{repo}}..."; \
    curl -sL "https://huggingface.co/{{repo}}/resolve/main/model.onnx" -o "{{dir}}/model.onnx"; \
    if [ -f "{{dir}}/model.onnx" ] && [ "$(wc -c < '{{dir}}/model.onnx')" -gt 100 ]; then \
        echo "  model.onnx (root)"; \
    else \
        curl -sL "https://huggingface.co/{{repo}}/resolve/main/onnx/model.onnx" -o "{{dir}}/model.onnx" && echo "  model.onnx (onnx/)"; \
    fi; \
    curl -sL "https://huggingface.co/{{repo}}/resolve/main/tokenizer.json" -o "{{dir}}/tokenizer.json" && echo "  tokenizer.json"; \
    curl -sL "https://huggingface.co/{{repo}}/resolve/main/config.json" -o "{{dir}}/config.json" && echo "  config.json"; \
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

# Bump version: edit VERSION with $EDITOR, then update gem lockfiles
version:
	$EDITOR VERSION
	cd gems/emb && bundle
	cd gems/emb-server && bundle

# Serve the static product site (website/) at http://localhost:8080
website port="8080":
    python3 -m http.server {{port}} --directory website

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

# Assert that the set of files this folder publishes is the set we mean.
#
# `website/` is edited and `website/` is published, so `.assetsignore` is the
# only thing between an authoring file and a public URL -- which makes this
# check load-bearing rather than a convenience. It fails both ways: a file that
# would ship without being expected, and an expected file that is absent or
# excluded. Do not delete it while tidying up; without it the boundary is a
# comment. See `website/README.md`.
website-published:
    python3 website/tools/published-tree.py

# Write VERSION into every element carrying `data-emb-version` on both surfaces,
# or (`--check`) fail if any stamped value has drifted. Run after bumping VERSION.
website-version:
    python3 website/tools/stamp-version.py

# Verify the stamped versions match VERSION. This is what a CI job or pre-commit
# hook runs; the emb-top capture once shipped `v0.4.0` against a `0.4.0.pre4`
# VERSION, which is the drift this exists to stop.
website-version-check:
    python3 website/tools/stamp-version.py --check

# Clean build artifacts
clean:
    rm -rf bin/ dist/
