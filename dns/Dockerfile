# The zone image: one machine runs two processes — the emb server that holds
# the model on loopback, and emb-dns, which speaks DNS and HTTP in front of it
# and holds no model at all.
#
# Build from the repository root, so the Go module and the ORT/libtokenizers
# downloads are available to the build:
#
#     docker build -f dns/Dockerfile -t emb-dns .
#
# or via `just dns-image`.
#
# Stage 1: build the server (CGo + ONNX Runtime) and the zone proxy (pure Go).
FROM golang:1.26-bookworm AS builder

ARG TARGETARCH
ARG ORT_VERSION=v1.27.0
ARG TOKENIZERS_VERSION=v1.27.0
ARG EMB_VERSION=""

RUN set -eux; \
    case ${TARGETARCH} in \
      amd64) ORT_ARCH=x64 ;; \
      arm64) ORT_ARCH=aarch64 ;; \
      *) echo "unsupported arch: ${TARGETARCH}"; exit 1 ;; \
    esac; \
    ORT_VER="${ORT_VERSION#v}"; \
    curl -fsSL "https://github.com/microsoft/onnxruntime/releases/download/${ORT_VERSION}/onnxruntime-linux-${ORT_ARCH}-${ORT_VER}.tgz" \
      -o /tmp/onnx.tgz; \
    tar xzf /tmp/onnx.tgz -C /opt; \
    rm /tmp/onnx.tgz; \
    ls /opt/onnxruntime-linux-${ORT_ARCH}-${ORT_VER}/lib/

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
      amd64) ORT_ARCH=x64 ;; \
      arm64) ORT_ARCH=aarch64 ;; \
    esac; \
    ORT_VER="${ORT_VERSION#v}"; \
    ORT_DIR=/opt/onnxruntime-linux-${ORT_ARCH}-${ORT_VER}; \
    EMB_VER="${EMB_VERSION:-$(cat /src/VERSION 2>/dev/null || echo dev)}"; \
    CGO_ENABLED=1 \
    CGO_CFLAGS="-I${ORT_DIR}/include" \
    CGO_LDFLAGS="-L${ORT_DIR}/lib -lonnxruntime -L/opt/libtokenizers -Wl,-rpath,\$ORIGIN" \
    go build -ldflags="-X main.version=${EMB_VER}" -o /emb ./cmd/emb && \
    CGO_ENABLED=0 go build -ldflags="-X main.version=${EMB_VER}" -o /emb-dns ./cmd/emb-dns

# Copy ONNX libs to a version-independent path for the runtime stage.
RUN set -eux; \
    ORT_VER="${ORT_VERSION#v}"; \
    rm -rf /opt/onnx-libs; \
    cp -a $(find /opt -maxdepth 1 -name "onnxruntime-linux-*-${ORT_VER}" -type d)/lib /opt/onnx-libs

# Stage 2: minimal runtime image
FROM debian:bookworm-slim

RUN apt-get update && apt-get install -y --no-install-recommends \
    ca-certificates \
    && rm -rf /var/lib/apt/lists/*

COPY --from=builder /opt/onnx-libs/libonnxruntime.so* /usr/lib/
COPY --from=builder /emb /usr/local/bin/emb
COPY --from=builder /emb-dns /usr/local/bin/emb-dns
# The two configs, the preset, and the vocabulary travel under the paths the
# configs name, so the shipped files run verbatim: both `emb.yaml` and the zone
# config name `/scripts/emoji.lua`, and the zone config names
# `dns/emoji-vocab.json`, resolved from `/`.
COPY --from=builder /src/dns/emb.yaml /etc/emb/emb.yaml
COPY --from=builder /src/dns/config.yaml /etc/emb/dns.yaml
COPY --from=builder /src/scripts/emoji.lua /scripts/emoji.lua
COPY --from=builder /src/dns/emoji-vocab.json /dns/emoji-vocab.json
COPY --from=builder /src/dns/run.sh /usr/local/bin/run.sh
RUN chmod +x /usr/local/bin/run.sh && ldconfig

# The zone is the only public surface; emb binds loopback.
EXPOSE 53/udp 53/tcp 8080
ENTRYPOINT ["/usr/local/bin/run.sh"]
