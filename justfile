default:
    @just --list

ort_lib := `echo "${DYLD_LIBRARY_PATH:-}" | grep -o '/nix/store/[^:]*onnxruntime[^:]*/lib' | head -1`
libtokenizers_dir := "./lib/libtokenizers"
redis_benchmark := `which redis-benchmark 2>/dev/null || echo ""`
image_tag := `git describe --tags --dirty --always 2>/dev/null || echo "dev"`
docker_user := "elcuervo"
image_name := "{{docker_user}}/emb"

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

# Run all benchmarks (no baseline comparison)
bench:
    go test -bench=. -benchmem ./...

# Capture benchmark baseline
baseline:
    go test -bench=. -benchmem ./... | tee benchmark-baseline.txt

build:
    @mkdir -p bin
    go build -ldflags="-X main.version={{image_tag}}" -o ./bin/emb ./cmd/emb

# Build and run the server
dev: download-libtokenizers build
    DYLD_LIBRARY_PATH="{{ort_lib}}:$DYLD_LIBRARY_PATH" ./bin/emb -config config.yaml

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
    case "$$(uname -s),$$(uname -m)" in \
        Darwin,arm64)  ARCH="darwin-arm64" ;; \
        Darwin,x86_64) ARCH="darwin-x86_64" ;; \
        Linux,aarch64) ARCH="linux-aarch64" ;; \
        Linux,x86_64)  ARCH="linux-x86_64" ;; \
        *) echo "unsupported platform: $$(uname -s)-$$(uname -m)"; exit 1 ;; \
    esac && \
    curl -fsSL "https://github.com/daulet/tokenizers/releases/download/{{libtokenizers-version}}/libtokenizers.$${ARCH}.tar.gz" \
      -o /tmp/libtokenizers.tar.gz && \
    tar xzf /tmp/libtokenizers.tar.gz -C {{libtokenizers_dir}} && \
    rm /tmp/libtokenizers.tar.gz && \
    echo "✓ Downloaded libtokenizers.a ($$ARCH)"

# Download a model from HuggingFace
# Usage: just download-model [huggingface_repo] [output_dir]
download-model repo="Xenova/all-MiniLM-L6-v2" dir="./models/minilm":
    @mkdir -p {{dir}}
    @test -f {{dir}}/model.onnx && echo "✓ Already exists at {{dir}}" && exit 0
    @echo "Downloading {{repo}}..."
    @# Try root model.onnx first, then onnx/model.onnx (newer repos)
    @curl -sL "https://huggingface.co/{{repo}}/resolve/main/model.onnx" -o "{{dir}}/model.onnx"
    @if [ -f "{{dir}}/model.onnx" ] && [ "$$(wc -c < '{{dir}}/model.onnx')" -gt 100 ]; then \
        echo "  model.onnx (root)"; \
    else \
        curl -sL "https://huggingface.co/{{repo}}/resolve/main/onnx/model.onnx" -o "{{dir}}/model.onnx" && echo "  model.onnx (onnx/)"; \
    fi
    @curl -sL "https://huggingface.co/{{repo}}/resolve/main/tokenizer.json" -o "{{dir}}/tokenizer.json" && echo "  tokenizer.json"
    @curl -sL "https://huggingface.co/{{repo}}/resolve/main/config.json" -o "{{dir}}/config.json" && echo "  config.json"

# Run redis-benchmark with a single-threaded server
# Uses 1 client, 1 pipeline, 500 requests (~2s at 280 req/s)
# Requires: redis-benchmark, downloaded model at ./models/minilm
bench-redis-single: build
    @if [ "{{redis_benchmark}}" = "" ]; then echo "ERROR: redis-benchmark not found. Install: brew install redis"; exit 1; fi
    @echo "Starting server (GOMAXPROCS=1)..."
    DYLD_LIBRARY_PATH="{{ort_lib}}:$DYLD_LIBRARY_PATH" GOMAXPROCS=1 ./bin/emb -config config.yaml & echo $! > /tmp/emb-srv.pid
    sleep 10
    @echo "Running: redis-benchmark -p 6379 -q -c 1 -P 1 -n 500 EMB minilm hello world"
    {{redis_benchmark}} -p 6379 -q -c 1 -P 1 -n 500 EMB minilm hello world
    -kill `cat /tmp/emb-srv.pid` 2>/dev/null
    rm -f /tmp/emb-srv.pid

# Run redis-benchmark with a multi-threaded server
# Uses 16 clients, 1 pipeline, 2000 requests (~5s at 400 req/s)
# Requires: redis-benchmark, downloaded model at ./models/minilm
bench-redis-multi: build
    @if [ "{{redis_benchmark}}" = "" ]; then echo "ERROR: redis-benchmark not found. Install: brew install redis"; exit 1; fi
    @echo "Starting server (GOMAXPROCS=0)..."
    DYLD_LIBRARY_PATH="{{ort_lib}}:$DYLD_LIBRARY_PATH" GOMAXPROCS=0 ./bin/emb -config config.yaml & echo $! > /tmp/emb-srv.pid
    sleep 10
    @echo "Running: redis-benchmark -p 6379 -q -c 16 -P 1 -n 2000 EMB minilm hello world"
    {{redis_benchmark}} -p 6379 -q -c 16 -P 1 -n 2000 EMB minilm hello world
    -kill `cat /tmp/emb-srv.pid` 2>/dev/null
    rm -f /tmp/emb-srv.pid

# Run all redis-benchmark variants (single-threaded + multi-threaded)
bench-redis: bench-redis-single bench-redis-multi

# Verify embeddings match Python reference (requires downloaded model)
verify-embeddings: build
    @echo "Generating reference embeddings..."
    @if [ ! -f reference-embeddings.json ]; then \
        python3 -m venv /tmp/emb-verify-venv; \
        . /tmp/emb-verify-venv/bin/activate; \
        pip install -q sentence-transformers torch --extra-index-url https://download.pytorch.org/whl/cpu; \
        python3 cmd/emb-verify/generate-reference.py; \
        rm -rf /tmp/emb-verify-venv; \
    else echo "✓ reference-embeddings.json exists"; fi
    @echo "Starting server..."
    DYLD_LIBRARY_PATH="{{ort_lib}}:$DYLD_LIBRARY_PATH" ./bin/emb -config config.yaml & echo $! > /tmp/emb-srv.pid
    sleep 3
    @echo "Running verification..."
    CGO_ENABLED=0 go run ./cmd/emb-verify
    -kill `cat /tmp/emb-srv.pid` 2>/dev/null
    rm -f /tmp/emb-srv.pid

# Download two models and test EMB.MULTI across them
# Usage: just verify-emb-multi
verify-emb-multi:
    @echo "Ensuring models are downloaded..."
    just download-model "Xenova/all-MiniLM-L6-v2" "./models/minilm"
    just download-model "onnx-community/siglip2-base-patch16-224-ONNX" "./models/siglip2"
    @echo "Generating e2e config..."
    @printf 'listen: ":6379"\nmodels:\n  minilm:\n    onnx: ./models/minilm/model.onnx\n    tokenizer: ./models/minilm/tokenizer.json\n    max_length: 256\n    pooling: mean\n    normalize: true\n  siglip2:\n    onnx: ./models/siglip2/text_model.onnx\n    tokenizer: ./models/siglip2/tokenizer.json\n    max_length: 256\n    output_tensor: pooler_output\n    pooling: none\n    normalize: true\n    dim: 768\n' > /tmp/emb-multi-config.yaml
    @echo "Starting server with both models..."
    DYLD_LIBRARY_PATH="{{ort_lib}}:$DYLD_LIBRARY_PATH" ./bin/emb -config /tmp/emb-multi-config.yaml & echo $! > /tmp/emb-srv.pid
    sleep 3
    @echo "Running EMB.MULTI verification..."
    CGO_ENABLED=0 go run ./cmd/emb-multi-verify
    -kill `cat /tmp/emb-srv.pid` 2>/dev/null
    rm -f /tmp/emb-srv.pid /tmp/emb-multi-config.yaml

# Build Docker image (native platform)
docker:
    @echo "Building {{image_name}}:{{image_tag}} for $(shell uname -m)..."
    docker buildx build \
        --load \
        -t {{image_name}}:{{image_tag}} \
        -t {{image_name}}:latest \
        .

# Build and push multi-arch Docker image to Docker Hub
docker-push:
    @echo "Building and pushing {{image_name}}:{{image_tag}} for linux/amd64,linux/arm64..."
    docker buildx build \
        --platform linux/amd64,linux/arm64 \
        --push \
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

# Run GoReleaser snapshot (dry-run, produces dist/ locally)
release-dry-run:
    @echo "Running GoReleaser snapshot..."
    goreleaser release --snapshot --clean

# Clean build artifacts
clean:
    rm -rf bin/ dist/
