#!/usr/bin/env bash
set -uo pipefail

if [[ -x $HOME/.local/go/bin/go ]]; then
  export PATH="$HOME/.local/go/bin:$PATH"
fi

root=$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)
results=${ARENA_VERIFY_RESULTS:-/opt/arena/results/adapter-verify}
mkdir -p "$results"
overall=0

for runner in "$root"/benchmarks/arena/adapters/*/run.sh; do
  id=$(basename "$(dirname "$runner")")
  printf '[prepare] %s\n' "$id"
  if bash "$runner" prepare >"$results/$id.prepare.log" 2>&1; then
    printf '[verify] %s\n' "$id"
    if bash "$runner" verify >"$results/$id.verify.json" 2>"$results/$id.verify.log"; then
      printf '[pass] %s\n' "$id"
    else
      status=$?
      printf '[verify-fail] %s status=%d\n' "$id" "$status"
      tail -20 "$results/$id.verify.log"
      overall=1
    fi
  else
    status=$?
    printf '[prepare-fail] %s status=%d\n' "$id" "$status"
    tail -20 "$results/$id.prepare.log"
    overall=1
  fi
done

exit "$overall"
