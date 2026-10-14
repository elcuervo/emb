#!/usr/bin/env bash
# What `asciinema record` runs for `just website-stressviz`: one real
# `emb-stress` run against the scenario's fleet. See README.md here.
#
# The recording is the tool's own output, so the capture the documentation
# carries is the run itself and not a picture of one. The report is the last
# thing to appear -- a run spends its levels working and prints the table at the
# end -- so the capture is rendered with an idle-time cap rather than a
# speed-up: the frames that exist are the frames the run produced.
#
# The command is echoed first because a recording starts mid-session: without
# it the reader sees a table with no invocation above it, and the first thing
# the documentation has to answer is "what produced this".
set -uo pipefail

HERE=$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)
ROOT=$(cd "$HERE/../../.." && pwd)

PORT="${EMB_STRESSVIZ_PORT:-6379}"
CFG="${EMB_STRESSVIZ_CONFIG:-$ROOT/website/tools/topviz/models.yaml}"
RUNS="${EMB_STRESSVIZ_RUNS:-$HERE/runs}"
LEVELS="${EMB_STRESSVIZ_LEVELS:-1,2,4,8}"
DWELL="${EMB_STRESSVIZ_DWELL:-12s}"
WARMUP="${EMB_STRESSVIZ_WARMUP:-4s}"
SEED="${EMB_STRESSVIZ_SEED:-3}"

mkdir -p "$RUNS"

printf '$ ./bin/emb-stress -config %s -target localhost -levels %s -dwell %s -warmup %s -seed %s\n\n' \
  "${CFG#"$ROOT"/}" "$LEVELS" "$DWELL" "$WARMUP" "$SEED"

"$ROOT/bin/emb-stress" -config "$CFG" -target localhost -levels "$LEVELS" \
  -dwell "$DWELL" -warmup "$WARMUP" -seed "$SEED" 2>&1 |
  tee "$RUNS/stress.txt"
exit "${PIPESTATUS[0]}"
