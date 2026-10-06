#!/usr/bin/env bash
set -euo pipefail
dir=$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)
dart=${ARENA_DART:-/opt/arena/dart-sdk/bin/dart}
packages=${ARENA_NYXX_PACKAGES:-/opt/arena/work/dart_nyxx/.dart_tool/package_config.json}
bin="$dir/.arena-bin"
export ARENA_FIXTURES=${ARENA_FIXTURES:-$(cd "$dir/../../fixtures" && pwd)}
export ARENA_COMMIT=${ARENA_COMMIT:-40e12c14524af372cf8f816ffdc680fdfb74c183}
case ${1:-} in
  prepare) "$dart" compile exe --packages="$packages" "$dir/main.dart" -o "$bin" >/dev/null ;;
  *) [[ -x $bin ]] || "$0" prepare; exec "$bin" "$@" ;;
esac
