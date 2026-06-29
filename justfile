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

# Clean build artifacts
clean:
    rm -rf bin/
