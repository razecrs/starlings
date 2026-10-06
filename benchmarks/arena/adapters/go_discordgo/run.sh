#!/usr/bin/env bash
set -euo pipefail

adapter_dir=$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)
root=$(cd "$adapter_dir/../../../.." && pwd)
binary=${ARENA_ADAPTER_BIN:-$adapter_dir/.arena-bin}
export ARENA_FIXTURES=${ARENA_FIXTURES:-$root/benchmarks/arena/fixtures}

prepare() {
  (cd "$root" && go build -trimpath -ldflags='-s -w' -o "$binary" ./benchmarks/arena/adapters/go_discordgo)
}

if [[ ${1:-} == prepare ]]; then
  prepare
  exit 0
fi
[[ -x $binary ]] || prepare
exec "$binary" "$@"
