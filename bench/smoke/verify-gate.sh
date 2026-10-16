#!/usr/bin/env bash
#
# Gate self-verification: a gate that cannot fail is not a gate. This applies
# each committed regression under bench/smoke/mutants/ to a scratch copy of the
# tree and requires the smoke gate to go red on it, naming the assertion that
# caught it. The clean tree has to stay green.
#
# Run inside `nix develop` (the gate links ONNX Runtime):
#
#   bash bench/smoke/verify-gate.sh
#
# The tree under test is materialized in a detached git worktree at HEAD plus
# the current tree's tracked modifications and untracked files, so the working
# tree is never modified. Cost: one gate run (~1 min) per mutant, plus the clean
# run.

set -uo pipefail
shopt -s nullglob

repo="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
mutants="$repo/bench/smoke/mutants"
work="$(mktemp -d "${TMPDIR:-/tmp}/emb-smoke-verify.XXXXXX")"
sandbox="$work/tree"
log="$work/gate.log"

cleanup() {
  git -C "$repo" worktree remove --force "$sandbox" >/dev/null 2>&1 || true
  rm -rf "$work"
}
trap cleanup EXIT

failures=0
pass() { printf '✓ %s\n' "$*"; }
bad() { printf '✗ %s\n' "$*"; failures=$((failures + 1)); }

# ---- materialize the tree under test --------------------------------------

if ! git -C "$repo" rev-parse --git-dir >/dev/null 2>&1; then
  echo "verify-gate: $repo is not a git working tree" >&2
  exit 2
fi
git -C "$repo" worktree add --detach "$sandbox" HEAD >/dev/null || {
  echo "verify-gate: could not create a worktree at HEAD" >&2
  exit 2
}
# `git diff` is configured to run through an external diff tool on this
# machine, so the patch has to ask for the plain format explicitly.
diff_file="$work/working.diff"
git -C "$repo" diff --no-ext-diff --no-color --binary HEAD >"$diff_file"
if [ -s "$diff_file" ] && ! git -C "$sandbox" apply "$diff_file"; then
  echo "verify-gate: could not apply the working tree's changes to the scratch tree" >&2
  exit 2
fi
if [ -n "$(git -C "$repo" ls-files --others --exclude-standard)" ]; then
  if ! (cd "$repo" && git ls-files --others --exclude-standard -z | tar --null -T - -cf -) | tar -xf - -C "$sandbox"; then
    echo "verify-gate: could not copy the untracked files into the scratch tree" >&2
    exit 2
  fi
fi
echo "verify-gate: scratch tree at $sandbox"

# ---- the runs --------------------------------------------------------------

# run_gate runs the gate in the scratch tree and stores its log; it prints the
# gate's exit status.
run_gate() {
  (
    cd "$sandbox" || exit 2
    EMB_BENCH_SMOKE=1 go test ./internal/server/ -run Smoke -count=1 -v -timeout 600s
  ) >"$log" 2>&1
  echo $?
}

# gate_summary prints the gate's machine-readable summary lines — one per arm —
# when the run got far enough to write them. The test framework's file:line
# prefix is stripped so a recorded sweep stays comparable as the test file
# grows, and every arm is kept, de-duplicated (the control arm writes its line
# to the test log and to stderr): a run missing an arm's summary says so here
# instead of hiding behind whichever line matched last.
gate_summary() {
  grep -h 'emb-smoke: arm=' "$log" \
    | sed -E 's/^[[:space:]]*[A-Za-z0-9_]+_test\.go:[0-9]+: //' \
    | sort -u | tr '\n' ' ' | sed -E 's/ +$//'
}

echo
echo "== clean tree (must pass) =="
status="$(run_gate)"
if [ "$status" = "0" ] && [ -n "$(gate_summary)" ]; then
  pass "clean tree: gate green ($(gate_summary))"
elif [ -z "$(gate_summary)" ]; then
  bad "clean tree: the gate did not run (build or startup failure; see $log)"
  sed -n '1,40p' "$log"
else
  bad "clean tree: the gate reported a failure — a real regression or a flaky assertion"
  grep -h 'assertion .* failed' "$log" | sed 's/^/    /'
fi

for patch in "$mutants"/*.patch; do
  name="$(basename "$patch")"
  want="$(sed -n 's/^# assertion: //p' "$patch" | head -1)"
  echo
  echo "== $name (must fail on: ${want:-?}) =="
  # Restore the tree under test before the next mutant: `checkout -- .` drops the
  # working tree's changes that were applied above, so re-apply them. Without
  # this the mutants are tested against HEAD, and a mutant that only the working
  # tree's new coverage can catch (e.g. calibration-on-request, caught by the
  # lazy arm) would falsely pass.
  git -C "$sandbox" checkout -- . >/dev/null 2>&1
  if [ -s "$diff_file" ]; then
    git -C "$sandbox" apply "$diff_file" >/dev/null 2>&1 || true
  fi
  if ! git -C "$sandbox" apply "$patch"; then
    bad "$name: the mutant patch did not apply"
    continue
  fi
  status="$(run_gate)"
  got="$(gate_summary)"
  if [ -z "$got" ]; then
    bad "$name: the gate did not run — the mutant did not build"
    sed -n '1,30p' "$log"
    continue
  fi
  if [ "$status" = "0" ]; then
    bad "$name: the gate PASSED on a planted regression ($got)"
    continue
  fi
  if [ -z "$want" ]; then
    bad "$name: the patch names no expected assertion in its header"
    continue
  fi
  if grep -q "assertion $want failed" "$log"; then
    pass "$name: red, and named $want ($got)"
  else
    bad "$name: the gate went red but never named $want"
    grep -h 'assertion .* failed' "$log" | sed 's/^/    /'
  fi
done

git -C "$sandbox" checkout -- . >/dev/null 2>&1

echo
if [ "$failures" -gt 0 ]; then
  echo "verify-gate: $failures check(s) failed — the gate is not trustworthy yet"
  exit 1
fi
echo "verify-gate: every mutant was caught and the clean tree passed"
