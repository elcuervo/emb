# Stage 1: build the Go binary with ONNX Runtime
FROM golang:1.25-bookworm AS builder

ARG TARGETARCH
ARG ORT_VERSION=v1.27.0
ARG TOKENIZERS_VERSION=v1.27.0

# Install ONNX Runtime pre-built shared library
RUN set -eux; \
    case ${TARGETARCH} in \
      amd64) ORT_ARCH=x86_64 ;; \
      arm64) ORT_ARCH=aarch64 ;; \
      *) echo "unsupported arch: ${TARGETARCH}"; exit 1 ;; \
    esac; \
    ORT_VER="${ORT_VERSION#v}"; \
    curl -fsSL "https://github.com/microsoft/onnxruntime/releases/download/${ORT_VERSION}/onnxruntime-linux-${ORT_ARCH}-${ORT_VER}.tgz" \
      -o /tmp/onnx.tgz; \
    tar xzf /tmp/onnx.tgz -C /opt; \
    rm /tmp/onnx.tgz; \
    ls /opt/onnxruntime-linux-${ORT_ARCH}-${ORT_VER}/lib/

# Install libtokenizers pre-built static library (same arch mapping as ONNX)
RUN set -eux; \
    case ${TARGETARCH} in \
      amd64) LT_ARCH=linux-x86_64 ;; \
      arm64) LT_ARCH=linux-aarch64 ;; \
    esac; \
    curl -fsSL "https://github.com/daulet/tokenizers/releases/download/${TOKENIZERS_VERSION}/libtokenizers.${LT_ARCH}.tar.gz" \
      -o /tmp/libtokenizers.tgz; \
    mkdir -p /opt/libtokenizers; \
    tar xzf /tmp/libtokenizers.tgz -C /opt/libtokenizers; \
    rm /tmp/libtokenizers.tgz

WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .

RUN set -eux; \
    case ${TARGETARCH} in \
      amd64) ORT_ARCH=x86_64 ;; \
      arm64) ORT_ARCH=aarch64 ;; \
    esac; \
    ORT_VER="${ORT_VERSION#v}"; \
    ORT_DIR=/opt/onnxruntime-linux-${ORT_ARCH}-${ORT_VER}; \
    CGO_ENABLED=1 \
    CGO_CFLAGS="-I${ORT_DIR}/include" \
    CGO_LDFLAGS="-L${ORT_DIR}/lib -lonnxruntime -L/opt/libtokenizers" \
    go build -o /emb ./cmd/emb

# Copy ONNX libs to a version-independent path for the runtime stage
RUN set -eux; \
    ORT_VER="${ORT_VERSION#v}"; \
    rm -rf /opt/onnx-libs; \
    cp -a $(find /opt -maxdepth 1 -name "onnxruntime-linux-*-${ORT_VER}" -type d)/lib /opt/onnx-libs

# Stage 2: minimal runtime image
FROM debian:bookworm-slim

ARG TARGETARCH

RUN apt-get update && apt-get install -y --no-install-recommends \
    ca-certificates \
    && rm -rf /var/lib/apt/lists/*

COPY --from=builder /opt/onnx-libs/libonnxruntime.so* /usr/lib/
COPY --from=builder /emb /usr/local/bin/emb

RUN ldconfig

RUN mkdir -p /etc/emb && echo 'listen: ":6379"\nmodels: {}' > /etc/emb/config.yaml

EXPOSE 6379
ENTRYPOINT ["emb", "-config", "/etc/emb/config.yaml"]
