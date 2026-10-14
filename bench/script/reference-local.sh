#!/usr/bin/env bash
# Reproduce the dispatched reference workflow locally and expose its separate
# Fargate evidence bundle. Requires the GLiNER2 and MiniLM models, Docker, Redis
# CLI, and the normal nix develop runtime environment.
set -euo pipefail

out=$(mktemp -d "${TMPDIR:-/tmp}/emb-reference-local.XXXXXX")
echo "Reference artifacts: $out"

if go run ./bench/hostcheck/cmd/host-check -json >"$out/host.json"; then
  echo "host contract: qualified"
else
  echo "host contract: unqualified (continuing for local reproduction)" >&2
  cat "$out/host.json" >&2
fi

bash bench/script/shape.sh 2>&1 | tee "$out/shape.log"
go build -ldflags "-X main.version=$(cat VERSION)" -o "$out/emb" ./cmd/emb
BASE="$out/emb" CAND="$out/emb" SAMPLES=3 bash bench/script/run.sh 2>&1 | tee "$out/ab.log"

go run ./bench/fargate -mode run -out "$out/bundles"
bundle=$(find "$out/bundles" -maxdepth 1 -name 'run.*.json' -print -quit)
[ -n "$bundle" ] || { echo "no benchmark bundle written under $out/bundles" >&2; exit 1; }
go run ./bench/fargate -mode diff -before "$bundle" -after "$bundle"
echo "Reference logs: $out/{host.json,shape.log,ab.log}; bundle: $bundle"
