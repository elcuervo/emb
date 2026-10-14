#!/usr/bin/env bash
# Run one reproduction arm: start a server binary with its config, record the
# effective layout, drive plain EMB at several concurrency levels, probe
# single-request latency under load, probe head-of-line blocking, then stop.
#
# Must run inside `nix develop` (the binary needs the ORT library path and the
# driver needs the dev-shell python3).
#
#   run-arm.sh --bin /tmp/emb-repro/pre4/bin/emb \
#              --cfg /tmp/emb-repro/cfg/pre4-default.yaml \
#              --label pre4-default --port 17001 --model e5 \
#              --out /tmp/emb-repro/results
set -euo pipefail

bin="" cfg="" label="" model="e5" out="." port="17001"
levels="1,8,32,128" requests="200" modes="sweep,probe,hol" ready_timeout=90
while [ $# -gt 0 ]; do
  case "$1" in
    --bin) bin=$2; shift 2 ;;
    --cfg) cfg=$2; shift 2 ;;
    --label) label=$2; shift 2 ;;
    --port) port=$2; shift 2 ;;
    --model) model=$2; shift 2 ;;
    --out) out=$2; shift 2 ;;
    --levels) levels=$2; shift 2 ;;
    --requests) requests=$2; shift 2 ;;
    --modes) modes=$2; shift 2 ;;
    *) echo "unknown argument: $1" >&2; exit 2 ;;
  esac
done
[ -n "$bin" ] && [ -n "$cfg" ] && [ -n "$label" ] || { echo "need --bin --cfg --label" >&2; exit 2; }

# macOS SIP strips DYLD_* from a script launched through `#!/usr/bin/env bash`,
# so the dev shell's ORT library path does not survive into this script. The
# caller hands it in: EMB_ORT_LIB="$DYLD_LIBRARY_PATH" run-matrix.sh ...
if [ -n "${EMB_ORT_LIB:-}" ]; then
  export DYLD_LIBRARY_PATH="${EMB_ORT_LIB}:${DYLD_LIBRARY_PATH:-}"
  export LD_LIBRARY_PATH="${EMB_ORT_LIB}:${LD_LIBRARY_PATH:-}"
fi

dir="$out/$label"
mkdir -p "$dir"
here=$(cd "$(dirname "$0")" && pwd)

log="$dir/server.log"
: >"$log"
start_ns=$(python3 -c 'import time;print(time.time_ns())')
"$bin" -config "$cfg" -listen "127.0.0.1:$port" >>"$log" 2>&1 &
pid=$!
trap 'kill $pid 2>/dev/null || true; wait $pid 2>/dev/null || true' EXIT

ready=0
for _ in $(seq 1 $((ready_timeout * 2))); do
  if redis-cli -h 127.0.0.1 -p "$port" EMB.READY 2>/dev/null | grep -q OK; then ready=1; break; fi
  if ! kill -0 $pid 2>/dev/null; then echo "server exited during startup" >&2; tail -20 "$log" >&2; exit 1; fi
  sleep 0.5
done
if [ "$ready" != 1 ]; then echo "server not ready within ${ready_timeout}s" >&2; tail -20 "$log" >&2; exit 1; fi
ready_ns=$(python3 -c 'import time;print(time.time_ns())')
python3 - "$dir" "$start_ns" "$ready_ns" <<'PY'
import json, sys
d, a, b = sys.argv[1], int(sys.argv[2]), int(sys.argv[3])
json.dump({"start_to_ready_s": round((b - a) / 1e9, 3)}, open(d + "/startup.json", "w"), indent=1)
PY

# Cold first request: the incident's shape (a request arriving at a cold model).
# A driver failure aborts the arm: evidence from a partial run is not evidence.
python3 "$here/drive.py" --port "$port" --model "$model" --mode hol --texts 1 --follow 0 >"$dir/cold-first.json" 2>&1

redis-cli -p "$port" EMB.INFO "$model" >"$dir/info.before.txt" 2>&1 || true
redis-cli -p "$port" EMB.STATS >"$dir/stats.before.txt" 2>&1 || true

IFS=',' read -r -a mode_list <<<"$modes"
for mode in "${mode_list[@]}"; do
  case "$mode" in
    sweep) python3 "$here/drive.py" --port "$port" --model "$model" --mode sweep \
             --levels "$levels" --requests "$requests" >"$dir/sweep.json" 2>&1 ;;
    probe) python3 "$here/drive.py" --port "$port" --model "$model" --mode probe \
             --background 128 --samples 25 >"$dir/probe.json" 2>&1 ;;
    hol)   python3 "$here/drive.py" --port "$port" --model "$model" --mode hol \
             --texts 200 --follow 20 >"$dir/hol.json" 2>&1 ;;
    *) echo "unknown mode $mode" >&2; exit 2 ;;
  esac
done

redis-cli -p "$port" EMB.STATS >"$dir/stats.after.txt" 2>&1 || true
kill $pid 2>/dev/null || true
wait $pid 2>/dev/null || true
trap - EXIT
echo "arm $label: results in $dir"
