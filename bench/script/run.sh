#!/usr/bin/env bash
# Interleaved A/B harness for scripted inference.
#
# Runs a baseline and a candidate emb binary against the same corpus at each
# concurrency level, alternating which binary goes first, and fails when their
# replies differ. Latency/throughput lines are printed with a "base " / "cand "
# prefix so medians can be compared.
#
# Env:
#   BASE, CAND      baseline and candidate binaries (required)
#   CORPUS          one text per line (default bench/script/gliner-corpus.txt)
#   SCRIPT          Lua script (default examples/scripts/reference/gliner2.lua)
#   ARGS            space-separated script ARGV, e.g. entity labels
#   MODEL           model name (default gliner2)
#   WORKERS         script_workers for the generated config (default 4; empty omits it)
#   INTRA           intra_op_threads; empty exercises the unset default
#   CAPACITY        capacity profile (auto|latency|throughput); empty omits it
#   AUTOTUNE        autotune mode (auto|callers|off); empty omits it
#   CONCS           concurrency list (default 1,4,8,16)
#   N               requests per concurrency level (default 200)
#   SAMPLES         interleaved A/B rounds (default 3)
#   PORT            server port (default 16399)
set -euo pipefail

BASE=${BASE:?usage: BASE=<baseline-bin> CAND=<candidate-bin> bash bench/script/run.sh}
CAND=${CAND:?usage: BASE=<baseline-bin> CAND=<candidate-bin> bash bench/script/run.sh}
CORPUS=${CORPUS:-bench/script/gliner-corpus.txt}
SCRIPT=${SCRIPT:-examples/scripts/reference/gliner2.lua}
ARGS_RAW=${ARGS:-PERSON ORG PRODUCT LOCATION EVENT FEATURE CURRENCY DATE}
MODEL=${MODEL:-gliner2}
WORKERS=${WORKERS:-4}
INTRA=${INTRA:-}
CONCS=${CONCS:-1,4,8,16}
N=${N:-200}
SAMPLES=${SAMPLES:-3}
PORT=${PORT:-16399}
ONNX=${ONNX:-./models/gliner2/model_int8.onnx}
TOKENIZER=${TOKENIZER:-./models/gliner2/tokenizer.json}

tmp=$(mktemp -d /tmp/evalbench.XXXXXX)
server_pid=""
cleanup() {
  [ -n "$server_pid" ] && kill "$server_pid" 2>/dev/null || true
  rm -rf "$tmp"
}
trap cleanup EXIT

go build -o "$tmp/evalbench" ./cmd/evalbench

cfg="$tmp/gliner.yaml"
{
  echo "listen: 127.0.0.1:$PORT"
  echo "models:"
  echo "  $MODEL:"
  echo "    onnx: $ONNX"
  echo "    tokenizer: $TOKENIZER"
  echo "    script_preload: true"
  [ -n "$WORKERS" ] && echo "    script_workers: $WORKERS"
  [ -n "$INTRA" ] && echo "    intra_op_threads: $INTRA"
  [ -n "${CAPACITY:-}" ] && echo "    capacity: $CAPACITY"
  [ -n "${AUTOTUNE:-}" ] && echo "    autotune: $AUTOTUNE"
} > "$cfg"

argflags=()
for a in $ARGS_RAW; do argflags+=(-arg "$a"); done
texts=$(grep -c . "$CORPUS")

start() {
  "$1" -config "$cfg" >"$tmp/server.log" 2>&1 &
  server_pid=$!
}

wait_ready() {
  local deadline=$((SECONDS + 120))
  until redis-cli -p "$PORT" ping >/dev/null 2>&1; do
    if ! kill -0 "$server_pid" 2>/dev/null; then
      echo "server exited during startup:" >&2
      cat "$tmp/server.log" >&2
      return 1
    fi
    if [ "$SECONDS" -ge "$deadline" ]; then
      echo "server not ready within 120s" >&2
      return 1
    fi
    sleep 0.5
  done
}

stop() {
  kill "$server_pid" 2>/dev/null || true
  wait "$server_pid" 2>/dev/null || true
  server_pid=""
}

run_bin() {
  local name=$1 bin=$2
  start "$bin"
  wait_ready
  "$tmp/evalbench" -addr "127.0.0.1:$PORT" -model "$MODEL" -script "$SCRIPT" -corpus "$CORPUS" \
    -concurrency "$CONCS" -n "$N" "${argflags[@]}" | sed "s/^/$name /"
  "$tmp/evalbench" -addr "127.0.0.1:$PORT" -model "$MODEL" -script "$SCRIPT" -corpus "$CORPUS" \
    -concurrency 1 -n "$texts" "${argflags[@]}" -dump "$tmp/$name.dump" >/dev/null
  stop
}

for s in $(seq 1 "$SAMPLES"); do
  if [ $((s % 2)) -eq 1 ]; then order="base cand"; else order="cand base"; fi
  for which in $order; do
    if [ "$which" = base ]; then bin=$BASE; else bin=$CAND; fi
    echo "== sample $s: $which ($bin) workers=${WORKERS:-unset} intra=${INTRA:-unset} capacity=${CAPACITY:-unset} autotune=${AUTOTUNE:-unset}" >&2
    run_bin "$which" "$bin"
  done
  if ! cmp -s "$tmp/base.dump" "$tmp/cand.dump"; then
    echo "FAIL: replies differ between BASE and CAND (sample $s)" >&2
    diff "$tmp/base.dump" "$tmp/cand.dump" | head -20 >&2
    exit 1
  fi
done

echo "OK: replies identical across $SAMPLES samples" >&2
