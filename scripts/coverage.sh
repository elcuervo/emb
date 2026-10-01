#!/usr/bin/env bash
# Cross-package coverage report and gate for emb.
#
# `report` prints per-package statement coverage plus the production total
# (excluding the cmd/* entrypoints and the bench/* harness) and the
# whole-repository total. `gate` additionally enforces coverage-floors.txt,
# failing when a production package is below its recorded floor or has no
# recorded floor.
#
# Run inside `nix develop` (the CGo suites need the ONNX runtime).
set -euo pipefail
cd "$(dirname "$0")/.."

profile=${EMB_COVER_PROFILE:-/tmp/emb-cover.out}
floors=${EMB_COVER_FLOORS:-coverage-floors.txt}

if [ "${EMB_COVER_SKIP:-}" != "1" ]; then
  go test -coverpkg=./... -coverprofile="$profile" -covermode=atomic ./... >/dev/null 2>&1 || true
fi

# Collapse duplicate profile rows: under -coverpkg every test binary emits every
# block, so a block counts as covered when any row recorded a hit.
report() {
  awk '
    NR > 1 {
      split($1, a, ":"); blk = a[1] " " a[2];
      n[blk] = $2;
      if (($3 + 0) > (c[blk] + 0)) c[blk] = $3 + 0;
      p = a[1]; sub(/\/[^\/]*$/, "", p); P[blk] = p;
    }
    END {
      for (b in n) {
        p = P[b];
        tot[p] += n[b]; if (c[b] > 0) cov[p] += n[b];
        wtot += n[b]; if (c[b] > 0) wcov += n[b];
        if (p ~ /\/cmd\/|\/bench\//) continue;
        ptot += n[b]; if (c[b] > 0) pcov += n[b];
      }
      for (p in tot) printf "PKG\t%s\t%.1f\t%d\t%d\n", p, 100 * cov[p] / tot[p], cov[p], tot[p];
      printf "TOTAL_PROD\t\t%.1f\t%d\t%d\n", 100 * pcov / ptot, pcov, ptot;
      printf "TOTAL_ALL\t\t%.1f\t%d\t%d\n", 100 * wcov / wtot, wcov, wtot;
    }' "$profile"
}

print_report() {
  report | awk -F'\t' '
    $1 == "PKG" { printf "  %6.1f%%  %6d/%-6d  %s\n", $3, $4, $5, $2 }
    $1 == "TOTAL_PROD" { prod = sprintf("%.1f%% (%d/%d)", $3, $4, $5) }
    $1 == "TOTAL_ALL" { all = sprintf("%.1f%% (%d/%d)", $3, $4, $5) }
    END { printf "\nproduction (no cmd/bench): %s\nwhole repository:          %s\n", prod, all }
  '
}

gate() {
  if [ ! -f "$floors" ]; then
    echo "missing floors file: $floors" >&2
    exit 2
  fi
  report | awk -F'\t' -v floors="$floors" '
    BEGIN {
      while ((getline line < floors) > 0) {
        if (line ~ /^[[:space:]]*#/ || line == "") continue;
        split(line, f, /[[:space:]]+/);
        floor[f[1]] = f[2];
      }
    }
    $1 == "PKG" || $1 == "TOTAL_PROD" {
      if ($1 == "PKG" && $2 ~ /\/cmd\/|\/bench\//) next;
      p = ($1 == "TOTAL_PROD") ? "TOTAL_PROD" : $2;
      pct = $3 + 0;
      if (!(p in floor)) { printf "✗ %s: no recorded floor (add it to %s)\n", p, floors; fail = 1; next }
      if (pct < floor[p] + 0) { printf "✗ %s: %.1f%% < floor %.1f%%\n", p, pct, floor[p]; fail = 1 }
      else printf "✓ %s: %.1f%% (floor %.1f%%)\n", p, pct, floor[p];
    }
    END { exit fail ? 1 : 0 }'
}

case "${1:-report}" in
  report) print_report ;;
  gate)
    print_report
    echo
    gate
    ;;
  *)
    echo "usage: $0 [report|gate]" >&2
    exit 2
    ;;
esac
