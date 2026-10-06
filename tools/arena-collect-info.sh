#!/usr/bin/env bash
set -euo pipefail

root=$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)
results=${ARENA_BENCH_RESULTS:-/opt/arena/results/final}
mkdir -p "$results"

for runner in "$root"/benchmarks/arena/adapters/*/run.sh; do
  id=$(basename "$(dirname "$runner")")
  bash "$runner" info >"$results/$id.info.json"
  jq -e --arg library "$id" \
    '.schema == 1 and .library == $library and (.supported | type == "array") and ((.unsupported // {}) | type == "object")' \
    "$results/$id.info.json" >/dev/null
done

printf 'arena: collected adapter metadata in %s\n' "$results"
