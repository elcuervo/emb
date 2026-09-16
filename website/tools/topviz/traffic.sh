#!/usr/bin/env bash
# The load `just website-topviz` records. See README.md in this directory and
# openspec/changes/website-emb-top-recording.
#
# One `redis-benchmark` band per model, each with its own start and stop offset,
# so emb-top's activity heatmap shows a band per model rather than one solid
# block. Text length differs per model, so tok/s and latency differ per row; the
# `__rand_int__` placeholder draws each text from a bounded pool, so a model's
# repeat rate — and therefore the recorded cache hit ratio — climbs through the
# run instead of starting at 100%. Nothing here is meant to fail.
#
# THE CLOCK: the schedule is driven by `sleep` and nothing else, because `sleep`
# is the only timer available to a shell that does not follow the wall clock. A
# take was once emptied halfway through by the machine's clock stepping forward
# 17 seconds: every band read `date +%s`, saw its deadline in the past and quit
# at the same instant, and the second half of the recording was an idle node. So
# the parent owns the timeline and each band is a plain infinite loop it starts
# and kills.
set -u

PORT="${EMB_TOPVIS_PORT:-16379}"
BATCH="${EMB_TOPVIS_BATCH:-24}" # requests per redis-benchmark invocation

# model start(s) stop(s) gap(s) concurrency text-pool text
#
#   start/stop  offsets from the start of the run; staggered so the heatmap
#               bands overlap rather than stack
#   gap         sleep between batches; the pace knob
#   conc        connections each batch holds — 2 to 4 keeps churn low
#   text-pool   distinct texts in play. SIZED TO THE RUN, not to taste: the
#               final hit ratio lands near 1 - pool/requests, so a pool of
#               roughly 0.6x the request count leaves req/s visible *and* the
#               cache gauge moving.
PLAN='
minilm 0 26 0.17 3 1600 the quick brown fox jumps over the lazy dog
bge-small 4 25 0.20 3 600 a bge-small request carries a sentence of about twenty words so its tokens per second and latency differ from minilm
jina-small 8 30 0.28 2 480 the jina model is the one set to a different width here and its request text is deliberately the longest of the four so its tokens per second and latency are visibly its own row in the panel
e5-small 12 32 0.14 4 750 e5-small takes a medium length sentence with a random suffix so its cache hit ratio climbs
'

# One model's band: batch until the parent kills it. It reads no clock.
band() {
  local model="$1" gap="$2" conc="$3" pool="$4" text="$5"
  while :; do
    redis-benchmark -p "$PORT" -q -c "$conc" -n "$BATCH" -r "$pool" \
      EMB "$model" "$text __rand_int__" >/dev/null 2>&1
    sleep "$gap"
  done
}

pids_dir=$(mktemp -d "${TMPDIR:-/tmp}/emb-topviz-bands.XXXXXX")
trap 'kill $(cat "$pids_dir"/* 2>/dev/null) 2>/dev/null; rm -rf "$pids_dir"' EXIT INT TERM

# Start/stop events in order: every band's start and every band's stop, by time.
# Matched on the shape of the timing fields, not on the field count: the row
# carries free text, so the count is not fixed.
events() {
  printf '%s\n' "$PLAN" | awk '$2 ~ /^[0-9]+$/ && $3 ~ /^[0-9]+$/ {
    print $2, "start", $1
    print $3, "stop",  $1
  }' | sort -n -k1,1
}

now=0
while read -r when action model; do
  [ -n "${when:-}" ] || continue
  sleep "$(( when - now ))"
  now="$when"

  if [ "$action" = start ]; then
    # shellcheck disable=SC2086 # the row is a whitespace-separated plan line
    set -- $(printf '%s\n' "$PLAN" | awk -v m="$model" '$1 == m')
    band "$1" "$4" "$5" "$6" "${*:7}" &
    echo $! > "$pids_dir/$model"
  else
    # The band's own redis-benchmark child may outlive this by one batch; that
    # is a fraction of a second and the node is the only thing that sees it.
    kill "$(cat "$pids_dir/$model" 2>/dev/null)" 2>/dev/null
    rm -f "$pids_dir/$model"
  fi
done < <(events)

wait 2>/dev/null
