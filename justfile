default:
    @just --list

ort_lib := `echo "${DYLD_LIBRARY_PATH:-}" | grep -o '/nix/store/[^:]*onnxruntime[^:]*/lib' | head -1`
libtokenizers_dir := "./lib/libtokenizers"

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

# Build the emb binary (version from git tag)
image_tag := `git describe --tags --dirty --always 2>/dev/null || echo "dev"`

build: download-libtokenizers
    @mkdir -p bin
    CGO_ENABLED=1 CGO_LDFLAGS="-L{{libtokenizers_dir}}" go build \
        -ldflags="-X main.version={{image_tag}}" \
        -o ./bin/emb ./cmd/emb

# Build and run the server
dev: build
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

# Run end-to-end response time benchmark (requires downloaded model on :6379)
bench-e2e: build
    @echo "Starting server..."
    DYLD_LIBRARY_PATH="{{ort_lib}}:$DYLD_LIBRARY_PATH" ./bin/emb -config config.yaml & echo $! > /tmp/emb-srv.pid
    sleep 3
    @echo "Running 50 EMB requests..."
    CGO_ENABLED=0 go run ./cmd/emb-bench
    -kill `cat /tmp/emb-srv.pid` 2>/dev/null
    rm -f /tmp/emb-srv.pid

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

# Determine image tag from git
image_tag := `git rev-parse --short HEAD 2>/dev/null || echo "dev"`
docker_user := "elcuervo"
image_name := "{{docker_user}}/emb"

# Build multi-arch Docker image (native platform)
docker-build:
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
