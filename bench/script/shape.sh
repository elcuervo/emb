#!/usr/bin/env bash
# Traffic-shape autotune demonstration: starts a GLiNER2 server and drives
# serial then burst phases with cmd/evalbench, printing the classified traffic
# class and concurrency allowance after each phase.
#
# Env: SHAPE (serial|burst|mixed), PHASE (per-phase duration), BURST
# (burst concurrency), PORT, CORPUS, SCRIPT, ARGS, WORKERS, INTRA, ONNX, TOKENIZER.
set -euo pipefail

SHAPE=${SHAPE:-mixed}
PHASE=${PHASE:-12s}
BURST=${BURST:-8}
PORT=${PORT:-16499}
CORPUS=${CORPUS:-bench/script/gliner-corpus.txt}
SCRIPT=${SCRIPT:-examples/scripts/reference/gliner2.lua}
ARGS_RAW=${ARGS:-PERSON ORG PRODUCT LOCATION EVENT}
MODEL=${MODEL:-gliner2}
WORKERS=${WORKERS:-4}
INTRA=${INTRA:-}
ONNX=${ONNX:-./models/gliner2/model_int8.onnx}
TOKENIZER=${TOKENIZER:-./models/gliner2/tokenizer.json}

tmp=$(mktemp -d /tmp/emb-shape.XXXXXX)
server_pid=""
cleanup() {
  [ -n "$server_pid" ] && kill "$server_pid" 2>/dev/null || true
  rm -rf "$tmp"
}
trap cleanup EXIT

go build -o "$tmp/emb" ./cmd/emb
go build -o "$tmp/evalbench" ./cmd/evalbench

cfg="$tmp/gliner.yaml"
{
  echo "listen: 127.0.0.1:$PORT"
  echo "models:"
  echo "  $MODEL:"
  echo "    onnx: $ONNX"
  echo "    tokenizer: $TOKENIZER"
  echo "    script_preload: true"
  echo "    script_workers: $WORKERS"
  [ -n "$INTRA" ] && echo "    intra_op_threads: $INTRA"
} > "$cfg"

"$tmp/emb" -config "$cfg" >"$tmp/server.log" 2>&1 &
server_pid=$!
deadline=$((SECONDS + 180))
until redis-cli -p "$PORT" ping >/dev/null 2>&1; do
  if ! kill -0 "$server_pid" 2>/dev/null; then
    echo "server exited during startup:" >&2
    cat "$tmp/server.log" >&2
    exit 1
  fi
  [ "$SECONDS" -lt "$deadline" ] || { echo "server not ready within 180s" >&2; exit 1; }
  sleep 0.5
done

argflags=()
for a in $ARGS_RAW; do argflags+=(-arg "$a"); done

"$tmp/evalbench" -shape "$SHAPE" -shape-phase "$PHASE" -shape-burst "$BURST" \
  -addr "127.0.0.1:$PORT" -model "$MODEL" -script "$SCRIPT" -corpus "$CORPUS" "${argflags[@]}"
