default:
    @just --list

ort_lib := `echo "${DYLD_LIBRARY_PATH:-}" | grep -o '/nix/store/[^:]*onnxruntime[^:]*/lib' | head -1`

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

# Build the emb binary
build:
    @mkdir -p bin
    CGO_ENABLED=1 go build -o ./bin/emb ./cmd/emb

# Build and run the server
dev: build
    DYLD_LIBRARY_PATH="{{ort_lib}}:$DYLD_LIBRARY_PATH" ./bin/emb -config config.yaml

# Download a model from HuggingFace using optimum-cli
# Usage: just download-model [huggingface_repo] [output_dir]
download-model repo="sentence-transformers/all-MiniLM-L6-v2" dir="./models/minilm":
    @mkdir -p {{dir}}
    @test -f {{dir}}/model.onnx && echo "✓ Already exists at {{dir}}" || { \
        echo "Setting up venv..."; \
        python3 -m venv /tmp/emb-dl; \
        . /tmp/emb-dl/bin/activate; \
        pip install -q "optimum[onnxruntime]" torch --extra-index-url https://download.pytorch.org/whl/cpu; \
        optimum-cli export onnx --model {{repo}} {{dir}}; \
        rm -rf /tmp/emb-dl; \
    }

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

# Clean build artifacts
clean:
    rm -rf bin/
