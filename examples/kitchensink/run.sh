#!/usr/bin/env bash
# Kitchensink — run the end-to-end example against its two servers.
#
#   run.sh index README.md DESIGN.md   embed files into the index
#   run.sh search "a query" 3          rank the index against that query
#   run.sh stats                       server and index counters
#   run.sh stop                        stop both servers
#
# The servers stay up between invocations, which is what makes `cache: auto`
# mean anything: a repeated search is served from emb's cache. Everything the
# example persists lives in .state/ — delete that directory to start over.
#
# Run it inside `nix develop`.
set -euo pipefail

here="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
cd "$here/../.."                      # ./bin/emb and models/ are repo-relative
state="$here/.state"
mkdir -p "$state"

EMB_PORT="${EMB_PORT:-16400}"
REDIS_PORT="${REDIS_PORT:-6399}"
export EMB_URL="redis://127.0.0.1:$EMB_PORT"
export REDIS_URL="redis://127.0.0.1:$REDIS_PORT"

# ── helpers ──────────────────────────────────────────────────────────────────

alive()     { [ -s "$state/$1.pid" ] && kill -0 "$(cat "$state/$1.pid")" 2>/dev/null; }
up()        { redis-cli -p "$1" ping >/dev/null 2>&1; }
ready()     { [ "$(redis-cli -p "$EMB_PORT" EMB.READY 2>/dev/null)" = OK ]; }
wait_down() { for _ in $(seq 1 40); do up "$1" || return 0; sleep 0.25; done; }

# start <name> <port> <command...> — idempotent, and refuses to adopt a server
# it did not launch rather than quietly talking to it.
start() {
  local name="$1" port="$2"; shift 2
  alive "$name" && return 0
  if up "$port"; then
    echo "port $port is already serving something this example did not start" >&2
    exit 1
  fi
  nohup "$@" >"$state/$name.log" 2>&1 &
  echo $! >"$state/$name.pid"
}

# ── stop ─────────────────────────────────────────────────────────────────────

if [ "${1:-}" = stop ]; then
  for name in emb redis; do
    alive "$name" && kill "$(cat "$state/$name.pid")" 2>/dev/null || true
    rm -f "$state/$name.pid"
  done
  # Wait rather than just signal: Redis fsyncs its append log on the way out,
  # and returning early leaves the port busy for the next invocation.
  wait_down "$REDIS_PORT"
  wait_down "$EMB_PORT"
  exit 0
fi

# ── dependencies ─────────────────────────────────────────────────────────────

# ONNX Runtime: macOS drops DYLD_* when a script is executed through
# /usr/bin/env, so the dev shell's path never reaches us. Point the server at
# the library itself instead of fighting the loader.
ort="${ORT_LIB:-$(ls /nix/store/*onnxruntime-*/lib/libonnxruntime.* 2>/dev/null | grep -v -- -dev | head -1)}"
if [ -z "$ort" ]; then
  echo "cannot find libonnxruntime — run inside 'nix develop', or set ORT_LIB" >&2
  exit 1
fi

# Run the client this repository ships, not whatever version is installed.
export BUNDLE_GEMFILE="$PWD/gems/emb/Gemfile"
[ -d gems/emb/vendor/bundle ] && export BUNDLE_PATH="$PWD/gems/emb/vendor/bundle"

# ── the two servers ──────────────────────────────────────────────────────────

# Redis keeps the index append-only so it survives a crash, not just a clean
# stop: `index` and `search` are separate invocations.
start redis "$REDIS_PORT" redis-server --port "$REDIS_PORT" --dir "$state" \
  --appendonly yes --save ''

start emb "$EMB_PORT" ./bin/emb -config "$here/emb.yaml" \
  -listen "127.0.0.1:$EMB_PORT" -ort-lib "$ort"

# Wait for the model to load rather than racing it: with `preload: true`,
# EMB.READY only answers OK once the model is actually serving.
for _ in $(seq 1 120); do ready && break; sleep 0.5; done
if ! ready; then
  echo "emb did not become ready on :$EMB_PORT" >&2
  tail -n 20 "$state/emb.log" >&2
  exit 1
fi

# ── the application ──────────────────────────────────────────────────────────

bundle exec ruby "$here/app.rb" "$@"
