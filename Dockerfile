# Stage 1: build the Go binary with ONNX Runtime
FROM golang:1.25-bookworm AS builder

ARG TARGETARCH

# Install ONNX Runtime pre-built shared library
RUN set -eux; \
    case ${TARGETARCH} in \
      amd64) ORT_ARCH=x86_64 ;; \
      arm64) ORT_ARCH=aarch64 ;; \
      *) echo "unsupported arch: ${TARGETARCH}"; exit 1 ;; \
    esac; \
    curl -fsSL "https://github.com/microsoft/onnxruntime/releases/download/v1.27.0/onnxruntime-linux-${ORT_ARCH}-1.27.0.tgz" \
      -o /tmp/onnx.tgz; \
    tar xzf /tmp/onnx.tgz -C /opt; \
    rm /tmp/onnx.tgz; \
    ls /opt/onnxruntime-linux-${ORT_ARCH}-1.27.0/lib/

WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .

RUN set -eux; \
    case ${TARGETARCH} in \
      amd64) ORT_ARCH=x86_64 ;; \
      arm64) ORT_ARCH=aarch64 ;; \
    esac; \
    ORT_DIR=/opt/onnxruntime-linux-${ORT_ARCH}-1.27.0; \
    CGO_ENABLED=1 \
    CGO_CFLAGS="-I${ORT_DIR}/include" \
    CGO_LDFLAGS="-L${ORT_DIR}/lib -lonnxruntime" \
    go build -o /emb ./cmd/emb

# Stage 2: minimal runtime image
FROM debian:bookworm-slim

ARG TARGETARCH

RUN apt-get update && apt-get install -y --no-install-recommends \
    ca-certificates \
    && rm -rf /var/lib/apt/lists/*

COPY --from=builder /opt/onnxruntime-linux-*-1.27.0/lib/libonnxruntime.so* /usr/lib/
COPY --from=builder /emb /usr/local/bin/emb

RUN ldconfig

RUN mkdir -p /etc/emb && echo 'listen: ":6379"\nmodels: {}' > /etc/emb/config.yaml

EXPOSE 6379
ENTRYPOINT ["emb", "-config", "/etc/emb/config.yaml"]
