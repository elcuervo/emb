#!/usr/bin/env bash
# What `asciinema record` runs: the dashboard in front, the traffic scenario
# behind it. See README.md in this directory.
#
# The dashboard owns the terminal — it renders to the pty's alternate screen and
# reads keys from it — so it is started first and left in the foreground of the
# take. The scenario's output goes to a log, never to the terminal: the only
# thing on screen for the whole recording is emb-top.
#
# The take is bounded by a watchdog rather than by an operator pressing `q`,
# because a scripted recording cannot press keys. Whatever the dashboard emits
# while quitting lands outside the window agg renders (the rig selects a range),
# so the clip ends on a held frame of the last poll, not on a cleared screen.
set -u

HERE=$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)
ROOT=$(cd "$HERE/../../.." && pwd)

PORT="${EMB_TOPVIS_PORT:-16379}"
ADDR="${EMB_TOPVIS_ADDR:-127.0.0.1:$PORT}"
# Long enough to cover the dashboard's own first paint, which Bubble Tea delays
# ~5s on a pty that never answers its background-colour query (see trim.py),
# plus a few seconds of the idle node the clip opens on.
PREROLL="${EMB_TOPVIS_PREROLL:-8}"
SECONDS_LOAD="${EMB_TOPVIS_SECONDS:-32}"
TAIL="${EMB_TOPVIS_TAIL:-2}"          # polls after the last band leaves
WINDOW="${EMB_TOPVIS_WINDOW:-40}"     # history in polls; the heatmap fills to ~90% by the end
LOG="${EMB_TOPVIS_LOG:-/tmp/emb-topviz-traffic.log}"

cleanup() {
  [ -n "${top:-}" ] && kill "$top" 2>/dev/null
  [ -n "${traffic:-}" ] && kill "$traffic" 2>/dev/null
  [ -n "${stop:-}" ] && kill "$stop" 2>/dev/null
}
trap cleanup EXIT INT TERM

"$ROOT/bin/emb-top" -addr "$ADDR" -interval 1s -window "$WINDOW" &
top=$!

sleep "$PREROLL"
"$HERE/traffic.sh" >"$LOG" 2>&1 &
traffic=$!

( sleep "$((SECONDS_LOAD + TAIL))"; kill -TERM "$top" 2>/dev/null ) &
stop=$!

wait "$top" 2>/dev/null
