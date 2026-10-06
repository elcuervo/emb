#!/usr/bin/env bash
# The zone's one entrypoint: start emb on loopback, start emb-dns in front of
# it, and treat the two as one lifecycle.
#
#   * the shipped dns/config.yaml carries a checkout's loopback addresses; the
#     deployed addresses differ (a UDP socket must bind the platform's own
#     address, TCP must not), so this rewrites a copy of it exactly the way
#     `just dns-dev` does, with the platform values as defaults;
#   * emb-dns dials the upstream at boot and builds the vocabulary index
#     through it, so nothing starts until emb's listener accepts — a connect,
#     not a command, because emb loads every model before it binds;
#   * a stop signal is forwarded to both, so the server's own shutdown path
#     runs; if either process exits, the other is stopped and this script exits
#     non-zero, so the platform replaces the machine rather than leaving a zone
#     that answers with no model behind it.
set -euo pipefail

EMB_CONFIG="${EMB_CONFIG:-/etc/emb/emb.yaml}"
ZONE_CONFIG="${ZONE_CONFIG:-/etc/emb/dns.yaml}"
UPSTREAM="${UPSTREAM:-127.0.0.1:16389}"
DNS_LISTEN_UDP="${DNS_LISTEN_UDP:-fly-global-services:53}"
DNS_LISTEN_TCP="${DNS_LISTEN_TCP:-0.0.0.0:53}"
DNS_LISTEN_HTTP="${DNS_LISTEN_HTTP:-0.0.0.0:8080}"
APEX_ADDRESS="${APEX_ADDRESS:-}"
APEX_ADDRESS_V6="${APEX_ADDRESS_V6:-}"

zone_cfg=/tmp/dns.yaml
sed -e "s|^listen_udp: .*|listen_udp: \"$DNS_LISTEN_UDP\"|" \
    -e "s|^listen_tcp: .*|listen_tcp: \"$DNS_LISTEN_TCP\"|" \
    -e "s|^listen_http: .*|listen_http: \"$DNS_LISTEN_HTTP\"|" \
    -e "s|^apex_address: .*|apex_address: \"$APEX_ADDRESS\"|" \
    -e "s|^apex_address_v6: .*|apex_address_v6: \"$APEX_ADDRESS_V6\"|" \
    "$ZONE_CONFIG" > "$zone_cfg"

emb -config "$EMB_CONFIG" &
emb_pid=$!

host="${UPSTREAM%:*}"
port="${UPSTREAM##*:}"
for _ in $(seq 1 300); do
  if (exec 3<>"/dev/tcp/$host/$port") 2>/dev/null; then
    exec 3>&- 3<&-
    break
  fi
  if ! kill -0 "$emb_pid" 2>/dev/null; then
    echo "run.sh: emb exited before it was ready" >&2
    wait "$emb_pid" 2>/dev/null || true
    exit 1
  fi
  sleep 1
done

emb-dns -config "$zone_cfg" &
zone_pid=$!

stop_both() {
  kill -TERM "$emb_pid" 2>/dev/null || true
  kill -TERM "$zone_pid" 2>/dev/null || true
}
stopping=0
on_signal() { stopping=1; stop_both; }
trap 'on_signal' TERM INT

status=0
while kill -0 "$emb_pid" 2>/dev/null && kill -0 "$zone_pid" 2>/dev/null; do
  sleep 1
done

# A requested stop is not a failure; an unexpected exit is, so the platform
# replaces the machine instead of leaving a zone that answers with no model.
if [ "$stopping" -eq 0 ]; then
  if ! kill -0 "$emb_pid" 2>/dev/null; then
    echo "run.sh: emb exited; ending the unit" >&2
    status=1
  fi
  if ! kill -0 "$zone_pid" 2>/dev/null; then
    echo "run.sh: emb-dns exited; ending the unit" >&2
    status=1
  fi
fi

stop_both
wait "$emb_pid" 2>/dev/null || true
wait "$zone_pid" 2>/dev/null || true
exit "$status"
