#!/usr/bin/env bash
# Run the arms of the pre4 regression reproduction and print a summary table.
# Must run inside `nix develop`, with the two revisions built in detached
# worktrees (see README.md).
#
#   run-matrix.sh [--out DIR] [--arms label,label,...] [--requests N]
set -euo pipefail

out=/tmp/emb-repro/results
worktrees=/tmp/emb-repro
requests=200
only=""
here=$(cd "$(dirname "$0")" && pwd)
while [ $# -gt 0 ]; do
  case "$1" in
    --out) out=$2; shift 2 ;;
    --worktrees) worktrees=$2; shift 2 ;;
    --requests) requests=$2; shift 2 ;;
    --arms) only=$2; shift 2 ;;
    *) echo "unknown argument: $1" >&2; exit 2 ;;
  esac
done
mkdir -p "$out"

# label            revision  config                        port
arms=(
  "pre3-default     pre3  batched-default.yaml           17101"
  "pre4-default     pre4  batched-default.yaml           17102"
  "pre3-pool        pre3  pool-default.yaml              17103"
  "pre4-pool        pre4  pool-default.yaml              17104"
  "pre4-budget-off  pre4  budget-off-pre4.yaml           17105"
  "pre4-nocalib     pre4  no-calibrate-pre4.yaml         17106"
  "pre4-nocalib-pool pre4 no-calibrate-pool-pre4.yaml    17111"
  "pre3-gate        pre3  gate-pre3.yaml                 17107"
  "pre3-gate-spinoff pre3 gate-pre3-spinoff.yaml          17112"
  "pre4-gate-spinon pre4  gate-pre4-spinon.yaml           17113"
  "pre4-gate        pre4  gate-pre4.yaml                 17108"
  "pre3-preload     pre3  batched-preload.yaml           17109"
  "pre4-preload     pre4  batched-preload.yaml           17110"
)

for arm in "${arms[@]}"; do
  read -r label rev cfg port <<<"$arm"
  if [ -n "$only" ] && [[ ",$only," != *",$label,"* ]]; then continue; fi
  wt="$worktrees/$rev"
  [ -x "$wt/bin/emb" ] || { echo "missing binary for $rev at $wt/bin/emb" >&2; exit 1; }
  echo "== $label ($rev, $cfg, port $port)"
  ( cd "$wt" && "$here/run-arm.sh" --bin "$wt/bin/emb" --cfg "$here/cfg/$cfg" \
      --label "$label" --port "$port" --model e5 --out "$out" --requests "$requests" )
done

echo
echo "== summary =="
python3 - "$out" <<'PY'
import json, os, sys
root = sys.argv[1]
rows = []
for label in sorted(os.listdir(root)):
    d = os.path.join(root, label)
    if not os.path.isdir(d) or not os.path.exists(os.path.join(d, "sweep.json")):
        continue
    def load(name):
        try:
            with open(os.path.join(d, name)) as f:
                return json.load(f)
        except (OSError, ValueError):
            return None
    sweep, probe, startup = load("sweep.json"), load("probe.json"), load("startup.json")
    levels = {}
    if sweep and "levels" in sweep:
        for lv in sweep["levels"]:
            lat = lv.get("latency") or {}
            levels[lv["concurrency"]] = "%s req/s p99=%sms" % (lv.get("req_s"), lat.get("p99_ms"))
    rows.append((label,
                 (startup or {}).get("start_to_ready_s"),
                 levels,
                 (probe or {}).get("latency", {}).get("p50_ms") if probe else None,
                 (probe or {}).get("latency", {}).get("p99_ms") if probe else None))
w = max([len(r[0]) for r in rows] + [5])
print("%-*s %8s %s" % (w, "arm", "ready_s", "level throughput / p99 (probe: single request under 128-way load)"))
for label, ready, levels, pp50, pp99 in rows:
    lv = " | ".join("c%s: %s" % (k, v) for k, v in sorted(levels.items()))
    print("%-*s %8s %s | probe p50=%sms p99=%sms" % (w, label, ready, lv, pp50, pp99))
PY
