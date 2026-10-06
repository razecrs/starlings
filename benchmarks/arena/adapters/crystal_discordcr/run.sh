#!/usr/bin/env bash
set -euo pipefail
dir=$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)
src=${ARENA_SOURCE:-/opt/arena/sources/crystal_discordcr}
bin="$dir/.arena-bin"
export ARENA_FIXTURES=${ARENA_FIXTURES:-$(cd "$dir/../../fixtures" && pwd)}
export ARENA_COMMIT=${ARENA_COMMIT:-0e03deb8ffa247814f2fec4e197cba7a62534f85}
case ${1:-} in
  prepare) CRYSTAL_PATH="$src/src:$src/lib:/usr/lib/crystal/lib" crystal build --release --no-debug "$dir/main.cr" -o "$bin" ;;
  *) [[ -x $bin ]] || "$0" prepare; exec "$bin" "$@" ;;
esac
