#!/usr/bin/env bash
# The sandbox's one entrypoint: start emb on loopback, start the bridge in
# front of it, and treat the two as one lifecycle.
#
#   * a stop signal is forwarded to emb, and the bridge is stopped too, so the
#     server's own shutdown path runs rather than being killed outright;
#   * if either process exits, the other is stopped and this script exits
#     non-zero, so the platform replaces the machine instead of leaving a
#     bridge that answers with no server behind it.
set -euo pipefail

EMB_CONFIG="${EMB_CONFIG:-/etc/emb/sandbox.yaml}"
BRIDGE_LISTEN="${BRIDGE_LISTEN:-0.0.0.0:8080}"
UPSTREAM="${UPSTREAM:-127.0.0.1:6379}"
ORIGINS="${ORIGINS:-https://emb.is,https://www.emb.is}"

emb -config "$EMB_CONFIG" &
emb_pid=$!

repl -listen "$BRIDGE_LISTEN" -upstream "$UPSTREAM" -config "$EMB_CONFIG" -origins "$ORIGINS" &
repl_pid=$!

stop_both() {
  kill -TERM "$emb_pid" 2>/dev/null || true
  kill -TERM "$repl_pid" 2>/dev/null || true
}
stopping=0
on_signal() { stopping=1; stop_both; }
trap 'on_signal' TERM INT

status=0
while kill -0 "$emb_pid" 2>/dev/null && kill -0 "$repl_pid" 2>/dev/null; do
  sleep 1
done

# A requested stop is not a failure; an unexpected exit is, so the platform
# replaces the machine instead of leaving a bridge with no server behind it.
if [ "$stopping" -eq 0 ]; then
  if ! kill -0 "$emb_pid" 2>/dev/null; then
    echo "run.sh: emb exited; ending the unit" >&2
    status=1
  fi
  if ! kill -0 "$repl_pid" 2>/dev/null; then
    echo "run.sh: the bridge exited; ending the unit" >&2
    status=1
  fi
fi

stop_both
wait "$emb_pid" 2>/dev/null || true
wait "$repl_pid" 2>/dev/null || true
exit "$status"
