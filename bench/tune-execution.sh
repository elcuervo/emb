#!/usr/bin/env bash
# A/B: ORT execution mode × intra_op_threads on the twin EMB.MULTI path
# (siglip2 int8 + minilm fp32), measured with redis-benchmark.
# Usage (inside nix develop): bash bench/tune-execution.sh
set -euo pipefail
cd "$(dirname "$0")/.."

PORT=${PORT:-16381}
N=${N:-3000}
C=${C:-16}
CMD='EMB.MULTI siglip2 hello world minilm a_vector_of_embedding_text_for_dense_search'

run_combo() {
  local mode=$1 intra=$2
  local cfg="/tmp/emb-tune-$mode-$intra.yaml"
  cat > "$cfg" <<YAML
listen: ":$PORT"
cache: "32MB"
models:
  siglip2:
    onnx: ./models/siglip2/text_model_int8.onnx
    tokenizer: ./models/siglip2/tokenizer.json
    output_tensor: text_embeds
    pooling: none
    normalize: true
    dim: 768
    max_length: 64
    intra_op_threads: $intra
    execution_mode: $mode
    preload: true
  minilm:
    onnx: ./models/minilm/model.onnx
    tokenizer: ./models/minilm/tokenizer.json
    preload: true
    intra_op_threads: $intra
    execution_mode: $mode
YAML
  ./bin/emb -config "$cfg" -listen ":$PORT" >/tmp/emb-tune.log 2>&1 &
  local pid=$!
  # wait for readiness
  for _ in $(seq 1 120); do
    if redis-cli -p "$PORT" EMB.READY 2>/dev/null | grep -q OK; then break; fi
    sleep 1
  done
  echo "=== mode=$mode intra=$intra ==="
  redis-benchmark -h 127.0.0.1 -p "$PORT" -c "$C" -n "$N" EMB.MULTI siglip2 "text __rand_int__" minilm "text __rand_int__" 2>/dev/null | grep -E "P50|P99|requests per second" || true
  kill "$pid" 2>/dev/null || true
  wait "$pid" 2>/dev/null || true
  sleep 1
}

rm -rf /tmp/emb-tune.out && mkdir -p /tmp/emb-tune.out

for mode in sequential parallel; do
  for intra in 1 4; do
    run_combo "$mode" "$intra"
  done
done