#!/usr/bin/env bash
set -euo pipefail
dir=$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)
src=${ARENA_SOURCE:-/opt/arena/sources/elixir_nostrum}
export ARENA_COMMIT=${ARENA_COMMIT:-03b06ba1c5094b83991097b1ce76b5fe2740324c}
export MIX_ENV=prod

if [[ ${1:-} == prepare ]]; then
  (cd "$src" && mix deps.get && mix compile)
  exit 0
fi
cd "$src"
exec mix run --no-start "$dir/main.exs" -- "$@"
