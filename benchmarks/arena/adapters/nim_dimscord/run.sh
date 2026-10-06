#!/usr/bin/env bash
set -euo pipefail
dir=$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)
src=${ARENA_SOURCE:-/opt/arena/sources/nim_dimscord}
bin="$dir/.arena-bin"
export ARENA_FIXTURES=${ARENA_FIXTURES:-$(cd "$dir/../../fixtures" && pwd)}
export ARENA_COMMIT=${ARENA_COMMIT:-1467f15419a9f05b6b406d583482665bbb6755d9}
case ${1:-} in
  prepare) nim c -d:release --opt:speed --path:"$src" -o:"$bin" "$dir/main.nim" ;;
  *) [[ -x $bin ]] || "$0" prepare; exec "$bin" "$@" ;;
esac
