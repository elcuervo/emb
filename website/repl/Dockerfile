# The sandbox image: one machine runs three processes — the emb server on
# loopback, the sidekick monitor on loopback, and the bridge that is the only
# public surface.
#
# Build from the repository root, so the Go module and the ORT/libtokenizers
# downloads are available to the build:
#
#     docker build -f website/repl/Dockerfile -t emb-sandbox .
#
# or via `just sandbox-image`.
#
# Stage 1: build the server (CGo + ONNX Runtime), the bridge, the dashboard
# producer and the sidekick (all pure Go).
FROM golang:1.26-bookworm AS builder

ARG TARGETARCH
# microsoft/onnxruntime publishes no release archive on GitHub any more: the
# runtime comes from the official NuGet package, headers under
# build/native/include and the shared library under runtimes/<rid>/native.
ARG ORT_VERSION=1.27.1
ARG TOKENIZERS_VERSION=v1.27.0
ARG EMB_VERSION=""

RUN set -eux; \
    case ${TARGETARCH} in \
      amd64) ORT_RID=linux-x64 ;; \
      arm64) ORT_RID=linux-arm64 ;; \
      *) echo "unsupported arch: ${TARGETARCH}"; exit 1 ;; \
    esac; \
    curl -fsSL "https://api.nuget.org/v3-flatcontainer/microsoft.ml.onnxruntime/${ORT_VERSION}/microsoft.ml.onnxruntime.${ORT_VERSION}.nupkg" \
      -o /tmp/onnx.zip; \
    unzip -q /tmp/onnx.zip -d /tmp/onnx; \
    mkdir -p /opt/onnxruntime/include /opt/onnxruntime/lib; \
    cp /tmp/onnx/build/native/include/*.h /opt/onnxruntime/include/; \
    cp /tmp/onnx/runtimes/${ORT_RID}/native/libonnxruntime*.so /opt/onnxruntime/lib/; \
    rm -rf /tmp/onnx /tmp/onnx.zip

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
    EMB_VER="${EMB_VERSION:-$(cat /src/VERSION 2>/dev/null || echo dev)}"; \
    CGO_ENABLED=1 \
    CGO_CFLAGS="-I/opt/onnxruntime/include" \
    CGO_LDFLAGS="-L/opt/onnxruntime/lib -lonnxruntime -L/opt/libtokenizers -Wl,-rpath,\$ORIGIN" \
    go build -ldflags="-X main.version=${EMB_VER}" -o /emb ./cmd/emb && \
    CGO_ENABLED=0 go build -o /repl ./website/repl && \
    CGO_ENABLED=0 go build -ldflags="-X main.version=${EMB_VER}" -o /emb-top ./cmd/emb-top && \
    CGO_ENABLED=0 go build -ldflags="-X main.version=${EMB_VER}" -o /emb-sidekick ./cmd/emb-sidekick

# `COPY` preserves symlinks, and several preset scripts are symlinks into
# scripts/. Dereference the directory here so the runtime image gets the real
# files instead of links pointing outside /etc/emb/presets. One `cp -RL` keeps
# this correct as presets are added, rather than a COPY line per script.
RUN cp -RL /src/website/repl/presets /opt/emb-presets

# Stage 2: minimal runtime image
FROM debian:bookworm-slim

RUN apt-get update && apt-get install -y --no-install-recommends \
    ca-certificates \
    && rm -rf /var/lib/apt/lists/*

COPY --from=builder /opt/onnxruntime/lib/libonnxruntime.so* /usr/lib/
COPY --from=builder /emb /usr/local/bin/emb
COPY --from=builder /repl /usr/local/bin/repl
# The sidekick monitor listens only on loopback; the bridge is its only path in.
COPY --from=builder /emb-sidekick /usr/local/bin/emb-sidekick
# The live dashboard at /stats is fed by this producer: the bridge spawns it
# and turns its frames into the page.
COPY --from=builder /emb-top /usr/local/bin/emb-top
# The server config and the preset scripts travel together: the config names
# `presets/*.lua` relative to its own directory, and the bridge hashes the same
# files to build the digest manifest it accepts EMB.EVSHA for.
COPY --from=builder /src/website/repl/sandbox.yaml /etc/emb/sandbox.yaml
COPY --from=builder /opt/emb-presets /etc/emb/presets
# The decision miniature travels in the image, so the decision plate needs no
# download and no volume write. `downloadModel` early-returns when the
# configured onnx file exists, so the sandbox entry needs no `model_repo` for
# it.
COPY --from=builder /src/testdata/laya /etc/emb/laya-model
COPY --from=builder /src/website/repl/run.sh /usr/local/bin/run.sh
RUN chmod +x /usr/local/bin/run.sh && ldconfig

# The bridge is the only listener the platform proxies to; emb binds loopback.
EXPOSE 8080
ENTRYPOINT ["/usr/local/bin/run.sh"]
