#!/usr/bin/env bash
set -euo pipefail
adapter_dir=$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)
root=$(cd "$adapter_dir/../../../.." && pwd)
export ARENA_FIXTURES=${ARENA_FIXTURES:-$root/benchmarks/arena/fixtures}
export ARENA_SOURCE=${ARENA_SOURCE:-/opt/arena/work/js_discord-js}

case ${1:-} in
  prepare)
    test -f "$ARENA_SOURCE/packages/discord.js/src/index.js"
    node --check "$adapter_dir/main.js"
    ;;
  *) exec node "$adapter_dir/main.js" "$@" ;;
esac
