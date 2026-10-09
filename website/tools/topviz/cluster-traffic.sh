#!/usr/bin/env bash
# The load `just website-clusterviz` records: traffic to both nodes, unevenly so
# the fleet rows show a real share against their 1/N expectation. Node A runs
# the topviz plan at a lighter batch, node B at a lighter one still, so the two
# rows differ without either going idle.
set -u

HERE=$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)
A="${EMB_CLUSTERVIZ_A:-127.0.0.1:16379}"
B="${EMB_CLUSTERVIZ_B:-127.0.0.1:16380}"

port_of() { echo "${1##*:}"; }

EMB_TOPVIS_BATCH="${EMB_CLUSTERVIZ_BATCH_A:-16}" EMB_TOPVIS_PORT="$(port_of "$A")" "$HERE/traffic.sh" &
pids="$!"
EMB_TOPVIS_BATCH="${EMB_CLUSTERVIZ_BATCH_B:-6}" EMB_TOPVIS_PORT="$(port_of "$B")" "$HERE/traffic.sh" &
pids="$pids $!"

trap 'kill $pids 2>/dev/null' EXIT INT TERM
wait 2>/dev/null
