#!/usr/bin/env bash
set -euo pipefail
dir=$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)
src=${ARENA_SOURCE:-/opt/arena/sources/lua_discordia}
export ARENA_SOURCE="$src"
luvit=${ARENA_LUVIT:-/opt/arena/luvit/luvit}
export LUA_PATH="$src/?.lua;$src/libs/?.lua;$src/libs/?/init.lua;$src/deps/?.lua;$src/deps/?/init.lua;;"
export ARENA_FIXTURES=${ARENA_FIXTURES:-$(cd "$dir/../../fixtures" && pwd)}
export ARENA_COMMIT=${ARENA_COMMIT:-bc8261ca21318a45365062f164d0ca62163fdbb7}
case ${1:-} in
  prepare)
    [[ -f $src/deps/coro-http.lua ]] || (cd "$src" && /opt/arena/luvit/lit install)
    "$luvit" -e "local loaded = require('$src/init.lua'); assert(loaded.Client)"
    ;;
  *) exec "$luvit" "$dir/main.lua" "$@" ;;
esac
