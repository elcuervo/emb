#!/usr/bin/env bash
# The fleet `just website-stressviz` records: two nodes from one configuration,
# one on each address family, both on the scenario's port. See README.md here.
#
# Two nodes on one port is the shape the tools exist for. A bare name that
# resolves to both families (`localhost`) expands to two endpoints, so one
# `emb-stress -target` and one `emb-top -node` cover the fleet -- which is what
# makes the offered share and the served share comparable at all.
#
# The name has to be given *bare*: both tools treat `host:port` as an address
# and expand only a name, which takes the default port. So the fleet listens on
# 6379, and `just website-dev` -- which points the site's console at 6379 -- is
# the one thing that can be in the way (the guard below says so).
#
# Refuses to start when either address already answers: the alternative is
# recording whatever was already there under this run's name.
set -eu

HERE=$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)
ROOT=$(cd "$HERE/../../.." && pwd)

PORT="${EMB_STRESSVIZ_PORT:-6379}"
CFG="${EMB_STRESSVIZ_CONFIG:-$ROOT/website/tools/topviz/models.yaml}"
RUNS="${EMB_STRESSVIZ_RUNS:-$HERE/runs}"
DEADLINE="${EMB_STRESSVIZ_READY_TIMEOUT:-300}"

# The two endpoints of one name: the IPv4 loopback and the IPv6 one, same port.
V4=127.0.0.1
V6=::1

mkdir -p "$RUNS"

# macOS hands a `#!/usr/bin/env bash` script an environment with every DYLD_*
# variable removed -- the loader strips them for protected binaries, and
# `/usr/bin/env` is one -- so `emb` started from here cannot find ONNX Runtime
# unless the script runs under the dev shell's own bash, which keeps it. The
# justfile invokes it as `bash …/bringup.sh` for this reason; say so rather than
# failing with the loader's own message, which names no cause.
if [ "$(uname)" = Darwin ] && [ -z "${DYLD_LIBRARY_PATH:-}" ]; then
  echo "stressviz: DYLD_LIBRARY_PATH is empty, so emb cannot load ONNX Runtime." >&2
  echo "  run this as: bash $HERE/bringup.sh  (or through 'just website-stressviz')" >&2
  exit 1
fi

# The configuration names its models relative to the repository root, and the
# server resolves them against its own working directory.
cd "$ROOT"

for onnx in $(grep -E '^[[:space:]]+onnx:' "$CFG" | awk '{print $2}'); do
  if [ ! -f "$ROOT/$onnx" ]; then
    echo "stressviz: missing $onnx" >&2
    echo "  fetch it with: just download-model <repo> $ROOT/$(dirname "$onnx")" >&2
    exit 1
  fi
done

for host in "$V4" "$V6"; do
  if redis-cli -h "$host" -p "$PORT" ping >/dev/null 2>&1; then
    echo "stressviz: something already answers on [$host]:$PORT" >&2
    exit 1
  fi
done

start() {
  local host="$1" tag="$2"
  "$ROOT/bin/emb" -config "$CFG" -listen "[$host]:$PORT" >"$RUNS/node-$tag.log" 2>&1 &
  echo $! >"$RUNS/node-$tag.pid"
}

wait_ready() {
  local host="$1" tag="$2" deadline
  deadline=$(( $(date +%s) + DEADLINE ))
  until redis-cli -h "$host" -p "$PORT" EMB.READY 2>/dev/null | grep -q OK; do
    if ! kill -0 "$(cat "$RUNS/node-$tag.pid")" 2>/dev/null; then
      echo "stressviz: the node on [$host]:$PORT exited during startup" >&2
      tail -20 "$RUNS/node-$tag.log" >&2
      exit 1
    fi
    if [ "$(date +%s)" -ge "$deadline" ]; then
      echo "stressviz: the node on [$host]:$PORT was not ready within ${DEADLINE}s" >&2
      tail -20 "$RUNS/node-$tag.log" >&2
      exit 1
    fi
    sleep 1
  done
  echo "stressviz: node on [$host]:$PORT is ready"
}

echo "stressviz: starting two nodes from $CFG"
start "$V4" v4
start "$V6" v6
wait_ready "$V4" v4
wait_ready "$V6" v6
