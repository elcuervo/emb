#!/usr/bin/env bash
# What `just website-clusterviz` records: the fleet view. Two nodes, one
# dashboard. See README.md in this directory.
#
# The dashboard owns the terminal, so it is started first and left in the
# foreground; the two nodes it watches are already running, and the traffic runs
# behind it. A watchdog ends the take because a scripted recording cannot press
# `q`.
set -u

HERE=$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)
ROOT=$(cd "$HERE/../../.." && pwd)

A="${EMB_CLUSTERVIZ_A:-127.0.0.1:16379}"
B="${EMB_CLUSTERVIZ_B:-127.0.0.1:16380}"
PREROLL="${EMB_CLUSTERVIZ_PREROLL:-8}"
SECONDS_LOAD="${EMB_CLUSTERVIZ_SECONDS:-30}"
TAIL="${EMB_CLUSTERVIZ_TAIL:-2}"
WINDOW="${EMB_CLUSTERVIZ_WINDOW:-40}"
LOG="${EMB_CLUSTERVIZ_LOG:-/tmp/emb-clusterviz-traffic.log}"

cleanup() {
  [ -n "${top:-}" ] && kill "$top" 2>/dev/null
  [ -n "${traffic:-}" ] && kill "$traffic" 2>/dev/null
  [ -n "${stop:-}" ] && kill "$stop" 2>/dev/null
}
trap cleanup EXIT INT TERM

"$ROOT/bin/emb-top" -nodes "$A,$B" -interval 1s -window "$WINDOW" &
top=$!

sleep "$PREROLL"
EMB_CLUSTERVIZ_A="$A" EMB_CLUSTERVIZ_B="$B" "$HERE/cluster-traffic.sh" >"$LOG" 2>&1 &
traffic=$!

( sleep "$((SECONDS_LOAD + TAIL))"; kill -TERM "$top" 2>/dev/null ) &
stop=$!

wait "$top" 2>/dev/null
