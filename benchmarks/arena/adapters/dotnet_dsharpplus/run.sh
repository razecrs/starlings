#!/usr/bin/env bash
set -euo pipefail
dir=$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)
root=$(cd "$dir/../../../.." && pwd)
[[ -f $root/tools/arena-env.sh ]] && source "$root/tools/arena-env.sh"
export ARENA_FIXTURES=${ARENA_FIXTURES:-$root/benchmarks/arena/fixtures}
case ${1:-} in
  prepare) dotnet build "$dir/DSharpPlus.Tests.csproj" -c Release --nologo >/dev/null ;;
  *) [[ -f $dir/bin/Release/net10.0/DSharpPlus.Tests.dll ]] || "$0" prepare
     exec dotnet "$dir/bin/Release/net10.0/DSharpPlus.Tests.dll" "$@" ;;
esac
